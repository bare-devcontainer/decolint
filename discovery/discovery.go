// Package discovery locates the configuration files inside a devcontainer directory. It determines
// what kind of directory a path is (a dev container definition, a Feature, or a Template) and
// enumerates the configuration files it contains at the locations defined by the devcontainer
// specification, each confined to the boundary it must be read through.
package discovery

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/bare-devcontainer/decolint/linter"
)

// ConfigFile is a discovered configuration file and the boundary it must be read through.
type ConfigFile struct {
	// Root is the os.Root confining access to the file: the lint root, or its .devcontainer
	// sub-root.
	Root *os.Root
	// Path is the file's location relative to Root, for reading through it.
	Path string
	Type linter.FileType
}

// VisitConfigs determines the kind of devcontainer directory root is opened on and calls fn once
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
// the remaining files from being visited must be handled inside fn. The entry's Root is only
// valid during the fn call. All file access is confined to root; files under the .devcontainer
// directory are visited with a Root confined to that directory, and a caller resolving a path
// relative to the file (e.g. a local Feature reference) must access it through the entry's Root
// so the resolution cannot escape that boundary. Symbolic links are followed only while they
// resolve inside the boundary, and a link escaping it is treated as nonexistent.
func VisitConfigs(root *os.Root, fn func(ConfigFile) error) error {
	if p := "devcontainer-feature.json"; isFile(root, p) {
		if err := fn(ConfigFile{root, p, linter.Feature}); err != nil {
			return err
		}
		return nil
	}
	if p := "devcontainer-template.json"; isFile(root, p) {
		if err := fn(ConfigFile{root, p, linter.Template}); err != nil {
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
func visitDevcontainerConfigs(root *os.Root, fn func(ConfigFile) error) error {
	if p := ".devcontainer.json"; isFile(root, p) {
		if err := fn(ConfigFile{root, p, linter.Devcontainer}); err != nil {
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
		if err := fn(ConfigFile{sub, p, linter.Devcontainer}); err != nil {
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
		if err := fn(ConfigFile{sub, p, linter.Devcontainer}); err != nil {
			return err
		}
	}
	return nil
}

func isFile(root *os.Root, path string) bool {
	info, err := root.Stat(path)
	return err == nil && !info.IsDir()
}
