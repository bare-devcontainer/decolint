package rules

// docsBaseURL is where the rule reference is published. The pages themselves live in the
// repository's docs directory, one Markdown file per rule ID.
const docsBaseURL = "https://bare-devcontainer.github.io/decolint/rules/"

// DocsURL returns the address of the page documenting the rule with the given ID: what it checks,
// why, and configuration it accepts and rejects. The address is derived from the ID, so it is
// returned for any id, including one no built-in rule has.
func DocsURL(id string) string {
	return docsBaseURL + id + "/"
}

// DocsCategoryURL returns the address of the rule reference's listing for the named category. The
// listing is an anchor on the reference's index rather than a page of its own, so the address is
// returned for any name, including one no category has.
func DocsCategoryURL(name string) string {
	return docsBaseURL + "#" + name
}
