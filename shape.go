package main

import (
	"regexp"
	"strings"
)

type shapeElement struct {
	Key  string // original key text, e.g. "0", "1", "path"
	Type string
}

// splitTopLevel splits s on sep, honoring nested {}, (), <> so a shape's
// own nested shapes/generics aren't split on their internal separators.
func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '(', '<':
			depth++
		case '}', ')', '>':
			if depth > 0 {
				depth--
			}
		default:
			if s[i] == sep && depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// parseArrayShape parses a type string like "array{0: string, 1: string}"
// or "list<array{0: string, 1: string}>" into its ordered elements, plus
// whether it was list<...>-wrapped. ok is false for anything else (plain
// "array", "list<string>", "Type|null", ...) -- those have no per-slot
// elements to humanize.
func parseArrayShape(t string) (elements []shapeElement, wrapped bool, ok bool) {
	inner := t
	if strings.HasPrefix(t, "list<") && strings.HasSuffix(t, ">") {
		inner = t[len("list<") : len(t)-1]
		wrapped = true
	}
	if !strings.HasPrefix(inner, "array{") || !strings.HasSuffix(inner, "}") {
		return nil, false, false
	}
	body := inner[len("array{") : len(inner)-1]

	for _, part := range splitTopLevel(body, ',') {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idx := strings.Index(part, ":")
		if idx == -1 {
			return nil, false, false
		}
		key := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(part[:idx]), "?"))
		valType := strings.TrimSpace(part[idx+1:])
		elements = append(elements, shapeElement{Key: key, Type: valType})
	}
	if len(elements) == 0 {
		return nil, false, false
	}
	return elements, wrapped, true
}

// allIntegerKeys reports whether every element's key is a bare integer --
// the signal that a shape is really a positional tuple, not a named
// struct. A string key like "path" carries real information; a numeric
// key is just a position and carries none.
func allIntegerKeys(elements []shapeElement) bool {
	for _, e := range elements {
		if e.Key == "" {
			return false
		}
		for _, c := range e.Key {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

var bracketNamesRe = regexp.MustCompile(`\[\s*([A-Za-z_][A-Za-z0-9_]*(?:\s*,\s*[A-Za-z_][A-Za-z0-9_]*)*)\s*\]`)

// extractBracketNames pulls a "[name1, name2]" hint out of free-text prose
// -- the convention this codebase's own @return/@param descriptions
// already use (e.g. DkimSigner::splitMessage() -- "[headerBlock, body]").
// Returns nil if there's no such hint.
func extractBracketNames(desc string) []string {
	m := bracketNamesRe.FindStringSubmatch(desc)
	if m == nil {
		return nil
	}
	var names []string
	for _, n := range strings.Split(m[1], ",") {
		names = append(names, strings.TrimSpace(n))
	}
	return names
}

// humanizeType renders a shaped-array type for human prose: a numeric-keyed
// shape becomes a positional tuple "[Type, Type]", naming each slot as
// "Type/name" when a name is available -- preferring the docblock
// description's "[name, name]" hint, falling back to real variable names
// from a "return [$a, $b];" statement. Named-key shapes
// (array{path: string}) are returned unchanged: the key itself is already
// the name. Anything that isn't a shape at all is also returned unchanged.
// humanizeType accepts multiple candidate description sources (e.g. the
// @return tag's own trailing text, and the docblock's prose summary) and
// tries each in turn for a "[name, name]" hint -- some docblocks put it
// right after the type, others put it in the summary instead, and nothing
// else tells us which.
func humanizeType(t string, descHints []string, varNames []string) string {
	// A nullable/union return ("array{0: string, 1: string}|null") is a
	// very common real pattern -- split on top-level "|" first so the
	// shape member still gets humanized and "null" passes through as-is.
	if members := splitTopLevel(t, '|'); len(members) > 1 {
		out := make([]string, len(members))
		for i, m := range members {
			out[i] = humanizeSingleType(strings.TrimSpace(m), descHints, varNames)
		}
		return strings.Join(out, "|")
	}
	return humanizeSingleType(t, descHints, varNames)
}

func humanizeSingleType(t string, descHints []string, varNames []string) string {
	elements, wrapped, ok := parseArrayShape(t)
	if !ok || !allIntegerKeys(elements) {
		return t
	}

	var names []string
	for _, hint := range descHints {
		if n := extractBracketNames(hint); len(n) == len(elements) {
			names = n
			break
		}
	}
	if names == nil && len(varNames) == len(elements) {
		names = varNames
	}

	// Always rebuild as "array{...}" -- the same bracket family a
	// naturally-named shape (array{code: int, text: string}) already
	// uses elsewhere in this same doc. A square-bracket "[...]" tuple was
	// a second, invented notation for what's otherwise the identical
	// kind of information; dropping it means there's exactly one style
	// for "this is a shaped array", whether its keys came from the
	// docblock directly or were resolved from a description/variable
	// name. With no name for a slot, show its bare type -- that's still
	// strictly better than a numeric "0: string" key, which carries no
	// information at all.
	parts := make([]string, len(elements))
	for i, e := range elements {
		if names != nil {
			parts[i] = names[i] + ": " + e.Type
		} else {
			parts[i] = e.Type
		}
	}

	shape := "array{" + strings.Join(parts, ", ") + "}"
	if wrapped {
		return "list<" + shape + ">"
	}
	return shape
}

var (
	wsRunRe            = regexp.MustCompile(`\s+`)
	openThenSpaceRe    = regexp.MustCompile(`([{<(])\s+`)
	spaceBeforeCloseRe = regexp.MustCompile(`\s+([}>)\]])`)
	sepThenSpaceRe     = regexp.MustCompile(`([,:])\s*`)
)

// normalizeShapeSpacing reformats a PHPDoc type string to one consistent
// style, independent of whatever spacing the original docblock happened to
// use -- "array{path:string,name?:string}", "array{ path:   string }" and
// "array{path: string}" all render identically. Applies to every type
// shown in prose, not just the ones humanizeType turns into a tuple, since
// a plain "array{...}" with named keys never goes through that path at
// all but has exactly the same raw-copy-from-docblock spacing problem.
func normalizeShapeSpacing(t string) string {
	t = wsRunRe.ReplaceAllString(t, " ")
	t = sepThenSpaceRe.ReplaceAllString(t, "$1 ")
	t = openThenSpaceRe.ReplaceAllString(t, "$1")
	t = spaceBeforeCloseRe.ReplaceAllString(t, "$1")
	return strings.TrimSpace(t)
}
