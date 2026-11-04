package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"kasinlo-open-platform/internal/app"
	"kasinlo-open-platform/internal/erpclient"
	"kasinlo-open-platform/internal/oauth"
	"kasinlo-open-platform/migrations"
)

func main() {
	if err := run(); err != nil {
		slog.Error("open platform stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dsn := os.Getenv("PLATFORM_DATABASE_URL")
	signingSecret := os.Getenv("PLATFORM_ERP_SIGNING_SECRET")
	bridgeSecret := os.Getenv("PLATFORM_BRIDGE_SECRET")
	issuer := strings.TrimSpace(os.Getenv("PLATFORM_ERP_ISSUER"))
	audience := strings.TrimSpace(os.Getenv("PLATFORM_ERP_AUDIENCE"))
	erpURL := strings.TrimRight(os.Getenv("PLATFORM_ERP_API_URL"), "/")
	verificationURI := os.Getenv("PLATFORM_VERIFICATION_URI")
	if dsn == "" || len(signingSecret) < 32 || len(bridgeSecret) < 32 || signingSecret == bridgeSecret ||
		issuer == "" || audience == "" || !validURL(erpURL, false) || !validURL(verificationURI, true) {
		return errors.New("invalid platform configuration; see open-platform/README.md")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return errors.New("invalid platform database configuration")
	}
	defer db.Close()
	db.SetMaxOpenConns(12)
	db.SetMaxIdleConns(4)
	startup, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = migrations.Apply(startup, db)
	cancel()
	if err != nil {
		return fmt.Errorf("platform database migration failed (%T)", err)
	}
	store := oauth.Store{DB: db}
	go prune(ctx, store)

	addr := os.Getenv("PLATFORM_ADDR")
	if addr == "" {
		addr = ":6090"
	}
	service := app.Server{
		Store: store,
		ERP: erpclient.Client{
			HTTP: &http.Client{
				Timeout: 10 * time.Second,
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
			},
			URL: erpURL, Issuer: issuer, Audience: audience, Secret: []byte(signingSecret),
		},
		VerificationURI: verificationURI, BridgeSecret: bridgeSecret,
	}
	server := &http.Server{
		Addr: addr, Handler: service.Handler(),
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10,
	}
	done := make(chan error, 1)
	go func() { done <- server.ListenAndServe() }()
	slog.Info("open platform listening", "address", addr)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return server.Shutdown(shutdown)
	}
}

func validURL(raw string, browser bool) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" {
		return false
	}
	if !browser {
		return u.Fragment == "" && (u.Scheme == "https" || u.Scheme == "http")
	}
	loopback := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	return u.Fragment == "/open-platform/authorize" &&
		(u.Scheme == "https" || (u.Scheme == "http" && loopback))
}

func prune(ctx context.Context, store oauth.Store) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pruneCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := store.Prune(pruneCtx); err != nil && ctx.Err() == nil {
				slog.Warn("platform grant cleanup failed")
			}
			cancel()
		}
	}
}
