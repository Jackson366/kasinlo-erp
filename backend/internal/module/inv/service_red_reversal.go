package inv

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"nova-erp/internal/authz"
	"nova-erp/internal/database"
	"nova-erp/internal/model"
	"nova-erp/internal/pkg/errcode"
	"nova-erp/internal/pkg/textutil"
)

func (s *Service) redReversalOrder(tx *gorm.DB, id int64, actor *authz.Identity) (model.InvOrder, error) {
	var order model.InvOrder
	if err := s.applyCreator(tx, actor).First(&order, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return order, errcode.New(errcode.ErrNotFound, "单据不存在")
		}
		return order, err
	}
	if !actor.HasAnyPerm("inv:" + order.BizType + ":reverse") {
		return order, errcode.New(errcode.ErrForbidden, "没有该单据的红冲权限")
	}
	return order, nil
}

func redReversalBlock(order model.InvOrder) string {
	switch {
	case order.Status == model.InvStatusRedReversed:
		return "该单据已红冲"
	case order.Status != model.InvStatusDone:
		return "仅已审核的原单可红冲"
	case order.SettledAmount != 0:
		return "本单仍有已结算金额，请先从收付款明细处理款项。"
	default:
		return ""
	}
}

func redReversalStock(tx *gorm.DB, order model.InvOrder) (model.InvStockOrder, []model.InvStockItem, error) {
	var stock model.InvStockOrder
	var items []model.InvStockItem
	if err := tx.Where("source_type = ? AND source_id = ? AND status = 1 AND reversal_of IS NULL",
		order.BizType, order.ID).Take(&stock).Error; err != nil {
		return stock, nil, err
	}
	if err := tx.Where("stock_order_id = ?", stock.ID).Order("id ASC").Find(&items).Error; err != nil {
		return stock, nil, err
	}
	if len(items) == 0 {
		return stock, nil, gorm.ErrRecordNotFound
	}
	return stock, items, nil
}

func oppositeStockDirection(direction string) string {
	if direction == model.InvStockOut {
		return model.InvStockIn
	}
	return model.InvStockOut
}

func (s *Service) OrderRedReversalPreview(id int64, actor *authz.Identity) (*RedReversalPreview, error) {
	order, err := s.redReversalOrder(s.db, id, actor)
	if err != nil {
		return nil, saveErr(err)
	}
	preview := &RedReversalPreview{
		Outstanding: round2(order.TotalAmount - order.SettledAmount),
		Items:       make([]RedReversalStockLine, 0),
		BlockReason: redReversalBlock(order),
	}
	if preview.BlockReason != "" {
		return preview, nil
	}
	stock, items, err := redReversalStock(s.db, order)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		preview.BlockReason = "找不到有效的原出入库明细，请核对原单的过账记录。"
		return preview, nil
	}
	if err != nil {
		return nil, saveErr(err)
	}
	preview.StockOrderID = stock.ID
	preview.Direction = oppositeStockDirection(stock.Direction)
	quantities := make(map[int64]float64)
	names := make(map[int64]string)
	for _, item := range items {
		preview.Items = append(preview.Items, RedReversalStockLine{
			ProductName: item.ProductName, Spec: item.Spec, Qty: item.Qty, Unit: item.Unit,
		})
		quantities[item.SpecID] += item.QtyBase
		names[item.SpecID] = strings.TrimSpace(item.ProductName + " " + item.Spec)
	}
	var specs []model.InvProductSpec
	ids := make([]int64, 0, len(quantities))
	for id := range quantities {
		ids = append(ids, id)
	}
	if err := s.db.Where("id IN ?", ids).Order("id ASC").Find(&specs).Error; err != nil {
		return nil, saveErr(err)
	}
	if len(specs) != len(quantities) {
		preview.BlockReason = "原出入库明细中的商品规格已不存在，请核对商品档案。"
		return preview, nil
	}
	if preview.Direction == model.InvStockOut {
		for _, spec := range specs {
			needed := round4(quantities[spec.ID])
			if spec.Stock-needed < -0.00005 {
				preview.BlockReason = fmt.Sprintf("「%s」库存不足，现有 %s，需 %s（基本单位）。",
					names[spec.ID], fmtQty(spec.Stock), fmtQty(needed))
				break
			}
		}
	}
	return preview, nil
}

