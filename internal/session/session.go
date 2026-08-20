package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/m1neroma/neko/internal/core"
)

type Change struct {
	Path         string    `json:"path"`
	Before       string    `json:"before,omitempty"`
	After        string    `json:"after,omitempty"`
	BeforeExists bool      `json:"before_exists"`
	At           time.Time `json:"at"`
}

type Checkpoint struct {
	ID           string    `json:"id"`
	Label        string    `json:"label"`
	ChangeIndex  int       `json:"change_index"`
	MessageIndex int       `json:"message_index"`
	CreatedAt    time.Time `json:"created_at"`
}

type Session struct {
	ID          string          `json:"id"`
	ProjectRoot string          `json:"project_root"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Mode        string          `json:"mode"`
	Messages    []core.Message  `json:"messages"`
	Summary     string          `json:"summary,omitempty"`
	Usage       core.Usage      `json:"usage"`
	Plan        []core.PlanItem `json:"plan,omitempty"`
	Changes     []Change        `json:"changes,omitempty"`
	Checkpoints []Checkpoint    `json:"checkpoints,omitempty"`
}

type Manager struct {
	root string
}

func NewManager(projectRoot string) (*Manager, error) {
	dataDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(projectRoot))
	root := filepath.Join(dataDir, "neko", "projects", hex.EncodeToString(h[:8]), "sessions")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Manager{root: root}, nil
}

func (m *Manager) New(projectRoot string) (*Session, error) {
	id, err := uuid()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s := &Session{ID: id, ProjectRoot: projectRoot, CreatedAt: now, UpdatedAt: now, Mode: core.ModeBuild}
	return s, m.Save(s)
}

func (m *Manager) Save(s *Session) error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(m.root, s.ID+".json.tmp")
	path := filepath.Join(m.root, s.ID+".json")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (m *Manager) Load(id string) (*Session, error) {
	if strings.ContainsAny(id, `/\\`) {
		return nil, errors.New("invalid session id")
	}
	data, err := os.ReadFile(filepath.Join(m.root, id+".json"))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	// Sessions written by older versions may hold an unknown or empty mode.
	s.Mode = core.NormalizeMode(s.Mode)
	return &s, nil
}

func (m *Manager) Latest() (*Session, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	type candidate struct {
		id string
		at time.Time
	}
	var list []candidate
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err == nil {
			list = append(list, candidate{id: strings.TrimSuffix(e.Name(), ".json"), at: info.ModTime()})
		}
	}
	if len(list) == 0 {
		return nil, os.ErrNotExist
	}
	sort.Slice(list, func(i, j int) bool { return list[i].at.After(list[j].at) })
	return m.Load(list[0].id)
}

func (m *Manager) List() ([]Session, error) {
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		s, err := m.Load(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil {
			out = append(out, *s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func uuid() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
