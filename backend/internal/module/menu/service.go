package menu

import (
	"errors"
	"sort"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
)

// Service 菜单服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建菜单服务。
func NewService() *Service { return &Service{db: database.DB()} }

// List 返回菜单树，可按关键字与状态过滤。
func (s *Service) List(keyword string, status *int16) ([]*Node, error) {
	tx := s.db.Model(&model.Menu{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("title ILIKE ? OR name ILIKE ? OR permission ILIKE ?", kw, kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	var menus []model.Menu
	if err := tx.Order("sort ASC, id ASC").Find(&menus).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return buildTree(menus), nil
}

// Get 查询菜单详情。
func (s *Service) Get(id int64) (*Node, error) {
	var m model.Menu
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "菜单不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	n := toNode(m)
	return &n, nil
}

// Create 新增菜单。
//
// actor 不可省：非超管创建带 permission 的菜单时，该码必须是自己持有的
// （否则造出一个带任意权限码的菜单，再由角色管理挂上即完成授权扩散）。
func (s *Service) Create(req CreateReq, actor *authz.Identity) (int64, error) {
	if err := authz.EnsurePermissionGrantable(actor, req.Permission); err != nil {
		return 0, err
	}
	if err := s.checkParent(req.ParentID, 0); err != nil {
		return 0, err
	}
	if err := s.ensureNameFree(req.Name, 0); err != nil {
		return 0, err
	}
	m := req.toModel()
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&m).Error; err != nil {
			return err
		}
		// 目录类菜单自动挂载到超管角色，保证新增后立即可见
		return attachToSuperRole(tx, m.ID)
	})
	if err != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return m.ID, nil
}

