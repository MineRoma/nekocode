package project

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type Context struct {
	Root         string
	Instructions string
	Tree         string
}

func Discover() (Context, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Context{}, err
	}
	root := cwd
	if out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	instructions, err := loadInstructions(root, cwd)
	if err != nil {
		return Context{}, err
	}
	tree, err := buildTree(root, 700)
	if err != nil {
		return Context{}, err
	}
	return Context{Root: root, Instructions: instructions, Tree: tree}, nil
}

func loadInstructions(root, cwd string) (string, error) {
	var files []string
	if cfg, err := os.UserConfigDir(); err == nil {
		files = append(files, filepath.Join(cfg, "neko", "AGENTS.md"))
	}
	files = append(files, filepath.Join(root, "AGENTS.md"))
	rel, err := filepath.Rel(root, cwd)
	if err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		current := root
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			files = append(files, filepath.Join(current, "AGENTS.md"))
		}
	}
	var sections []string
	seen := map[string]bool{}
	for _, path := range files {
		if seen[path] {
			continue
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		name, _ := filepath.Rel(root, path)
		sections = append(sections, "## "+name+"\n"+string(data))
	}
	return strings.Join(sections, "\n\n"), nil
}

func buildTree(root string, limit int) (string, error) {
	ignored := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, ".idea": true,
		".vscode": true, "dist": true, "build": true, "target": true,
		"__pycache__": true, ".next": true, ".cache": true,
	}
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == root {
			return nil
		}
		if d.IsDir() && ignored[d.Name()] {
			return filepath.SkipDir
		}
		if len(paths) >= limit {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if d.IsDir() {
			rel += "/"
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	if len(paths) == limit {
		paths = append(paths, "... tree truncated ...")
	}
	return strings.Join(paths, "\n"), nil
}

func ResolveInside(root, requested string) (string, error) {
	if requested == "" {
		return "", errors.New("path is required")
	}
	path := requested
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes the project root")
	}
	return path, nil
}

func ReadLines(path string, offset, limit int) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if offset < 1 {
		offset = 1
	}
	if limit <= 0 || limit > 2000 {
		limit = 400
	}
	var out strings.Builder
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)
	line := 0
	written := 0
	for scanner.Scan() {
		line++
		if line < offset {
			continue
		}
		if written >= limit {
			fmt.Fprintf(&out, "... truncated after %d lines ...\n", limit)
			break
		}
		fmt.Fprintf(&out, "%6d | %s\n", line, scanner.Text())
		written++
	}
	return out.String(), scanner.Err()
}
