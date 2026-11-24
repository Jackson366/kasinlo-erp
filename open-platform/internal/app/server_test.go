package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"kasinlo-open-platform/internal/erpclient"
	"kasinlo-open-platform/internal/oauth"
	"kasinlo-open-platform/migrations"
)

func TestDeviceAuthorizationInventoryAndRevocation(t *testing.T) {
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PLATFORM_TEST_DATABASE_URL is required")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	admin := stdlib.OpenDB(*config)
	defer admin.Close()
	schema := "platform_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	if config.RuntimeParams == nil {
		config.RuntimeParams = make(map[string]string)
	}
	config.RuntimeParams["search_path"] = schema
	db := stdlib.OpenDB(*config)
	defer db.Close()
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	// Rows from an existing deployment only receive the original inventory scope.
	if _, err := db.Exec("ALTER TABLE access_grants DROP COLUMN scope; ALTER TABLE device_authorizations DROP COLUMN scope"); err != nil {
		t.Fatal(err)
	}
	const legacyToken = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := db.Exec(`INSERT INTO access_grants(id, token_hash, client_id, user_id, token_epoch, expires_at)
		VALUES ('legacy', sha256($1::bytea), 'kas', 99, 4, now() + interval '1 hour')`, legacyToken); err != nil {
		t.Fatal(err)
	}
	if err := migrations.Apply(context.Background(), db); err != nil {
		t.Fatal(err)
	}

	key := []byte("independent-platform-test-key-32-characters")
	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := jwt.Parse(bearer(r), func(*jwt.Token) (any, error) { return key, nil },
			jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("test-platform"), jwt.WithAudience("test-erp"))
		var scope string
		switch r.URL.Path {
		case "/internal/open-platform/contracts", "/internal/open-platform/contracts/15":
			scope = oauth.ContractRead
		case "/internal/open-platform/contract-report":
			scope = oauth.ContractReportRead
			q := r.URL.Query()
			if q.Get("bizType") != "sales" || q.Get("startDate") != "2026-11-01" || q.Get("keyword") != "HT15" || q.Has("page") || q.Has("pageSize") {
				t.Errorf("unexpected contract report query: %v", q)
			}
		case "/internal/open-platform/inventory-report":
			scope = oauth.InventoryReportRead
			if len(r.URL.Query()) != 0 {
				t.Errorf("unexpected inventory report query: %v", r.URL.Query())
			}
		case "/internal/open-platform/contract-ledger":
			scope = oauth.ContractLedgerRead
			q := r.URL.Query()
			if q.Get("bizType") != "sales" || q.Get("status") != "1" || q.Get("from") != "2026-11-01" ||
				q.Get("to") != "2026-11-30" || q.Get("limit") != "10" || q.Get("keyword") != "HT15" ||
				q.Has("page") || q.Has("pageSize") {
				t.Errorf("unexpected ledger filters: %v", q)
			}
		case "/internal/open-platform/contract-activities/15", "/internal/open-platform/contract-plan-settlements/16":
			scope = oauth.ContractHistoryRead
		case "/internal/open-platform/inventory":
			scope = oauth.Scope
		case "/internal/open-platform/products", "/internal/open-platform/products/9", "/internal/open-platform/product-categories", "/internal/open-platform/product-categories/3":
			scope = oauth.ProductRead
		case "/internal/open-platform/customers", "/internal/open-platform/customers/12":
			scope = oauth.CustomerRead
		case "/internal/open-platform/suppliers", "/internal/open-platform/suppliers/13":
			scope = oauth.SupplierRead
		case "/internal/open-platform/purchase-orders":
			scope = oauth.PurchaseRead
			if r.URL.Query().Get("status") != "2" || r.URL.Query().Get("startDate") != "2026-11-01" || r.URL.Query().Has("bizType") {
				t.Errorf("unexpected ERP query: %v", r.URL.Query())
			}
		case "/internal/open-platform/purchase-orders/10":
			scope = oauth.PurchaseRead
		case "/internal/open-platform/stock-orders", "/internal/open-platform/stock-orders/11":
			scope = oauth.StockRead
		case "/internal/open-platform/sales-orders", "/internal/open-platform/sales-orders/14":
			scope = oauth.SalesRead
		case "/internal/open-platform/payables", "/internal/open-platform/receivables", "/internal/open-platform/order-settlements/10":
			scope = oauth.FinanceRead
		default:
			t.Errorf("unexpected ERP request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err != nil || !token.Valid || token.Claims.(jwt.MapClaims)["sub"] != "27" ||
			token.Claims.(jwt.MapClaims)["scope"] != scope || token.Claims.(jwt.MapClaims)["epoch"] != float64(4) {
			t.Error("ERP request did not carry the approved user and scope")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/internal/open-platform/products/9" {
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":9,"name":"Product","specs":[]}}`)
			return
		}
		if r.URL.Path == "/internal/open-platform/purchase-orders/10" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/internal/open-platform/contracts":
			if r.URL.Query().Get("bizType") != "sales" || r.URL.Query().Get("stage") != "pending" || r.URL.Query().Get("partyId") != "12" || r.URL.Query().Get("status") != "1" || r.URL.Query().Get("startDate") != "2026-11-01" || r.URL.Query().Get("endDate") != "2026-11-30" {
				t.Errorf("contract filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"total":1,"list":[{"id":15,"contractNo":"HT15"}]}}`)
			return
		case "/internal/open-platform/contracts/15":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":15,"plans":[{"id":16,"outstanding":70}]}}`)
			return
		case "/internal/open-platform/contract-report":
			_, _ = io.WriteString(w, `{"code":0,"data":{"rows":[{"contractId":15,"outstanding":70}],"outstanding":70,"count":1,"truncated":false}}`)
			return
		case "/internal/open-platform/inventory-report":
			_, _ = io.WriteString(w, `{"code":0,"data":{"totalValue":125,"specCount":1,"warnCount":1,"categoryValue":[],"warnList":[{"stock":2,"warnStock":3}]}}`)
			return
		case "/internal/open-platform/contract-ledger":
			_, _ = io.WriteString(w, `{"code":0,"data":{"rows":[{"settleId":17,"amount":30}]}}`)
			return
		case "/internal/open-platform/contract-activities/15":
			_, _ = io.WriteString(w, `{"code":0,"data":{"contractId":15,"rows":[{"action":"audit"}]}}`)
			return
		case "/internal/open-platform/contract-plan-settlements/16":
			_, _ = io.WriteString(w, `{"code":0,"data":{"planId":16,"rows":[{"amount":30}]}}`)
			return
		case "/internal/open-platform/product-categories":
			if r.URL.Query().Get("parentId") != "0" || r.URL.Query().Get("status") != "1" || r.URL.Query().Get("keyword") != "Steel" {
				t.Errorf("category filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"id":3,"parentId":0,"name":"Steel","status":1}]}}`)
			return
		case "/internal/open-platform/product-categories/3":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":3,"parentId":0,"name":"Steel","status":1}}`)
			return
		case "/internal/open-platform/products":
			if r.URL.Query().Get("status") != "0" || r.URL.Query().Get("categoryId") != "3" {
				t.Errorf("product filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"id":9,"name":"Product"}]}}`)
			return
		case "/internal/open-platform/customers", "/internal/open-platform/suppliers":
			if r.URL.Query().Get("keyword") != "Trade" || r.URL.Query().Get("status") != "1" {
				t.Errorf("party filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"total":1,"list":[{"name":"Trade"}]}}`)
			return
		case "/internal/open-platform/customers/12", "/internal/open-platform/suppliers/13":
			_, _ = io.WriteString(w, `{"code":0,"data":{"name":"Trade"}}`)
			return
		case "/internal/open-platform/stock-orders":
			if r.URL.Query().Get("sourceType") != "purchase" || r.URL.Query().Get("sourceId") != "10" {
				t.Errorf("source filters not forwarded: %v", r.URL.Query())
			}
			if r.URL.Query().Get("direction") != "out" || r.URL.Query().Get("includeVoided") != "true" {
				t.Errorf("stock filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"id":11,"sourceId":10}]}}`)
			return
		case "/internal/open-platform/stock-orders/11":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":11,"items":[]}}`)
			return
		case "/internal/open-platform/payables", "/internal/open-platform/receivables":
			if r.URL.Query().Get("settle") != "overdue" || r.URL.Query().Get("source") != "order" {
				t.Errorf("finance filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[],"summary":{"outstanding":70}}}`)
			return
		case "/internal/open-platform/order-settlements/10":
			_, _ = io.WriteString(w, `{"code":0,"data":{"orderId":10,"outstanding":70,"rows":[]}}`)
			return
		case "/internal/open-platform/sales-orders":
			if r.URL.Query().Get("settleStatus") != "paid" || r.URL.Query().Get("endDate") != "2026-11-14" {
				t.Errorf("sales filters not forwarded: %v", r.URL.Query())
			}
			_, _ = io.WriteString(w, `{"code":0,"data":{"list":[{"id":14,"orderNo":"XS14"}]}}`)
			return
		case "/internal/open-platform/sales-orders/14":
			_, _ = io.WriteString(w, `{"code":0,"data":{"id":14,"orderNo":"XS14"}}`)
			return
		}
		if scope == oauth.PurchaseRead {
			_, _ = io.WriteString(w, `{"code":0,"data":{"page":1,"pageSize":10,"total":1,"list":[{"id":10,"orderNo":"CG10"}]}}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"page":1,"pageSize":10,"total":1,"list":[{"productId":1,"stock":15,"purchasePrice":42}]}}`)
	}))
	defer erp.Close()
	const bridgeSecret = "independent-bridge-test-key-32-characters"
	handler := (Server{
		Store:        oauth.Store{DB: db},
		ERP:          erpclient.Client{HTTP: erp.Client(), URL: erp.URL, Issuer: "test-platform", Audience: "test-erp", Secret: key},
		BridgeSecret: bridgeSecret, VerificationURI: "https://erp.test/#/open-platform/authorize",
	}).Handler()

	call := func(method, path, token, contentType, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		handler.ServeHTTP(res, req)
		return res
	}
	form := func(path string, fields url.Values) *httptest.ResponseRecorder {
		fields.Set("client_id", "kas")
		return call(http.MethodPost, path, "", "application/x-www-form-urlencoded", fields.Encode())
	}
	if res := call(http.MethodGet, "/oauth/status", legacyToken, "", ""); res.Code != http.StatusOK ||
		!strings.Contains(res.Body.String(), `"scope":"inventory.read"`) {
		t.Fatal("migration did not preserve the legacy grant's inventory scope")
	}
	if res := call(http.MethodGet, "/openapi/v1/inventory-report", legacyToken, "", ""); res.Code != http.StatusForbidden {
		t.Fatal("inventory-only grant could read the inventory report")
	}
	if res := call(http.MethodGet, "/openapi/v1/contract-ledger", legacyToken, "", ""); res.Code != http.StatusForbidden {
		t.Fatal("legacy grant could read contract ledger")
	}
	if res := call(http.MethodGet, "/openapi/v1/contract-report", legacyToken, "", ""); res.Code != http.StatusForbidden {
		t.Fatalf("legacy inventory grant accessed contract report: %d", res.Code)
	}
	if res := call(http.MethodGet, "/openapi/v1/products", legacyToken, "", ""); res.Code != http.StatusForbidden {
		t.Fatal("migration broadened a legacy grant")
	}
	if res := form("/oauth/device_authorization", url.Values{"scope": {"sales.write"}}); res.Code != http.StatusBadRequest {
		t.Fatal("unknown scope was accepted")
	}

	start := form("/oauth/device_authorization", url.Values{"scope": {oauth.Scope}})
	var device struct {
		DeviceCode string `json:"device_code"`
		UserCode   string `json:"user_code"`
	}
	if start.Code != http.StatusOK || json.Unmarshal(start.Body.Bytes(), &device) != nil ||
		device.DeviceCode == "" || device.UserCode == "" {
		t.Fatalf("device authorization start failed: %d", start.Code)
	}
	poll := func() *httptest.ResponseRecorder {
		return form("/oauth/token", url.Values{"device_code": {device.DeviceCode}, "grant_type": {deviceGrant}})
	}
	decision := `{"userId":27,"epoch":4,"approve":true,"userCode":"` + device.UserCode + `"}`
	const decisionPath = "/internal/erp/device/decision"
	if res := call(http.MethodPost, decisionPath, "", "application/json", decision); res.Code != http.StatusUnauthorized {
		t.Fatal("decision without ERP identity was accepted")
	}
	if res := poll(); res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "slow_down") {
		t.Fatal("early polling did not request backoff")
	}
	if res := call(http.MethodPost, decisionPath, bridgeSecret, "application/json", decision); res.Code != http.StatusOK {
		t.Fatalf("decision failed: %d", res.Code)
	}
	if _, err := db.Exec("UPDATE device_authorizations SET next_poll_at = now()"); err != nil {
		t.Fatal(err)
	}
	var results [2]*httptest.ResponseRecorder
	var wg sync.WaitGroup
	for i := range results {
		wg.Go(func() { results[i] = poll() })
	}
	wg.Wait()
	var accessToken string
	for _, res := range results {
		if res.Code == http.StatusOK {
			if accessToken != "" {
				t.Fatal("device code was redeemed twice")
			}
			var token struct {
				AccessToken string `json:"access_token"`
			}
			if err := json.Unmarshal(res.Body.Bytes(), &token); err != nil {
				t.Fatal(err)
			}
			accessToken = token.AccessToken
		} else if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), "expired_token") {
			t.Fatalf("unexpected duplicate redemption response: %d", res.Code)
		}
	}
	if accessToken == "" {
		t.Fatal("no access token issued")
	}
	const inventory = "/openapi/v1/inventory"
	res := call(http.MethodGet, inventory, accessToken, "", "")
	if res.Code != http.StatusOK || strings.Contains(res.Body.String(), "purchasePrice") ||
		!strings.Contains(res.Body.String(), `"stock":15`) {
		t.Fatal("inventory response did not project the permitted fields")
	}
	for _, path := range []string{"/openapi/v1/customers", "/openapi/v1/suppliers/13", "/openapi/v1/product-categories", "/openapi/v1/product-categories/3"} {
		res := call(http.MethodGet, path, accessToken, "", "")
		if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), "insufficient_scope") {
			t.Fatalf("inventory-only token reached %s: %d %s", path, res.Code, res.Body.String())
		}
	}
	var grants []oauth.Grant
	res = call(http.MethodPost, "/internal/erp/grants/list", bridgeSecret, "application/json", `{"userId":27,"epoch":4}`)
	if res.Code != http.StatusOK || json.Unmarshal(res.Body.Bytes(), &grants) != nil || len(grants) != 1 {
		t.Fatal("approved grant not visible to its owner")
	}
	revoke := func(userID string) int {
		return call(http.MethodPost, "/internal/erp/grants/revoke", bridgeSecret, "application/json",
			`{"userId":`+userID+`,"epoch":4,"grantId":"`+grants[0].ID+`"}`).Code
	}
	revoke("28")
	if res := call(http.MethodGet, inventory, accessToken, "", ""); res.Code != http.StatusOK {
		t.Fatal("another user revoked the grant")
	}
	if revoke("27") != http.StatusOK {
		t.Fatal("owner could not revoke the grant")
	}
	if res := call(http.MethodGet, inventory, accessToken, "", ""); res.Code != http.StatusUnauthorized {
		t.Fatal("revoked token was accepted")
	}

	if binary := os.Getenv("KAS_TEST_BINARY"); binary != "" {
		t.Run("cli shares and revokes credentials", func(t *testing.T) {
			testCLI(t, binary, handler, bridgeSecret)
		})
	}
}

func testCLI(t *testing.T, binary string, handler http.Handler, bridgeSecret string) {
	server := httptest.NewServer(handler)
	defer server.Close()
	configDir := t.TempDir()
	run := func(want int, args ...string) []byte {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, append([]string{"--server", server.URL, "--profile", "shared"}, args...)...)
		cmd.Env = append(os.Environ(), "KAS_CONFIG_DIR="+configDir)
		out, err := cmd.Output()
		code := 0
		if err != nil {
			var exit *exec.ExitError
			if !errors.As(err, &exit) {
				t.Fatalf("could not run kas: %v", err)
			}
			code = exit.ExitCode()
		}
		if code != want {
			t.Fatalf("kas %s %s exited %d, want %d: %v", args[0], args[1], code, want, err)
		}
		return out
	}

	type loginSession struct {
		SessionID string `json:"session_id"`
		UserCode  string `json:"user_code"`
	}
	start := func(scope string) loginSession {
		t.Helper()
		var session loginSession
		pending := run(0, "auth", "login", "--no-wait", "--scope", scope)
		if json.Unmarshal(pending, &session) != nil || session.SessionID == "" || session.UserCode == "" ||
			strings.Contains(string(pending), "device_code") {
			t.Fatal("expected a resumable login with a public verification code")
		}
		return session
	}
	const scope = "inventory.read inventory.report.read product.read purchase.read stock.read finance.read customer.read supplier.read contract.read contract.history.read contract.report.read contract.ledger.read"
	session := start(scope)
	bridge := func(path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+bridgeSecret)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	const decisionPath = "/internal/erp/device/decision"
	base := `{"userId":27,"epoch":4,"userCode":"` + session.UserCode + `"`
	inspect := bridge("/internal/erp/device/inspect", base+"}")
	if inspect.Code != http.StatusOK || !strings.Contains(inspect.Body.String(), scope) {
		t.Fatal("browser could not inspect the requested scopes")
	}
	if res := bridge(decisionPath, base+`,"approve":true,"scope":"inventory.read"}`); res.Code != http.StatusNotFound {
		t.Fatal("device was approved with scope different from the one displayed")
	}
	if res := bridge(decisionPath, base+`,"approve":true,"scope":"`+scope+`"}`); res.Code != http.StatusOK {
		t.Fatal("browser authorization decision was rejected")
	}
	run(0, "auth", "login", "--session", session.SessionID)
	if out := run(0, "auth", "status"); !strings.Contains(string(out), `"authenticated":true`) ||
		strings.Contains(string(out), "access_token") {
		t.Fatal("separate CLI process could not see the shared grant")
	}
	if out := run(0, "inventory", "list", "--page-size", "10"); !strings.Contains(string(out), `"stock":15`) {
		t.Fatal("separate CLI process could not query inventory")
	}

	if out := run(0, "inventory", "report"); !strings.Contains(string(out), `"totalValue":125`) || !strings.Contains(string(out), `"warnStock":3`) {
		t.Fatalf("could not read inventory report: %s", out)
	}
	run(2, "inventory", "report", "--page", "2")
	run(2, "inventory", "report", "--keyword", "Steel")

	if out := run(0, "contract", "list", "--biz-type", "sales", "--stage", "pending", "--party-id", "12", "--status", "1", "--start-date", "2026-11-01", "--end-date", "2026-11-30"); !strings.Contains(string(out), `"contractNo":"HT15"`) {
		t.Fatal("could not list contracts with filters")
	}
	if out := run(0, "contract", "get", "--id", "15"); !strings.Contains(string(out), `"outstanding":70`) {
		t.Fatal("could not read contract plans")
	}
	if out := run(0, "contract", "report", "--biz-type", "sales", "--start-date", "2026-11-01", "--keyword", "HT15"); !strings.Contains(string(out), `"outstanding":70`) {
		t.Fatalf("could not read contract report: %s", out)
	}
	run(2, "contract", "report", "--page", "2")
	if out := run(0, "contract", "ledger", "--biz-type", "sales", "--status", "1", "--from", "2026-11-01", "--to", "2026-11-30", "--limit", "10", "--keyword", "HT15"); !strings.Contains(string(out), `"settleId":17`) {
		t.Fatalf("could not read contract ledger: %s", out)
	}
	run(2, "contract", "ledger", "--page", "2")
	run(2, "contract", "get", "--id", "0")
	if out := run(0, "contract", "activity", "--id", "15"); !strings.Contains(string(out), `"action":"audit"`) {
		t.Fatalf("could not read contract activity: %s", out)
	}
	if out := run(0, "contract", "settlements", "--plan-id", "16"); !strings.Contains(string(out), `"amount":30`) {
		t.Fatalf("could not read contract plan settlement history: %s", out)
	}
	run(2, "contract", "settlements", "--plan-id", "0")
	if out := run(0, "category", "list", "--keyword", "Steel", "--parent-id", "0", "--status", "1"); !strings.Contains(string(out), `"id":3`) {
		t.Fatal("could not list product categories with root and status filters")
	}
	if out := run(0, "category", "get", "--id", "3"); !strings.Contains(string(out), `"name":"Steel"`) {
		t.Fatal("could not read product category detail")
	}
	run(2, "category", "get", "--id", "0")
	if out := run(0, "product", "get", "--id", "9"); !strings.Contains(string(out), `"name":"Product"`) {
		t.Fatal("separate CLI process could not read the product detail")
	}
	if out := run(0, "product", "list", "--status", "0", "--category-id", "3"); !strings.Contains(string(out), `"id":9`) {
		t.Fatal("separate CLI process could not list products with filters")
	}
	if out := run(0, "purchase", "list", "--status", "2", "--start-date", "2026-11-01"); !strings.Contains(string(out), `"orderNo":"CG10"`) {
		t.Fatal("separate CLI process could not query purchase orders")
	}
	for _, party := range []struct{ command, id string }{{"customer", "12"}, {"supplier", "13"}} {
		if out := run(0, party.command, "list", "--keyword", "Trade", "--status", "1"); !strings.Contains(string(out), `"list":[{"name":"Trade"}]`) {
			t.Fatalf("could not list %s records: %s", party.command, out)
		}
		if out := run(0, party.command, "get", "--id", party.id); !strings.Contains(string(out), `"name":"Trade"`) {
			t.Fatalf("could not read %s detail: %s", party.command, out)
		}
	}
	run(1, "purchase", "get", "--id", "10")
	run(4, "sales", "list")
	run(4, "sales", "get", "--id", "14")

	if out := run(0, "stock", "list", "--source-type", "purchase", "--source-id", "10", "--direction", "out", "--include-voided"); !strings.Contains(string(out), `"sourceId":10`) {
		t.Fatal("could not correlate a stock order")
	}
	run(4, "stock", "list", "--source-type", "sales", "--source-id", "10")
	run(0, "stock", "get", "--id", "11")
	for _, command := range []string{"payables", "receivables"} {
		run(0, "finance", command, "--settle-status", "overdue", "--source", "order")
	}
	if out := run(0, "finance", "settlements", "--order-id", "10"); !strings.Contains(string(out), `"outstanding":70`) {
		t.Fatal("could not query settlement history")
	}

	// Save the current token only for verifying the server revoked it.
	configs, err := os.ReadDir(configDir)
	if err != nil || len(configs) != 1 {
		t.Fatal("expected one shared credential profile")
	}
	raw, err := os.ReadFile(filepath.Join(configDir, configs[0].Name(), "credential.json"))
	var cached struct {
		Token string `json:"token"`
	}
	if err != nil || json.Unmarshal(raw, &cached) != nil || cached.Token == "" {
		t.Fatal("expected a saved local credential")
	}
	run(0, "auth", "logout")
	run(3, "contract", "list")
	run(3, "contract", "get", "--id", "15")
	run(3, "inventory", "list")
	run(3, "category", "list")
	run(3, "category", "get", "--id", "3")
	for _, path := range []string{"/oauth/status", "/openapi/v1/contract-report", "/openapi/v1/contract-ledger", "/openapi/v1/contract-activities/15", "/openapi/v1/contract-plan-settlements/16"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+cached.Token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("revoked grant could call %s: %d", path, rec.Code)
		}
	}

	salesSession := start("sales.read")
	if res := bridge(decisionPath, `{"userId":27,"epoch":4,"userCode":"`+salesSession.UserCode+`","approve":true,"scope":"sales.read"}`); res.Code != http.StatusOK {
		t.Fatal("sales-only authorization decision was rejected")
	}
	run(0, "auth", "login", "--session", salesSession.SessionID)
	if out := run(0, "sales", "list", "--settle-status", "paid", "--end-date", "2026-11-14"); !strings.Contains(string(out), `"orderNo":"XS14"`) {
		t.Fatal("separate CLI process could not list sales orders with filters")
	}
	if out := run(0, "sales", "get", "--id", "14"); !strings.Contains(string(out), `"id":14`) {
		t.Fatal("separate CLI process could not read the sales order detail")
	}
	run(4, "stock", "list", "--source-type", "sales", "--source-id", "14")
	run(4, "contract", "list")
	run(4, "contract", "get", "--id", "15")
	run(4, "contract", "activity", "--id", "15")
	run(4, "contract", "settlements", "--plan-id", "16")
	run(4, "category", "list")
	run(4, "category", "get", "--id", "3")
	run(0, "auth", "logout")
}
