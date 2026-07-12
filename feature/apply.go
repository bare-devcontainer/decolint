package feature

import (
	"strings"

	"github.com/tailscale/hujson"
)

// lifecycleHooks are the lifecycle script properties a Feature can contribute.
var lifecycleHooks = []string{
	"onCreateCommand",
	"updateContentCommand",
	"postCreateCommand",
	"postStartCommand",
	"postAttachCommand",
}

// mergeState applies contributors to the root object of a devcontainer.json one by one, in
// installation order, and tracks which values originate from the user's file so that those always
// win, as the user's configuration is applied last per the specification's merge logic.
type mergeState struct {
	root *hujson.Object
	// userEnvKeys are the containerEnv keys the user's file defines.
	userEnvKeys map[string]bool
	// userMountTargets are the mount targets the user's file defines.
	userMountTargets map[string]bool
	// customizations accumulates the contributors' customizations, detached from the tree; finish
	// merges the user's own customizations on top and grafts the result.
	customizations       *hujson.Value
	customizationsAnchor int
	// lifecycle accumulates contributed lifecycle hook commands, keyed by hook name.
	lifecycle map[string][]lifecycleEntry
}

type lifecycleEntry struct {
	id     string
	anchor int
	value  hujson.Value
}

func newMergeState(root *hujson.Object) *mergeState {
	s := &mergeState{
		root:             root,
		userEnvKeys:      map[string]bool{},
		userMountTargets: map[string]bool{},
		lifecycle:        map[string][]lifecycleEntry{},
	}
	if i := findMember(root, "containerEnv"); i >= 0 {
		if obj, ok := root.Members[i].Value.Value.(*hujson.Object); ok {
			for _, m := range obj.Members {
				if lit, ok := m.Name.Value.(hujson.Literal); ok && lit.Kind() == '"' {
					s.userEnvKeys[lit.String()] = true
				}
			}
		}
	}
	if i := findMember(root, "mounts"); i >= 0 {
		if arr, ok := root.Members[i].Value.Value.(*hujson.Array); ok {
			for j := range arr.Elements {
				if target := mountTarget(&arr.Elements[j]); target != "" {
					s.userMountTargets[target] = true
				}
			}
		}
	}
	return s
}

// apply merges the properties contributed by c into the tree.
func (s *mergeState) apply(c *contributor) {
	s.mergeBool(c, "init")
	s.mergeBool(c, "privileged")
	s.mergeUnion(c, "capAdd")
	s.mergeUnion(c, "securityOpt")
	s.mergeEnv(c)
	s.mergeMounts(c)
	s.collectCustomizations(c)
	s.collectLifecycle(c)
}

// finish grafts the accumulated customizations and lifecycle hooks into the tree.
func (s *mergeState) finish() {
	s.finishCustomizations()
	s.finishLifecycle()
}

// mergeBool applies a boolean-OR property (init, privileged): the effective value is true if any
// contributor or the user sets it to true.
func (s *mergeState) mergeBool(c *contributor, name string) {
	v := c.md.Root.Find("/" + name)
	if v == nil {
		return
	}
	lit, ok := v.Value.(hujson.Literal)
	if !ok || !lit.Bool() {
		return
	}
	i := findMember(s.root, name)
	if i < 0 {
		appendMember(s.root, name, anchoredValue(hujson.Bool(true), c.anchor), c.anchor)
		return
	}
	// Only overwrite an explicit false or null; leave a malformed value for correctness rules to
	// report.
	if existing, ok := s.root.Members[i].Value.Value.(hujson.Literal); ok && (existing.Kind() == 'f' || existing.Kind() == 'n') {
		s.root.Members[i].Value = anchoredValue(hujson.Bool(true), c.anchor)
	}
}

// mergeUnion applies a union-of-arrays property (capAdd, securityOpt): the effective array is the
// deduplicated union of every contributor's and the user's entries.
func (s *mergeState) mergeUnion(c *contributor, name string) {
	v := c.md.Root.Find("/" + name)
	if v == nil {
		return
	}
	src, ok := v.Value.(*hujson.Array)
	if !ok {
		return
	}
	i := findMember(s.root, name)
	if i < 0 {
		i = appendMember(s.root, name, anchoredValue(&hujson.Array{}, c.anchor), c.anchor)
	}
	dst, ok := s.root.Members[i].Value.Value.(*hujson.Array)
	if !ok {
		return
	}
	for j := range src.Elements {
		lit, ok := src.Elements[j].Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			continue
		}
		if arrayContainsString(dst, lit.String()) {
			continue
		}
		dst.Elements = append(dst.Elements, anchored(src.Elements[j], c.anchor))
	}
}

