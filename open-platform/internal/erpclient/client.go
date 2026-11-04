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
	HTTP *http.Client
	URL string
	Issuer string
	Audience string
	Secret []byte
}

type InventoryRow struct {
	ProductID int64 `json:"productId"`
	ProductCode string `json:"productCode"`
	ProductName string `json:"productName"`
	SpecID int64 `json:"specId"`
	Spec string `json:"spec"`
	Unit string `json:"unit"`
	Stock float64 `json:"stock"`
}

type InventoryPage struct {
	List []InventoryRow `json:"list"`
	Total int64 `json:"total"`
	Page int `json:"page"`
	PageSize int `json:"pageSize"`
}

func (c Client) Inventory(ctx context.Context, grant oauth.Grant, query url.Values) (InventoryPage, int, error) {
	var out InventoryPage
	now := time.Now().UTC()
	claims := jwt.MapClaims{
		"iss": c.Issuer, "aud": c.Audience, "sub": strconv.FormatInt(grant.UserID, 10),
		"iat": now.Unix(), "exp": now.Add(30*time.Second).Unix(),
		"epoch": grant.Epoch, "scope": oauth.Scope, "grant_id": grant.ID,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(c.Secret)
	if err != nil {
		return out, 0, err
	}
	params := url.Values{}
	for _, name := range []string{"page", "pageSize", "keyword"} {
		if v := query.Get(name); v != "" {
			params.Set(name, v)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL+"/internal/open-platform/inventory?"+params.Encode(), nil)
	if err != nil {
		return out, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return out, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return out, resp.StatusCode, nil
	}
	if resp.StatusCode != http.StatusOK {
		return out, 0, errors.New("unexpected ERP response status")
	}
	var body struct {
		Code *int `json:"code"`
		Data *InventoryPage `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&body); err != nil {
		return out, 0, err
	}
	if body.Code == nil || *body.Code != 0 || body.Data == nil || body.Data.List == nil {
		return out, 0, errors.New("invalid ERP inventory response")
	}
	return *body.Data, http.StatusOK, nil
}
