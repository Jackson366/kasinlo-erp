package test

import (
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"nova-erp/internal/database"
	"nova-erp/internal/model"
)

func TestOrderRedReversal(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	suffix := time.Now().UnixNano()
	partyName := fmt.Sprintf("红冲验证%d", suffix)
	product := model.InvProduct{Code: fmt.Sprintf("RR%d", suffix), Name: "整单红冲验证商品", Status: 1}
	supplier := model.InvSupplier{Code: product.Code, Name: partyName, Status: 1}
	customer := model.InvCustomer{Code: product.Code, Name: partyName, Status: 1}
	for _, row := range []any{&product, &supplier, &customer} {
		if err := testDB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := testDB.Unscoped().Delete(row).Error; err != nil {
				t.Error(err)
			}
		})
	}
	spec := model.InvProductSpec{ProductID: product.ID, Spec: "A", Stock: 30, Status: 1}
	if err := testDB.Create(&spec).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testDB.Unscoped().Delete(&spec).Error; err != nil {
			t.Error(err)
		}
	})
	unit := model.InvProductUnit{SpecID: spec.ID, Unit: "件", Rate: 1, IsBase: true, PurchasePrice: 2, Status: 1}
	if err := testDB.Create(&unit).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := testDB.Unscoped().Delete(&unit).Error; err != nil {
			t.Error(err)
		}
	})
	get := func(path string) *response { return call(t, http.MethodGet, "/api/v1/inv"+path, token, nil) }
	write := func(method, path string, body any) *response {
		return call(t, method, "/api/v1/inv"+path, token, body)
	}
	now := time.Now().In(database.Location())
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	today := now.Format("2006-01-02")
	makeOrder := func(biz string, partyID int64, qty int) (int64, string) {
		t.Helper()
		created := write(http.MethodPost, "/orders", map[string]any{
			"bizType": biz, "partyId": partyID, "orderDate": yesterday,
			"items": []map[string]any{{"productId": product.ID, "specId": spec.ID, "unitId": unit.ID, "qty": qty, "price": 10}},
		})
		assertCode(t, created, 0, "新增单据")
		id := num(created.obj(t), "id")
		t.Cleanup(func() { cleanupRedReversalOrder(t, id) })
		path := fmt.Sprintf("/orders/%d", id)
		assertCode(t, write(http.MethodPut, path+"/status", map[string]any{"status": 2}), 0, "审核单据")
		return id, path
	}
	stock := func() float64 {
		t.Helper()
		var row model.InvProductSpec
		if err := testDB.First(&row, spec.ID).Error; err != nil {
			t.Fatal(err)
		}
		return row.Stock
	}

	for _, tc := range []struct {
		biz, direction, finance string
		partyID                 int64
		postedStock             float64
	}{
		{"purchase", "out", "payables", supplier.ID, 33},
		{"sales", "in", "receivables", customer.ID, 27},
	} {
		id, path := makeOrder(tc.biz, tc.partyID, 3)
		original := get(path).obj(t)
		finance := "/finance/" + tc.finance + "?source=order&keyword=" + url.QueryEscape(str(original, "orderNo"))
		if stock() != tc.postedStock || len(get(finance).List) != 1 {
			t.Fatalf("原单未过账: %s", tc.biz)
		}
		preview := get(path + "/red-reversal").obj(t)
		stockID := num(preview, "stockOrderId")
		if stockID == 0 || str(preview, "direction") != tc.direction || num(preview, "outstanding") != 30 {
			t.Fatalf("红冲预览错误: %v", preview)
		}
		assertCode(t, write(http.MethodPost, path+"/red-reversal", map[string]any{
			"stockOrderId": stockID + 1, "reason": "过账版本已变化",
		}), 40900, "过期预览不能过账")
		req := map[string]any{"stockOrderId": stockID, "reason": "重复录入"}
		var posted [2]*response
		var calls sync.WaitGroup
		for i := range posted {
			calls.Go(func() { posted[i] = write(http.MethodPost, path+"/red-reversal", req) })
		}
		calls.Wait()
		assertCode(t, posted[0], 0, "确认红冲")
		assertCode(t, posted[1], 0, "并发重试返回已生成红冲单")
		redID := num(posted[0].obj(t), "id")
		if redID == 0 || num(posted[1].obj(t), "id") != redID || stock() != 30 {
			t.Fatalf("红冲重复执行或库存错误: %v, %v", posted[0].Data, posted[1].Data)
		}
		original = get(path).obj(t)
		redPath := fmt.Sprintf("/orders/%d", redID)
		red := get(redPath).obj(t)
		if num(original, "status") != 4 || num(original, "totalAmount") != 30 ||
			num(red, "status") != 5 || num(red, "totalAmount") != -30 ||
			num(original["linkedOrder"].(map[string]any), "id") != redID ||
			num(red["linkedOrder"].(map[string]any), "id") != id ||
			str(red, "remark") != "重复录入" || len(get(finance).List) != 0 {
			t.Fatalf("原单、红冲单或财务状态错误: %v, %v", original, red)
		}
		var originalStock, redStock model.InvStockOrder
		if err := testDB.First(&originalStock, stockID).Error; err != nil || originalStock.Status != 1 {
			t.Fatalf("原库存单应保留过账: %+v, %v", originalStock, err)
		}
		if err := testDB.Where("reversal_of = ?", stockID).Take(&redStock).Error; err != nil ||
			redStock.SourceID != redID || redStock.Direction != tc.direction {
			t.Fatalf("红冲库存记录错误: %+v, %v", redStock, err)
		}
		assertCode(t, write(http.MethodDelete, fmt.Sprintf("/stock-orders/%d", redStock.ID), nil), 40900, "保护反向出入库")
		assertCode(t, write(http.MethodDelete, redPath, nil), 40900, "保护红冲单")
		if write(http.MethodPut, path+"/reverse", nil).Code == 0 ||
			write(http.MethodPost, "/finance/settlements", map[string]any{"orderId": id, "amount": 1}).Code == 0 {
			t.Fatal("已红冲原单仍可反审核或收付款")
		}
	}

	for _, entry := range []struct {
		date string
		want int64
	}{{yesterday, 30}, {today, -30}} {
		report := get("/report/detail?biz=purchase&from=" + entry.date + "&to=" + entry.date + "&party=" + url.QueryEscape(partyName))
		assertCode(t, report, 0, "按各自过账日期查询报表")
		if num(report.obj(t), "amount") != entry.want {
			t.Fatalf("报表没有在 %s 记录 %d: %v", entry.date, entry.want, report.Data)
		}
	}

	paidID, paidPath := makeOrder("purchase", supplier.ID, 3)
	assertCode(t, write(http.MethodPost, "/finance/settlements", map[string]any{
		"orderId": paidID, "amount": 1,
	}), 0, "登记部分付款")
	paidPreview := get(paidPath + "/red-reversal").obj(t)
	if str(paidPreview, "blockReason") == "" {
		t.Fatal("已付款单据未提示阻止红冲")
	}
	assertCode(t, write(http.MethodPost, paidPath+"/red-reversal", map[string]any{
		"stockOrderId": 1, "reason": "已付款",
	}), 40900, "后端阻止已付款红冲")

	_, shortagePath := makeOrder("purchase", supplier.ID, 40)
	ready := get(shortagePath + "/red-reversal").obj(t)
	makeOrder("sales", customer.ID, 45)
	blocked := get(shortagePath + "/red-reversal").obj(t)
	if str(blocked, "blockReason") == "" {
		t.Fatal("库存不足时没有阻止红冲")
	}
	before := stock()
	assertCode(t, write(http.MethodPost, shortagePath+"/red-reversal", map[string]any{
		"stockOrderId": num(ready, "stockOrderId"), "reason": "库存已变化",
	}), 40900, "确认时重新校验库存")
	failed := get(shortagePath).obj(t)
	if stock() != before || num(failed, "status") != 2 || failed["linkedOrder"] != nil {
		t.Fatalf("失败过账留下副作用: %v", failed)
	}
}

func cleanupRedReversalOrder(t *testing.T, id int64) {
	t.Helper()
	orders := testDB.Unscoped().Model(&model.InvOrder{}).Select("id").Where("id = ? OR reversal_of = ?", id, id)
	stocks := testDB.Unscoped().Model(&model.InvStockOrder{}).Select("id").Where("source_id IN (?)", orders)
	for _, entry := range []struct {
		model any
		query string
		ids   any
	}{
		{&model.InvStockItem{}, "stock_order_id IN (?)", stocks},
		{&model.InvStockOrder{}, "source_id IN (?)", orders},
		{&model.InvSettlement{}, "order_id IN (?)", orders},
		{&model.InvOrderAudit{}, "order_id IN (?)", orders},
		{&model.InvOrderItem{}, "order_id IN (?)", orders},
		{&model.InvOrder{}, "id = ? OR reversal_of = ?", nil},
	} {
		args := []any{entry.ids}
		if entry.ids == nil {
			args = []any{id, id}
		}
		if err := testDB.Unscoped().Where(entry.query, args...).Delete(entry.model).Error; err != nil {
			t.Error(err)
		}
	}
}
