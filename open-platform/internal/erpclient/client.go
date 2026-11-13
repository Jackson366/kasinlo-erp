package erpclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"kasinlo-open-platform/internal/oauth"
)

type Client struct {
	HTTP     *http.Client
	URL      string
	Issuer   string
	Audience string
	Secret   []byte
}

type Resource struct {
	Path    string
	Scope   string
	detail  bool
	filters []string
}

var (
	Inventory      = Resource{Path: "inventory", Scope: oauth.Scope}
	Products       = Resource{Path: "products", Scope: oauth.ProductRead, filters: []string{"status", "categoryId"}}
	Product        = Resource{Path: "products", Scope: oauth.ProductRead, detail: true}
	PurchaseOrders = Resource{Path: "purchase-orders", Scope: oauth.PurchaseRead, filters: orderFilters}
	PurchaseOrder  = Resource{Path: "purchase-orders", Scope: oauth.PurchaseRead, detail: true}
	SalesOrders    = Resource{Path: "sales-orders", Scope: oauth.SalesRead, filters: orderFilters}
	SalesOrder     = Resource{Path: "sales-orders", Scope: oauth.SalesRead, detail: true}
)

var orderFilters = []string{"status", "settleStatus", "startDate", "endDate"}

func (c Client) Read(ctx context.Context, grant oauth.Grant, resource Resource, id string, query url.Values) (json.RawMessage, int, error) {
	path := "/internal/open-platform/" + resource.Path
	if resource.detail {
		number, err := strconv.ParseInt(id, 10, 64)
		if err != nil || number <= 0 {
			return nil, http.StatusBadRequest, nil
		}
		path += "/" + strconv.FormatInt(number, 10)
	} else {
		params := url.Values{}
		for _, name := range append([]string{"page", "pageSize", "keyword"}, resource.filters...) {
			if v := query.Get(name); v != "" {
				params.Set(name, v)
			}
		}
		path += "?" + params.Encode()
	}

	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss": c.Issuer, "aud": c.Audience, "sub": strconv.FormatInt(grant.UserID, 10),
		"iat": now.Unix(), "exp": now.Add(30 * time.Second).Unix(),
		"epoch": grant.Epoch, "scope": resource.Scope, "grant_id": grant.ID,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.Secret)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest, http.StatusNotFound:
		return nil, resp.StatusCode, nil
	case http.StatusOK:
	default:
		return nil, 0, errors.New("unexpected ERP response status")
	}
	var body struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return nil, 0, err
	}
	if body.Code == nil || *body.Code != 0 || len(body.Data) == 0 || body.Data[0] != '{' {
		return nil, 0, errors.New("invalid ERP read response")
	}
	if resource.Scope == oauth.Scope {
		return inventoryResponse(body.Data)
	}
	if !resource.detail {
		var page struct {
			List []json.RawMessage `json:"list"`
		}
		if err := json.Unmarshal(body.Data, &page); err != nil || page.List == nil {
			return nil, 0, errors.New("invalid ERP paginated response")
		}
	}
	return body.Data, http.StatusOK, nil
}

type inventoryRow struct {
	ProductID   int64   `json:"productId"`
	ProductCode string  `json:"productCode"`
	ProductName string  `json:"productName"`
	SpecID      int64   `json:"specId"`
	Spec        string  `json:"spec"`
	Unit        string  `json:"unit"`
	Stock       float64 `json:"stock"`
}

func inventoryResponse(data json.RawMessage) (json.RawMessage, int, error) {
	var page struct {
		List     []inventoryRow `json:"list"`
		Total    int64          `json:"total"`
		Page     int            `json:"page"`
		PageSize int            `json:"pageSize"`
	}
	if err := json.Unmarshal(data, &page); err != nil || page.List == nil {
		return nil, 0, errors.New("invalid ERP inventory response")
	}
	out, err := json.Marshal(page)
	return out, http.StatusOK, err
}
