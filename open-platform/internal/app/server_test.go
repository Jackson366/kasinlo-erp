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
		case "/internal/open-platform/inventory":
			scope = oauth.Scope
		case "/internal/open-platform/products/9":
			scope = oauth.ProductRead
		case "/internal/open-platform/purchase-orders":
			scope = oauth.PurchaseRead
			if r.URL.Query().Get("status") != "2" || r.URL.Query().Get("startDate") != "2026-11-01" || r.URL.Query().Has("bizType") {
				t.Errorf("unexpected ERP query: %v", r.URL.Query())
			}
		case "/internal/open-platform/purchase-orders/10":
			scope = oauth.PurchaseRead
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

	var session struct {
		SessionID string `json:"session_id"`
		UserCode  string `json:"user_code"`
	}
	const scope = "inventory.read product.read purchase.read"
	pending := run(0, "auth", "login", "--no-wait", "--scope", scope)
	if json.Unmarshal(pending, &session) != nil || session.SessionID == "" || session.UserCode == "" ||
		strings.Contains(string(pending), "device_code") {
		t.Fatal("expected a resumable login with a public verification code")
	}
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

	if out := run(0, "product", "get", "--id", "9"); !strings.Contains(string(out), `"name":"Product"`) {
		t.Fatal("separate CLI process could not read the product detail")
	}
	if out := run(0, "purchase", "list", "--status", "2", "--start-date", "2026-11-01"); !strings.Contains(string(out), `"orderNo":"CG10"`) {
		t.Fatal("separate CLI process could not query purchase orders")
	}
	run(1, "purchase", "get", "--id", "10")
	run(4, "sales", "list")

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
	run(3, "inventory", "list")
	req := httptest.NewRequest(http.MethodGet, "/oauth/status", nil)
	req.Header.Set("Authorization", "Bearer "+cached.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatal("logout did not revoke the server-side grant")
	}
}
