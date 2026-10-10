package user

import (
	"errors"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/hash"
	"nova-admin/internal/pkg/page"
)

// AdminUserID 是内置超级管理员的用户 ID，禁止删除与禁用。
const AdminUserID int64 = 1

// Service 用户服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建用户服务。
func NewService() *Service { return &Service{db: database.DB()} }

var userOrderWhitelist = map[string]string{
	"createdAt":   "created_at",
	"updatedAt":   "updated_at",
	"username":    "username",
	"lastLoginAt": "last_login_at",
}

// List 分页查询用户。actor 的数据范围会叠加在筛选条件之上。
func (s *Service) List(q page.Query, f ListQuery, actor *authz.Identity) ([]VO, int64, error) {
	// 过滤条件集中在一处，保证 count 与 find 使用完全一致的 where 子句；
	// 数据范围也放进同一个闭包，否则统计数与列表可能落在不同范围上
	filter := s.scopedFilter(actor, s.buildFilter(q.Keyword, f))

	var total int64
	if err := filter(s.db.Model(&model.User{})).Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	var users []model.User
	if err := filter(s.db.Model(&model.User{})).
		Preload("Dept").
		Preload("Roles").
		Order(q.SafeOrderBy(userOrderWhitelist, "id DESC")).
		Offset(q.Offset()).Limit(q.Limit()).
		Find(&users).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	items := make([]VO, 0, len(users))
	for i := range users {
		items = append(items, toVO(&users[i]))
	}
	return items, total, nil
}

// buildFilter 构造列表查询的 where 子句。
// List 与 Excel 导出共用，保证「导出的数据」与「页面看到的数据」范围完全一致。
func (s *Service) buildFilter(keyword string, f ListQuery) func(*gorm.DB) *gorm.DB {
	// deptTreeIDs 要全表扫一次 sys_depts，必须在闭包外算。
	// 闭包在 List 里会被执行两次（Count 与 Find），写在里面就是两次全表扫。
	var deptIDs []int64
	if f.DeptID != nil {
		deptIDs = s.deptTreeIDs(*f.DeptID)
	}
	return func(tx *gorm.DB) *gorm.DB {
		if keyword != "" {
			kw := "%" + keyword + "%"
			tx = tx.Where("username ILIKE ? OR nickname ILIKE ? OR phone ILIKE ?", kw, kw, kw)
		}
		if f.Status != nil {
			tx = tx.Where("status = ?", *f.Status)
		}
		if f.DeptID != nil {
			// 含子部门
			tx = tx.Where("dept_id IN (?)", deptIDs)
		}
		if f.RoleID != nil {
			tx = tx.Where("id IN (?)", s.db.Model(&model.UserRole{}).
				Select("user_id").Where("role_id = ?", *f.RoleID))
		}
		return tx
	}
}

// Options 返回精简用户列表，用于下拉选择。
func (s *Service) Options(keyword string, actor *authz.Identity) ([]Option, error) {
	tx := s.scopeQuery(actor, s.db.Model(&model.User{})).Where("status = ?", model.StatusEnabled)
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("username ILIKE ? OR nickname ILIKE ?", kw, kw)
	}
	var out []Option
	if err := tx.Order("id ASC").Limit(100).
		Select("id", "username", "nickname").Scan(&out).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return out, nil
}

// ---------------- 数据范围（数据权限） ----------------
//
// 数据范围来自角色上的 data_scope，多个角色取最宽松者（见 authz.build）：
//
//	1 全部         不加限制
//	2 本部门       仅本部门
//	3 本部门及以下 本部门及其所有子部门
//	4 仅本人       只有自己
//
// 读与写必须一起收窄。只收窄列表会留下「看不到但能改」的洞：
// 「本部门」管理员虽然列不出别的部门的人，却仍能拿已知 ID 重置对方密码。
func (s *Service) scopeQuery(actor *authz.Identity, tx *gorm.DB) *gorm.DB {
	if actor == nil || actor.IsSuper || actor.DataScope == model.DataScopeAll {
		return tx
	}
	if actor.DataScope == model.DataScopeSelf || actor.DeptID == nil {
		// 没有部门的人谈不上「本部门」，退化成只能看自己
		return tx.Where("id = ?", actor.UserID)
	}
	if actor.DataScope == model.DataScopeDept {
		return tx.Where("dept_id = ?", *actor.DeptID)
	}
	return tx.Where("dept_id IN ?", s.deptTreeIDs(*actor.DeptID))
}