// mergeEnv applies containerEnv: an object merge where a later contributor overrides an earlier
// one and the user's own entries always win.
func (s *mergeState) mergeEnv(c *contributor) {
	v := c.md.Root.Find("/containerEnv")
	if v == nil {
		return
	}
	src, ok := v.Value.(*hujson.Object)
	if !ok {
		return
	}
	i := findMember(s.root, "containerEnv")
	if i < 0 {
		i = appendMember(s.root, "containerEnv", anchoredValue(&hujson.Object{}, c.anchor), c.anchor)
	}
	dst, ok := s.root.Members[i].Value.Value.(*hujson.Object)
	if !ok {
		return
	}
	for _, m := range src.Members {
		lit, ok := m.Name.Value.(hujson.Literal)
		if !ok || lit.Kind() != '"' {
			continue
		}
		key := lit.String()
		if s.userEnvKeys[key] {
			continue
		}
		if j := findMember(dst, key); j >= 0 {
			dst.Members[j].Value = anchored(m.Value, c.anchor)
		} else {
			appendMember(dst, key, anchored(m.Value, c.anchor), c.anchor)
		}
	}
}

// mergeMounts applies mounts: entries are collected across contributors, deduplicated by mount
// target with the last contributor winning; a target the user's file mounts always wins.
func (s *mergeState) mergeMounts(c *contributor) {
	v := c.md.Root.Find("/mounts")
	if v == nil {
		return
	}
	src, ok := v.Value.(*hujson.Array)
	if !ok {
		return
	}
	i := findMember(s.root, "mounts")
	if i < 0 {
		i = appendMember(s.root, "mounts", anchoredValue(&hujson.Array{}, c.anchor), c.anchor)
	}
	dst, ok := s.root.Members[i].Value.Value.(*hujson.Array)
	if !ok {
		return
	}
	for j := range src.Elements {
		target := mountTarget(&src.Elements[j])
		if target != "" && s.userMountTargets[target] {
			continue
		}
		merged := anchored(src.Elements[j], c.anchor)
		replaced := false
		if target != "" {
			for k := range dst.Elements {
				if mountTarget(&dst.Elements[k]) == target {
					dst.Elements[k] = merged
					replaced = true
					break
				}
			}
		}
		if !replaced {
			dst.Elements = append(dst.Elements, merged)
		}
	}
}

// collectCustomizations deep-merges c's customizations into the detached accumulator: objects
// merge member-wise, arrays concatenate, and a later contributor wins scalar conflicts.
func (s *mergeState) collectCustomizations(c *contributor) {
	v := c.md.Root.Find("/customizations")
	if v == nil {
		return
	}
	if _, ok := v.Value.(*hujson.Object); !ok {
		return
	}
	av := anchored(*v, c.anchor)
	if s.customizations == nil {
		s.customizations = &av
		s.customizationsAnchor = c.anchor
		return
	}
	deepMerge(s.customizations, av)
}

// finishCustomizations merges the user's own customizations on top of the accumulated ones,
// reusing the user's original nodes so their positions survive, and grafts the result.
func (s *mergeState) finishCustomizations() {
	if s.customizations == nil {
		return
	}
	if i := findMember(s.root, "customizations"); i >= 0 {
		merged := *s.customizations
		deepMerge(&merged, s.root.Members[i].Value)
		s.root.Members[i].Value = merged
	} else {
		appendMember(s.root, "customizations", *s.customizations, s.customizationsAnchor)
	}
}

// deepMerge merges src into dst: objects merge member-wise recursively, arrays append the src
// elements missing from dst, and src wins any other conflict. src subtrees are attached as-is, so
// they must already carry the offsets they should report.
func deepMerge(dst *hujson.Value, src hujson.Value) {
	if dstObj, ok := dst.Value.(*hujson.Object); ok {
		if srcObj, ok := src.Value.(*hujson.Object); ok {
			for _, m := range srcObj.Members {
				lit, ok := m.Name.Value.(hujson.Literal)
				if !ok || lit.Kind() != '"' {
					continue
				}
				if i := findMember(dstObj, lit.String()); i >= 0 {
					deepMerge(&dstObj.Members[i].Value, m.Value)
				} else {
					dstObj.Members = append(dstObj.Members, m)
				}
			}
			return
		}
	}
	if dstArr, ok := dst.Value.(*hujson.Array); ok {
		if srcArr, ok := src.Value.(*hujson.Array); ok {
			for _, e := range srcArr.Elements {
				if lit, ok := e.Value.(hujson.Literal); ok && lit.Kind() == '"' && arrayContainsString(dstArr, lit.String()) {
					continue
				}
				dstArr.Elements = append(dstArr.Elements, e)
			}
			return
		}
	}
	*dst = src
}

// collectLifecycle records the lifecycle hook commands c contributes.
func (s *mergeState) collectLifecycle(c *contributor) {
	for _, hook := range lifecycleHooks {
		v := c.md.Root.Find("/" + hook)
		if v == nil {
			continue
		}
		s.lifecycle[hook] = append(s.lifecycle[hook], lifecycleEntry{
			id:     c.displayID(),
			anchor: c.anchor,
			value:  anchored(*v, c.anchor),
		})
	}
}

