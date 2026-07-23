package rules

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"slices"
)

// templateOptionPattern matches a ${templateOption:name} reference, allowing surrounding whitespace
// around the name. It mirrors the reference implementation's substitution pattern (devcontainers/cli,
// src/spec-configuration/containerTemplatesOCI.ts).
var templateOptionPattern = regexp.MustCompile(`\$\{templateOption:\s*(\w+?)\s*\}`)

// templateOptionExcludedFiles are the root-level files the reference implementation never
// substitutes template options into when applying a template, so a reference in one of them does not
// count as a use.
var templateOptionExcludedFiles = map[string]bool{
	"devcontainer-template.json": true,
	"README.md":                  true,
	"NOTES.md":                   true,
}

// maxTemplateFileBytes caps a template file scanned for option references, mirroring the Feature
// pipeline's per-file cap. Files larger than this are skipped.
const maxTemplateFileBytes = 4 << 20 // 4 MB

// templateOptionRefs walks dir and returns, for each option name, the sorted relative slash-paths of
// the files that reference it as ${templateOption:name}. It mirrors how the reference implementation
// applies a template: every file is scanned except the root-level files in
// templateOptionExcludedFiles (a same-named file in a subdirectory still counts). It also skips
// ".git" directories, non-regular files, and files larger than maxTemplateFileBytes.
//
// Any walk or read error is returned so callers can report nothing rather than guess from a partial
// scan.
func templateOptionRefs(dir fs.FS) (map[string][]string, error) {
	refs := map[string]map[string]bool{}
	err := fs.WalkDir(dir, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" && p != "." {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		if base := path.Base(p); path.Dir(p) == "." && templateOptionExcludedFiles[base] {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", p, err)
		}
		if info.Size() > maxTemplateFileBytes {
			return nil
		}
		data, err := fs.ReadFile(dir, p)
		if err != nil {
			return fmt.Errorf("read %s: %w", p, err)
		}
		for _, m := range templateOptionPattern.FindAllSubmatch(data, -1) {
			name := string(m[1])
			if refs[name] == nil {
				refs[name] = map[string]bool{}
			}
			refs[name][p] = true
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk template directory: %w", err)
	}

	out := make(map[string][]string, len(refs))
	for name, files := range refs {
		paths := make([]string, 0, len(files))
		for f := range files {
			paths = append(paths, f)
		}
		slices.Sort(paths)
		out[name] = paths
	}
	return out, nil
}