// scopedFilter 把数据范围叠在既有的 where 构造之上。
func (s *Service) scopedFilter(actor *authz.Identity, base func(*gorm.DB) *gorm.DB) func(*gorm.DB) *gorm.DB {
	return func(tx *gorm.DB) *gorm.DB { return s.scopeQuery(actor, base(tx)) }
}

// scopeDeptIDs 返回允许的部门 ID 集合；返回 nil 表示不受限制。
func (s *Service) scopeDeptIDs(actor *authz.Identity) map[int64]bool {
	if actor == nil || actor.IsSuper || actor.DataScope == model.DataScopeAll {
		return nil
	}
	if actor.DataScope == model.DataScopeSelf || actor.DeptID == nil {
		return map[int64]bool{}
	}
	ids := []int64{*actor.DeptID}
	if actor.DataScope == model.DataScopeDeptAndSub {
		ids = s.deptTreeIDs(*actor.DeptID)
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

// ensureInScope 校验目标用户是否落在数据范围内。
// 越界一律按「不存在」处理，避免用返回信息探测其他部门有哪些用户。
func (s *Service) ensureInScope(actor *authz.Identity, targetID int64) error {
	var cnt int64
	if err := s.scopeQuery(actor, s.db.Model(&model.User{})).
		Where("id = ?", targetID).Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt == 0 {
		return errcode.New(errcode.ErrNotFound, "用户不存在")
	}
	return nil
}

// checkDeptInScope 校验所属部门是否在数据范围内（新增用户 / 调整部门时使用）。
func (s *Service) checkDeptInScope(actor *authz.Identity, deptID *int64) error {
	allowed := s.scopeDeptIDs(actor)
	if allowed == nil {
		return nil
	}
	if deptID == nil {
		return errcode.New(errcode.ErrBadRequest, "请选择你数据范围内的部门")
	}
	if !allowed[*deptID] {
		return errcode.New(errcode.ErrForbidden, "目标部门超出你的数据范围")
	}
	return nil
}

// Get 查询用户详情。超出数据范围时按「不存在」返回。
func (s *Service) Get(id int64, actor *authz.Identity) (*VO, error) {
	var u model.User
	tx := s.scopeQuery(actor, s.db.Model(&model.User{})).Preload("Dept").Preload("Roles")
	if err := tx.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "用户不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	vo := toVO(&u)
	return &vo, nil
}

// Create 新增用户。
func (s *Service) Create(req CreateReq, actor *authz.Identity) (int64, error) {
	if err := s.checkDeptInScope(actor, req.DeptID); err != nil {
		return 0, err
	}
	if err := s.ensureUsernameFree(req.Username, 0); err != nil {
		return 0, err
	}
	hashed, err := hash.Password(req.Password)
	if err != nil {
		return 0, errcode.Wrap(errcode.ErrInternal, err)
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	u := model.User{
		Username: req.Username,
		Password: hashed,
		Nickname: req.Nickname,
		Email:    req.Email,
		Phone:    req.Phone,
		DeptID:   req.DeptID,
		Status:   status,
		Remark:   req.Remark,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&u).Error; err != nil {
			return err
		}
		return replaceRoles(tx, u.ID, req.RoleIDs, actor)
	})
	if err != nil {
		return 0, saveErr(err)
	}
	return u.ID, nil
}

// Update 修改用户。
func (s *Service) Update(id int64, req UpdateReq, actor *authz.Identity) error {
	if err := s.ensureInScope(actor, id); err != nil {
		return err
	}
	// 把用户挪到自己管不到的部门同样是越界
	if err := s.checkDeptInScope(actor, req.DeptID); err != nil {
		return err
	}
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "用户不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}

	updates := map[string]any{
		"nickname": req.Nickname,
		"email":    req.Email,
		"phone":    req.Phone,
		"dept_id":  req.DeptID,
		"remark":   req.Remark,
	}
	if req.Status != nil {
		if id == AdminUserID && *req.Status != model.StatusEnabled {
			return errcode.New(errcode.ErrBadRequest, "内置管理员不允许禁用")
		}
		updates["status"] = *req.Status
		if *req.Status != model.StatusEnabled {
			// 禁用时同时作废已签发令牌，见 UpdateStatus 的说明
			updates["token_epoch"] = gorm.Expr("token_epoch + 1")
		}
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		// roleIds 为 nil 表示不调整角色
		if req.RoleIDs != nil {
			// 目标持有超管角色时，非超管不得改动（含摘掉其超管角色）
			if err := s.ensureTargetNotSuper(tx, actor, id); err != nil {
				return err
			}
			return replaceRoles(tx, id, req.RoleIDs, actor)
		}
		return nil
	})
	if err != nil {
		return saveErr(err)
	}
	authz.Invalidate(id)
	return nil
}

