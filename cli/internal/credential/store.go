package credential

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type Credential struct {
	Token string `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Pending struct {
	DeviceCode string `json:"deviceCode"`
	ExpiresAt time.Time `json:"expiresAt"`
	Interval int `json:"interval"`
}

type Store struct { dir string }

var ErrChanged = errors.New("login changed in another kas process; run auth status")

func New(root, server, profile string) Store {
	id := sha256.Sum256([]byte(server + "\x00" + profile))
	return Store{dir: filepath.Join(root, hex.EncodeToString(id[:]))}
}

func (s Store) Credential() (Credential, error) {
	var out Credential
	err := read(filepath.Join(s.dir, "credential.json"), &out)
	if err == nil && (out.Token == "" || out.ExpiresAt.IsZero()) {
		err = errors.New("invalid saved credential")
	}
	return out, err
}

func (s Store) Replace(expected string, next Credential) error {
	return s.update(expected, func(path string) error { return write(path, next) })
}

func (s Store) Remove(expected string) error {
	return s.update(expected, os.Remove)
}

func (s Store) update(expected string, apply func(string) error) error {
	if err := s.ensureDir(); err != nil {
		return err
	}
	lock := filepath.Join(s.dir, "credential.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return errors.New("credential store is busy; retry the command")
		}
		return err
	}
	defer os.Remove(lock)
	current, err := s.Credential()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if current.Token != expected {
		return ErrChanged
	}
	return apply(filepath.Join(s.dir, "credential.json"))
}

func (s Store) SavePending(p Pending) (string, error) {
	if err := s.ensureDir(); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		name := entry.Name()
		id := strings.TrimPrefix(name, "pending-")
		if id != name && validSession(strings.TrimSuffix(id, ".json")) && strings.HasSuffix(name, ".json") {
			if pending, err := s.Pending(strings.TrimSuffix(id, ".json")); err == nil && !time.Now().Before(pending.ExpiresAt) {
				_ = os.Remove(filepath.Join(s.dir, name))
			}
		}
	}
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	return id, write(s.pendingPath(id), p)
}

func validSession(id string) bool {
	_, err := hex.DecodeString(id)
	return len(id) == 32 && err == nil
}

func (s Store) pendingPath(id string) string {
	return filepath.Join(s.dir, "pending-"+id+".json")
}

func (s Store) Pending(id string) (Pending, error) {
	var out Pending
	if !validSession(id) {
		return out, errors.New("invalid local authorization session")
	}
	err := read(s.pendingPath(id), &out)
	if err == nil && (out.DeviceCode == "" || out.ExpiresAt.IsZero() || out.Interval < 1 || out.Interval > 60) {
		err = errors.New("invalid saved authorization session")
	}
	return out, err
}

func (s Store) ClearPending(id string) {
	if validSession(id) {
		_ = os.Remove(s.pendingPath(id))
	}
}

func (s Store) ensureDir() error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}
	info, err := os.Lstat(s.dir)
	if err != nil {
		return err
	}
	if !info.IsDir() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("credential directory must be private to this OS user")
	}
	return nil
}

func read(path string, out any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0) {
		return errors.New("credential file must be private to this OS user")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(io.LimitReader(f, 8192)).Decode(out)
}

func write(path string, data any) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".kas-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := json.NewEncoder(f).Encode(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
