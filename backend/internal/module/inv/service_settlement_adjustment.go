package inv

import (
	"database/sql"
	"errors"
	"slices"
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

const settlementOffsetIn = "offset_in"

func (s *Service) financeOrder(tx *gorm.DB, id int64, actor *authz.Identity) (model.InvOrder, error) {
	var order model.InvOrder
	err := s.applyCreator(tx, actor).First(&order, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return order, errcode.New(errcode.ErrNotFound, "单据不存在")
	}
	return order, err
}

func (s *Service) OrderSettlementHistory(id int64, actor *authz.Identity) (*OrderSettlements, error) {
	var order model.InvOrder
	var entries []model.InvSettlement
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		order, err = s.financeOrder(tx, id, actor)
		if err != nil {
			return err
		}
		return tx.Where("order_id = ? AND source = ?", id, "order").Order("id DESC").Find(&entries).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, saveErr(err)
	}
	history := &OrderSettlements{Rows: []OrderSettlementRow{}, Settled: order.SettledAmount}
	if order.Status == model.InvStatusDone {
		history.Outstanding = round2(order.TotalAmount - order.SettledAmount)
		history.CanAdjust = actor.HasAnyPerm("inv:finance:adjust")
	}
	operators := make([]int64, 0, len(entries))
	counterIDs := make([]int64, 0)
	adjusted := make(map[int64]float64)
	for _, entry := range entries {
		operators = append(operators, entry.CreatedBy)
		if entry.CounterOrderID != nil {
			counterIDs = append(counterIDs, *entry.CounterOrderID)
		}
		if entry.Amount < 0 && entry.AdjustsID != nil {
			adjusted[*entry.AdjustsID] += entry.Amount
		}
	}
	names, err := s.operatorNames(operators)
	if err != nil {
		return nil, err
	}
	counters := make(map[int64]RedReversalOrderLink)
	if len(counterIDs) > 0 {
		var links []RedReversalOrderLink
		if err := s.applyCreator(s.db.Model(&model.InvOrder{}), actor).Select("id", "order_no").
			Where("id IN ?", counterIDs).Find(&links).Error; err != nil {
			return nil, saveErr(err)
		}
		for _, link := range links {
			counters[link.ID] = link
		}
	}
	for _, entry := range entries {
		row := OrderSettlementRow{
			ID: entry.ID, Amount: entry.Amount, Kind: entry.Kind, Method: entry.Method,
			SettledAt: entry.SettledAt, Remark: entry.Remark, Reference: entry.Reference,
			Operator: names[entry.CreatedBy], AdjustsID: entry.AdjustsID,
		}
		if entry.Amount > 0 {
			row.Available = round2(entry.Amount + adjusted[entry.ID])
		}
		if entry.CounterOrderID != nil {
			if link, ok := counters[*entry.CounterOrderID]; ok {
				row.CounterOrder = &link
			}
		}
		history.Rows = append(history.Rows, row)
	}
	return history, nil
}

func (s *Service) SettlementOffsetTargets(id int64, keyword string, actor *authz.Identity) ([]SettlementOffsetTarget, error) {
	order, err := s.financeOrder(s.db, id, actor)
	if err != nil {
		return nil, saveErr(err)
	}
	targets := make([]SettlementOffsetTarget, 0)
	if order.Status != model.InvStatusDone || order.SettledAmount <= 0 {
		return targets, nil
	}
	q := s.applyCreator(s.db.Model(&model.InvOrder{}), actor).
		Select("id, order_no, total_amount - settled_amount AS outstanding").
		Where("id <> ? AND status = ? AND biz_type = ? AND party_id = ? AND total_amount > settled_amount",
			id, model.InvStatusDone, order.BizType, order.PartyID)
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		q = q.Where(likeExpr("order_no"), likeKW(keyword))
	}
	err = q.Order("order_date DESC, id DESC").Limit(30).Find(&targets).Error
	return targets, saveErr(err)
}