// saveErr 保留事务里抛出的业务错误码（如 403 无权授予角色），其余归一到数据库错误。
//
// 不能无脑 Wrap(ErrDatabase)：那会把「无权授予超管角色」这类明确的业务错误
// 冲成 50100「数据库操作失败」，前端只能看到一句误导性的提示。
func saveErr(err error) error {
	var be *errcode.Error
	if errors.As(err, &be) {
		return be
	}
	return errcode.Wrap(errcode.ErrDatabase, err)
}

// Delete 删除用户（软删除）。
func (s *Service) Delete(id int64, operatorID int64, actor *authz.Identity) error {
	if err := s.ensureInScope(actor, id); err != nil {
		return err
	}
	if err := s.ensureTargetNotSuper(s.db, actor, id); err != nil {
		return err
	}
	if id == AdminUserID {
		return errcode.New(errcode.ErrBadRequest, "内置管理员不允许删除")
	}
	if id == operatorID {
		return errcode.New(errcode.ErrBadRequest, "不能删除当前登录用户")
	}
	var u model.User
	if err := s.db.First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "用户不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.User{}, id).Error
	})
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.Invalidate(id)
	return nil
}

// UpdateStatus 启用/禁用用户。
func (s *Service) UpdateStatus(id int64, status int16, operatorID int64, actor *authz.Identity) error {
	if err := s.ensureInScope(actor, id); err != nil {
		return err
	}
	if err := s.ensureTargetNotSuper(s.db, actor, id); err != nil {
		return err
	}
	if id == AdminUserID && status != model.StatusEnabled {
		return errcode.New(errcode.ErrBadRequest, "内置管理员不允许禁用")
	}
	if id == operatorID && status != model.StatusEnabled {
		return errcode.New(errcode.ErrBadRequest, "不能禁用当前登录用户")
	}
	updates := map[string]any{"status": status}
	if status != model.StatusEnabled {
		// 禁用时把令牌版本号 +1：既让禁用立即生效（authz 缓存不必等 30 秒），
		// 也避免「禁用期间签发的旧令牌」在重新启用后复活。
		updates["token_epoch"] = gorm.Expr("token_epoch + 1")
	}
	res := s.db.Model(&model.User{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "用户不存在")
	}
	authz.Invalidate(id)
	return nil
}

// ResetPassword 管理员重置指定用户密码。
func (s *Service) ResetPassword(id int64, plain string, actor *authz.Identity) error {
	if err := s.ensureInScope(actor, id); err != nil {
		return err
	}
	if err := s.ensureTargetNotSuper(s.db, actor, id); err != nil {
		return err
	}
	if len(plain) < 6 {
		return errcode.New(errcode.ErrBadRequest, "密码长度不能少于 6 位")
	}
	hashed, err := hash.Password(plain)
	if err != nil {
		return errcode.Wrap(errcode.ErrInternal, err)
	}
	res := s.db.Model(&model.User{}).Where("id = ?", id).Updates(map[string]any{
		"password":    hashed,
		"token_epoch": gorm.Expr("token_epoch + 1"),
	})
	if res.Error != nil {
		return errcode.Wrap(errcode.ErrDatabase, res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "用户不存在")
	}
	authz.Invalidate(id)
	return nil
}

