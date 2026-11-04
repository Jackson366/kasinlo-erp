package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestPlatformAuthRejectsInvalidDelegation(t *testing.T) {
	key := []byte("platform-test-key-with-32-bytes-or-more")
	webKey := []byte("web-session-test-key-with-32-bytes-or-more")
	now := time.Now().UTC().Truncate(time.Second)
	epoch := int64(1)
	valid := platformClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "open-platform",
			Subject:   "1",
			Audience:  jwt.ClaimStrings{"erp-a"},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(30 * time.Second)),
		},
		Scope:   "inventory.read",
		GrantID: "grant-1",
		Epoch:   &epoch,
	}

	router := gin.New()
	router.GET("/inventory", PlatformAuth(key, "open-platform", "erp-a", "inventory.read"), func(c *gin.Context) {
		t.Error("unauthorized request reached inventory")
		c.Status(http.StatusOK)
	})

	for _, tc := range []struct {
		name   string
		mutate func(*platformClaims)
		key    []byte
		status int
	}{
		{"web signing key", nil, webKey, http.StatusUnauthorized},
		{"wrong instance", func(c *platformClaims) { c.Audience = jwt.ClaimStrings{"erp-b"} }, key, http.StatusUnauthorized},
		{"missing epoch", func(c *platformClaims) { c.Epoch = nil }, key, http.StatusUnauthorized},
		{"excessive lifetime", func(c *platformClaims) { c.ExpiresAt = jwt.NewNumericDate(now.Add(time.Hour)) }, key, http.StatusUnauthorized},
		{"wrong scope", func(c *platformClaims) { c.Scope = "sales.read" }, key, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := valid
			if tc.mutate != nil {
				tc.mutate(&claims)
			}
			raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(tc.key)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/inventory", nil)
			req.Header.Set("Authorization", "Bearer "+raw)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.status, rec.Body.String())
			}
		})
	}
}
