package app

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"kasinlo-open-platform/internal/erpclient"
	"kasinlo-open-platform/internal/oauth"
)

const deviceGrant = "urn:ietf:params:oauth:grant-type:device_code"

type Server struct {
	Store oauth.Store
	ERP erpclient.Client
	VerificationURI string
	BridgeSecret string
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("POST /oauth/device_authorization", s.start)
	mux.HandleFunc("POST /oauth/token", s.token)
	mux.HandleFunc("POST /oauth/revoke", s.revoke)
	mux.HandleFunc("GET /openapi/v1/inventory", s.inventory)
	mux.HandleFunc("POST /internal/erp/device/inspect", s.bridge(s.inspect))
	mux.HandleFunc("POST /internal/erp/device/decision", s.bridge(s.decide))
	mux.HandleFunc("POST /internal/erp/grants/list", s.bridge(s.list))
	mux.HandleFunc("POST /internal/erp/grants/revoke", s.bridge(s.revokeGrant))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func reject(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}

func failure(w http.ResponseWriter, operation string, err error) {
	// Avoid logging database or HTTP errors that may include connection strings.
	slog.Error("platform request failed", "operation", operation, "error_type", fmt.Sprintf("%T", err))
	reject(w, http.StatusInternalServerError, "server_error")
}

func validHex(s string, length int) bool {
	if len(s) != length {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		reject(w, http.StatusBadRequest, "invalid_request")
		return false
	}
	return true
}

func (s Server) start(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if r.PostForm.Get("client_id") != oauth.ClientID {
		reject(w, http.StatusBadRequest, "invalid_client")
		return
	}
	if scope := r.PostForm.Get("scope"); scope != "" && scope != oauth.Scope {
		reject(w, http.StatusBadRequest, "invalid_scope")
		return
	}
	device, code, err := s.Store.Start(r.Context())
	if err != nil {
		failure(w, "start_device_authorization", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"device_code": device, "user_code": code, "verification_uri": s.VerificationURI,
		"expires_in": int(oauth.DeviceLifetime.Seconds()), "interval": oauth.PollInterval,
	})
}

func (s Server) token(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if r.PostForm.Get("client_id") != oauth.ClientID {
		reject(w, http.StatusBadRequest, "invalid_client")
		return
	}
	if r.PostForm.Get("grant_type") != deviceGrant {
		reject(w, http.StatusBadRequest, "unsupported_grant_type")
		return
	}
	device := r.PostForm.Get("device_code")
	if !validHex(device, 64) {
		reject(w, http.StatusBadRequest, "invalid_request")
		return
	}
	token, oauthError, err := s.Store.Poll(r.Context(), device)
	if err != nil {
		failure(w, "poll_device_authorization", err)
		return
	}
	if oauthError != "" {
		reject(w, http.StatusBadRequest, oauthError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": token, "token_type": "Bearer", "scope": oauth.Scope,
		"expires_in": int(oauth.AccessLifetime.Seconds()),
	})
}

func (s Server) revoke(w http.ResponseWriter, r *http.Request) {
	if !parseForm(w, r) {
		return
	}
	if r.PostForm.Get("client_id") != oauth.ClientID {
		reject(w, http.StatusBadRequest, "invalid_client")
		return
	}
	token := r.PostForm.Get("token")
	if validHex(token, 64) {
		if err := s.Store.RevokeToken(r.Context(), token); err != nil {
			failure(w, "revoke_token", err)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
}

func bearer(r *http.Request) string {
	prefix, value, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") {
		return ""
	}
	return value
}

func (s Server) inventory(w http.ResponseWriter, r *http.Request) {
	token := bearer(r)
	if !validHex(token, 64) {
		reject(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	grant, err := s.Store.Lookup(r.Context(), token)
	if errors.Is(err, sql.ErrNoRows) {
		reject(w, http.StatusUnauthorized, "invalid_token")
		return
	}
	if err != nil {
		failure(w, "lookup_access_grant", err)
		return
	}
	page, status, err := s.ERP.Inventory(r.Context(), grant, r.URL.Query())
	if err != nil {
		slog.Warn("ERP inventory request failed", "grant_id", grant.ID)
		reject(w, http.StatusBadGateway, "erp_unavailable")
		return
	}
	slog.Info("inventory requested", "grant_id", grant.ID, "user_id", grant.UserID, "status", status)
	switch status {
	case http.StatusUnauthorized:
		reject(w, status, "erp_identity_invalid")
	case http.StatusForbidden:
		reject(w, status, "insufficient_permission")
	default:
		writeJSON(w, http.StatusOK, page)
	}
}

type bridgeRequest struct {
	UserID int64 `json:"userId"`
	Epoch int64 `json:"epoch"`
	UserCode string `json:"userCode"`
	Approve *bool `json:"approve,omitempty"`
	GrantID string `json:"grantId"`
}

func (s Server) bridge(next func(http.ResponseWriter, *http.Request, bridgeRequest)) http.HandlerFunc {
	expected := sha256.Sum256([]byte(s.BridgeSecret))
	return func(w http.ResponseWriter, r *http.Request) {
		got := sha256.Sum256([]byte(bearer(r)))
		if s.BridgeSecret == "" || subtle.ConstantTimeCompare(got[:], expected[:]) != 1 {
			reject(w, http.StatusUnauthorized, "invalid_service_identity")
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		var req bridgeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.UserID <= 0 || req.Epoch < 0 {
			reject(w, http.StatusBadRequest, "invalid_request")
			return
		}
		req.UserCode = strings.ToUpper(strings.TrimSpace(req.UserCode))
		next(w, r, req)
	}
}

func (s Server) inspect(w http.ResponseWriter, r *http.Request, req bridgeRequest) {
	if !validHex(req.UserCode, 12) {
		reject(w, http.StatusBadRequest, "invalid_request")
		return
	}
	expiry, err := s.Store.Pending(r.Context(), req.UserCode)
	if errors.Is(err, sql.ErrNoRows) {
		reject(w, http.StatusNotFound, "device_request_unavailable")
		return
	}
	if err != nil {
		failure(w, "inspect_device_authorization", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"clientId": oauth.ClientID, "scope": oauth.Scope, "expiresAt": expiry,
		"accessLifetimeSeconds": int(oauth.AccessLifetime.Seconds()),
	})
}

func (s Server) decide(w http.ResponseWriter, r *http.Request, req bridgeRequest) {
	if !validHex(req.UserCode, 12) || req.Approve == nil {
		reject(w, http.StatusBadRequest, "invalid_request")
		return
	}
	ok, err := s.Store.Decide(r.Context(), req.UserCode, req.UserID, req.Epoch, *req.Approve)
	if err != nil {
		failure(w, "decide_device_authorization", err)
		return
	}
	if !ok {
		reject(w, http.StatusNotFound, "device_request_unavailable")
		return
	}
	slog.Info("device authorization decided", "user_id", req.UserID, "approved", *req.Approve)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s Server) list(w http.ResponseWriter, r *http.Request, req bridgeRequest) {
	grants, err := s.Store.List(r.Context(), req.UserID, req.Epoch)
	if err != nil {
		failure(w, "list_grants", err)
		return
	}
	writeJSON(w, http.StatusOK, grants)
}

func (s Server) revokeGrant(w http.ResponseWriter, r *http.Request, req bridgeRequest) {
	if !validHex(req.GrantID, 32) {
		reject(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if err := s.Store.RevokeGrant(r.Context(), req.UserID, req.GrantID); err != nil {
		failure(w, "revoke_grant", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
