package store

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	DB *sql.DB
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// SQLite in WAL mode: single writer, many readers; small writes are cheap.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("pragma %s: %w", pragma, err)
		}
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password TEXT NOT NULL,
			is_superuser INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			song_name TEXT NOT NULL DEFAULT '',
			artist_name TEXT NOT NULL DEFAULT '',
			full_path TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'wait',
			parent_path TEXT NOT NULL DEFAULT '',
			filename TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_path)`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_full_path ON tasks(full_path)`,
		`CREATE TABLE IF NOT EXISTS task_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			song_name TEXT NOT NULL DEFAULT '',
			artist_name TEXT NOT NULL DEFAULT '',
			full_path TEXT NOT NULL DEFAULT '',
			tag_source TEXT NOT NULL DEFAULT '',
			icon TEXT NOT NULL DEFAULT 'icon-folder',
			state TEXT NOT NULL DEFAULT 'wait',
			extra TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			batch TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_records_batch ON task_records(batch)`,
	}
	for _, stmt := range stmts {
		if _, err := s.DB.Exec(stmt); err != nil {
			return fmt.Errorf("migrate: %w", err)
		}
	}
	return nil
}

// EnsureBootstrap imports users from a legacy Django db (auth_user table)
// when present, otherwise seeds the documented default admin/admin.
func (s *Store) EnsureBootstrap(legacyDBPath, adminPassword string) error {
	count := 0
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	if legacyDBPath != "" {
		if n, err := s.importLegacyUsers(legacyDBPath); err == nil && n > 0 {
			return nil
		}
	}
	if adminPassword != "" {
		return s.CreateUser("admin", adminPassword, true)
	}
	return s.CreateUser("admin", "admin", true)
}

// importLegacyUsers copies username/password hashes from a Django 2.2
// auth_user table so existing logins keep working. Passwords remain
// pbkdf2_sha256 hashes and are verified in place.
func (s *Store) importLegacyUsers(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, err
	}
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		return 0, err
	}
	defer legacy.Close()
	rows, err := legacy.Query(`SELECT username, password, COALESCE(is_superuser, 0) FROM auth_user`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var username, password string
		var super int
		if err := rows.Scan(&username, &password, &super); err != nil {
			continue
		}
		if username == "" || password == "" {
			continue
		}
		if _, err := s.DB.Exec(`INSERT OR IGNORE INTO users (username, password, is_superuser, created_at) VALUES (?,?,?,?)`,
			username, password, super, nowStr()); err != nil {
			continue
		}
		n++
	}
	return n, nil
}

func nowStr() string { return time.Now().Format("2006-01-02 15:04:05") }

// ---- users ----

func (s *Store) GetUser(username string) (*User, error) {
	u := &User{}
	err := s.DB.QueryRow(`SELECT id, username, password, is_superuser FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.Password, &u.IsSuperuser)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (s *Store) CreateUser(username, password string, super bool) error {
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO users (username, password, is_superuser, created_at) VALUES (?,?,?,?)`,
		username, hash, boolInt(super), nowStr())
	return err
}

// VerifyPassword checks a plain password against a stored hash. Both new
// hashes and Django's "pbkdf2_sha256$<iters>$<salt>$<b64>" format verify here,
// so legacy databases need no rehash on import.
func VerifyPassword(stored, plain string) bool {
	parts := strings.SplitN(stored, "$", 4)
	if len(parts) != 4 || parts[0] != "pbkdf2_sha256" {
		return false
	}
	var iters int
	if _, err := fmt.Sscanf(parts[1], "%d", &iters); err != nil || iters <= 0 || iters > 10_000_000 {
		return false
	}
	salt := parts[2]
	want, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, plain, []byte(salt), iters, len(want))
	if err != nil {
		return false
	}
	return string(got) == string(want)
}

func HashPassword(plain string) (string, error) {
	salt := make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	const iters = 260000
	key, err := pbkdf2.Key(sha256.New, plain, salt, iters, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2_sha256$%d$%s$%s", iters, string(salt), base64.StdEncoding.EncodeToString(key)), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
