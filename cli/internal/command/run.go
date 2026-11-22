package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kasinlo-cli/internal/client"
	"kasinlo-cli/internal/credential"
)

const usage = `kas [--server URL] [--profile NAME] auth login [--scope "inventory.read product.read customer.read supplier.read purchase.read sales.read stock.read finance.read contract.read contract.history.read contract.report.read"] [--no-wait | --session ID]
kas [--server URL] [--profile NAME] auth status
kas [--server URL] [--profile NAME] auth logout
kas [--server URL] [--profile NAME] inventory list [--keyword TEXT] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] product list [--keyword TEXT] [--status 0|1] [--category-id ID] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] product get --id ID
kas [--server URL] [--profile NAME] category list [--keyword TEXT] [--status 0|1] [--parent-id ID] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] category get --id ID
kas [--server URL] [--profile NAME] customer list [--keyword TEXT] [--status 0|1] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] customer get --id ID
kas [--server URL] [--profile NAME] supplier list [--keyword TEXT] [--status 0|1] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] supplier get --id ID
kas [--server URL] [--profile NAME] purchase list [--keyword TEXT] [--status 1..5] [--settle-status STATUS] [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] purchase get --id ID
kas [--server URL] [--profile NAME] sales list [--keyword TEXT] [--status 1..5] [--settle-status STATUS] [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] sales get --id ID
kas [--server URL] [--profile NAME] stock list [--direction in|out] [--source-type purchase|sales --source-id ID] [--include-voided] [--keyword TEXT] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] stock get --id ID
kas [--server URL] [--profile NAME] finance receivables|payables [--settle-status STATUS] [--source order|plan] [--keyword TEXT] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] finance settlements --order-id ID
kas [--server URL] [--profile NAME] contract list [--biz-type purchase|sales] [--status 1|2|3] [--stage STAGE] [--party-id ID] [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--keyword TEXT] [--page N] [--page-size N]
kas [--server URL] [--profile NAME] contract report [--biz-type purchase|sales] [--status 1|2|3] [--stage STAGE] [--party-id ID] [--start-date YYYY-MM-DD] [--end-date YYYY-MM-DD] [--keyword TEXT]
kas [--server URL] [--profile NAME] contract get --id ID
kas [--server URL] [--profile NAME] contract activity --id ID
kas [--server URL] [--profile NAME] contract settlements --plan-id ID
`

type problem struct {
	code    string
	message string
	exit    int
}

func (p *problem) Error() string { return p.message }

func usageError(message string) error { return &problem{"invalid_arguments", message, 2} }

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	err := execute(ctx, args, stdout, stderr)
	if err == nil {
		return 0
	}
	p := classify(err)
	_ = json.NewEncoder(stderr).Encode(map[string]string{"error": p.code, "message": p.message})
	return p.exit
}

func classify(err error) *problem {
	var p *problem
	var api *client.APIError
	switch {
	case errors.As(err, &p):
		return p
	case errors.Is(err, context.Canceled):
		return &problem{"cancelled", "operation cancelled", 130}
	case errors.As(err, &api):
		switch {
		case api.Code == "access_denied" || api.Code == "expired_token" || api.Status == http.StatusUnauthorized:
			return &problem{api.Code, "authorization is unavailable; run kas auth login", 3}
		case api.Code == "insufficient_scope":
			return &problem{api.Code, "this grant lacks the required scope; use a separately authorized profile or log out and request the scope", 4}
		case api.Status == http.StatusBadRequest:
			return &problem{api.Code, "invalid request parameters; check kas --help", 2}
		case api.Status == http.StatusNotFound:
			return &problem{api.Code, "record not found in the accessible data", 1}
		case api.Status == http.StatusForbidden:
			return &problem{api.Code, "the ERP user cannot perform this action", 4}
		default:
			return &problem{api.Code, "the open platform rejected the request", 1}
		}
	default:
		return &problem{"operation_failed", err.Error(), 1}
	}
}