func (s *Service) AdjustOrderSettlement(id, settlementID int64, req SettlementAdjustmentReq, actor *authz.Identity) (int64, error) {
	amount := round2(req.Amount)
	reason := textutil.Fit(strings.TrimSpace(req.Reason), 255)
	reference := textutil.Fit(strings.TrimSpace(req.Reference), 128)
	now := time.Now().In(database.Location())
	postedAt := now
	var ids = []int64{id}
	if amount <= 0 || reason == "" {
		return 0, errcode.New(errcode.ErrBadRequest, "请填写处理原因和至少 0.01 元的处理金额")
	}
	switch req.Kind {
	case "correction":
	case "refund":
		date, err := parseDate(req.SettledAt)
		if err != nil {
			return 0, err
		}
		if reference == "" || date.IsZero() || date.After(now) {
			return 0, errcode.New(errcode.ErrBadRequest, "请填写已实际退款的日期和凭证号")
		}
		postedAt = date
	case "offset":
		if req.TargetOrderID <= 0 || req.TargetOrderID == id {
			return 0, errcode.New(errcode.ErrBadRequest, "请选择另一张可抵扣单据")
		}
		ids = append(ids, req.TargetOrderID)
		slices.Sort(ids)
	default:
		return 0, errcode.New(errcode.ErrBadRequest, "不支持该处理方式")
	}
	var result int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		orders := make(map[int64]model.InvOrder, len(ids))
		for _, orderID := range ids {
			order, err := s.financeOrder(tx.Clauses(clause.Locking{Strength: "UPDATE"}), orderID, actor)
			if err != nil {
				return err
			}
			orders[orderID] = order
		}
		order := orders[id]
		var prior model.InvSettlement
		err := tx.Where("adjustment_key = ? AND order_id = ?", req.Key, id).Take(&prior).Error
		if err == nil {
			targetMatches := prior.CounterOrderID == nil
			if req.Kind == "offset" {
				targetMatches = prior.CounterOrderID != nil && *prior.CounterOrderID == req.TargetOrderID
			}
			if prior.AdjustsID != nil && *prior.AdjustsID == settlementID && prior.Kind == req.Kind &&
				prior.Amount == -amount && prior.Remark == reason && prior.Reference == reference && targetMatches &&
				(req.Kind != "refund" || prior.SettledAt.Format(time.DateOnly) == postedAt.Format(time.DateOnly)) {
				result = prior.ID
				return nil
			}
			return errcode.New(errcode.ErrConflict, "该请求已用于另一笔处理，请重新打开操作")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if order.Status != model.InvStatusDone {
			return errcode.New(errcode.ErrConflict, "仅已审核的原单可处理收付款")
		}
		var original model.InvSettlement
		err = tx.Where("id = ? AND order_id = ? AND source = ? AND amount > 0",
			settlementID, id, "order").Take(&original).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errcode.New(errcode.ErrNotFound, "原收付款记录不存在")
		}
		if err != nil {
			return err
		}
		if req.Kind == "correction" && original.Kind == settlementOffsetIn {
			return errcode.New(errcode.ErrConflict, "抵扣转入可转抵其他单据或登记实际退款")
		}
		if req.Kind == "refund" && postedAt.Format(time.DateOnly) < original.SettledAt.Format(time.DateOnly) {
			return errcode.New(errcode.ErrBadRequest, "退款日期不能早于原收付款日期")
		}
		if req.Kind == "refund" {
			var count int64
			if err := tx.Model(&model.InvSettlement{}).Where("adjusts_id = ? AND kind = ? AND reference = ?",
				settlementID, req.Kind, reference).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errcode.New(errcode.ErrConflict, "该退款凭证已用于此流水，请核对收付款明细")
			}
		}
		var adjusted float64
		if err := tx.Model(&model.InvSettlement{}).Select("COALESCE(SUM(amount), 0)").
			Where("adjusts_id = ? AND amount < 0", settlementID).Scan(&adjusted).Error; err != nil {
			return err
		}
		available := min(round2(original.Amount+adjusted), order.SettledAmount)
		if amount > available {
			return errcode.Newf(errcode.ErrConflict, "可处理金额已变化，当前最多可处理 %.2f 元", available)
		}
		entry := model.InvSettlement{
			OrderID: id, Source: "order", BizType: order.BizType, PartyID: order.PartyID, Amount: -amount,
			Method: original.Method, SettledAt: postedAt, Remark: reason, Reference: reference,
			CreatedBy: actorUID(actor), AdjustsID: &settlementID, AdjustmentKey: &req.Key, Kind: req.Kind,
		}
		if req.Kind == "offset" {
			target := orders[req.TargetOrderID]
			if target.Status != model.InvStatusDone || target.BizType != order.BizType || target.PartyID != order.PartyID {
				return errcode.New(errcode.ErrConflict, "抵扣单须为同一往来单位、同业务类型的已审核单据")
			}
			if amount > round2(target.TotalAmount-target.SettledAmount) {
				return errcode.New(errcode.ErrConflict, "抵扣金额超过目标单据未结余额")
			}
			entry.CounterOrderID = &target.ID
			entry.Method = "往来抵扣"
			if err := tx.Create(&entry).Error; err != nil {
				return err
			}
			in := entry
			in.BaseModel = model.BaseModel{}
			in.OrderID, in.Amount, in.Kind = target.ID, amount, settlementOffsetIn
			in.CounterOrderID, in.AdjustmentKey, in.AdjustsID = &id, nil, nil
			if err := tx.Create(&in).Error; err != nil {
				return err
			}
			if err := tx.Model(&target).UpdateColumn("settled_amount", round2(target.SettledAmount+amount)).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&entry).Error; err != nil {
			return err
		}
		if err := tx.Model(&order).UpdateColumn("settled_amount", round2(order.SettledAmount-amount)).Error; err != nil {
			return err
		}
		result = entry.ID
		return nil
	})
	return result, saveErr(err)
}
