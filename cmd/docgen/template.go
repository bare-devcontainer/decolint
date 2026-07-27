package main

import (
	"embed"
	"fmt"
	"strings"
	"text/template"
)

// Every document docgen writes is laid out by a template under templates/, so a page's Markdown
// structure is read and edited in one place instead of across format verbs. The Go code's job is to
// build the data a template renders; the templates hold no logic beyond ranging and optional
// sections.

//go:embed templates/*.tmpl
var templateFS embed.FS

// templates holds every template in templates/, each named after its file (e.g. "rule.md.tmpl"),
// plus the named templates they define ("snippet").
var templates = template.Must(template.New("docgen").
	Funcs(template.FuncMap{"yamlQuote": yamlSingleQuoted}).
	ParseFS(templateFS, "templates/*.tmpl"))

// mustRender executes the template named name against data, panicking if it fails. Every template
// is embedded and every value handed to one is built in this package, so a failure is a bug in this
// generator rather than something a run can hit — the same reasoning [template.Must] applies to
// parsing.
func mustRender(name string, data any) string {
	var b strings.Builder
	if err := templates.ExecuteTemplate(&b, name, data); err != nil {
		panic(fmt.Sprintf("docgen: render %s: %v", name, err))
	}
	return b.String()
}
