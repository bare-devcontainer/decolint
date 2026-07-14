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

// configEntry is a discovered configuration file and the boundary it must be read through: root is
// the os.Root confining access (the lint root, or its .devcontainer sub-root), and path is relative
// to that root. rel is the path relative to the lint directory, for display.
type configEntry struct {
	root *os.Root
	path string
	rel  string
	typ  FileType
}

// Transform mutates a parsed configuration file before rules run, e.g. to merge Feature-contributed
// properties into the effective configuration. It may modify ctx.Root in place; ctx.Src is left
// untouched, so any node a Transform adds must carry offsets pointing into the original source. An
// error aborts the lint of that file.
type Transform func(ctx context.Context, fctx *Context) error

// Linter runs a set of rules against devcontainer configuration files.
type Linter struct {
	patterns map[FileType][]pattern
	// severities holds the effective severity of each rule, keyed by rule ID, as specified when the
	// rule was registered via RegisterRule.
	severities map[string]Severity
	// transform, if set, mutates each parsed file before rules run. See SetTransform.
	transform Transform
}

// New returns an empty Linter. Use RegisterRule to add rules to it.
func New() *Linter {
	return &Linter{patterns: map[FileType][]pattern{}, severities: map[string]Severity{}}
}

// SetTransform installs t to run on each parsed file before rules are applied. Only one transform
// can be installed; a later call replaces the previous one.
func (l *Linter) SetTransform(t Transform) {
	l.transform = t
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

// LintDir determines the kind of devcontainer directory root is opened on (a dev container
// definition, a Feature, or a Template), and lints every configuration file it contains. It is an
// error if the directory contains no configuration. All file access happens through root, so it is
// confined to that directory; configuration files under its .devcontainer directory are only
// accessed within that directory. Symbolic links are followed only while they resolve inside that
// boundary, and a link escaping it is treated as nonexistent. Issue paths are the files' locations
// joined onto root's name.
func (l *Linter) LintDir(ctx context.Context, root *os.Root) ([]Issue, error) {
	dir := root.Name()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("aborted %s: %w", dir, err)
	}
	var issues []Issue
	var errs []error
	found := false
	err := visitConfigs(root, func(f configEntry) error {
		found = true
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("aborted %s: %w", filepath.Join(dir, f.rel), err)
		}
		fileIssues, err := l.lintConfig(ctx, dir, f)
		if err != nil {
			// A broken file must not stop the remaining files from being linted, so record the
			// error and keep visiting.
			errs = append(errs, err)
			return nil
		}
		issues = append(issues, fileIssues...)
		return nil
	})
	if err != nil {
		return issues, errors.Join(append(errs, err)...)
	}
	if !found {
		return nil, fmt.Errorf("no devcontainer configuration found in %s", dir)
	}
	return issues, errors.Join(errs...)
}

// lintConfig reads and lints the single configuration file f, reporting issues under
// filepath.Join(dir, f.rel). The file is read through f.root, so its resolution cannot escape that
// boundary.
func (l *Linter) lintConfig(ctx context.Context, dir string, f configEntry) ([]Issue, error) {
	display := filepath.Join(dir, f.rel)
	src, err := f.root.ReadFile(f.path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", display, err)
	}
	return l.Lint(ctx, display, src, f.typ)
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
	if l.transform != nil {
		if err := l.transform(ctx, rctx); err != nil {
			return nil, fmt.Errorf("transform %s: %w", path, err)
		}
	}
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

// visitConfigs determines the kind of devcontainer directory root is opened on and calls fn once
// for each configuration file it contains:
//
//   - a Feature (dir contains devcontainer-feature.json): that file;
//   - a Template (dir contains devcontainer-template.json): that file, plus the dev container
//     configuration the template ships;
//   - otherwise, a dev container definition: the configuration files at the locations defined by
//     the devcontainer specification: .devcontainer.json, .devcontainer/devcontainer.json, and
//     .devcontainer/<folder>/devcontainer.json (one level deep), in that order.
//
// fn never being called means the directory contains no devcontainer configuration. A non-nil
// error from fn aborts the visit and is returned as is; a per-file problem that should not stop
// the remaining files from being visited must be handled inside fn. The entry's root is only
// valid during the fn call. Everything under the .devcontainer directory is accessed through a
// root confined to that directory: the future Feature/dependsOn resolver receives the same
// boundary, so local Feature references — including Features stored inside the active
// .devcontainer directory — resolve within it.
func visitConfigs(root *os.Root, fn func(configEntry) error) error {
	if p := "devcontainer-feature.json"; isFile(root, p) {
		if err := fn(configEntry{root, p, p, Feature}); err != nil {
			return err
		}
		return nil
	}
	if p := "devcontainer-template.json"; isFile(root, p) {
		if err := fn(configEntry{root, p, p, Template}); err != nil {
			return err
		}
	}
	return visitDevcontainerConfigs(root, fn)
}

// devcontainerDir is the directory that holds a dev container definition's configuration, and the
// boundary that access to that configuration is confined to.
const devcontainerDir = ".devcontainer"

// visitDevcontainerConfigs calls fn for each devcontainer.json under root at the locations defined
// by the devcontainer specification. Files inside the .devcontainer directory are visited with a
// root confined to that directory, opened once for the whole visit.
func visitDevcontainerConfigs(root *os.Root, fn func(configEntry) error) error {
	if p := ".devcontainer.json"; isFile(root, p) {
		if err := fn(configEntry{root, p, p, Devcontainer}); err != nil {
			return err
		}
	}
	sub, err := root.OpenRoot(devcontainerDir)
	if err != nil {
		return nil
	}
	// The root is only read from, so a close error is inconsequential.
	defer func() { _ = sub.Close() }()
	if p := "devcontainer.json"; isFile(sub, p) {
		if err := fn(configEntry{sub, p, filepath.Join(devcontainerDir, p), Devcontainer}); err != nil {
			return err
		}
	}
	entries, err := fs.ReadDir(sub.FS(), ".")
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p := filepath.Join(e.Name(), "devcontainer.json")
		if !isFile(sub, p) {
			continue
		}
		if err := fn(configEntry{sub, p, filepath.Join(devcontainerDir, p), Devcontainer}); err != nil {
			return err
		}
	}
	return nil
}

func isFile(root *os.Root, path string) bool {
	info, err := root.Stat(path)
	return err == nil && !info.IsDir()
}
