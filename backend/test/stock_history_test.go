package test

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"nova-erp/internal/model"
)

// Reproduces the mismatch between a visible historical link and an active-only list.
func TestStockHistoryList(t *testing.T) {
	token := loginAs(t, adminAccount, adminSecret)
	profile := call(t, http.MethodGet, "/api/v1/profile", token, nil)
	assertCode(t, profile, 0, "读取测试操作人")
	order := model.InvStockOrder{
		OrderNo: fmt.Sprintf("HISTORY%d", time.Now().UnixNano()), Direction: model.InvStockIn,
		SourceType: model.InvSourceManual, OrderDate: time.Now(), Status: 2,
		CreatedBy: num(profile.obj(t), "id"),
	}
	if err := testDB.Create(&order).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&order) })
	base := "/api/v1/inv/stock-orders?direction=in&keyword=" + url.QueryEscape(order.OrderNo)
	normal := call(t, http.MethodGet, base, token, nil)
	assertCode(t, normal, 0, "普通列表不含作废单")
	if len(normal.List) != 0 {
		t.Fatalf("ordinary list contains voided record: %v", normal.List)
	}
	historical := call(t, http.MethodGet, base+"&includeVoided=true", token, nil)
	assertCode(t, historical, 0, "历史链接应显示作废单")
	if len(historical.List) != 1 {
		t.Fatalf("historical record missing: %v", historical.Raw)
	}
	row := historical.List[0].(map[string]any)
	if num(row, "id") != order.ID || num(row, "status") != 2 {
		t.Fatalf("unexpected record: %v", row)
	}
	invalid := call(t, http.MethodGet, base+"&includeVoided=wrong", token, nil)
	if invalid.Code == 0 {
		t.Fatal("invalid history parameter accepted")
	}
}
