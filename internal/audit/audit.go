// Package audit provides an append-only, hash-chained event log. Every tool
// call, approval decision, and agent output is recorded here. The chain lets
// users verify nothing has been silently tampered with or deleted.
package audit

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Kind tags what kind of event is being logged.
type Kind string

const (
	KindToolCall   Kind = "tool_call"
	KindApproval   Kind = "approval"
	KindToolResult Kind = "tool_result"
	KindAgentText  Kind = "agent_text"
	KindUserInput  Kind = "user_input"
	KindError      Kind = "error"
	KindSchedule   Kind = "schedule"
)

// Entry is one immutable log record.
type Entry struct {
	ID        int64
	Timestamp time.Time
	Kind      Kind
	SessionID string
	Tool      string
	Data      json.RawMessage
	PrevHash  string
	Hash      string
}

// Log is the tamper-evident append-only store.
type Log struct {
	mu       sync.Mutex
	db       *sql.DB
	lastHash string
}

// Open opens or creates the audit log at the given SQLite path.
func Open(path string) (*Log, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS audit (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		ts        TEXT NOT NULL,
		kind      TEXT NOT NULL,
		session   TEXT NOT NULL DEFAULT '',
		tool      TEXT NOT NULL DEFAULT '',
		data      TEXT NOT NULL DEFAULT '{}',
		prev_hash TEXT NOT NULL,
		hash      TEXT NOT NULL
	)`)
	if err != nil {
		return nil, err
	}
	l := &Log{db: db}
	// Load last hash to chain from.
	row := db.QueryRow(`SELECT hash FROM audit ORDER BY id DESC LIMIT 1`)
	_ = row.Scan(&l.lastHash)
	return l, nil
}

// Append records a new event and returns its hash.
func (l *Log) Append(kind Kind, sessionID, tool string, data any) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	raw, err := json.Marshal(data)
	if err != nil {
		raw = json.RawMessage(`{}`)
	}
	ts := time.Now().UTC()
	payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s", ts.Format(time.RFC3339Nano), kind, sessionID, tool, raw, l.lastHash)
	sum := sha256.Sum256([]byte(payload))
	hash := hex.EncodeToString(sum[:])

	_, err = l.db.Exec(`INSERT INTO audit(ts,kind,session,tool,data,prev_hash,hash) VALUES(?,?,?,?,?,?,?)`,
		ts.Format(time.RFC3339Nano), kind, sessionID, tool, string(raw), l.lastHash, hash)
	if err != nil {
		return "", err
	}
	l.lastHash = hash
	return hash, nil
}

// Recent returns the last n entries in reverse-chronological order.
func (l *Log) Recent(n int) ([]Entry, error) {
	rows, err := l.db.Query(`SELECT id,ts,kind,session,tool,data,prev_hash,hash FROM audit ORDER BY id DESC LIMIT ?`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.Kind, &e.SessionID, &e.Tool, &e.Data, &e.PrevHash, &e.Hash); err != nil {
			return nil, err
		}
		e.Timestamp, _ = time.Parse(time.RFC3339Nano, ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

// Verify checks every entry's hash chain; returns the number of entries and
// whether the chain is intact.
func (l *Log) Verify() (int, bool, error) {
	rows, err := l.db.Query(`SELECT ts,kind,session,tool,data,prev_hash,hash FROM audit ORDER BY id ASC`)
	if err != nil {
		return 0, false, err
	}
	defer rows.Close()
	prev := ""
	count := 0
	for rows.Next() {
		var ts, kind, session, tool string
		var data json.RawMessage
		var prevHash, hash string
		if err := rows.Scan(&ts, &kind, &session, &tool, &data, &prevHash, &hash); err != nil {
			return count, false, err
		}
		if prevHash != prev {
			return count, false, nil
		}
		payload := fmt.Sprintf("%s|%s|%s|%s|%s|%s", ts, kind, session, tool, data, prev)
		sum := sha256.Sum256([]byte(payload))
		if hex.EncodeToString(sum[:]) != hash {
			return count, false, nil
		}
		prev = hash
		count++
	}
	return count, true, rows.Err()
}

func (l *Log) Close() error { return l.db.Close() }