func execute(ctx context.Context, args []string, out, diag io.Writer) error {
	global := flag.NewFlagSet("kas", flag.ContinueOnError)
	global.SetOutput(io.Discard)
	server := global.String("server", os.Getenv("KAS_SERVER"), "open platform URL")
	profile := global.String("profile", os.Getenv("KAS_PROFILE"), "local credential profile")
	if err := global.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.WriteString(out, usage)
			return err
		}
		return usageError(err.Error())
	}
	args = global.Args()
	if len(args) == 0 || len(args) == 1 && args[0] == "help" {
		_, err := io.WriteString(out, usage)
		return err
	}
	if len(args) < 2 {
		return usageError("expected a command; run kas --help")
	}
	name := args[0] + " " + args[1]
	queryCommand, isRead := readCommands[name]
	switch name {
	case "auth login", "auth status", "auth logout":
	default:
		if !isRead {
			return usageError("unknown command; run kas --help")
		}
	}
	if *profile == "" {
		*profile = "default"
	}
	if strings.TrimSpace(*profile) != *profile || len(*profile) > 100 {
		return usageError("invalid profile name")
	}
	api, err := client.New(*server)
	if err != nil {
		return usageError(err.Error())
	}
	root := os.Getenv("KAS_CONFIG_DIR")
	if root == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			return errors.New("could not determine the user configuration directory")
		}
		root = filepath.Join(dir, "kas")
	}
	store := credential.New(root, api.Base(), *profile)
	switch name {
	case "auth login":
		return login(ctx, api, store, args[2:], out, diag)
	case "auth status":
		if len(args) != 2 {
			return usageError("auth status takes no arguments")
		}
		return status(ctx, api, store, *profile, out)
	case "auth logout":
		if len(args) != 2 {
			return usageError("auth logout takes no arguments")
		}
		return logout(ctx, api, store, out)
	default:
		return queryCommand.run(ctx, api, store, name, args[2:], out)
	}
}

type loginSession struct {
	VerificationURI string    `json:"verification_uri"`
	UserCode        string    `json:"user_code"`
	SessionID       string    `json:"session_id"`
	ExpiresAt       time.Time `json:"expires_at"`
	Scope           string    `json:"scope"`
}

