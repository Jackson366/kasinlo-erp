package dept

import (
	"errors"

	"gorm.io/gorm"

	"nova-admin/internal/authz"
	"nova-admin/internal/database"
	"nova-admin/internal/model"
	"nova-admin/internal/pkg/errcode"
)

// Service 部门服务。
type Service struct {
	db *gorm.DB
}

// NewService 创建部门服务。
func NewService() *Service { return &Service{db: database.DB()} }

// List 返回部门树。actor 的数据范围会同时约束「能看到哪些部门」
// 与「每个部门显示多少人」——只过滤前者的话，部门树虽然被裁掉了，
// 但仍然会通过 userCount 泄露其他部门的人数。
func (s *Service) List(keyword string, status *int16, actor *authz.Identity) ([]*Node, error) {
	scope := authz.ResolveScope(actor)
	tx := s.db.Model(&model.Dept{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("name ILIKE ? OR leader ILIKE ?", kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	// 非超管只能看到范围内的部门；裁剪后原父节点可能不在集合内，
	// buildTree 会把这些节点当作根处理，页面不会出现悬空层级。
	if cond, args, ok := scope.DeptIn(""); ok {
		tx = tx.Where(cond, args...)
	}
	var depts []model.Dept
	if err := tx.Order("sort ASC, id ASC").Find(&depts).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	counts, err := s.userCounts(scope)
	if err != nil {
		return nil, err
	}
	return buildTree(depts, counts), nil
}

// Get 查询部门详情。超出数据范围时按「不存在」返回。
func (s *Service) Get(id int64, actor *authz.Identity) (*Node, error) {
	var d model.Dept
	if err := s.db.First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "部门不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	// 先查库再判范围：越界时也返回「部门不存在」，不泄露该 ID 是否真实存在
	if err := authz.ResolveScope(actor).EnsureDeptVisible(id); err != nil {
		return nil, err
	}
	n := Node{
		ID: d.ID, ParentID: d.ParentID, Name: d.Name, Leader: d.Leader,
		Phone: d.Phone, Email: d.Email, Sort: d.Sort, Status: d.Status, CreatedAt: d.CreatedAt,
	}
	return &n, nil
}

// Create 新增部门。
func (s *Service) Create(req CreateReq, actor *authz.Identity) (int64, error) {
	if err := s.checkParent(req.ParentID, 0); err != nil {
		return 0, err
	}
	if err := authz.ResolveScope(actor).EnsureDeptWritable(req.ParentID); err != nil {
		return 0, err
	}
	if err := s.ensureNameFree(req.ParentID, req.Name, 0); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	d := model.Dept{
		ParentID: req.ParentID, Name: req.Name, Leader: req.Leader,
		Phone: req.Phone, Email: req.Email, Sort: req.Sort, Status: status,
	}
	if err := s.db.Create(&d).Error; err != nil {
		return 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	return d.ID, nil
}

// Update 修改部门。
func (s *Service) Update(id int64, req UpdateReq, actor *authz.Identity) error {
	scope := authz.ResolveScope(actor)
	// 目标部门必须先在范围内，再谈改内容：
	// 否则「拿已知 ID 改别人的部门名」是一条与读权限无关的旁路。
	if err := scope.EnsureDeptWritable(id); err != nil {
		return err
	}
	// 挂到范围外的父部门下，等于把自己挪出管辖范围，数据范围会随之失效
	if err := scope.EnsureDeptWritable(req.ParentID); err != nil {
		return err
	}
	var d model.Dept
	if err := s.db.First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "部门不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if req.ParentID == id {
		return errcode.New(errcode.ErrBadRequest, "上级部门不能是自己")
	}
	if err := s.checkParent(req.ParentID, id); err != nil {
		return err
	}
	if err := s.ensureNameFree(req.ParentID, req.Name, id); err != nil {
		return err
	}

	updates := map[string]any{
		"parent_id": req.ParentID,
		"name":      req.Name,
		"leader":    req.Leader,
		"phone":     req.Phone,
		"email":     req.Email,
		"sort":      req.Sort,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	if err := s.db.Model(&model.Dept{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	return nil
}

// Delete 删除部门（有子部门或在职用户时禁止删除）。
func (s *Service) Delete(id int64, actor *authz.Identity) error {
	if err := authz.ResolveScope(actor).EnsureDeptWritable(id); err != nil {
		return err
	}
	var d model.Dept
	if err := s.db.First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "部门不存在")
		}
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	var childCount int64
	if err := s.db.Model(&model.Dept{}).Where("parent_id = ?", id).Count(&childCount).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if childCount > 0 {
		return errcode.Newf(errcode.ErrConflict, "存在 %d 个子部门，请先删除子部门", childCount)
	}
	var userCount int64
	if err := s.db.Model(&model.User{}).Where("dept_id = ?", id).Count(&userCount).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if userCount > 0 {
		return errcode.Newf(errcode.ErrConflict, "该部门下还有 %d 个用户，请先调整用户部门", userCount)
	}
	if err := s.db.Delete(&model.Dept{}, id).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	return nil
}

// userCounts 返回每个部门「含所有子部门」的用户数。
//
// 旧实现只按 dept_id 分组，统计的是直属成员：于是「总公司」这种只做汇总的
// 顶层部门会显示 0 人，页面看起来像数据没加载出来。这里把子部门的人数向上累加。
//
// scope 决定「谁的部门算在内」：非超管只能统计自己数据范围内的用户，
// 否则部门树被裁剪了、人数却还是全库的，等于换了个字段继续泄露。
func (s *Service) userCounts(scope *authz.Scope) (map[int64]int64, error) {
	type row struct {
		DeptID int64
		Total  int64
	}
	var rows []row
	if err := scope.ApplyUser(s.db.Model(&model.User{}).Select("dept_id")).
		Where("dept_id IS NOT NULL").
		Group("dept_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	direct := make(map[int64]int64, len(rows))
	for _, r := range rows {
		direct[r.DeptID] = r.Total
	}

	var all []model.Dept
	if err := s.db.Select("id", "parent_id").Find(&all).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	children := make(map[int64][]int64, len(all))
	for _, d := range all {
		children[d.ParentID] = append(children[d.ParentID], d.ID)
	}

	// out 同时充当结果缓存与「已计算」标记，避免重复递归；
	// onCycle 用来打断异常数据（parent_id 手工改错造成的环），
	// 否则 sum 会在环上无限递归直到栈溢出。
	out := make(map[int64]int64, len(all))
	onCycle := make(map[int64]bool, len(all))
	var sum func(id int64) int64
	sum = func(id int64) int64 {
		if v, ok := out[id]; ok {
			return v
		}
		if onCycle[id] {
			return 0
		}
		onCycle[id] = true
		total := direct[id]
		for _, c := range children[id] {
			total += sum(c)
		}
		delete(onCycle, id)
		out[id] = total
		return total
	}
	for _, d := range all {
		sum(d.ID)
	}
	return out, nil
}

func (s *Service) ensureNameFree(parentID int64, name string, excludeID int64) error {
	tx := s.db.Model(&model.Dept{}).Where("parent_id = ? AND name = ?", parentID, name)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	var cnt int64
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "同级下已存在部门 %s", name)
	}
	return nil
}

func (s *Service) checkParent(parentID, selfID int64) error {
	if parentID == 0 {
		return nil
	}
	var cnt int64
	if err := s.db.Model(&model.Dept{}).Where("id = ?", parentID).Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt == 0 {
		return errcode.New(errcode.ErrBadRequest, "上级部门不存在")
	}
	if selfID > 0 {
		var all []model.Dept
		if err := s.db.Select("id", "parent_id").Find(&all).Error; err != nil {
			return errcode.Wrap(errcode.ErrDatabase, err)
		}
		children := map[int64][]int64{}
		for _, d := range all {
			children[d.ParentID] = append(children[d.ParentID], d.ID)
		}
		queue := []int64{selfID}
		visited := map[int64]bool{selfID: true}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == parentID {
				return errcode.New(errcode.ErrBadRequest, "不能选择自己的子部门作为上级")
			}
			// visited 防止异常数据成环时 BFS 无限循环
			for _, cid := range children[cur] {
				if visited[cid] {
					continue
				}
				visited[cid] = true
				queue = append(queue, cid)
			}
		}
	}
	return nil
}

func buildTree(depts []model.Dept, counts map[int64]int64) []*Node {
	nodes := make(map[int64]*Node, len(depts))
	for _, d := range depts {
		nodes[d.ID] = &Node{
			ID: d.ID, ParentID: d.ParentID, Name: d.Name, Leader: d.Leader,
			Phone: d.Phone, Email: d.Email, Sort: d.Sort, Status: d.Status,
			UserCount: counts[d.ID], CreatedAt: d.CreatedAt,
		}
	}
	roots := make([]*Node, 0)
	for _, d := range depts {
		node := nodes[d.ID]
		if parent, ok := nodes[d.ParentID]; ok && d.ParentID != 0 {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}
	return roots
}
