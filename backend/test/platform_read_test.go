package test

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"nova-erp/internal/config"
	"nova-erp/internal/model"
	"nova-erp/internal/module/inv"
)

func TestPlatformReadRespectsBusinessLineAndDataScope(t *testing.T) {
	setup(t)
	key := rand.Text() + rand.Text()
	cfg := config.OpenPlatformConfig{Secret: key, Issuer: "test-platform", Audience: "test-erp"}
	mod, enabled := inv.PlatformInventoryModule(cfg)
	if !enabled {
		t.Fatal("platform routes were not registered")
	}
	engine := gin.New()
	mod.Register(engine.Group("/api/v1"))

	marker := "OpenRead" + strconv.FormatInt(time.Now().UnixNano(), 36)
	role := model.Role{Name: marker, Code: marker, Status: 1, DataScope: model.DataScopeSelf}
	if err := testDB.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&role) })
	for _, perm := range []string{"inv:purchase:list", "inv:product:list", "inv:customer:list", "inv:supplier:list", "inv:stock:list", "inv:finance:list", "inv:contract:list", "inv:report:view"} {
		link := model.RoleMenu{RoleID: role.ID, MenuID: menuID(t, perm)}
		if err := testDB.Create(&link).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Delete(&link) })
	}
	newUser := func(suffix string) model.User {
		user := model.User{Username: marker + suffix, Password: "unused", Status: 1}
		if err := testDB.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Unscoped().Delete(&user) })
		return user
	}
	actor, other := newUser("Owner"), newUser("Other")
	link := model.UserRole{UserID: actor.ID, RoleID: role.ID}
	if err := testDB.Create(&link).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Delete(&link) })

	newOrder := func(suffix, bizType string, userID int64) model.InvOrder {
		order := model.InvOrder{OrderNo: marker + suffix, BizType: bizType, OrderDate: time.Now(), Status: 1, CreatedBy: userID}
		if err := testDB.Create(&order).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Unscoped().Delete(&order) })
		return order
	}
	purchase := newOrder("P", model.InvBizPurchase, actor.ID)
	hidden := newOrder("H", model.InvBizPurchase, other.ID)
	sale := newOrder("S", model.InvBizSales, actor.ID)

	product := model.InvProduct{Code: marker, Name: marker, Status: 1, RefPurchasePrice: 123}
	if err := testDB.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&product) })

	customer := model.InvCustomer{Code: marker + "%", Name: "Customer " + marker, Contact: marker + "FixtureContact", Status: 0}
	if err := testDB.Create(&customer).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&customer) })
	supplier := model.InvSupplier{Code: marker, Name: "Supplier " + marker, Status: 1}
	if err := testDB.Create(&supplier).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&supplier) })
	readAs := func(user model.User, scope, path string, want int) []byte {
		t.Helper()
		now := time.Now()
		claims := jwt.MapClaims{
			"iss": cfg.Issuer, "aud": cfg.Audience, "sub": strconv.FormatInt(user.ID, 10),
			"iat": now.Unix(), "exp": now.Add(30 * time.Second).Unix(),
			"epoch": user.TokenEpoch, "scope": scope, "grant_id": "test-grant",
		}
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(key))
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/open-platform"+path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		res := httptest.NewRecorder()
		engine.ServeHTTP(res, req)
		if res.Code != want {
			t.Fatalf("%s with %s: got %d, want %d: %s", path, scope, res.Code, want, res.Body.String())
		}
		return res.Body.Bytes()
	}
	read := func(scope, path string, want int) []byte {
		t.Helper()
		return readAs(actor, scope, path, want)
	}

	const inventoryReportScope = "inventory.report.read"
	const inventoryReportPath = "/inventory-report"
	read("inventory.read", inventoryReportPath, http.StatusForbidden)
	readAs(other, inventoryReportScope, inventoryReportPath, http.StatusForbidden)
	reportData := read(inventoryReportScope, inventoryReportPath, http.StatusOK)
	var inventoryReport struct {
		Data struct {
			TotalValue    *float64          `json:"totalValue"`
			CategoryValue []json.RawMessage `json:"categoryValue"`
			WarnList      []json.RawMessage `json:"warnList"`
		} `json:"data"`
	}
	if err := json.Unmarshal(reportData, &inventoryReport); err != nil ||
		inventoryReport.Data.TotalValue == nil || inventoryReport.Data.CategoryValue == nil || inventoryReport.Data.WarnList == nil {
		t.Fatalf("unexpected inventory report response: %s", reportData)
	}

	const rankingScope = "inventory.ranking.read"
	const rankingPath = "/stock-ranking"
	read(inventoryReportScope, rankingPath, http.StatusForbidden)
	readAs(other, rankingScope, rankingPath, http.StatusForbidden)
	read(rankingScope, rankingPath+"?limit=1001", http.StatusBadRequest)
	rankingData := read(rankingScope, rankingPath+"?limit=1", http.StatusOK)
	var ranking struct {
		Data struct {
			Rows []inv.ReportStockRankRow `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rankingData, &ranking); err != nil || len(ranking.Data.Rows) != 1 || ranking.Data.Rows[0].Rank != 1 {
		t.Fatalf("unexpected stock ranking response: %s", rankingData)
	}

	result := read("purchase.read", "/purchase-orders?keyword="+marker+"&bizType=sales", http.StatusOK)
	var page struct {
		Data struct {
			Total int64                `json:"total"`
			List  []struct{ ID int64 } `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(result, &page); err != nil || page.Data.Total != 1 ||
		len(page.Data.List) != 1 || page.Data.List[0].ID != purchase.ID {
		t.Fatalf("purchase list or total crossed its data scope: %s", result)
	}
	for _, order := range []model.InvOrder{hidden, sale} {
		read("purchase.read", fmt.Sprintf("/purchase-orders/%d", order.ID), http.StatusNotFound)
	}
	read("purchase.read", fmt.Sprintf("/purchase-orders/%d", purchase.ID), http.StatusOK)
	read("purchase.read", "/sales-orders", http.StatusForbidden)
	read("sales.read", "/sales-orders", http.StatusForbidden)
	read("product.read", "/inventory", http.StatusForbidden)
	for _, order := range []model.InvOrder{purchase, hidden} {
		if err := testDB.Model(&order).Updates(map[string]any{"status": model.InvStatusDone, "total_amount": 100, "settled_amount": 30}).Error; err != nil {
			t.Fatal(err)
		}
		// The stock order can be visible while its source order is outside scope.
		stock := model.InvStockOrder{OrderNo: order.OrderNo + "IN", Direction: model.InvStockIn, SourceType: model.InvBizPurchase,
			SourceID: order.ID, CreatedBy: actor.ID, OrderDate: time.Now(), Status: 1}
		if err := testDB.Create(&stock).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Unscoped().Delete(&stock) })
		if order.ID == purchase.ID {
			read("stock.read", fmt.Sprintf("/stock-orders/%d", stock.ID), http.StatusOK)
		}
	}
	const summaryScope = "business.report.read"
	const summaryPath = "/business-summary"
	read(inventoryReportScope, summaryPath, http.StatusForbidden)
	readAs(other, summaryScope, summaryPath, http.StatusForbidden)
	read(summaryScope, summaryPath+"?from=invalid", http.StatusBadRequest)
	summaryData := read(summaryScope, summaryPath+"?range=year", http.StatusOK)
	var summary struct {
		Data inv.ReportSummary `json:"data"`
	}
	if err := json.Unmarshal(summaryData, &summary); err != nil || summary.Data.Purchases != 100 || summary.Data.PurchaseOrders != 1 {
		t.Fatalf("business summary included orders outside the user's data scope: %s", summaryData)
	}

	const trendScope = "business.trend.read"
	const trendPath = "/business-trend"
	read(summaryScope, trendPath, http.StatusForbidden)
	readAs(other, trendScope, trendPath, http.StatusForbidden)
	for _, query := range []string{
		"?days=0", "?days=91", "?days=bad", "?from=2026-11-01", "?to=2026-11-01",
		"?from=bad&to=2026-11-01", "?from=2026-11-02&to=2026-11-01",
		"?from=2026-01-01&to=2026-04-01", "?from=2026-11-01&to=2026-11-02&days=2",
		"?categoryId=-1",
	} {
		read(trendScope, trendPath+query, http.StatusBadRequest)
	}

	trendDate := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	if err := testDB.Model(&model.InvOrder{}).Where("id IN ?", []int64{purchase.ID, hidden.ID}).
		Update("order_date", trendDate).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Model(&sale).Updates(map[string]any{"order_date": trendDate, "status": model.InvStatusDone, "total_amount": 75}).Error; err != nil {
		t.Fatal(err)
	}
	trendData := read(trendScope, trendPath+"?from=2026-11-12&to=2026-11-13", http.StatusOK)
	var trend struct {
		Data struct {
			Rows []inv.ReportTrendPoint `json:"rows"`
		} `json:"data"`
	}
	if err := json.Unmarshal(trendData, &trend); err != nil || len(trend.Data.Rows) != 2 ||
		trend.Data.Rows[0] != (inv.ReportTrendPoint{Date: "2026-11-12", SalesAmount: 75, PurchaseAmount: 100}) ||
		trend.Data.Rows[1] != (inv.ReportTrendPoint{Date: "2026-11-13"}) {
		t.Fatalf("unexpected trend totals or date range: %s", trendData)
	}
	for _, query := range []struct {
		value string
		rows  int
	}{
		{"", 30},
		{"?days=1", 1},
		{"?from=2026-01-01&to=2026-03-31", 90},
	} {
		data := read(trendScope, trendPath+query.value, http.StatusOK)
		if err := json.Unmarshal(data, &trend); err != nil || len(trend.Data.Rows) != query.rows {
			t.Fatalf("unexpected trend period %q: %s", query.value, data)
		}
	}

	stockPath := "/stock-orders?sourceType=purchase&sourceId="
	for _, tc := range []struct {
		id    int64
		total string
	}{{purchase.ID, "1"}, {hidden.ID, "0"}} {
		path := stockPath + strconv.FormatInt(tc.id, 10)
		if data := read("stock.read", path, http.StatusOK); !strings.Contains(string(data), `"total":`+tc.total) {
			t.Fatalf("stock source scope mismatch: %s", data)
		}
	}
	read("stock.read", "/stock-orders?sourceType=sales&sourceId="+strconv.FormatInt(sale.ID, 10), http.StatusForbidden)
	read("inventory.read", "/stock-orders", http.StatusForbidden)
	read("finance.read", "/receivables?source=invalid", http.StatusBadRequest)
	if data := read("finance.read", "/payables?source=order&keyword="+marker, http.StatusOK); !strings.Contains(string(data), `"total":1`) ||
		!strings.Contains(string(data), `"outstanding":70`) || strings.Contains(string(data), hidden.OrderNo) {
		t.Fatalf("payables list or total crossed its scope: %s", data)
	}
	settlement := model.InvSettlement{OrderID: purchase.ID, Source: "order", Amount: 30, SettledAt: time.Now(), Reference: "fixture-reference"}
	if err := testDB.Create(&settlement).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&settlement) })
	if data := read("finance.read", fmt.Sprintf("/order-settlements/%d", purchase.ID), http.StatusOK); !strings.Contains(string(data), `"amount":30`) ||
		!strings.Contains(string(data), `"outstanding":70`) || strings.Contains(string(data), settlement.Reference) {
		t.Fatalf("unexpected settlement response: %s", data)
	}
	read("finance.read", fmt.Sprintf("/order-settlements/%d", hidden.ID), http.StatusNotFound)

	customerPath := fmt.Sprintf("/customers/%d", customer.ID)
	for _, path := range []string{"/customers?keyword=" + marker + "%25&status=0", customerPath} {
		data := string(read("customer.read", path, http.StatusOK))
		if !strings.Contains(data, customer.Name) || strings.Contains(data, "FixtureContact") ||
			strings.Contains(data, `"balance"`) || strings.Contains(data, `"orderCount"`) {
			t.Fatalf("unexpected party response: %s", data)
		}
	}
	if data := string(read("customer.read", "/customers?keyword="+marker+"&status=1", http.StatusOK)); !strings.Contains(data, `"list":[]`) || !strings.Contains(data, `"total":0`) {
		t.Fatalf("disabled customer matched enabled filter: %s", data)
	}
	read("customer.read", "/customers?status=2", http.StatusBadRequest)
	read("supplier.read", customerPath, http.StatusForbidden)
	read("customer.read", "/suppliers", http.StatusForbidden)
	readAs(other, "customer.read", customerPath, http.StatusForbidden)
	readAs(other, "supplier.read", "/suppliers", http.StatusForbidden)
	supplierPath := fmt.Sprintf("/suppliers/%d", supplier.ID)
	if data := string(read("supplier.read", supplierPath, http.StatusOK)); !strings.Contains(data, supplier.Name) {
		t.Fatalf("supplier detail loaded the wrong record: %s", data)
	}
	if err := testDB.Delete(&supplier).Error; err != nil {
		t.Fatal(err)
	}
	read("supplier.read", supplierPath, http.StatusNotFound)
	if data := string(read("supplier.read", "/suppliers?keyword="+marker, http.StatusOK)); !strings.Contains(data, `"total":0`) {
		t.Fatalf("deleted supplier appeared in list: %s", data)
	}

	newCategory := func(name string, parentID int64, status int16, sort int) model.InvCategory {
		row := model.InvCategory{Name: name, ParentID: parentID, Status: status, Sort: sort, Remark: "fixture-category-note"}
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Unscoped().Delete(&row) })
		return row
	}
	root := newCategory(marker+"%_Root", 0, 1, 0)
	child := newCategory(marker+"%_Child", root.ID, 0, 0)
	newCategory(marker+"%_Next", root.ID, 1, 1)
	newCategory(marker+"XYDecoy", 0, 1, 0)
	categoryPath := fmt.Sprintf("/product-categories/%d", child.ID)
	read("inventory.read", "/product-categories", http.StatusForbidden)
	read("inventory.read", categoryPath, http.StatusForbidden)
	readAs(other, "product.read", "/product-categories", http.StatusForbidden)
	readAs(other, "product.read", categoryPath, http.StatusForbidden)
	for _, query := range []string{"status=2", "status=abc", "parentId=-1", "parentId=abc", "parentId=9223372036854775808"} {
		read("product.read", "/product-categories?"+query, http.StatusBadRequest)
	}
	read("product.read", "/product-categories/abc", http.StatusBadRequest)
	read("product.read", "/product-categories/0", http.StatusBadRequest)
	checkCategories := func(query string, wantTotal int64, wantID int64) {
		t.Helper()
		result := read("product.read", "/product-categories?"+query, http.StatusOK)
		if err := json.Unmarshal(result, &page); err != nil || page.Data.Total != wantTotal || len(page.Data.List) != 1 || page.Data.List[0].ID != wantID {
			t.Fatalf("unexpected category page: %s", result)
		}
		if strings.Contains(string(result), "fixture-category-note") || strings.Contains(string(result), `"productCount"`) {
			t.Fatalf("category response exposed internal fields: %s", result)
		}
	}
	// Literal wildcard search must exclude the decoy; omitted parentId includes every level.
	checkCategories("keyword="+marker+"%25_&pageSize=1&page=2", 3, child.ID)
	checkCategories("keyword="+marker+"%25_&parentId=0", 1, root.ID)
	checkCategories(fmt.Sprintf("parentId=%d&status=0", root.ID), 1, child.ID)
	categoryDetail := string(read("product.read", categoryPath, http.StatusOK))
	if !strings.Contains(categoryDetail, fmt.Sprintf(`"parentId":%d`, root.ID)) || !strings.Contains(categoryDetail, child.Name) || strings.Contains(categoryDetail, "fixture-category-note") {
		t.Fatalf("unexpected category detail: %s", categoryDetail)
	}
	if err := testDB.Model(&product).Update("category_id", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	item := model.InvOrderItem{OrderID: purchase.ID, ProductID: product.ID, Amount: 20}
	if err := testDB.Create(&item).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&item) })
	categoryTrend := read(trendScope, trendPath+fmt.Sprintf("?from=2026-11-12&to=2026-11-12&categoryId=%d", root.ID), http.StatusOK)
	if err := json.Unmarshal(categoryTrend, &trend); err != nil || len(trend.Data.Rows) != 1 ||
		trend.Data.Rows[0] != (inv.ReportTrendPoint{Date: "2026-11-12", PurchaseAmount: 100}) {
		t.Fatalf("trend did not select descendant categories using order totals: %s", categoryTrend)
	}
	if err := testDB.Delete(&child).Error; err != nil {
		t.Fatal(err)
	}
	read("product.read", categoryPath, http.StatusNotFound)
	if data := string(read("product.read", fmt.Sprintf("/product-categories?parentId=%d&status=0", root.ID), http.StatusOK)); !strings.Contains(data, `"list":[]`) || !strings.Contains(data, `"total":0`) {
		t.Fatalf("deleted category appeared in list: %s", data)
	}

	newContract := func(suffix string, owner int64) model.InvContract {
		row := model.InvContract{ContractNo: marker + suffix, Name: marker + suffix, BizType: model.InvBizSales,
			PartyID: customer.ID, PartyName: customer.Name, SignDate: time.Now(), Amount: 100,
			Status: model.InvContractActive, CreatedBy: owner, Remark: "fixture-contract-note", Salesman: "fixture-salesman"}
		if err := testDB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { testDB.Unscoped().Delete(&row) })
		return row
	}
	ownContract := newContract("C", actor.ID)
	hiddenContract := newContract("H", other.ID)
	plan := model.InvContractPlan{ContractID: ownContract.ID, Seq: 1, Name: "Deposit", Amount: 100, SettledAmount: 30, Remark: "fixture-plan-note"}
	if err := testDB.Create(&plan).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&plan) })
	contractPath := fmt.Sprintf("/contracts/%d", ownContract.ID)
	read("inventory.read", "/contracts", http.StatusForbidden)
	read("finance.read", contractPath, http.StatusForbidden)
	readAs(other, "contract.read", contractPath, http.StatusForbidden)
	read("contract.read", fmt.Sprintf("/contracts/%d", hiddenContract.ID), http.StatusNotFound)
	for _, query := range []string{"bizType=invalid", "stage=invalid", "status=0", "status=4", "partyId=-1", "partyId=abc", "startDate=invalid", "startDate=2026-11-20&endDate=2026-11-01"} {
		read("contract.read", "/contracts?"+query, http.StatusBadRequest)
	}
	read("contract.read", "/contracts/0", http.StatusBadRequest)
	read("contract.read", "/contracts/abc", http.StatusBadRequest)
	result = read("contract.read", "/contracts?keyword="+marker+"&bizType=sales&status=1&stage=pending&partyId="+strconv.FormatInt(customer.ID, 10), http.StatusOK)
	if err := json.Unmarshal(result, &page); err != nil || page.Data.Total != 1 || len(page.Data.List) != 1 || page.Data.List[0].ID != ownContract.ID {
		t.Fatalf("contract list or count crossed scope: %s", result)
	}
	if strings.Contains(string(result), `"plans"`) {
		t.Fatalf("list returned plan details: %s", result)
	}
	for _, path := range []string{"/contracts?keyword=" + marker, contractPath} {
		data := string(read("contract.read", path, http.StatusOK))
		for _, field := range []string{"remark", "operator", "auditor", "salesman", "attachmentCount", "fixture-"} {
			if strings.Contains(data, field) {
				t.Fatalf("contract exposed %s: %s", field, data)
			}
		}
	}
	if data := string(read("contract.read", contractPath, http.StatusOK)); !strings.Contains(data, `"outstanding":70`) || !strings.Contains(data, `"name":"Deposit"`) {
		t.Fatalf("contract plan amounts missing: %s", data)
	}
	const reportScope = "contract.report.read"
	const reportPath = "/contract-report"
	read("contract.read", reportPath, http.StatusForbidden)
	readAs(other, reportScope, reportPath, http.StatusForbidden)
	read(reportScope, reportPath+"?startDate=2026-12-01&endDate=2026-11-01", http.StatusBadRequest)
	reportQuery := reportPath + "?keyword=" + marker + "&bizType=sales&status=1&stage=pending"
	if data := string(read(reportScope, reportQuery, http.StatusOK)); !strings.Contains(data, `"count":1`) ||
		!strings.Contains(data, `"amount":100`) || !strings.Contains(data, `"outstanding":70`) ||
		strings.Contains(data, hiddenContract.ContractNo) || strings.Contains(data, "fixture-") {
		t.Fatalf("unexpected contract report or report crossed data scope: %s", data)
	}
	if data := string(read(reportScope, reportPath+"?keyword=no-"+marker, http.StatusOK)); !strings.Contains(data, `"rows":[]`) ||
		!strings.Contains(data, `"count":0`) {
		t.Fatalf("unexpected empty contract report: %s", data)
	}

	hiddenPlan := model.InvContractPlan{ContractID: hiddenContract.ID, Seq: 1, Name: "Hidden deposit"}
	if err := testDB.Create(&hiddenPlan).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&hiddenPlan) })
	planSettlement := model.InvSettlement{
		PlanID: plan.ID, Source: model.InvSettleSourcePlan, Amount: 30, Method: "transfer",
		SettledAt: time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC), Remark: "fixture-settlement-note",
	}
	if err := testDB.Create(&planSettlement).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&planSettlement) })
	hiddenSettlement := model.InvSettlement{
		PlanID: hiddenPlan.ID, Source: model.InvSettleSourcePlan, Amount: 999,
		SettledAt: planSettlement.SettledAt,
	}
	if err := testDB.Create(&hiddenSettlement).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Unscoped().Delete(&hiddenSettlement) })

	const ledgerScope = "contract.ledger.read"
	const ledgerPath = "/contract-ledger"
	read(reportScope, ledgerPath, http.StatusForbidden)
	readAs(other, ledgerScope, ledgerPath, http.StatusForbidden)
	for _, query := range []string{"?limit=0", "?limit=1001", "?limit=abc", "?from=2026-11-16&to=2026-11-15"} {
		read(ledgerScope, ledgerPath+query, http.StatusBadRequest)
	}
	ledgerQuery := ledgerPath + "?keyword=" + marker + "&bizType=sales&status=1&from=2026-11-15&to=2026-11-15"
	var ledger struct {
		Data struct {
			Rows []struct {
				SettleID int64
				Amount   float64
			}
		}
	}
	data := read(ledgerScope, ledgerQuery, http.StatusOK)
	if err := json.Unmarshal(data, &ledger); err != nil || len(ledger.Data.Rows) != 1 ||
		ledger.Data.Rows[0].SettleID != planSettlement.ID || ledger.Data.Rows[0].Amount != 30 ||
		strings.Contains(string(data), `"remark"`) {
		t.Fatalf("unexpected contract ledger or ledger crossed data scope: %s", data)
	}
	if data := string(read(ledgerScope, ledgerPath+"?keyword="+marker+"&from=2026-11-16", http.StatusOK)); !strings.Contains(data, `"rows":[]`) {
		t.Fatalf("unexpected ledger outside settlement date: %s", data)
	}

	log := model.OperationLog{
		Path: "/api/v1/inv" + contractPath + "/audit", Action: "audit contract",
		Method: http.MethodPost, Status: http.StatusOK, Username: "fixture-username", IP: "192.0.2.1",
	}
	if err := testDB.Create(&log).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testDB.Delete(&log) })

	const historyScope = "contract.history.read"
	activityPath := fmt.Sprintf("/contract-activities/%d", ownContract.ID)
	settlementsPath := fmt.Sprintf("/contract-plan-settlements/%d", plan.ID)
	for _, path := range []string{activityPath, settlementsPath} {
		read("contract.read", path, http.StatusForbidden)
		readAs(other, historyScope, path, http.StatusForbidden)
	}
	read(historyScope, "/contract-activities/0", http.StatusBadRequest)
	read(historyScope, "/contract-plan-settlements/abc", http.StatusBadRequest)
	read(historyScope, fmt.Sprintf("/contract-activities/%d", hiddenContract.ID), http.StatusNotFound)
	read(historyScope, fmt.Sprintf("/contract-plan-settlements/%d", hiddenPlan.ID), http.StatusNotFound)
	if data := string(read(historyScope, activityPath, http.StatusOK)); !strings.Contains(data, `"action":"audit contract"`) ||
		strings.Contains(data, `"username"`) || strings.Contains(data, `"ip"`) {
		t.Fatalf("unexpected contract activity: %s", data)
	}
	if data := string(read(historyScope, settlementsPath, http.StatusOK)); !strings.Contains(data, `"amount":30`) ||
		strings.Contains(data, `"remark"`) {
		t.Fatalf("unexpected contract plan settlements: %s", data)
	}

	if err := testDB.Delete(&ownContract).Error; err != nil {
		t.Fatal(err)
	}
	read("contract.read", contractPath, http.StatusNotFound)
	read(historyScope, activityPath, http.StatusNotFound)
	read(historyScope, settlementsPath, http.StatusNotFound)
	if data := string(read("contract.read", "/contracts?keyword="+marker, http.StatusOK)); !strings.Contains(data, `"total":0`) || !strings.Contains(data, `"list":[]`) {
		t.Fatalf("deleted or hidden contract appeared in list: %s", data)
	}

	detail := string(read("product.read", fmt.Sprintf("/products/%d", product.ID), http.StatusOK))
	for _, field := range []string{"refPurchasePrice", "refSalePrice", "purchasePrice", "salePrice", "totalStock", "stock"} {
		if strings.Contains(detail, `"`+field+`"`) {
			t.Fatalf("product detail exposed %s: %s", field, detail)
		}
	}
}