// Update 修改菜单。
//
// permission 的子集校验只看「本次提交值」：把码从 A 改成自己没有的 B
// 等价于给所有挂载角色批量发新权限；改回空串或自己的码都放行。
// 历史上菜单挂了什么角色不改变这一判定——角色侧的授予校验
// （EnsureRoleMenusGrantable）负责另一条轴。
func (s *Service) Update(id int64, req UpdateReq, actor *authz.Identity) error {
	if err := authz.EnsurePermissionGrantable(actor, req.Permission); err != nil {
		return err
	}
	var m model.Menu
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "菜单不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if req.ParentID == id {
		return errcode.New(errcode.ErrBadRequest, "上级菜单不能是自己")
	}
	if err := s.checkParent(req.ParentID, id); err != nil {
		return err
	}
	if err := s.ensureNameFree(req.Name, id); err != nil {
		return err
	}

	updates := map[string]any{
		"parent_id":   req.ParentID,
		"name":        req.Name,
		"path":        req.Path,
		"component":   req.Component,
		"redirect":    req.Redirect,
		"title":       req.Title,
		"icon":        req.Icon,
		"type":        req.Type,
		"permission":  req.Permission,
		"sort":        req.Sort,
		"hidden":      req.Hidden,
		"keep_alive":  req.KeepAlive,
		"is_external": req.IsExternal,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.db.Model(&model.Menu{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return nil
}

// Delete 删除菜单（存在子节点时禁止删除）。
func (s *Service) Delete(id int64) error {
	var m model.Menu
	if err := s.db.First(&m, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "菜单不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	var childCount int64
	if err := s.db.Model(&model.Menu{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if childCount > 0 {
		return errcode.Newf(errcode.ErrConflict, "存在 %d 个子菜单，请先删除子菜单", childCount)
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("menu_id = ?", id).Delete(&model.RoleMenu{}).Error; err != nil {
			return err
		}
		return tx.Delete(&model.Menu{}, id).Error
	})
	if err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	authz.InvalidateAll()
	return nil
}

// RoutesFor 返回指定身份可访问的前端路由树。
func (s *Service) RoutesFor(id *authz.Identity) ([]*Route, error) {
	var menus []model.Menu
	tx := s.db.Where("status = ? AND type IN ?", model.StatusEnabled, []string{model.MenuTypeDir, model.MenuTypeMenu})

	if !id.IsSuper {
		ids, err := s.menuIDsOfRoles(id.RoleIDs)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return []*Route{}, nil
		}
		ids, err = s.withAncestors(ids)
		if err != nil {
			return nil, err
		}
		tx = tx.Where("id IN ?", ids)
	}

	if err := tx.Order("sort ASC, id ASC").Find(&menus).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return buildRouteTree(menus), nil
}

// menuIDsOfRoles 查询角色关联的菜单 ID。
func (s *Service) menuIDsOfRoles(roleIDs []int64) ([]int64, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	var ids []int64
	if err := s.db.Model(&model.RoleMenu{}).Where("role_id IN ?", roleIDs).
		Distinct().Pluck("menu_id", &ids).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return ids, nil
}

// withAncestors 补齐所有祖先菜单，避免授权了子菜单却缺少父级目录。
func (s *Service) withAncestors(ids []int64) ([]int64, error) {
	var all []model.Menu
	if err := s.db.Select("id", "parent_id").Find(&all).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	parent := make(map[int64]int64, len(all))
	for _, m := range all {
		parent[m.ID] = m.ParentID
	}
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		cur := id
		for cur > 0 {
			if _, ok := set[cur]; ok {
				break
			}
			set[cur] = struct{}{}
			cur = parent[cur]
		}
	}
	out := make([]int64, 0, len(set))
	for id := range set {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (s *Service) checkParent(parentID, selfID int64) error {
	if parentID == 0 {
		return nil
	}
	var cnt int64
	if err := s.db.Model(&model.Menu{}).Where("id = ?", parentID).Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt == 0 {
		return errcode.New(errcode.ErrBadRequest, "上级菜单不存在")
	}
	if selfID > 0 {
		// 禁止把菜单挂到自己的子孙节点下，造成环
		desc, err := s.descendantIDs(selfID)
		if err != nil {
			return err
		}
		for _, d := range desc {
			if d == parentID {
				return errcode.New(errcode.ErrBadRequest, "不能选择自己的子菜单作为上级")
			}
		}
	}
	return nil
}

func (s *Service) descendantIDs(rootID int64) ([]int64, error) {
	var all []model.Menu
	if err := s.db.Select("id", "parent_id").Find(&all).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	children := map[int64][]int64{}
	for _, m := range all {
		children[m.ParentID] = append(children[m.ParentID], m.ID)
	}
	var out []int64
	visited := map[int64]bool{rootID: true}
	queue := []int64{rootID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, c := range children[cur] {
			if visited[c] {
				// parent_id 数据被手工改成环时防止死循环（dept/authz 同款实现均有 visited）
				continue
			}
			visited[c] = true
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out, nil
}

func (s *Service) ensureNameFree(name string, excludeID int64) error {
	if name == "" {
		return nil
	}
	tx := s.db.Model(&model.Menu{}).Where("name = ?", name)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "路由名称 %s 已存在", name)
	}
	return nil
}

func (r CreateReq) toModel() model.Menu {
	status := model.StatusEnabled
	if r.Status != nil {
		status = *r.Status
	}
	return model.Menu{
		ParentID:   r.ParentID,
		Name:       r.Name,
		Path:       r.Path,
		Component:  r.Component,
		Redirect:   r.Redirect,
		Title:      r.Title,
		Icon:       r.Icon,
		Type:       r.Type,
		Permission: r.Permission,
		Sort:       r.Sort,
		Hidden:     r.Hidden,
		KeepAlive:  r.KeepAlive,
		IsExternal: r.IsExternal,
		Status:     status,
	}
}

func toNode(m model.Menu) Node {
	return Node{
		ID:         m.ID,
		ParentID:   m.ParentID,
		Name:       m.Name,
		Path:       m.Path,
		Component:  m.Component,
		Redirect:   m.Redirect,
		Title:      m.Title,
		Icon:       m.Icon,
		Type:       m.Type,
		Permission: m.Permission,
		Sort:       m.Sort,
		Hidden:     m.Hidden,
		KeepAlive:  m.KeepAlive,
		IsExternal: m.IsExternal,
		Status:     m.Status,
	}
}

func buildTree(menus []model.Menu) []*Node {
	nodes := make(map[int64]*Node, len(menus))
	for _, m := range menus {
		n := toNode(m)
		nodes[m.ID] = &n
	}
	roots := make([]*Node, 0)
	for _, m := range menus {
		node := nodes[m.ID]
		if parent, ok := nodes[m.ParentID]; ok && m.ParentID != 0 {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}
	return roots
}

func buildRouteTree(menus []model.Menu) []*Route {
	nodes := make(map[int64]*Route, len(menus))
	for _, m := range menus {
		nodes[m.ID] = &Route{
			ID:        m.ID,
			Name:      m.Name,
			Path:      m.Path,
			Component: m.Component,
			Redirect:  m.Redirect,
			Meta: RouteMeta{
				Title:      m.Title,
				Icon:       m.Icon,
				Hidden:     m.Hidden,
				KeepAlive:  m.KeepAlive,
				IsExternal: m.IsExternal,
				Permission: m.Permission,
			},
		}
	}
	roots := make([]*Route, 0)
	for _, m := range menus {
		node := nodes[m.ID]
		if parent, ok := nodes[m.ParentID]; ok && m.ParentID != 0 {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}
	return roots
}

// attachToSuperRole 将新菜单授权给超管角色，超管也会走角色授权（冗余但更直观）。
func attachToSuperRole(tx *gorm.DB, menuID int64) error {
	var super model.Role
	if err := tx.Where("code = ?", authz.SuperRoleCode).First(&super).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var cnt int64
	if err := tx.Model(&model.RoleMenu{}).
		Where("role_id = ? AND menu_id = ?", super.ID, menuID).Count(&cnt).Error; err != nil {
		return err
	}
	if cnt > 0 {
		return nil
	}
	return tx.Create(&model.RoleMenu{RoleID: super.ID, MenuID: menuID}).Error
}