func (s *Service) ensureUsernameFree(username string, excludeID int64) error {
	tx := s.db.Model(&model.User{}).Where("username = ?", username)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "用户名 %s 已存在", username)
	}
	return nil
}

// deptTreeIDs 返回部门及其所有子部门 ID。
func (s *Service) deptTreeIDs(rootID int64) []int64 {
	var all []model.Dept
	if err := s.db.Select("id", "parent_id").Find(&all).Error; err != nil {
		return []int64{rootID}
	}
	children := map[int64][]int64{}
	for _, d := range all {
		children[d.ParentID] = append(children[d.ParentID], d.ID)
	}
	result := []int64{rootID}
	queue := []int64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, cid := range children[cur] {
			result = append(result, cid)
			queue = append(queue, cid)
		}
	}
	return result
}

// replaceRoles 覆盖式设置用户的角色，并校验调用者有权授予这些角色。
//
// 授予校验（authz.EnsureGrantable）是必须的：本函数此前只过滤「不存在的角色」，
// 不校验范围，于是拥有 system:user:create 的系统管理员可以带 roleIds:[1]（超管）
// 直接造出一个超管账号，或把自己/下属改成超管 —— 一次请求即可提权。
func replaceRoles(tx *gorm.DB, userID int64, roleIDs []int64, actor *authz.Identity) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return err
	}
	if len(roleIDs) == 0 {
		return nil
	}
	// 只保留库中真实存在的角色，避免脏数据
	var roles []model.Role
	if err := tx.Model(&model.Role{}).
		Select("id", "name", "code").
		Where("id IN ?", roleIDs).Find(&roles).Error; err != nil {
		return err
	}
	if err := authz.EnsureGrantable(actor, roles); err != nil {
		return err
	}
	rows := make([]model.UserRole, 0, len(roles))
	for _, r := range roles {
		rows = append(rows, model.UserRole{UserID: userID, RoleID: r.ID})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}

// ensureTargetNotSuper 阻止非超管操作「持有超管角色的用户」。
//
// 数据范围只回答「这个人归不归我管」，不回答「我能不能动一个超管」。
// 若只靠 ensureInScope，一个恰好管着超管所在部门的系统管理员，
// 就能把这个超管禁用、删号、重置密码，或者摘掉他的超管角色。
func (s *Service) ensureTargetNotSuper(tx *gorm.DB, actor *authz.Identity, userID int64) error {
	if actor == nil || actor.IsSuper {
		return nil
	}
	var cnt int64
	if err := tx.Model(&model.Role{}).
		Joins("JOIN sys_user_roles ur ON ur.role_id = sys_roles.id").
		Where("ur.user_id = ? AND sys_roles.code = ?", userID, authz.SuperRoleCode).
		Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.New(errcode.ErrForbidden, "无权操作持有「超级管理员」角色的用户")
	}
	return nil
}

func toVO(u *model.User) VO {
	vo := VO{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Phone:       u.Phone,
		Avatar:      u.Avatar,
		DeptID:      u.DeptID,
		Status:      u.Status,
		Remark:      u.Remark,
		LastLoginAt: u.LastLoginAt,
		LastLoginIP: u.LastLoginIP,
		CreatedAt:   u.CreatedAt,
		RoleIDs:     make([]int64, 0, len(u.Roles)),
		RoleNames:   make([]string, 0, len(u.Roles)),
	}
	if u.Dept != nil {
		vo.DeptName = u.Dept.Name
	}
	for _, r := range u.Roles {
		vo.RoleIDs = append(vo.RoleIDs, r.ID)
		vo.RoleNames = append(vo.RoleNames, r.Name)
	}
	return vo
}