// finishLifecycle rewrites each lifecycle hook at least one Feature contributes to into the
// specification's object form, with one member per contributed command keyed by the Feature's ID.
// The user's own command is preserved: an object-form value contributes its members verbatim (they
// win name conflicts), any other form is kept under the "devcontainer.json" key.
func (s *mergeState) finishLifecycle() {
	for _, hook := range lifecycleHooks {
		entries := s.lifecycle[hook]
		if len(entries) == 0 {
			continue
		}
		merged := &hujson.Object{}
		for _, e := range entries {
			if i := findMember(merged, e.id); i >= 0 {
				merged.Members[i].Value = e.value
			} else {
				appendMember(merged, e.id, e.value, e.anchor)
			}
		}

		i := findMember(s.root, hook)
		if i < 0 {
			appendMember(s.root, hook, hujson.Value{
				Value:       merged,
				StartOffset: entries[0].anchor,
				EndOffset:   entries[0].anchor,
			}, entries[0].anchor)
			continue
		}
		user := s.root.Members[i].Value
		if userObj, ok := user.Value.(*hujson.Object); ok {
			for _, m := range userObj.Members {
				lit, ok := m.Name.Value.(hujson.Literal)
				if !ok || lit.Kind() != '"' {
					continue
				}
				if j := findMember(merged, lit.String()); j >= 0 {
					merged.Members[j] = m
				} else {
					merged.Members = append(merged.Members, m)
				}
			}
		} else {
			merged.Members = append(merged.Members, hujson.ObjectMember{
				Name:  anchoredValue(hujson.String("devcontainer.json"), user.StartOffset),
				Value: user,
			})
		}
		s.root.Members[i].Value = hujson.Value{
			Value:       merged,
			StartOffset: user.StartOffset,
			EndOffset:   user.EndOffset,
		}
	}
}

// findMember returns the index of obj's member named name, or -1.
func findMember(obj *hujson.Object, name string) int {
	for i := range obj.Members {
		if lit, ok := obj.Members[i].Name.Value.(hujson.Literal); ok && lit.Kind() == '"' && lit.String() == name {
			return i
		}
	}
	return -1
}

// appendMember appends a member to obj with a synthesized name carrying the anchor offset, and
// returns its index.
func appendMember(obj *hujson.Object, name string, value hujson.Value, anchor int) int {
	obj.Members = append(obj.Members, hujson.ObjectMember{
		Name:  anchoredValue(hujson.String(name), anchor),
		Value: value,
	})
	return len(obj.Members) - 1
}

// anchoredValue wraps a constructed value with offsets pointing at anchor.
func anchoredValue(v hujson.ValueTrimmed, anchor int) hujson.Value {
	return hujson.Value{Value: v, StartOffset: anchor, EndOffset: anchor}
}

// anchored returns a deep copy of v with every offset in the subtree set to anchor and all
// comments stripped, so findings on the copy resolve to the anchor position in the original file.
func anchored(v hujson.Value, anchor int) hujson.Value {
	c := v.Clone()
	for n := range c.All() {
		n.StartOffset, n.EndOffset = anchor, anchor
		n.BeforeExtra, n.AfterExtra = nil, nil
		switch t := n.Value.(type) {
		case *hujson.Object:
			t.AfterExtra = nil
		case *hujson.Array:
			t.AfterExtra = nil
		}
	}
	return c
}

// arrayContainsString reports whether arr has a string element equal to s.
func arrayContainsString(arr *hujson.Array, s string) bool {
	for i := range arr.Elements {
		if lit, ok := arr.Elements[i].Value.(hujson.Literal); ok && lit.Kind() == '"' && lit.String() == s {
			return true
		}
	}
	return false
}

// mountTarget extracts the mount target path from a "mounts" entry, which is either a
// "key=value,..." string or an object with "target"/"dst" members. It returns "" if v is neither
// or declares no target.
func mountTarget(v *hujson.Value) string {
	switch val := v.Value.(type) {
	case hujson.Literal:
		if val.Kind() != '"' {
			return ""
		}
		for _, part := range strings.Split(val.String(), ",") {
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			switch strings.TrimSpace(key) {
			case "target", "dst", "destination":
				return strings.TrimSpace(value)
			}
		}
	case *hujson.Object:
		for _, m := range val.Members {
			name, ok := m.Name.Value.(hujson.Literal)
			if !ok {
				continue
			}
			value, ok := m.Value.Value.(hujson.Literal)
			if !ok || value.Kind() != '"' {
				continue
			}
			switch name.String() {
			case "target", "dst", "destination":
				return value.String()
			}
		}
	}
	return ""
}
