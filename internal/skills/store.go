package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/m1neroma/neko/internal/config"
)

type Store struct {
	config *config.Store
}

func New(store *config.Store) *Store {
	return &Store{config: store}
}

func (s *Store) Add(name, path string) error {
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" || path == "" {
		return errors.New("skill name and path are required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("skill path must be a directory")
	}
	if _, err := os.Stat(filepath.Join(abs, "SKILL.md")); err != nil {
		return errors.New("skill directory must contain SKILL.md")
	}
	return s.config.Update(func(cfg *config.Config) error {
		for i := range cfg.Skills {
			if cfg.Skills[i].Name == name {
				cfg.Skills[i] = config.Skill{Name: name, Path: abs, Enabled: true}
				return nil
			}
		}
		cfg.Skills = append(cfg.Skills, config.Skill{Name: name, Path: abs, Enabled: true})
		return nil
	})
}

func (s *Store) Load(name string) (string, error) {
	cfg := s.config.Get()
	for _, skill := range cfg.Skills {
		if skill.Name == name && skill.Enabled {
			data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
			if err != nil {
				return "", err
			}
			if len(data) > 128*1024 {
				return "", errors.New("SKILL.md exceeds 128 KiB")
			}
			return string(data), nil
		}
	}
	return "", fmt.Errorf("skill %q not found or disabled", name)
}

func (s *Store) List() []config.Skill {
	return s.config.Get().Skills
}
