// Package linter implements the decolint engine: it determines what kind of devcontainer
// directory a path is (a dev container definition, a Feature, or a Template), locates the
// configuration files it contains, parses them as HuJSON (JSONC), runs lint rules against the
// syntax tree, and filters findings suppressed by ignore comments.
package linter

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/tailscale/hujson"
)

// Issue is a rule finding resolved to a file position.
type Issue struct {
	Path     string   `json:"path"`
	Line     int      `json:"line"` // 1-based
	Col      int      `json:"col"`  // 1-based, in bytes
	RuleID   string   `json:"ruleId"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

func (i Issue) String() string {
	return fmt.Sprintf("%s:%d:%d: %s: %s (%s)", i.Path, i.Line, i.Col, i.Severity, i.Message, i.RuleID)
}

// ConfigFile is a configuration file detected in a directory.
type ConfigFile struct {
	Path string
	Type FileType
}

// Linter runs a set of rules against devcontainer configuration files.
type Linter struct {
	patterns map[FileType][]pattern
	// severities holds the effective severity of each rule, keyed by rule ID, as specified when the
	// rule was registered via RegisterRule.
	severities map[string]Severity
}

// New returns an empty Linter. Use RegisterRule to add rules to it.
func New() *Linter {
	return &Linter{patterns: map[FileType][]pattern{}, severities: map[string]Severity{}}
}

// RegisterRule adds r to the linter, to run at the given severity.
func (l *Linter) RegisterRule(r *Rule, severity Severity) {
	l.severities[r.ID] = severity
	if severity == SeverityOff {
		return
	}
	for _, t := range r.FileTypes {
		l.patterns[t] = append(l.patterns[t], compilePatterns(r)...)
	}
}

// LintDir determines the kind of devcontainer directory dir is (a dev container definition, a
// Feature, or a Template), and lints every configuration file it contains. It is an error if dir is
// not a directory or contains no configuration. Configuration files under dir's .devcontainer
// directory are only accessed within that directory, and other configuration files only within dir:
// symbolic links are followed only while they resolve inside that boundary, and a link escaping it
// is treated as nonexistent.
func (l *Linter) LintDir(ctx context.Context, dir string) ([]Issue, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("aborted %s: %w", dir, err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("not a directory: %s", dir)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve directory: %w", err)
	}
	// The root is only read from, so a close error is inconsequential.
	defer func() { _ = root.Close() }()
	files := findConfigs(root)
	if len(files) == 0 {
		return nil, fmt.Errorf("no devcontainer configuration found in %s", dir)
	}
	var issues []Issue
	var errs []error
	for _, f := range files {
		display := filepath.Join(dir, f.Path)
		if err := ctx.Err(); err != nil {
			return issues, errors.Join(append(errs, fmt.Errorf("aborted %s: %w", display, err))...)
		}
		src, err := readConfig(root, f.Path)
		if err != nil {
			errs = append(errs, fmt.Errorf("read config %s: %w", display, err))
			continue
		}
		fileIssues, err := l.Lint(ctx, display, src, f.Type)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		issues = append(issues, fileIssues...)
	}
	return issues, errors.Join(errs...)
}

// Lint lints src, which is the content of a configuration file of the given type. path is used only
// for reporting.
func (l *Linter) Lint(ctx context.Context, path string, src []byte, fileType FileType) ([]Issue, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("aborted %s: %w", path, err)
	}
	root, err := hujson.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	patterns := l.patterns[fileType]
	if len(patterns) == 0 {
		return nil, nil
	}
	rctx := &Context{Path: path, Type: fileType, Src: src, Root: &root}
	pos := newPositions(src)
	ignores := buildIgnoreIndex(&root, pos)

	var issues []Issue
	walk(&root, "", nil, patterns, func(r *Rule, node *Node) {
		id := r.ID
		severity := l.severities[id]
		for _, f := range safeCheck(r, rctx, node) {
			line, col := pos.lineCol(f.Offset)
			if ignores.ignores(line, id) {
				continue
			}
			issues = append(issues, Issue{
				Path:     path,
				Line:     line,
				Col:      col,
				RuleID:   id,
				Message:  f.Message,
				Severity: severity,
			})
		}
	})
	sort.Slice(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Col != b.Col {
			return a.Col < b.Col
		}
		return a.RuleID < b.RuleID
	})
	return issues, nil
}

// safeCheck calls r.Check and recovers from any panic, so that a defect in one rule (e.g. a nil
// dereference on an unexpected value shape) is reported as that rule's finding instead of aborting
// the rest of the lint run.
func safeCheck(r *Rule, rctx *Context, node *Node) (findings []Finding) {
	defer func() {
		if rec := recover(); rec != nil {
			findings = []Finding{{
				Message: fmt.Sprintf("rule panicked: %v", rec),
				Offset:  node.Value.StartOffset,
			}}
		}
	}()
	return r.Check(rctx, node)
}

// findConfigs determines the kind of devcontainer directory root is opened on and returns the
// configuration files it contains, as root-relative paths:
//
//   - a Feature (dir contains devcontainer-feature.json): that file;
//   - a Template (dir contains devcontainer-template.json): that file, plus the dev container
//     configuration the template ships;
//   - otherwise, a dev container definition: the configuration files at the locations defined by
//     the devcontainer specification: .devcontainer/devcontainer.json, .devcontainer.json, and
//     .devcontainer/<folder>/devcontainer.json (one level deep).
//
// An empty result means the directory contains no devcontainer configuration. Everything under the
// .devcontainer directory is accessed through a root confined to that directory: the future
// Feature/dependsOn resolver receives the same boundary, so local Feature references — including
// Features stored inside the active .devcontainer directory — resolve within it.
func findConfigs(root *os.Root) []ConfigFile {
	if p := "devcontainer-feature.json"; isFile(root, p) {
		return []ConfigFile{{Path: p, Type: Feature}}
	}
	if p := "devcontainer-template.json"; isFile(root, p) {
		files := []ConfigFile{{Path: p, Type: Template}}
		return append(files, devcontainerConfigs(root)...)
	}
	return devcontainerConfigs(root)
}

// devcontainerDir is the directory that holds a dev container definition's configuration, and the
// boundary that access to that configuration is confined to.
const devcontainerDir = ".devcontainer"

// devcontainerConfigs returns the devcontainer.json files under root at the locations defined by
// the devcontainer specification, as root-relative paths.
func devcontainerConfigs(root *os.Root) []ConfigFile {
	var paths []string
	if isFile(root, ".devcontainer.json") {
		paths = append(paths, ".devcontainer.json")
	}
	if sub, err := root.OpenRoot(devcontainerDir); err == nil {
		// The root is only read from, so a close error is inconsequential.
		defer func() { _ = sub.Close() }()
		if isFile(sub, "devcontainer.json") {
			paths = append(paths, filepath.Join(devcontainerDir, "devcontainer.json"))
		}
		entries, err := fs.ReadDir(sub.FS(), ".")
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				if isFile(sub, filepath.Join(e.Name(), "devcontainer.json")) {
					paths = append(paths, filepath.Join(devcontainerDir, e.Name(), "devcontainer.json"))
				}
			}
		}
	}
	sort.Strings(paths)
	files := make([]ConfigFile, 0, len(paths))
	for _, p := range paths {
		files = append(files, ConfigFile{Path: p, Type: Devcontainer})
	}
	return files
}

// readConfig reads the configuration file at name, which is relative to root. A file under the
// .devcontainer directory is accessed through a root confined to that directory, so its resolution
// cannot reach files outside of it.
func readConfig(root *os.Root, name string) ([]byte, error) {
	rest, ok := strings.CutPrefix(name, devcontainerDir+string(filepath.Separator))
	if !ok {
		return root.ReadFile(name)
	}
	sub, err := root.OpenRoot(devcontainerDir)
	if err != nil {
		return nil, err
	}
	// The root is only read from, so a close error is inconsequential.
	defer func() { _ = sub.Close() }()
	return sub.ReadFile(rest)
}

func isFile(root *os.Root, path string) bool {
	info, err := root.Stat(path)
	return err == nil && !info.IsDir()
}
