package main

import (
	"fmt"
	"strings"
	"unicode"
)

// githubSlug replicates github-slugger's slug() function: lowercase, strip
// punctuation (keeping letters, digits, "_", "-"), turn spaces into "-".
// Our heading text is always either a bare PHP identifier (letters/digits/
// underscore -- never a backslash, since headings show the bare name, not
// the FQCN) wrapped in backticks we add ourselves, or that plus "()" for a
// method -- so stripping everything but letters/digits/underscore/hyphen
// matches GitHub's real algorithm for every heading we actually emit,
// without needing its full Unicode punctuation table.
func githubSlug(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == ' ':
			b.WriteByte('-')
		case r == '-' || r == '_':
			b.WriteRune(r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		}
	}
	return b.String()
}

// GithubSlugger assigns the same slug GitHub will assign to a heading with
// this text, tracking occurrences across the whole page so a repeated
// heading (e.g. "__construct()" in ten different classes) gets the same
// "-1", "-2", ... suffix GitHub's own de-duper would produce. This only
// matches reality if headings are fed to it in the exact order they're
// emitted in the document: the counter is global and sequential across the
// whole page, not per-section.
type GithubSlugger struct {
	occurrences map[string]int
}

func NewGithubSlugger() *GithubSlugger {
	return &GithubSlugger{occurrences: map[string]int{}}
}

func (g *GithubSlugger) Slug(headingText string) string {
	base := githubSlug(headingText)
	result := base
	for {
		if _, exists := g.occurrences[result]; !exists {
			break
		}
		g.occurrences[base]++
		result = fmt.Sprintf("%s-%d", base, g.occurrences[base])
	}
	g.occurrences[result] = 0
	return result
}
