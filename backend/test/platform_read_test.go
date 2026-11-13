package test

import (
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
	const key = "platform-test-key-with-at-least-32-bytes"
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
	for _, perm := range []string{"inv:purchase:list", "inv:product:list"} {
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

	read := func(scope, path string, want int) []byte {
		t.Helper()
		now := time.Now()
		claims := jwt.MapClaims{
			"iss": cfg.Issuer, "aud": cfg.Audience, "sub": strconv.FormatInt(actor.ID, 10),
			"iat": now.Unix(), "exp": now.Add(30 * time.Second).Unix(),
			"epoch": actor.TokenEpoch, "scope": scope, "grant_id": "test-grant",
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
	detail := string(read("product.read", fmt.Sprintf("/products/%d", product.ID), http.StatusOK))
	for _, field := range []string{"refPurchasePrice", "refSalePrice", "purchasePrice", "salePrice", "totalStock", "stock"} {
		if strings.Contains(detail, `"`+field+`"`) {
			t.Fatalf("product detail exposed %s: %s", field, detail)
		}
	}
}