func (s *Service) OrderRedReversal(id int64, req RedReversalRequest, actor *authz.Identity) (RedReversalOrderLink, error) {
	var result RedReversalOrderLink
	reason := textutil.Fit(strings.TrimSpace(req.Reason), 255)
	if reason == "" {
		return result, errcode.New(errcode.ErrBadRequest, "请填写红冲原因")
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		order, err := s.redReversalOrder(tx.Clauses(clause.Locking{Strength: "UPDATE"}), id, actor)
		if err != nil {
			return err
		}
		if order.Status == model.InvStatusRedReversed {
			return tx.Model(&model.InvOrder{}).Where("reversal_of = ?", id).Take(&result).Error
		}
		if msg := redReversalBlock(order); msg != "" {
			return errcode.New(errcode.ErrConflict, msg)
		}
		stock, stockItems, err := redReversalStock(tx, order)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrConflict, "找不到有效的原出入库明细，请核对原单的过账记录")
		}
		if err != nil {
			return err
		}
		if stock.ID != req.StockOrderID {
			return errcode.New(errcode.ErrConflict, "原单已重新过账，请刷新红冲影响后再确认")
		}
		var items []model.InvOrderItem
		if err := tx.Where("order_id = ?", id).Order("id ASC").Find(&items).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return errcode.New(errcode.ErrConflict, "原单缺少单据明细，无法红冲")
		}
		now := time.Now().In(database.Location())
		userID := actorUID(actor)
		red := model.InvOrder{
			OrderNo: "HC-" + rand.Text(), BizType: order.BizType, PartyID: order.PartyID,
			PartyName: order.PartyName, OrderDate: now, Status: model.InvStatusRedReversal,
			TotalAmount: -order.TotalAmount, Remark: reason, Salesman: order.Salesman,
			CreatedBy: order.CreatedBy, AuditorID: &userID, AuditedAt: &now, ReversalOf: &id,
		}
		if err := tx.Create(&red).Error; err != nil {
			return err
		}
		for i := range items {
			items[i].BaseModel = model.BaseModel{}
			items[i].OrderID = red.ID
			items[i].Qty = -items[i].Qty
			items[i].QtyBase = -items[i].QtyBase
			items[i].Amount = -items[i].Amount
		}
		if err := tx.Create(&items).Error; err != nil {
			return err
		}
		reverseStock := model.InvStockOrder{
			OrderNo: "HCK-" + rand.Text(), Direction: oppositeStockDirection(stock.Direction),
			SourceType: order.BizType, SourceID: red.ID, OrderDate: now, Status: 1,
			Remark: reason, Salesman: stock.Salesman, CreatedBy: stock.CreatedBy,
			ReversalOf: &stock.ID, ReversalBy: &userID,
		}
		if err := tx.Create(&reverseStock).Error; err != nil {
			return err
		}
		for i := range stockItems {
			stockItems[i].BaseModel = model.BaseModel{}
			stockItems[i].StockOrderID = reverseStock.ID
		}
		if err := tx.Create(&stockItems).Error; err != nil {
			return err
		}
		if err := s.applyStock(tx, reverseStock.Direction, stockItems); err != nil {
			return err
		}
		if err := recordOrderAudit(tx, order, "red-reverse", &reverseStock.ID, actor, now); err != nil {
			return err
		}
		res := tx.Model(&model.InvOrder{}).Where("id = ? AND status = ? AND settled_amount = 0",
			id, model.InvStatusDone).Update("status", model.InvStatusRedReversed)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errcode.New(errcode.ErrConflict, "原单状态已变化，请重新查看红冲影响")
		}
		result = RedReversalOrderLink{ID: red.ID, OrderNo: red.OrderNo}
		return nil
	})
	return result, saveErr(err)
}
