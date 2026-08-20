package skills

import (
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/m1neroma/neko/internal/config"
	"github.com/m1neroma/neko/internal/core"
)

// bundled holds the skill sets shipped inside the binary. Each mode gets one
// subdirectory, and each skill is one directory containing SKILL.md.
//
//go:embed bundled
var bundled embed.FS

// Bundled describes one skill compiled into the binary.
type Bundled struct {
	Name    string
	Mode    string
	Summary string
	Body    string
}

// BundledForMode returns the skills shipped for a mode, sorted by name.
func BundledForMode(mode string) []Bundled {
	dir := "bundled/" + core.NormalizeMode(mode)
	entries, err := bundled.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []Bundled
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		data, err := bundled.ReadFile(dir + "/" + entry.Name() + "/SKILL.md")
		if err != nil {
			continue
		}
		descriptor := parseFrontmatter(string(data))
		if descriptor.Name == "" {
			descriptor.Name = entry.Name()
		}
		if descriptor.Mode == "" {
			descriptor.Mode = core.NormalizeMode(mode)
		}
		out = append(out, Bundled{
			Name: descriptor.Name, Mode: descriptor.Mode,
			Summary: descriptor.Summary, Body: string(data),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// InstallBundled writes the bundled skills for a mode under root and registers
// any that are not already known. It returns the names it added; empty means
// everything was already registered.
//
// Existing config entries are left alone so user edits and disabled flags
// survive, but files are rewritten every time so an upgraded binary refreshes
// their content.
func (s *Store) InstallBundled(root, mode string) ([]string, error) {
	items := BundledForMode(mode)
	if len(items) == 0 {
		return nil, nil
	}
	known := map[string]bool{}
	for _, skill := range s.config.Get().Skills {
		known[skill.Name] = true
	}
	base := filepath.Join(root, core.NormalizeMode(mode))
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, err
	}
	var added []string
	var pending []config.Skill
	for _, item := range items {
		dir := filepath.Join(base, item.Name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := writeFileAtomic(filepath.Join(dir, "SKILL.md"), []byte(item.Body)); err != nil {
			return nil, err
		}
		if known[item.Name] {
			continue
		}
		pending = append(pending, config.Skill{
			Name: item.Name, Path: dir, Enabled: true,
			Mode: item.Mode, Summary: item.Summary,
		})
		added = append(added, item.Name)
	}
	if len(pending) == 0 {
		return nil, nil
	}
	if err := s.config.Update(func(cfg *config.Config) error {
		cfg.Skills = append(cfg.Skills, pending...)
		return nil
	}); err != nil {
		return nil, err
	}
	return added, nil
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".neko-tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type Store struct {
	config *config.Store
}

func New(store *config.Store) *Store {
	return &Store{config: store}
}

// Descriptor is the frontmatter Neko reads from a SKILL.md.
type Descriptor struct {
	Name    string
	Mode    string
	Summary string
	Path    string
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
	descriptor, err := describe(abs)
	if err != nil {
		return err
	}
	return s.config.Update(func(cfg *config.Config) error {
		entry := config.Skill{Name: name, Path: abs, Enabled: true, Mode: descriptor.Mode, Summary: descriptor.Summary}
		for i := range cfg.Skills {
			if cfg.Skills[i].Name == name {
				cfg.Skills[i] = entry
				return nil
			}
		}
		cfg.Skills = append(cfg.Skills, entry)
		return nil
	})
}

func (s *Store) Load(name string) (string, error) {
	cfg := s.config.Get()
	for _, skill := range cfg.Skills {
		if skill.Name == name && skill.Enabled {
			data, err := os.ReadFile(filepath.Join(skill.Path, "SKILL.md"))
			if err == nil {
				if len(data) > 128*1024 {
					return "", errors.New("SKILL.md exceeds 128 KiB")
				}
				return string(data), nil
			}
			// A bundled skill whose file was removed still resolves from the binary.
			if body, ok := bundledBody(name); ok {
				return body, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("skill %q not found or disabled", name)
}

func bundledBody(name string) (string, bool) {
	for _, mode := range []string{core.ModeReverse, core.ModeBuild, core.ModePlan} {
		for _, item := range BundledForMode(mode) {
			if item.Name == name {
				return item.Body, true
			}
		}
	}
	return "", false
}

func (s *Store) List() []config.Skill {
	return s.config.Get().Skills
}

// ForMode returns enabled skills usable in the given mode: those tagged for it
// plus untagged skills, which apply everywhere.
func (s *Store) ForMode(mode string) []config.Skill {
	mode = core.NormalizeMode(mode)
	var out []config.Skill
	for _, skill := range s.config.Get().Skills {
		if !skill.Enabled {
			continue
		}
		if skill.Mode == "" || core.NormalizeMode(skill.Mode) == mode {
			out = append(out, skill)
		}
	}
	return out
}

// Discover finds skill directories bundled under dir. Each subdirectory holding
// a SKILL.md is one skill.
func Discover(dir string) ([]Descriptor, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []Descriptor
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		descriptor, err := describe(path)
		if err != nil {
			continue
		}
		if descriptor.Name == "" {
			descriptor.Name = entry.Name()
		}
		out = append(out, descriptor)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	if len(out) == 0 {
		return nil, fmt.Errorf("no SKILL.md found under %s", dir)
	}
	return out, nil
}

// describe reads and parses a skill directory's SKILL.md frontmatter.
func describe(dir string) (Descriptor, error) {
	path := filepath.Join(dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		return Descriptor{}, errors.New("skill directory must contain SKILL.md")
	}
	if len(data) > 128*1024 {
		return Descriptor{}, errors.New("SKILL.md exceeds 128 KiB")
	}
	descriptor := parseFrontmatter(string(data))
	descriptor.Path = dir
	return descriptor, nil
}

// parseFrontmatter reads a leading `---` delimited block of `key: value` pairs.
// Files without frontmatter are valid and yield an empty descriptor.
func parseFrontmatter(content string) Descriptor {
	var descriptor Descriptor
	content = strings.TrimPrefix(content, "\ufeff")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return descriptor
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		if trimmed == "---" {
			break
		}
		key, value, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "name":
			descriptor.Name = value
		case "mode":
			descriptor.Mode = strings.ToLower(value)
		case "summary", "description":
			descriptor.Summary = value
		}
	}
	return descriptor
}
