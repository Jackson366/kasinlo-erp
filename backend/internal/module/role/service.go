package role

import (
	"errors"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
	"nova-admin/internal/pkg/page"
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
	if err := tx.Order(q.SafeOrderBy(roleOrderWhitelist, "sys_roles.sort ASC, sys_roles.id ASC")).
		Offset(q.Offset()).Limit(q.Limit()).Find(&roles).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}

	items := s.toVOs(roles)
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

	var out []Option
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
	vo := s.toVO(&r)
	return &vo, nil
}

// Create 新增角色。
func (s *Service) Create(req CreateReq) (int64, error) {
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
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return r.ID, nil
}

// Update 修改角色。
func (s *Service) Update(id int64, req UpdateReq) error {
	var r model.Role
	if err := s.db.First(&r, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "角色不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
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

// Delete 删除角色。
func (s *Service) Delete(id int64) error {
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
func (s *Service) AssignMenus(id int64, menuIDs []int64) error {
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
func (s *Service) toVO(r *model.Role) VO {
	var menuIDs []int64
	_ = s.db.Model(&model.RoleMenu{}).Where("role_id = ?", r.ID).Pluck("menu_id", &menuIDs)
	var userCount int64
	_ = s.db.Model(&model.UserRole{}).Where("role_id = ?", r.ID).Count(&userCount).Error
	return buildRoleVO(r, menuIDs, userCount)
}

// toVOs 批量构造角色视图对象。
//
// 旧实现逐个角色各查一次「菜单 ID」和「用户数」，分页 20 条就是 40 次查询（N+1）。
// 这里改成两条聚合查询，查询次数与分页大小无关。
func (s *Service) toVOs(roles []model.Role) []VO {
	items := make([]VO, 0, len(roles))
	if len(roles) == 0 {
		return items
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
	_ = s.db.Model(&model.RoleMenu{}).
		Select("role_id, menu_id").Where("role_id IN ?", ids).Scan(&menuRows).Error
	menuByRole := make(map[int64][]int64, len(roles))
	for _, r := range menuRows {
		menuByRole[r.RoleID] = append(menuByRole[r.RoleID], r.MenuID)
	}

	type countRow struct {
		RoleID int64
		Total  int64
	}
	var countRows []countRow
	_ = s.db.Model(&model.UserRole{}).
		Select("role_id, COUNT(*) AS total").Where("role_id IN ?", ids).
		Group("role_id").Scan(&countRows).Error
	countByRole := make(map[int64]int64, len(roles))
	for _, r := range countRows {
		countByRole[r.RoleID] = r.Total
	}

	for i := range roles {
		items = append(items, buildRoleVO(&roles[i], menuByRole[roles[i].ID], countByRole[roles[i].ID]))
	}
	return items
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
