// Package session manages conversation session persistence.
//
// Sessions are stored as JSON files under ~/.claude/projects/<hash>/sessions/.
// The directory structure and file format are designed to match the official
// Claude Code CLI for interoperability.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/anthropics/claude-code-go/internal/api"
)

// Session represents a saved conversation.
type Session struct {
	ID        string        `json:"id"`
	Model     string        `json:"model"`
	CWD       string        `json:"cwd"`
	Messages  []api.Message `json:"messages"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

// Store manages reading and writing sessions to disk.
type Store struct {
	dir string // e.g. ~/.claude/projects/<hash>/sessions/
}

// NewStore creates a session store for the given working directory.
// Sessions are stored under ~/.claude/projects/<cwd-hash>/sessions/.
func NewStore(cwd string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home directory: %w", err)
	}

	// Hash the CWD to create a project-specific directory.
	h := sha256.Sum256([]byte(cwd))
	projectHash := hex.EncodeToString(h[:16]) // 32 hex chars

	dir := filepath.Join(home, ".claude", "projects", projectHash, "sessions")
	return &Store{dir: dir}, nil
}

// NewStoreWithDir creates a session store at a specific directory (for testing).
func NewStoreWithDir(dir string) *Store {
	return &Store{dir: dir}
}

// Dir returns the session storage directory.
func (s *Store) Dir() string {
	return s.dir
}

// Save persists a session to disk. It creates the directory if needed.
func (s *Store) Save(session *Session) error {
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return fmt.Errorf("creating session directory: %w", err)
	}

	session.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling session: %w", err)
	}

	path := filepath.Join(s.dir, session.ID+".json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing session file: %w", err)
	}

	return nil
}

// Load reads a session by ID from disk.
func (s *Store) Load(id string) (*Session, error) {
	path := filepath.Join(s.dir, id+".json")
	return s.loadFile(path)
}

// MostRecent returns the session with the latest UpdatedAt timestamp.
func (s *Store) MostRecent() (*Session, error) {
	sessions, err := s.List()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	// Sessions from List() are sorted by UpdatedAt descending.
	return sessions[0], nil
}

// List returns all sessions sorted by UpdatedAt (newest first).
func (s *Store) List() ([]*Session, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading session directory: %w", err)
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		sess, err := s.loadFile(filepath.Join(s.dir, entry.Name()))
		if err != nil {
			continue // skip corrupt files
		}
		sessions = append(sessions, sess)
	}

	// Sort by UpdatedAt, newest first.
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// loadFile reads and parses a session JSON file.
func (s *Store) loadFile(path string) (*Session, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading session file: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("parsing session file: %w", err)
	}

	return &sess, nil
}

// GenerateID creates a new session ID based on the current timestamp.
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
