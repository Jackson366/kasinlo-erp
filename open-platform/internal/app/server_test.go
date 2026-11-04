package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
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

	key := []byte("independent-platform-test-key-32-characters")
	erp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := jwt.Parse(bearer(r), func(*jwt.Token) (any, error) { return key, nil },
			jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("test-platform"), jwt.WithAudience("test-erp"))
		if err != nil || !token.Valid || token.Claims.(jwt.MapClaims)["sub"] != "27" ||
			token.Claims.(jwt.MapClaims)["scope"] != oauth.Scope || token.Claims.(jwt.MapClaims)["epoch"] != float64(4) {
			t.Error("ERP request did not carry the approved user and scope")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = io.WriteString(w, `{"code":0,"data":{"page":1,"pageSize":10,"total":1,"list":[{"productId":1,"stock":15,"purchasePrice":42}]}}`)
	}))
	defer erp.Close()
	const bridgeSecret = "independent-bridge-test-key-32-characters"
	handler := (Server{
		Store: oauth.Store{DB: db},
		ERP: erpclient.Client{HTTP: erp.Client(), URL: erp.URL, Issuer: "test-platform", Audience: "test-erp", Secret: key},
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
	start := form("/oauth/device_authorization", url.Values{"scope": {oauth.Scope}})
	var device struct {
		DeviceCode string `json:"device_code"`
		UserCode string `json:"user_code"`
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
			var token struct { AccessToken string `json:"access_token"` }
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
}
