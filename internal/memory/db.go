// Package memory manages all persistent state in a local SQLite database:
// sessions, task history, skill registry, and scheduled jobs.
package memory

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// DB is the central state store.
type DB struct {
	db *sql.DB
}

// DefaultPath returns the user-local database path.
func DefaultPath() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "velkrogo", "state.db"), nil
}

// Open opens or creates the database at path.
func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS sessions (
		id         TEXT PRIMARY KEY,
		created_at TEXT NOT NULL,
		title      TEXT NOT NULL DEFAULT ''
	);
	CREATE TABLE IF NOT EXISTS messages (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT NOT NULL,
		role       TEXT NOT NULL,
		content    TEXT NOT NULL,
		ts         TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS skills (
		id          TEXT PRIMARY KEY,
		name        TEXT NOT NULL,
		description TEXT NOT NULL,
		prompt      TEXT NOT NULL,
		created_at  TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS jobs (
		id          TEXT PRIMARY KEY,
		title       TEXT NOT NULL,
		prompt      TEXT NOT NULL,
		schedule    TEXT NOT NULL,
		next_run    TEXT,
		last_run    TEXT,
		enabled     INTEGER NOT NULL DEFAULT 1,
		created_at  TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS memory_facts (
		key        TEXT PRIMARY KEY,
		value      TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS session_rules (
		session_id TEXT NOT NULL,
		rule       TEXT NOT NULL,
		created_at TEXT NOT NULL,
		PRIMARY KEY (session_id, rule)
	);`)
	return err
}

func (d *DB) Close() error { return d.db.Close() }

// --- Sessions ---

type Session struct {
	ID        string
	CreatedAt time.Time
	Title     string
}

func (d *DB) CreateSession(id, title string) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO sessions(id,created_at,title) VALUES(?,?,?)`,
		id, time.Now().UTC().Format(time.RFC3339), title)
	return err
}

func (d *DB) ListSessions(limit int) ([]Session, error) {
	rows, err := d.db.Query(`SELECT id,created_at,title FROM sessions ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		var ts string
		if err := rows.Scan(&s.ID, &ts, &s.Title); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- Jobs (scheduler) ---

type Job struct {
	ID        string
	Title     string
	Prompt    string
	Schedule  string // cron expression or "once:RFC3339"
	NextRun   *time.Time
	LastRun   *time.Time
	Enabled   bool
	CreatedAt time.Time
}

func (d *DB) UpsertJob(j Job) error {
	var nr, lr *string
	if j.NextRun != nil {
		s := j.NextRun.UTC().Format(time.RFC3339)
		nr = &s
	}
	if j.LastRun != nil {
		s := j.LastRun.UTC().Format(time.RFC3339)
		lr = &s
	}
	_, err := d.db.Exec(`INSERT INTO jobs(id,title,prompt,schedule,next_run,last_run,enabled,created_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title,prompt=excluded.prompt,
		schedule=excluded.schedule,next_run=excluded.next_run,
		last_run=excluded.last_run,enabled=excluded.enabled`,
		j.ID, j.Title, j.Prompt, j.Schedule, nr, lr, boolInt(j.Enabled), j.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (d *DB) ListJobs() ([]Job, error) {
	rows, err := d.db.Query(`SELECT id,title,prompt,schedule,next_run,last_run,enabled,created_at FROM jobs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		var j Job
		var ca string
		var nr, lr *string
		var enabled int
		if err := rows.Scan(&j.ID, &j.Title, &j.Prompt, &j.Schedule, &nr, &lr, &enabled, &ca); err != nil {
			return nil, err
		}
		j.Enabled = enabled != 0
		j.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		if nr != nil {
			t, _ := time.Parse(time.RFC3339, *nr)
			j.NextRun = &t
		}
		if lr != nil {
			t, _ := time.Parse(time.RFC3339, *lr)
			j.LastRun = &t
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (d *DB) DeleteJob(id string) error {
	_, err := d.db.Exec(`DELETE FROM jobs WHERE id=?`, id)
	return err
}

// --- Skills ---

type Skill struct {
	ID          string
	Name        string
	Description string
	Prompt      string
	CreatedAt   time.Time
}

func (d *DB) UpsertSkill(s Skill) error {
	_, err := d.db.Exec(`INSERT INTO skills(id,name,description,prompt,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name,description=excluded.description,prompt=excluded.prompt`,
		s.ID, s.Name, s.Description, s.Prompt, s.CreatedAt.UTC().Format(time.RFC3339))
	return err
}

func (d *DB) ListSkills() ([]Skill, error) {
	rows, err := d.db.Query(`SELECT id,name,description,prompt,created_at FROM skills`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Skill
	for rows.Next() {
		var s Skill
		var ca string
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Prompt, &ca); err != nil {
			return nil, err
		}
		s.CreatedAt, _ = time.Parse(time.RFC3339, ca)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) DeleteSkill(id string) error {
	_, err := d.db.Exec(`DELETE FROM skills WHERE id=?`, id)
	return err
}

// --- Memory facts ---

// MemoryFact is a persistent key-value fact the agent can recall across sessions.
type MemoryFact struct {
	Key       string
	Value     string
	UpdatedAt time.Time
}

func (d *DB) SetMemory(key, value string) error {
	_, err := d.db.Exec(`INSERT INTO memory_facts(key,value,updated_at) VALUES(?,?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`,
		key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (d *DB) GetMemory(key string) (string, error) {
	var value string
	err := d.db.QueryRow(`SELECT value FROM memory_facts WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	return value, err
}

func (d *DB) ListMemory() ([]MemoryFact, error) {
	rows, err := d.db.Query(`SELECT key,value,updated_at FROM memory_facts ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryFact
	for rows.Next() {
		var f MemoryFact
		var ts string
		if err := rows.Scan(&f.Key, &f.Value, &ts); err != nil {
			return nil, err
		}
		f.UpdatedAt, _ = time.Parse(time.RFC3339, ts)
		out = append(out, f)
	}
	return out, rows.Err()
}

func (d *DB) DeleteMemory(key string) error {
	_, err := d.db.Exec(`DELETE FROM memory_facts WHERE key=?`, key)
	return err
}

// --- Session rules ---

func (d *DB) AddRule(sessionID, rule string) error {
	_, err := d.db.Exec(`INSERT OR IGNORE INTO session_rules(session_id,rule,created_at) VALUES(?,?,?)`,
		sessionID, rule, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (d *DB) ListRules(sessionID string) ([]string, error) {
	rows, err := d.db.Query(`SELECT rule FROM session_rules WHERE session_id=? ORDER BY created_at`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) DeleteRule(sessionID, rule string) error {
	_, err := d.db.Exec(`DELETE FROM session_rules WHERE session_id=? AND rule=?`, sessionID, rule)
	return err
}

var ErrNotFound = errors.New("not found")

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
