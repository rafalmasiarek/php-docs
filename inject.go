package main

import (
	"fmt"
	"strings"
)

const (
	beginMarker = "<!-- BEGIN_PHP_DOCS -->"
	endMarker   = "<!-- END_PHP_DOCS -->"
)

// defaultTemplate is what a target file gets seeded with when --output-mode
// inject points at a file that doesn't exist yet -- mirrors terraform-docs'
// own default template (BEGIN_TF_DOCS/END_TF_DOCS) for the same situation.
const defaultTemplate = beginMarker + "\n" + endMarker + "\n"

// injectBetweenMarkers replaces only the content between beginMarker and
// endMarker in an existing document, leaving everything before and after
// untouched -- the point of this mode is updating one fragment of a
// hand-written README (intro, install instructions, badges) without
// clobbering the rest of it, unlike --output-mode replace.
func injectBetweenMarkers(existing, content string) (string, error) {
	beginIdx := strings.Index(existing, beginMarker)
	endIdx := strings.Index(existing, endMarker)
	if beginIdx == -1 || endIdx == -1 {
		return "", fmt.Errorf("markers %s / %s not found in target file", beginMarker, endMarker)
	}
	if endIdx < beginIdx {
		return "", fmt.Errorf("%s appears before %s in target file", endMarker, beginMarker)
	}

	before := existing[:beginIdx+len(beginMarker)]
	after := existing[endIdx:]
	return before + "\n\n" + strings.TrimRight(content, "\n") + "\n\n" + after, nil
}
