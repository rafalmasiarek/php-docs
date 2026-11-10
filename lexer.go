package main

import (
	"strings"
)

type TokenType int

const (
	TIdent TokenType = iota // identifiers and keywords, may include \ for qualified names
	TVariable                // $name
	TString                   // quoted string literal, including its quotes
	TPunct                   // single-char punctuation and a few 2-char ones (:: -> #[ ...)
	TDocComment               // /** ... */ content, markers stripped
	TEOF
)

type Token struct {
	Type  TokenType
	Value string
	Line  int
}

// Lex tokenizes PHP source into significant tokens (TIdent/TVariable/TPunct)
// plus doc comments. Regular comments and whitespace are dropped. docFor
// maps a significant token's index to the nearest preceding docblock, so the
// parser can attach documentation without caring about comment layout.
func Lex(src string) (tokens []Token, docFor map[int]string) {
	docFor = map[int]string{}
	pending := ""
	line := 1

	i := 0
	n := len(src)

	emit := func(t TokenType, v string) {
		if pending != "" && t != TDocComment {
			docFor[len(tokens)] = pending
			pending = ""
		}
		tokens = append(tokens, Token{Type: t, Value: v, Line: line})
	}

	countLines := func(s string) {
		line += strings.Count(s, "\n")
	}

	isIdentStart := func(c byte) bool {
		return c == '_' || c == '\\' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	}
	isIdentPart := func(c byte) bool {
		return isIdentStart(c) || (c >= '0' && c <= '9')
	}

	for i < n {
		c := src[i]

		// whitespace
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			if c == '\n' {
				line++
			}
			i++
			continue
		}

		// line comments, but "#[" is an attribute start, not a comment
		if c == '#' {
			if i+1 < n && src[i+1] == '[' {
				emit(TPunct, "#[")
				i += 2
				continue
			}
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < n && src[i+1] == '/' {
			for i < n && src[i] != '\n' {
				i++
			}
			continue
		}

		// block comments and docblocks
		if c == '/' && i+1 < n && src[i+1] == '*' {
			isDoc := i+2 < n && src[i+2] == '*' && !(i+3 < n && src[i+3] == '/')
			start := i + 2
			if isDoc {
				start = i + 3
			}
			end := strings.Index(src[i:], "*/")
			if end == -1 {
				i = n
				break
			}
			end += i
			content := src[start:end]
			if isDoc {
				pending = content
			}
			countLines(src[i:end])
			i = end + 2
			continue
		}

		// strings
		if c == '\'' || c == '"' {
			quote := c
			j := i + 1
			for j < n && src[j] != quote {
				if src[j] == '\\' && j+1 < n {
					j += 2
					continue
				}
				j++
			}
			end := j + 1
			if end > n {
				end = n
			}
			emit(TString, src[i:end])
			countLines(src[i:end])
			i = end
			continue
		}

		// heredoc / nowdoc: <<<LABEL, <<<"LABEL", <<<'LABEL'
		if c == '<' && i+2 < n && src[i+1] == '<' && src[i+2] == '<' {
			j := i + 3
			for j < n && (src[j] == ' ' || src[j] == '\t') {
				j++
			}
			quote := byte(0)
			if j < n && (src[j] == '\'' || src[j] == '"') {
				quote = src[j]
				j++
			}
			labelStart := j
			for j < n && isIdentPart(src[j]) {
				j++
			}
			label := src[labelStart:j]
			if quote != 0 && j < n && src[j] == quote {
				j++
			}
			// skip to end of this line
			for j < n && src[j] != '\n' {
				j++
			}
			j++ // past newline

			// scan lines until one whose trimmed start matches label
			for j < n {
				lineStart := j
				for j < n && src[j] != '\n' {
					j++
				}
				lineText := src[lineStart:j]
				trimmed := strings.TrimLeft(lineText, " \t")
				if strings.HasPrefix(trimmed, label) {
					after := trimmed[len(label):]
					if after == "" || !isIdentPart(after[0]) {
						j = lineStart + (len(lineText) - len(trimmed)) + len(label)
						break
					}
				}
				j++ // past newline
			}
			countLines(src[i:j])
			i = j
			continue
		}

		// variables
		if c == '$' && i+1 < n && isIdentStart(src[i+1]) {
			j := i + 1
			for j < n && isIdentPart(src[j]) {
				j++
			}
			emit(TVariable, src[i+1:j])
			i = j
			continue
		}

		// identifiers / keywords, including namespaced \Foo\Bar
		if isIdentStart(c) {
			j := i
			for j < n && isIdentPart(src[j]) {
				j++
			}
			emit(TIdent, src[i:j])
			i = j
			continue
		}

		// multi-char punctuation worth keeping distinct
		if c == ':' && i+1 < n && src[i+1] == ':' {
			emit(TPunct, "::")
			i += 2
			continue
		}
		if c == '.' && i+2 < n && src[i+1] == '.' && src[i+2] == '.' {
			emit(TPunct, "...")
			i += 3
			continue
		}

		// everything else: single-char punctuation
		emit(TPunct, string(c))
		i++
	}

	emit(TEOF, "")
	return tokens, docFor
}
