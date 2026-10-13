package inv

import (
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/page"
)

// printTemplateFree 校验模板编码唯一。
func (s *Service) printTemplateFree(code string, excludeID int64) error {
	var cnt int64
	tx := s.db.Model(&model.InvPrintTemplate{}).Where("code = ?", code)
	if excludeID > 0 {
		tx = tx.Where("id <> ?", excludeID)
	}
	if err := tx.Count(&cnt).Error; err != nil {
		return errcode.Wrap(errcode.ErrDatabase, err)
	}
	if cnt > 0 {
		return errcode.Newf(errcode.ErrConflict, "模板编码 %s 已存在", code)
	}
	return nil
}

// PrintTemplateList 打印模板列表。
func (s *Service) PrintTemplateList(q page.Query) ([]PrintTemplateRow, int64, error) {
	tx := s.db.Model(&model.InvPrintTemplate{})
	if q.Keyword != "" {
		kw := likeKW(q.Keyword)
		tx = tx.Where(likeExpr("code")+" OR "+likeExpr("name")+" OR "+likeExpr("doc_type"), kw, kw, kw)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	var list []model.InvPrintTemplate
	if err := q.Paginate(tx.Order("id ASC")).Find(&list).Error; err != nil {
		return nil, 0, errcode.Wrap(errcode.ErrDatabase, err)
	}
	rows := make([]PrintTemplateRow, 0, len(list))
	for _, it := range list {
		rows = append(rows, PrintTemplateRow{
			ID: it.ID, Code: it.Code, Name: it.Name, DocType: it.DocType, Paper: it.Paper,
			Content: it.Content, Status: it.Status, Remark: it.Remark, CreatedAt: it.CreatedAt,
		})
	}
	return rows, total, nil
}

// PrintTemplateCreate 新增打印模板。
func (s *Service) PrintTemplateCreate(req PrintTemplateReq) (int64, error) {
	if err := s.printTemplateFree(req.Code, 0); err != nil {
		return 0, err
	}
	status := model.StatusEnabled
	if req.Status != nil {
		status = *req.Status
	}
	paper := req.Paper
	if paper == "" {
		paper = "A4"
	}
	it := model.InvPrintTemplate{
		Code: req.Code, Name: req.Name, DocType: req.DocType, Paper: paper,
		Content: req.Content, Status: status, Remark: req.Remark,
	}
	if err := s.db.Create(&it).Error; err != nil {
		return 0, saveErr(err)
	}
	return it.ID, nil
}

// PrintTemplateUpdate 修改打印模板。
func (s *Service) PrintTemplateUpdate(id int64, req PrintTemplateReq) error {
	if err := s.printTemplateFree(req.Code, id); err != nil {
		return err
	}
	updates := map[string]any{
		"code": req.Code, "name": req.Name, "doc_type": req.DocType,
		"paper": req.Paper, "content": req.Content, "remark": req.Remark,
	}
	if req.Status != nil {
		updates["status"] = *req.Status
	}
	res := s.db.Model(&model.InvPrintTemplate{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "模板不存在")
	}
	return nil
}

// PrintTemplateDelete 删除打印模板。
func (s *Service) PrintTemplateDelete(id int64) error {
	res := s.db.Delete(&model.InvPrintTemplate{}, id)
	if res.Error != nil {
		return saveErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return errcode.New(errcode.ErrNotFound, "模板不存在")
	}
	return nil
}
