package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	ClientID       = "kas"
	DeviceLifetime = 10 * time.Minute
	AccessLifetime = 8 * time.Hour
	PollInterval   = 5
)

type Store struct {
	DB *sql.DB
}

type Grant struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"-"`
	Epoch     int64     `json:"-"`
	ClientID  string    `json:"clientId"`
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func randomHex(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func digest(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func (s Store) Start(ctx context.Context, scope string) (string, string, error) {
	device, err := randomHex(32)
	if err != nil {
		return "", "", err
	}
	code, err := randomHex(6)
	if err != nil {
		return "", "", err
	}
	code = strings.ToUpper(code)
	now := time.Now()
	_, err = s.DB.ExecContext(ctx, `INSERT INTO device_authorizations
		(device_hash, user_code, client_id, expires_at, next_poll_at, scope)
		VALUES ($1,$2,$3,$4,$5,$6)`, digest(device), code, ClientID, now.Add(DeviceLifetime), now.Add(PollInterval*time.Second), scope)
	return device, code, err
}

func (s Store) Pending(ctx context.Context, code string) (time.Time, string, error) {
	var expiry time.Time
	var scope string
	err := s.DB.QueryRowContext(ctx, `SELECT expires_at, scope FROM device_authorizations
		WHERE user_code=$1 AND state='pending' AND expires_at>now()`, code).Scan(&expiry, &scope)
	return expiry, scope, err
}

func (s Store) Decide(ctx context.Context, code string, userID, epoch int64, approve bool, scope string) (bool, error) {
	state := "denied"
	if approve {
		state = "approved"
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE device_authorizations SET state=$2, user_id=$3, token_epoch=$4
		WHERE user_code=$1 AND state='pending' AND expires_at>now() AND (NOT $5 OR scope=$6)`, code, state, userID, epoch, approve, scope)
	if err != nil {
		return false, err
	}
	count, err := res.RowsAffected()
	return count == 1, err
}

// Poll serializes device-code redemption and stores the access grant atomically.
func (s Store) Poll(ctx context.Context, device string) (accessToken, scope, oauthError string, err error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", "", "", err
	}
	defer tx.Rollback()
	key := digest(device)
	var state string
	var expiry, next time.Time
	var interval int
	var userID, epoch sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT state, expires_at, next_poll_at, poll_interval, user_id, token_epoch, scope
		FROM device_authorizations WHERE device_hash=$1 AND client_id=$2 FOR UPDATE`,
		key, ClientID).Scan(&state, &expiry, &next, &interval, &userID, &epoch, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "expired_token", nil
	}
	if err != nil {
		return "", "", "", err
	}
	now := time.Now()
	if !now.Before(expiry) || state == "consumed" {
		return "", "", "expired_token", nil
	}
	if state == "denied" {
		return "", "", "access_denied", nil
	}
	if now.Before(next) || state == "pending" {
		status := "authorization_pending"
		if now.Before(next) {
			interval = min(interval+5, 60)
			status = "slow_down"
		}
		_, err = tx.ExecContext(ctx, `UPDATE device_authorizations SET next_poll_at=$2, poll_interval=$3
			WHERE device_hash=$1`, key, now.Add(time.Duration(interval)*time.Second), interval)
		if err != nil {
			return "", "", "", err
		}
		return "", "", status, tx.Commit()
	}

	access, err := randomHex(32)
	if err != nil {
		return "", "", "", err
	}
	id, err := randomHex(16)
	if err != nil {
		return "", "", "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO access_grants (id, token_hash, client_id, user_id, token_epoch, expires_at, scope)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, digest(access), ClientID, userID.Int64, epoch.Int64, now.Add(AccessLifetime), scope)
	if err != nil {
		return "", "", "", err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE device_authorizations SET state='consumed' WHERE device_hash=$1", key); err != nil {
		return "", "", "", err
	}
	if err := tx.Commit(); err != nil {
		return "", "", "", err
	}
	return access, scope, "", nil
}

func (s Store) Lookup(ctx context.Context, access string) (Grant, error) {
	var grant Grant
	err := s.DB.QueryRowContext(ctx, `SELECT id, client_id, user_id, token_epoch, expires_at, scope FROM access_grants
		WHERE token_hash=$1 AND expires_at>now() AND revoked_at IS NULL`, digest(access)).
		Scan(&grant.ID, &grant.ClientID, &grant.UserID, &grant.Epoch, &grant.ExpiresAt, &grant.Scope)
	return grant, err
}

func (s Store) RevokeToken(ctx context.Context, access string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE access_grants SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL`, digest(access))
	return err
}

func (s Store) List(ctx context.Context, userID int64, epoch int64) ([]Grant, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, client_id, expires_at, scope FROM access_grants
		WHERE user_id=$1 AND token_epoch=$2 AND expires_at>now() AND revoked_at IS NULL ORDER BY expires_at DESC`, userID, epoch)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	grants := make([]Grant, 0)
	for rows.Next() {
		var grant Grant
		if err := rows.Scan(&grant.ID, &grant.ClientID, &grant.ExpiresAt, &grant.Scope); err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	return grants, rows.Err()
}

func (s Store) RevokeGrant(ctx context.Context, userID int64, id string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE access_grants SET revoked_at=now()
		WHERE user_id=$1 AND id=$2 AND revoked_at IS NULL`, userID, id)
	return err
}

func (s Store) Prune(ctx context.Context) error {
	if _, err := s.DB.ExecContext(ctx, "DELETE FROM device_authorizations WHERE expires_at<now()"); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, "DELETE FROM access_grants WHERE expires_at < now() - interval '30 days'")
	return err
}
