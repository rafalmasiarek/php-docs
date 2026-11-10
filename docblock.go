package main

import "strings"

type ParamDoc struct {
	Name        string
	Type        string
	Description string
}

type ThrowsDoc struct {
	Type        string
	Description string
}

// MagicProperty is a virtual property declared only in a class docblock
// (@property/@property-read/@property-write) via __get/__set, with no
// matching real AST property declaration to attach to.
type MagicProperty struct {
	Mode        string // "read-write", "read-only", "write-only"
	Name        string
	Type        string
	Description string
}

// MagicMethod is a virtual method declared only in a class docblock
// (@method) via __call/__callStatic, with no matching real AST method.
type MagicMethod struct {
	Static      bool
	ReturnType  string
	Name        string
	Params      []ParamDoc
	Description string
}

type DocBlock struct {
	Summary        string
	Params         []ParamDoc
	ReturnType     string
	ReturnDesc     string
	Throws         []ThrowsDoc
	Since          string
	Deprecated     string
	VarType        string
	VarDesc        string
	Package        string
	Subpackage     string
	Category       string
	See            []string
	Link           []string
	Uses           []string
	Todo           []string
	Example        []string
	Internal       bool
	InternalDesc   string
	Api            bool
	Author         string
	Copyright      string
	License        string
	Version        string
	Ignore         bool
	IgnoreDesc     string
	Final          bool
	FinalDesc      string
	FileSource     bool
	Source         string
	MagicProperties []MagicProperty
	MagicMethods    []MagicMethod
}

// stripLeadingVarName removes a leading "$name" token from an @var tag's
// remainder, per the phpDocumentor grammar "@var [Type] [$name] [description]"
// -- the name is optional and conventionally omitted for a single property.
func stripLeadingVarName(s string) string {
	if !strings.HasPrefix(s, "$") {
		return s
	}
	parts := strings.SplitN(s, " ", 2)
	if len(parts) > 1 {
		return strings.TrimLeft(parts[1], " \t")
	}
	return ""
}

// splitTypeAndRest pulls a leading type expression off a PHPDoc tag's
// remaining text, honoring nesting so shaped types like
// "array{0: string, 1: string}" aren't cut at their internal spaces.
func splitTypeAndRest(s string) (typ string, rest string) {
	s = strings.TrimLeft(s, " \t")
	depth := 0
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '{', '(', '<':
			depth++
		case '}', ')', '>':
			if depth > 0 {
				depth--
			}
		case ' ', '\t':
			if depth == 0 {
				break
			}
		}
		if (c == ' ' || c == '\t') && depth == 0 {
			break
		}
		i++
	}
	return s[:i], strings.TrimLeft(s[i:], " \t")
}

// splitParamListText splits an @method tag's "(...)" parameter text on
// top-level commas, honoring nested <>, {} and () the same way splitTypeAndRest
// does for a single type -- a shaped-array default like "array{0: 1}" must
// not be split on its internal comma.
func splitParamListText(s string) []string {
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
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	if strings.TrimSpace(s[start:]) != "" {
		parts = append(parts, s[start:])
	}
	return parts
}

// parseMethodTag parses "@method [[static] return type] name(ParamType $param, ...) [description]".
func parseMethodTag(text string) MagicMethod {
	m := MagicMethod{ReturnType: "void"}

	text = strings.TrimSpace(text)
	isStatic := strings.HasPrefix(text, "static ")
	if isStatic {
		text = strings.TrimSpace(strings.TrimPrefix(text, "static "))
		m.Static = true
	}

	open := strings.Index(text, "(")
	if open == -1 {
		m.Name = text
		return m
	}

	before := strings.TrimSpace(text[:open])
	fields := strings.Fields(before)
	if len(fields) > 0 {
		m.Name = fields[len(fields)-1]
		m.ReturnType = strings.Join(fields[:len(fields)-1], " ")
		if m.ReturnType == "" {
			m.ReturnType = "void"
		}
	}

	depth := 0
	close := -1
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				close = i
			}
		}
		if close != -1 {
			break
		}
	}
	if close == -1 {
		return m
	}

	paramsText := text[open+1 : close]
	for _, p := range splitParamListText(paramsText) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		typ, rest := splitTypeAndRest(p)
		rest = strings.TrimPrefix(strings.TrimSpace(rest), "$")
		name := strings.SplitN(rest, "=", 2)[0]
		name = strings.TrimSpace(name)
		if !strings.Contains(p, "$") {
			// no variable at all, e.g. just "int" -- treat the whole thing as type
			typ = p
			name = ""
		}
		m.Params = append(m.Params, ParamDoc{Name: name, Type: typ})
	}

	m.Description = strings.TrimSpace(text[close+1:])
	return m
}

