package role

import (
	"errors"

	"gorm.io/gorm"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/dberr"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// Service 角色服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建角色服务。
func NewService() *Service { return &Service{db: database.DB()} }

var roleOrderWhitelist = map[string]string{
	"sort":      "sys_roles.sort",
	"createdAt": "sys_roles.created_at",
	"name":      "sys_roles.name",
}

// List 分页查询角色。
func (s *Service) List(q page.Query) ([]VO, int64, error) {
	tx := s.db.Model(&model.Role{})
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		tx = tx.Where("name ILIKE ? OR code ILIKE ?", kw, kw)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var roles []model.Role
	if err := q.Paginate(tx).
		Order(q.SafeOrderBy(roleOrderWhitelist, "sys_roles.sort ASC, sys_roles.id ASC")).
		Find(&roles).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	items, err := s.toVOs(roles)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Options 返回全部启用角色。
//
// 会过滤掉调用者「无权授予」的角色（超管角色，以及权限集合超出调用者自身的角色），
// 与 user 模块的授予校验同源 —— 否则下拉里会出现一个「选了必然 403」的选项。
func (s *Service) Options(actor *authz.Identity) ([]Option, error) {
	tx := s.db.Model(&model.Role{}).Where("status = ?", model.StatusEnabled)

	allowed, unrestricted, err := authz.GrantableRoleIDs(actor)
	if err != nil {
		return nil, err
	}
	if !unrestricted {
		if len(allowed) == 0 {
			return []Option{}, nil
		}
		ids := make([]int64, 0, len(allowed))
		for id := range allowed {
			ids = append(ids, id)
		}
		tx = tx.Where("id IN ?", ids)
	}

	out := make([]Option, 0, 16)
	if err := tx.Order("sort ASC, id ASC").
		Select("id", "name", "code").Scan(&out).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return out, nil
}

// Get 查询角色详情，含已授权菜单 ID。
func (s *Service) Get(id int64) (*VO, error) {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "角色不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	vo, err := s.toVO(&r)
	if err != nil {
		return nil, err
	}
	return &vo, nil
}

// Create 新增角色。
// Create 新增角色。
//
// actor 不可省（理由同 Update）：创建时就能直接指定 data_scope 与菜单组合，
// 没有「不能比调用者更宽 / 不能挂自己没有的权限码」两道校验，
// 持有 system:role:create 的非超管就能直接造出一个全库通行证角色。
func (s *Service) Create(req CreateReq, actor *authz.Identity) (int64, error) {
	if err := s.ensureCodeFree(req.Code, 0); err != nil {
		return 0, err
	}
	if req.Code == authz.SuperRoleCode {
		return 0, errcode.New(errcode.ErrConflict, "超管角色标识已被占用")
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	scope := req.DataScope
	if scope == 0 {
		scope = model.DataScopeAll
	}
	if actor != nil && !actor.IsSuper && scope < actor.DataScope {
		return 0, errcode.Newf(errcode.ErrForbidden,
			"不能创建数据范围比自身更宽的角色（当前 %s）", authz.ScopeText(scope))
	}
	if err := authz.EnsureRoleMenusGrantable(actor, req.MenuIDs); err != nil {
		return 0, err
	}
	r := model.Role{
		Name:      req.Name,
		Code:      req.Code,
		Sort:      req.Sort,
		Status:    status,
		DataScope: scope,
		Remark:    req.Remark,
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&r).Error; err != nil {
			return err
		}
		return replaceMenus(tx, r.ID, req.MenuIDs)
	})
	if err != nil {
		// ensureCodeFree 是「先查后插」，两个请求同时通过预检时靠唯一索引兜底。
		// 不映射的话前端看到的是 50100「数据库操作失败」，真相只是标识重名。
		if dberr.IsUniqueViolation(err) {
			return 0, errcode.Newf(errcode.ErrConflict, "角色标识 %s 已存在", req.Code)
		}
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return r.ID, nil
}

// Update 修改角色。
//
// actor 不可省：角色上的 data_scope 与菜单权限都是「授权能力的来源」，
// 没有调用者身份就等于任何持有 system:role:update 的人都能把任意角色
// 调成「全部数据」，等于给自己或同伙发了一张全库通行证。
func (s *Service) Update(id int64, req UpdateReq, actor *authz.Identity) error {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "角色不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if err := ensureRoleMutable(actor, r); err != nil {
		return err
	}
	// 修改角色的菜单同样是授权行为（见 AssignMenus）：否则任何持有
	// system:role:update 的非超管都能给一个已挂用户的角色挂上连自己
	// 都没有的权限码，绕过 EnsureGrantable（它只管授予用户这一步）。
	if req.MenuIDs != nil && r.Code != authz.SuperRoleCode {
		if err := authz.EnsureRoleMenusGrantable(actor, req.MenuIDs); err != nil {
			return err
		}
	}

	updates := map[string]any{
		"name":   req.Name,
		"sort":   req.Sort,
		"remark": req.Remark,
	}
	if req.Status != nil {
		if r.IsBuiltin && *req.Status != model.StatusEnabled {
			return errcode.New(errcode.ErrBadRequest, "内置角色不允许禁用")
		}
		updates["status"] = *req.Status
	}
	if req.DataScope != 0 {
		// 不允许把角色调到比调用者更宽的数据范围（1 最小 = 最宽）
		if actor != nil && !actor.IsSuper && req.DataScope < actor.DataScope {
			return errcode.Newf(errcode.ErrForbidden,
				"不能把角色的数据范围调得比自身更宽（当前 %s）", authz.ScopeText(req.DataScope))
		}
		updates["data_scope"] = req.DataScope
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Role{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if req.MenuIDs != nil {
			// 内置超管角色权限不可裁剪，始终拥有全部权限
			if r.Code == authz.SuperRoleCode {
				return nil
			}
			return replaceMenus(tx, id, req.MenuIDs)
		}
		return nil
	})
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return nil
}

// ensureRoleMutable 挡住「非超管改超管角色」与「非超管改比自己范围更宽的角色」。
//
// 第二道不止挡「把角色调宽」（那是 Update 里 req.DataScope 的职责）：
// 目标角色**当前**范围就比调用者更宽时，改名 / 禁用 / 裁剪菜单 / 删除同样该拦 ——
// 一个「本部门」管理员不该能对「全部数据」范围的角色动手，哪怕只是禁用它，
// 那等于让更高范围角色的持有者集体失权。
func ensureRoleMutable(actor *authz.Identity, r model.Role) error {
	if actor == nil || actor.IsSuper {
		return nil
	}
	if r.Code == authz.SuperRoleCode {
		return errcode.New(errcode.ErrForbidden, "无权修改「超级管理员」角色")
	}
	// 数值越小范围越宽，因此「比调用者更宽」等价于「小于调用者」
	if actor.DataScope > 0 && r.DataScope > 0 && r.DataScope < actor.DataScope {
		return errcode.Newf(errcode.ErrForbidden,
			"无权修改角色「%s」：它的数据范围（%s）比你的（%s）更宽",
			r.Name, authz.ScopeText(r.DataScope), authz.ScopeText(actor.DataScope))
	}
	return nil
}

// Delete 删除角色。
func (s *Service) Delete(id int64, actor *authz.Identity) error {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "角色不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if r.IsBuiltin {
		return errcode.New(errcode.ErrBadRequest, "内置角色不允许删除")
	}
	if err := ensureRoleMutable(actor, r); err != nil {
		return err
	}

	var userCount int64
	if err := s.db.Model(&model.UserRole{}).Where("role_id = ?", id).Count(&userCount).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if userCount > 0 {
		return errcode.Newf(errcode.ErrConflict, "该角色下还有 %d 个用户，请先解除关联", userCount)
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_id = ?", id).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Role{}, id).Error
	})
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return nil
}

