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

// List 分页查询用户。
func (s *Service) List(q page.Query, f ListQuery) ([]VO, int64, error) {
	// 过滤条件集中在一处，保证 count 与 find 使用完全一致的 where 子句
	filter := s.buildFilter(q.Keyword, f)

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
			tx = tx.Where("dept_id IN (?)", s.deptTreeIDs(*f.DeptID))
		}
		if f.RoleID != nil {
			tx = tx.Where("id IN (?)", s.db.Model(&model.UserRole{}).
				Select("user_id").Where("role_id = ?", *f.RoleID))
		}
		return tx
	}
}

// Options 返回精简用户列表，用于下拉选择。
func (s *Service) Options(keyword string) ([]Option, error) {
	tx := s.db.Model(&model.User{}).Where("status = ?", model.StatusEnabled)
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

// Get 查询用户详情。
func (s *Service) Get(id int64) (*VO, error) {
	var u model.User
	if err := s.db.Preload("Dept").Preload("Roles").First(&u, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "用户不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	vo := toVO(&u)
	return &vo, nil
}

// Create 新增用户。
func (s *Service) Create(req CreateReq) (int64, error) {
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
		return replaceRoles(tx, u.ID, req.RoleIDs)
	})
	if err != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return u.ID, nil
}

// Update 修改用户。
func (s *Service) Update(id int64, req UpdateReq) error {
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
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.User{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		// roleIds 为 nil 表示不调整角色
		if req.RoleIDs != nil {
			return replaceRoles(tx, id, req.RoleIDs)
		}
		return nil
	})
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.Invalidate(id)
	return nil
}

// Delete 删除用户（软删除）。
func (s *Service) Delete(id int64, operatorID int64) error {
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
func (s *Service) UpdateStatus(id int64, status int16, operatorID int64) error {
	if id == AdminUserID && status != model.StatusEnabled {
		return errcode.New(errcode.ErrBadRequest, "内置管理员不允许禁用")
	}
	if id == operatorID && status != model.StatusEnabled {
		return errcode.New(errcode.ErrBadRequest, "不能禁用当前登录用户")
	}
	res := s.db.Model(&model.User{}).Where("id = ?", id).Update("status", status)
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
func (s *Service) ResetPassword(id int64, plain string) error {
	if len(plain) < 6 {
		return errcode.New(errcode.ErrBadRequest, "密码长度不能少于 6 位")
	}
	hashed, err := hash.Password(plain)
	if err != nil {
		return errcode.Wrap(errcode.ErrInternal, err)
	}
	res := s.db.Model(&model.User{}).Where("id = ?", id).Update("password", hashed)
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

func replaceRoles(tx *gorm.DB, userID int64, roleIDs []int64) error {
	if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return err
	}
	if len(roleIDs) == 0 {
		return nil
	}
	// 过滤不存在的角色，避免脏数据
	var valid []int64
	if err := tx.Model(&model.Role{}).Where("id IN ?", roleIDs).Pluck("id", &valid).Error; err != nil {
		return err
	}
	rows := make([]model.UserRole, 0, len(valid))
	for _, rid := range valid {
		rows = append(rows, model.UserRole{UserID: userID, RoleID: rid})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
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