// parsePropertyTag parses "@property[-read|-write] [Type] [$name] [description]".
func parsePropertyTag(mode, text string) MagicProperty {
	typ, rest := splitTypeAndRest(text)
	rest = strings.TrimLeft(rest, " \t")
	name := ""
	if strings.HasPrefix(rest, "$") {
		parts := strings.SplitN(rest, " ", 2)
		name = strings.TrimPrefix(parts[0], "$")
		rest = ""
		if len(parts) > 1 {
			rest = parts[1]
		}
	}
	return MagicProperty{Mode: mode, Name: name, Type: typ, Description: strings.TrimSpace(rest)}
}

func ParseDocBlock(raw string) DocBlock {
	doc := DocBlock{}
	lines := strings.Split(raw, "\n")

	var summaryLines []string
	type rawTag struct {
		name string
		text string
	}
	var tags []rawTag
	var current *rawTag

	for _, line := range lines {
		l := strings.TrimSpace(line)
		l = strings.TrimPrefix(l, "*")
		l = strings.TrimSpace(l)

		if strings.HasPrefix(l, "@") {
			parts := strings.SplitN(l, " ", 2)
			name := parts[0]
			text := ""
			if len(parts) > 1 {
				text = parts[1]
			}
			tags = append(tags, rawTag{name: name, text: text})
			current = &tags[len(tags)-1]
			continue
		}

		if current != nil {
			if l != "" {
				current.text = strings.TrimSpace(current.text + " " + l)
			}
			continue
		}

		if l != "" {
			summaryLines = append(summaryLines, l)
		}
	}

	doc.Summary = strings.Join(summaryLines, " ")

	for _, t := range tags {
		switch t.name {
		case "@param":
			typ, rest := splitTypeAndRest(t.text)
			rest = strings.TrimPrefix(rest, "$")
			nameParts := strings.SplitN(rest, " ", 2)
			name := nameParts[0]
			desc := ""
			if len(nameParts) > 1 {
				desc = strings.TrimSpace(nameParts[1])
			}
			doc.Params = append(doc.Params, ParamDoc{Name: name, Type: typ, Description: desc})
		case "@return":
			typ, rest := splitTypeAndRest(t.text)
			doc.ReturnType = typ
			doc.ReturnDesc = rest
		case "@throws":
			typ, rest := splitTypeAndRest(t.text)
			doc.Throws = append(doc.Throws, ThrowsDoc{Type: typ, Description: rest})
		case "@since":
			doc.Since = strings.TrimSpace(t.text)
		case "@deprecated":
			doc.Deprecated = strings.TrimSpace(t.text)
		case "@var":
			typ, rest := splitTypeAndRest(t.text)
			doc.VarType = typ
			doc.VarDesc = strings.TrimSpace(stripLeadingVarName(rest))
		case "@package":
			doc.Package = strings.TrimSpace(t.text)
		case "@see":
			doc.See = append(doc.See, strings.TrimSpace(t.text))
		case "@internal":
			doc.Internal = true
			doc.InternalDesc = strings.TrimSpace(t.text)
		case "@api":
			doc.Api = true
		case "@subpackage":
			doc.Subpackage = strings.TrimSpace(t.text)
		case "@category":
			doc.Category = strings.TrimSpace(t.text)
		case "@link":
			doc.Link = append(doc.Link, strings.TrimSpace(t.text))
		case "@uses":
			doc.Uses = append(doc.Uses, strings.TrimSpace(t.text))
		case "@todo":
			doc.Todo = append(doc.Todo, strings.TrimSpace(t.text))
		case "@example":
			doc.Example = append(doc.Example, strings.TrimSpace(t.text))
		case "@author":
			doc.Author = strings.TrimSpace(t.text)
		case "@copyright":
			doc.Copyright = strings.TrimSpace(t.text)
		case "@license":
			doc.License = strings.TrimSpace(t.text)
		case "@version":
			doc.Version = strings.TrimSpace(t.text)
		case "@ignore":
			doc.Ignore = true
			doc.IgnoreDesc = strings.TrimSpace(t.text)
		case "@final":
			doc.Final = true
			doc.FinalDesc = strings.TrimSpace(t.text)
		case "@filesource":
			doc.FileSource = true
		case "@source":
			doc.Source = strings.TrimSpace(t.text)
		case "@property":
			doc.MagicProperties = append(doc.MagicProperties, parsePropertyTag("read-write", t.text))
		case "@property-read":
			doc.MagicProperties = append(doc.MagicProperties, parsePropertyTag("read-only", t.text))
		case "@property-write":
			doc.MagicProperties = append(doc.MagicProperties, parsePropertyTag("write-only", t.text))
		case "@method":
			doc.MagicMethods = append(doc.MagicMethods, parseMethodTag(t.text))
		}
	}

	return doc
}