// AssignMenus 为角色分配菜单权限。
func (s *Service) AssignMenus(id int64, menuIDs []int64, actor *authz.Identity) error {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "角色不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if r.Code == authz.SuperRoleCode {
		return errcode.New(errcode.ErrBadRequest, "超管角色默认拥有全部权限，无需分配")
	}
	// 改权限菜单同样是授权行为：非超管不能给一个角色挂上连自己都没有的权限码，
	// 否则「造角色 + 授权」两步就能绕过 EnsureGrantable（它只管授予用户，不管改角色）。
	if err := ensureRoleMutable(actor, r); err != nil {
		return err
	}
	if err := authz.EnsureRoleMenusGrantable(actor, menuIDs); err != nil {
		return err
	}
	if err := replaceMenus(s.db, id, menuIDs); err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return nil
}

func (s *Service) ensureCodeFree(code string, excludeID int64) error {
	tx := s.db.Model(&model.Role{}).Where("code = ?", code)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "角色标识 %s 已存在", code)
	}
	return nil
}

// toVO 构造单个角色的视图对象（用于详情）。
//
// 两个聚合查询的 error **不吞**：DB 出错时返回空值等于把「查不到」当成
// 「没有菜单 / 没有用户」展示给用户，是把故障伪装成业务结论。
func (s *Service) toVO(r *model.Role) (VO, error) {
	var menuIDs []int64
	if err := s.db.Model(&model.RoleMenu{}).Where("role_id = ?", r.ID).Pluck("menu_id", &menuIDs).Error; err != nil {
		return VO{}, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var userCount int64
	if err := s.db.Model(&model.UserRole{}).Where("role_id = ?", r.ID).Count(&userCount).Error; err != nil {
		return VO{}, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return buildRoleVO(r, menuIDs, userCount), nil
}

// toVOs 批量构造角色视图对象。
//
// 旧实现逐个角色各查一次「菜单 ID」和「用户数」，分页 20 条就是 40 次查询（N+1）。
// 这里改成两条聚合查询，查询次数与分页大小无关。
func (s *Service) toVOs(roles []model.Role) ([]VO, error) {
	items := make([]VO, 0, len(roles))
	if len(roles) == 0 {
		return items, nil
	}
	ids := make([]int64, 0, len(roles))
	for i := range roles {
		ids = append(ids, roles[i].ID)
	}

	type menuRow struct {
		RoleID int64
		MenuID int64
	}
	var menuRows []menuRow
	if err := s.db.Model(&model.RoleMenu{}).
		Select("role_id, menu_id").Where("role_id IN ?", ids).Scan(&menuRows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	menuByRole := make(map[int64][]int64, len(roles))
	for _, r := range menuRows {
		menuByRole[r.RoleID] = append(menuByRole[r.RoleID], r.MenuID)
	}

	type countRow struct {
		RoleID int64
		Total  int64
	}
	var countRows []countRow
	if err := s.db.Model(&model.UserRole{}).
		Select("role_id, COUNT(*) AS total").Where("role_id IN ?", ids).
		Group("role_id").Scan(&countRows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	countByRole := make(map[int64]int64, len(roles))
	for _, r := range countRows {
		countByRole[r.RoleID] = r.Total
	}

	for i := range roles {
		items = append(items, buildRoleVO(&roles[i], menuByRole[roles[i].ID], countByRole[roles[i].ID]))
	}
	return items, nil
}

func buildRoleVO(r *model.Role, menuIDs []int64, userCount int64) VO {
	return VO{
		ID:        r.ID,
		Name:      r.Name,
		Code:      r.Code,
		Sort:      r.Sort,
		Status:    r.Status,
		DataScope: r.DataScope,
		IsBuiltin: r.IsBuiltin,
		Remark:    r.Remark,
		UserCount: userCount,
		MenuIDs:   menuIDs,
		CreatedAt: r.CreatedAt,
	}
}

func replaceMenus(tx *gorm.DB, roleID int64, menuIDs []int64) error {
	if err := tx.Where("role_id = ?", roleID).Delete(&model.RoleMenu{}).Error; err != nil {
		return err
	}
	if len(menuIDs) == 0 {
		return nil
	}
	var valid []int64
	if err := tx.Model(&model.Menu{}).Where("id IN ?", menuIDs).Pluck("id", &valid).Error; err != nil {
		return err
	}
	rows := make([]model.RoleMenu, 0, len(valid))
	for _, mid := range valid {
		rows = append(rows, model.RoleMenu{RoleID: roleID, MenuID: mid})
	}
	if len(rows) == 0 {
		return nil
	}
	return tx.Create(&rows).Error
}
