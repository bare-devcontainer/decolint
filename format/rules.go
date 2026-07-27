package format

// RuleDoc is the full documentation of one built-in rule: the wire shape of
// "decolint -rules -format=json", for tooling outside this module that wants the rule catalog
// without importing the rules package. cmd/docgen, which builds the documentation site, imports
// rules directly instead of going through this JSON, so the two have no relationship to keep in
// sync — a change to one has no bearing on the other.
type RuleDoc struct {
	ID              string      `json:"id"`
	Description     string      `json:"description"`
	LongDescription string      `json:"longDescription"`
	References      []string    `json:"references"`
	Category        string      `json:"category"`
	Platforms       []string    `json:"platforms"`
	FileTypes       []string    `json:"fileTypes"`
	Example         RuleExample `json:"example"`
	// DocsURL is where the rule's page is published (see rules.DocsURL).
	DocsURL string `json:"docsUrl"`
	// Severity is the severity the rule is currently registered at: its category's default, unless
	// overridden by the config file in effect.
	Severity string `json:"severity"`
}

// RuleExample is a [linter.Example] adapted for JSON.
type RuleExample struct {
	Bad  RuleSnippet `json:"bad"`
	Good RuleSnippet `json:"good"`
	Note string      `json:"note,omitzero"`
}

// RuleSnippet is a [linter.Snippet] adapted for JSON.
type RuleSnippet struct {
	Files   []RuleExampleFile `json:"files"`
	DirName string            `json:"dirName,omitzero"`
}

// RuleExampleFile is a [linter.ExampleFile] adapted for JSON.
type RuleExampleFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	// Mode is the file's POSIX permission bits (e.g. 420 for 0644), 0 meaning the default.
	Mode uint32 `json:"mode,omitzero"`
}
