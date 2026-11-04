package client

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const deviceGrant = "urn:ietf:params:oauth:grant-type:device_code"

type Client struct {
	base string
	http *http.Client
}

type APIError struct {
	Status int
	Code string
}

func (e *APIError) Error() string { return e.Code }

type Device struct {
	DeviceCode string `json:"device_code"`
	UserCode string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn int `json:"expires_in"`
	Interval int `json:"interval"`
}

type Token struct {
	AccessToken string `json:"access_token"`
	TokenType string `json:"token_type"`
	ExpiresIn int `json:"expires_in"`
	Scope string `json:"scope"`
}

type Status struct {
	ClientID string `json:"clientId"`
	Scope string `json:"scope"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func validOrigin(u *url.URL) bool {
	if u.Hostname() == "" || u.User != nil {
		return false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return u.Scheme == "https" || u.Scheme == "http"
	default:
		return u.Scheme == "https"
	}
}

func New(raw string) (*Client, error) {
	u, err := url.Parse(raw)
	if err != nil || !validOrigin(u) || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("use --server or KAS_SERVER with an HTTPS platform URL")
	}
	return &Client{
		base: strings.TrimRight(u.String(), "/"),
		http: &http.Client{
			Timeout: 20 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *Client) Base() string { return c.base }

func isHex(s string, size int) bool {
	_, err := hex.DecodeString(s)
	return len(s) == size*2 && err == nil
}

func (c *Client) Start(ctx context.Context) (Device, error) {
	var out Device
	err := c.form(ctx, "/oauth/device_authorization", url.Values{"scope": {"inventory.read"}}, &out)
	if err != nil {
		return out, err
	}
	u, err := url.Parse(out.VerificationURI)
	if err != nil || !validOrigin(u) || !isHex(out.DeviceCode, 32) || !isHex(out.UserCode, 6) ||
		out.Interval < 1 || out.Interval > 60 || out.ExpiresIn < 1 || out.ExpiresIn > 3600 {
		return Device{}, errors.New("invalid device authorization response")
	}
	return out, nil
}

func (c *Client) Poll(ctx context.Context, deviceCode string) (Token, error) {
	var out Token
	err := c.form(ctx, "/oauth/token", url.Values{"device_code": {deviceCode}, "grant_type": {deviceGrant}}, &out)
	if err != nil {
		return out, err
	}
	if !isHex(out.AccessToken, 32) || !strings.EqualFold(out.TokenType, "Bearer") ||
		out.Scope != "inventory.read" || out.ExpiresIn < 1 || out.ExpiresIn > 86400 {
		return Token{}, errors.New("invalid access token response")
	}
	return out, nil
}

func (c *Client) Revoke(ctx context.Context, token string) error {
	return c.form(ctx, "/oauth/revoke", url.Values{"token": {token}}, nil)
}

func (c *Client) Status(ctx context.Context, token string) (Status, error) {
	var out Status
	err := c.request(ctx, http.MethodGet, "/oauth/status", token, nil, &out)
	if err == nil && (out.ClientID != "kas" || out.Scope != "inventory.read" || out.ExpiresAt.IsZero()) {
		err = errors.New("invalid authorization status response")
	}
	return out, err
}

func (c *Client) Inventory(ctx context.Context, token string, query url.Values) (json.RawMessage, error) {
	var out json.RawMessage
	err := c.request(ctx, http.MethodGet, "/openapi/v1/inventory?"+query.Encode(), token, nil, &out)
	return out, err
}

func (c *Client) form(ctx context.Context, path string, form url.Values, out any) error {
	form.Set("client_id", "kas")
	return c.request(ctx, http.MethodPost, path, "", strings.NewReader(form.Encode()), out)
}

func (c *Client) request(ctx context.Context, method, path, token string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body)
	if err != nil {
		return errors.New("could not construct platform request")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not reach the open platform")
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, 2<<20)
	if resp.StatusCode != http.StatusOK {
		var envelope struct { Error string `json:"error"` }
		_ = json.NewDecoder(limited).Decode(&envelope)
		switch envelope.Error {
		case "authorization_pending", "slow_down", "access_denied", "expired_token", "invalid_token",
			"insufficient_permission", "erp_identity_invalid", "invalid_client", "invalid_scope",
			"invalid_request", "unsupported_grant_type", "erp_unavailable", "server_error":
			return &APIError{Status: resp.StatusCode, Code: envelope.Error}
		default:
			return &APIError{Status: resp.StatusCode, Code: "platform_request_failed"}
		}
	}
	if out != nil {
		if err := json.NewDecoder(limited).Decode(out); err != nil {
			return errors.New("invalid JSON from the open platform")
		}
	}
	return nil
}
