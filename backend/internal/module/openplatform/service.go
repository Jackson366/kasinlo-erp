package openplatform

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"nova-erp/internal/pkg/errcode"
)

type bridge struct {
	url    string
	secret string
	client *http.Client
}

type request struct {
	UserID   int64  `json:"userId"`
	Epoch    int64  `json:"epoch"`
	UserCode string `json:"userCode,omitempty"`
	Approve  *bool  `json:"approve,omitempty"`
	Scope    string `json:"scope,omitempty"`
	GrantID  string `json:"grantId,omitempty"`
}

type deviceDetails struct {
	ClientID              string    `json:"clientId"`
	Scope                 string    `json:"scope"`
	ExpiresAt             time.Time `json:"expiresAt"`
	AccessLifetimeSeconds int       `json:"accessLifetimeSeconds"`
}

type grant struct {
	ID        string    `json:"id"`
	ClientID  string    `json:"clientId"`
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (b bridge) call(ctx context.Context, path string, data request, out any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.url+"/internal/erp/"+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+b.secret)
	req.Header.Set("Content-Type", "application/json")
	res, err := b.client.Do(req)
	if err != nil {
		return errcode.New(errcode.ErrInternal, "开放平台暂不可用")
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(out)
	case http.StatusBadRequest:
		return errcode.New(errcode.ErrBadRequest, "授权参数无效")
	case http.StatusNotFound:
		return errcode.New(errcode.ErrNotFound, "授权请求已过期或已处理")
	default:
		return errcode.New(errcode.ErrInternal, "开放平台暂不可用")
	}
}
