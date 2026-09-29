package dept

import (
	"errors"

	"gorm.io/gorm"

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

// List 返回部门树。
func (s *Service) List(keyword string, status *int16) ([]*Node, error) {
	tx := s.db.Model(&model.Dept{})
	if keyword != "" {
		kw := "%" + keyword + "%"
		tx = tx.Where("name ILIKE ? OR leader ILIKE ?", kw, kw)
	}
	if status != nil {
		tx = tx.Where("status = ?", *status)
	}
	var depts []model.Dept
	if err := tx.Order("sort ASC, id ASC").Find(&depts).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}

	counts, err := s.userCounts()
	if err != nil {
		return nil, err
	}
	return buildTree(depts, counts), nil
}

// Get 查询部门详情。
func (s *Service) Get(id int64) (*Node, error) {
	var d model.Dept
	if err := s.db.First(&d, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errcode.New(errcode.ErrNotFound, "部门不存在")
		}
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	n := Node{
		ID: d.ID, ParentID: d.ParentID, Name: d.Name, Leader: d.Leader,
		Phone: d.Phone, Email: d.Email, Sort: d.Sort, Status: d.Status, CreatedAt: d.CreatedAt,
	}
	return &n, nil
}

// Create 新增部门。
func (s *Service) Create(req CreateReq) (int64, error) {
	if err := s.checkParent(req.ParentID, 0); err != nil {
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
func (s *Service) Update(id int64, req UpdateReq) error {
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
func (s *Service) Delete(id int64) error {
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

func (s *Service) userCounts() (map[int64]int64, error) {
	type row struct {
		DeptID int64
		Total  int64
	}
	var rows []row
	if err := s.db.Model(&model.User{}).
		Select("dept_id, COUNT(*) AS total").
		Where("dept_id IS NOT NULL").
		Group("dept_id").Scan(&rows).Error; err != nil {
		return nil, errcode.Wrap(errcode.ErrDatabase, err)
	}
	out := make(map[int64]int64, len(rows))
	for _, r := range rows {
		out[r.DeptID] = r.Total
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
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == parentID {
				return errcode.New(errcode.ErrBadRequest, "不能选择自己的子部门作为上级")
			}
			queue = append(queue, children[cur]...)
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
