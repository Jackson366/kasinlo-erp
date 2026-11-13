package test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
)

func TestRedReversalFollowup(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	stamp := time.Now().UnixNano()
	code := fmt.Sprintf("P0%d", stamp)
	supplier := model.InvSupplier{Code: code, Name: code, Status: 1}
	product := model.InvProduct{Code: code, Name: code, Status: 1}
	save := func(row any) {
		t.Helper()
		if err := testDB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := testDB.Unscoped().Delete(row).Error; err != nil {
				t.Error(err)
			}
		})
	}
	save(&supplier)
	save(&product)
	spec := model.InvProductSpec{ProductID: product.ID, Spec: "A", Status: 1}
	save(&spec)
	unit := model.InvProductUnit{SpecID: spec.ID, Unit: "件", Rate: 1, IsBase: true, Status: 1}
	save(&unit)
	request := func(method, path string, body any) *response {
		return call(t, method, "/api/v1/inv"+path, token, body)
	}
	today := time.Now().In(database.Location()).Format(time.DateOnly)
	payload := map[string]any{
		"bizType": "purchase", "partyId": supplier.ID, "orderDate": today,
		"items": []map[string]any{{"productId": product.ID, "specId": spec.ID, "unitId": unit.ID, "qty": 5, "price": 10}},
	}
	makeOrder := func(approve bool) (int64, string) {
		t.Helper()
		res := request(http.MethodPost, "/orders", payload)
		assertCode(t, res, 0, "新增单据")
		id := num(res.obj(t), "id")
		t.Cleanup(func() { cleanupRedReversalOrder(t, id) })
		path := fmt.Sprintf("/orders/%d", id)
		if approve {
			assertCode(t, request(http.MethodPut, path+"/status", map[string]any{"status": 2}), 0, "审核单据")
		}
		return id, path
	}
	id, path := makeOrder(true)
	targetID, targetPath := makeOrder(true)
	finance := fmt.Sprintf("/finance/orders/%d", id)
	assertCode(t, request(http.MethodPost, "/finance/settlements", map[string]any{"orderId": id, "amount": 40}), 0, "登记付款")
	history := func(path string) map[string]any {
		t.Helper()
		res := request(http.MethodGet, path+"/settlements", nil)
		assertCode(t, res, 0, "读取收付款明细")
		return res.obj(t)
	}
	before := history(finance)
	viewer := loginAs(t, demoAccount, demoSecret)
	row := before["rows"].([]any)[0].(map[string]any)
	paymentID := num(row, "id")
	adjust := fmt.Sprintf("%s/settlements/%d/adjustments", finance, paymentID)
	assertCode(t, call(t, http.MethodPost, "/api/v1/inv"+adjust, viewer, map[string]any{
		"kind": "correction", "key": "a0000000-0000-4000-8000-000000000000", "reason": "未授权", "amount": 1,
	}), 40300, "只读账号不能更正款项")
	apply := func(kind string, n int, amount float64, extra map[string]any) *response {
		body := map[string]any{
			"kind": kind, "key": fmt.Sprintf("a0000000-0000-4000-8000-%012x", stamp%0x100000000000+int64(n)),
			"reason": "财务核对", "amount": amount,
		}
		for key, value := range extra {
			body[key] = value
		}
		return request(http.MethodPost, adjust, body)
	}
	assertCode(t, apply("refund", 1, 5, nil), 40000, "退款需要日期和凭证")
	first := apply("correction", 2, 10, nil)
	assertCode(t, first, 0, "更正误录金额")
	retry := apply("correction", 2, 10, nil)
	assertCode(t, retry, 0, "同一更正重试")
	if num(first.obj(t), "id") != num(retry.obj(t), "id") {
		t.Fatal("重试生成重复更正流水")
	}
	assertCode(t, apply("correction", 2, 11, nil), 40900, "幂等键不得用于不同金额")
	assertCode(t, apply("refund", 3, 5, map[string]any{"settledAt": today, "reference": "银行回单123"}), 0, "登记实际退款")
	assertCode(t, apply("refund", 6, 5, map[string]any{"settledAt": today, "reference": "银行回单123"}), 40900, "同一流水退款凭证不得重复登记")
	targets := request(http.MethodGet, finance+"/offset-targets", nil)
	assertCode(t, targets, 0, "读取可抵扣单据")
	if len(targets.List) != 1 || num(targets.List[0].(map[string]any), "id") != targetID {
		t.Fatalf("抵扣候选错误: %v", targets.Raw)
	}
	assertCode(t, apply("offset", 4, 26, map[string]any{"targetOrderId": targetID}), 40900, "不得超出原流水可处理金额")
	assertCode(t, apply("offset", 5, 25, map[string]any{"targetOrderId": targetID}), 0, "两张单据原子抵扣")
	after := history(finance)
	targetAfter := request(http.MethodGet, targetPath, nil).obj(t)
	if num(after, "settled") != 0 || num(after, "outstanding") != 50 ||
		len(after["rows"].([]any)) != 4 || num(targetAfter, "settledAmount") != 25 {
		t.Fatalf("处理后单据余额不一致: %v / %v", after, targetAfter)
	}

	preview := request(http.MethodGet, path+"/red-reversal", nil).obj(t)
	if str(preview, "blockReason") != "" {
		t.Fatalf("已结算金额清零后应可红冲: %v", preview)
	}
	red := request(http.MethodPost, path+"/red-reversal", map[string]any{
		"stockOrderId": num(preview, "stockOrderId"), "reason": "纠正错单",
	})
	assertCode(t, red, 0, "红冲原单")
	redID := num(red.obj(t), "id")
	payload["reopenedFrom"] = id
	draftID, draftPath := makeOrder(false)
	draft := request(http.MethodGet, draftPath, nil).obj(t)
	original := request(http.MethodGet, path, nil).obj(t)
	redDetail := request(http.MethodGet, fmt.Sprintf("/orders/%d", redID), nil).obj(t)
	if num(draft, "status") != 1 || num(draft, "settledAmount") != 0 ||
		original["hasReopenedOrder"] != true ||
		num(draft["originalOrder"].(map[string]any), "id") != id ||
		num(draft["originalReversal"].(map[string]any), "id") != redID ||
		num(original["reopenedOrder"].(map[string]any), "id") != draftID ||
		num(redDetail["reopenedOrder"].(map[string]any), "id") != draftID {
		t.Fatalf("重开关联错误: %v / %v / %v", draft, original, redDetail)
	}
	assertCode(t, request(http.MethodPost, "/orders", payload), 40900, "一张原单仅有一张有效重开单")
	assertCode(t, request(http.MethodPut, draftPath+"/status", map[string]any{"status": 2}), 0, "重开单正常审核")
}