func login(ctx context.Context, api *client.Client, store credential.Store, args []string, out, diag io.Writer) error {
	flags := flag.NewFlagSet("auth login", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	noWait := flags.Bool("no-wait", false, "return a local session ID")
	session := flags.String("session", "", "resume a pending authorization")
	scope := flags.String("scope", "", "space-separated read scopes")
	if err := flags.Parse(args); err != nil {
		return usageError(err.Error())
	}
	if flags.NArg() != 0 || *session != "" && (*noWait || *scope != "") {
		return usageError("use auth login with --scope and/or --no-wait, or resume with --session ID")
	}
	*scope = strings.Join(strings.Fields(*scope), " ")
	if *scope == "" {
		*scope = "inventory.read"
	}
	if !client.ValidScope(*scope) {
		return usageError("unknown authorization scope; run kas --help")
	}

	current, err := store.Credential()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if current.Token != "" && time.Now().Before(current.ExpiresAt) {
		if _, err := api.Status(ctx, current.Token); err == nil {
			return &problem{"already_logged_in", "this profile already has an active grant; use auth status or auth logout", 3}
		} else if !unauthorized(err) {
			return err
		}
	}

	var pending credential.Pending
	if *session == "" {
		device, err := api.Start(ctx, *scope)
		if err != nil {
			return err
		}
		pending = credential.Pending{
			DeviceCode: device.DeviceCode,
			ExpiresAt:  time.Now().Add(time.Duration(device.ExpiresIn) * time.Second),
			Interval:   device.Interval,
		}
		id, err := store.SavePending(pending)
		if err != nil {
			return err
		}
		*session = id
		info := loginSession{device.VerificationURI, device.UserCode, id, pending.ExpiresAt, *scope}
		if *noWait {
			return json.NewEncoder(out).Encode(info)
		}
		fmt.Fprintf(diag, "Open %s and enter code %s. Resume with auth login --session %s.\n",
			info.VerificationURI, info.UserCode, info.SessionID)
	} else {
		pending, err = store.Pending(*session)
		if err != nil {
			return &problem{"session_unavailable", "local authorization session is unavailable; run kas auth login", 3}
		}
	}
	if !time.Now().Before(pending.ExpiresAt) {
		store.ClearPending(*session)
		return &problem{"expired_token", "authorization expired; run kas auth login", 3}
	}
	token, err := poll(ctx, api, pending)
	if err != nil {
		var apiErr *client.APIError
		if errors.As(err, &apiErr) && (apiErr.Code == "access_denied" || apiErr.Code == "expired_token") {
			store.ClearPending(*session)
		}
		return err
	}
	saved := credential.Credential{Token: token.AccessToken, ExpiresAt: time.Now().Add(time.Duration(token.ExpiresIn) * time.Second)}
	if err := store.Replace(current.Token, saved); err != nil {
		// The grant was just created for this process, so a failed local save can revoke it.
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if revokeErr := api.Revoke(cleanup, token.AccessToken); revokeErr != nil {
			return errors.New("could not save the credential or revoke the new grant; revoke it from the ERP authorization page")
		}
		return err
	}
	store.ClearPending(*session)
	return json.NewEncoder(out).Encode(map[string]any{"authenticated": true, "scope": token.Scope, "expires_at": saved.ExpiresAt})
}

func poll(ctx context.Context, api *client.Client, pending credential.Pending) (client.Token, error) {
	ctx, cancel := context.WithDeadline(ctx, pending.ExpiresAt)
	defer cancel()
	interval := pending.Interval
	for {
		timer := time.NewTimer(time.Duration(interval) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return client.Token{}, &problem{"expired_token", "authorization expired; run kas auth login", 3}
			}
			return client.Token{}, ctx.Err()
		case <-timer.C:
		}

		token, err := api.Poll(ctx, pending.DeviceCode)
		if err == nil {
			return token, nil
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return client.Token{}, &problem{"expired_token", "authorization expired; run kas auth login", 3}
		}
		var response *client.APIError
		if !errors.As(err, &response) {
			return client.Token{}, err
		}
		switch response.Code {
		case "authorization_pending":
		case "slow_down":
			interval = min(interval+5, 60)
		default:
			return client.Token{}, err
		}
	}
}

func unauthorized(err error) bool {
	var api *client.APIError
	return errors.As(err, &api) && api.Status == http.StatusUnauthorized
}

func status(ctx context.Context, api *client.Client, store credential.Store, profile string, out io.Writer) error {
	state := map[string]any{"authenticated": false, "server": api.Base(), "profile": profile}
	current, err := store.Credential()
	switch {
	case errors.Is(err, os.ErrNotExist):
		state["reason"] = "not_logged_in"
	case err != nil:
		return err
	case !time.Now().Before(current.ExpiresAt):
		state["reason"] = "expired"
	default:
		live, err := api.Status(ctx, current.Token)
		if unauthorized(err) {
			state["reason"] = "invalid_token"
		} else if err != nil {
			return err
		} else {
			state["authenticated"] = true
			state["scope"] = live.Scope
			state["expires_at"] = live.ExpiresAt
		}
	}
	return json.NewEncoder(out).Encode(state)
}

func logout(ctx context.Context, api *client.Client, store credential.Store, out io.Writer) error {
	current, err := store.Credential()
	if errors.Is(err, os.ErrNotExist) {
		return json.NewEncoder(out).Encode(map[string]bool{"logged_out": false})
	}
	if err != nil {
		return err
	}
	if err := api.Revoke(ctx, current.Token); err != nil {
		return err
	}
	if err := store.Remove(current.Token); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return json.NewEncoder(out).Encode(map[string]bool{"logged_out": true})
}
