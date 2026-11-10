package main

type parser struct {
	toks      []Token
	docFor    map[int]string
	pos       int
	namespace string
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func visibilityOf(modifiers []string) string {
	if contains(modifiers, "private") {
		return "private"
	}
	if contains(modifiers, "protected") {
		return "protected"
	}
	return "public"
}

func isClassModifier(v string) bool {
	return v == "final" || v == "abstract" || v == "readonly"
}

func isMemberModifierValue(v string) bool {
	switch v {
	case "public", "private", "protected", "static", "abstract", "final", "readonly":
		return true
	}
	return false
}

func (p *parser) cur() Token { return p.toks[p.pos] }

func (p *parser) advance() Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) isPunct(v string) bool {
	t := p.cur()
	return t.Type == TPunct && t.Value == v
}

func (p *parser) isVal(v string) bool {
	t := p.cur()
	return t.Type == TIdent && t.Value == v
}

func (p *parser) docAt(idx int) DocBlock {
	return ParseDocBlock(p.docFor[idx])
}

// skipAttributes discards one or more "#[...]" attribute groups. We never
// interpret attribute contents, only make sure their internal "(" / "["
// never gets mistaken for real structural brackets.
func (p *parser) skipAttributes() {
	for p.isPunct("#[") {
		p.advance()
		depth := 1
		for depth > 0 && p.cur().Type != TEOF {
			if p.isPunct("(") || p.isPunct("[") {
				depth++
			}
			if p.isPunct(")") || p.isPunct("]") {
				depth--
			}
			p.advance()
		}
	}
}

// Parse extracts every class/interface/trait/enum declaration from the
// token stream. It never inspects method bodies: it only balances braces to
// skip over them.
func Parse(toks []Token, docFor map[int]string) []ClassLike {
	p := &parser{toks: toks, docFor: docFor}
	var classes []ClassLike

	for p.cur().Type != TEOF {
		if p.isVal("namespace") {
			p.advance()
			if p.cur().Type == TIdent {
				p.namespace = p.advance().Value // lexer already joins "A\B\C" into one token
			}
			continue
		}
		if p.looksLikeClassStart() {
			classes = append(classes, p.parseClassLike())
			continue
		}
		p.advance()
	}

	return classes
}

func (p *parser) looksLikeClassStart() bool {
	i := p.pos
	for i < len(p.toks) && p.toks[i].Type == TIdent && isClassModifier(p.toks[i].Value) {
		i++
	}
	if i < len(p.toks) && p.toks[i].Type == TIdent {
		switch p.toks[i].Value {
		case "class", "interface", "trait", "enum":
			return true
		}
	}
	return false
}

func (p *parser) parseIdentList() []string {
	names := []string{p.advance().Value}
	for p.isPunct(",") {
		p.advance()
		names = append(names, p.advance().Value)
	}
	return names
}

func (p *parser) parseClassLike() ClassLike {
	startIdx := p.pos
	p.skipAttributes()
	var modifiers []string
	for p.cur().Type == TIdent && isClassModifier(p.cur().Value) {
		modifiers = append(modifiers, p.advance().Value)
	}
	classLine := p.cur().Line
	kind := p.advance().Value
	name := p.advance().Value

	backing := ""
	if p.isPunct(":") {
		p.advance()
		backing = p.advance().Value
	}

	var extends, implementsList []string
	if p.isVal("extends") {
		p.advance()
		extends = p.parseIdentList()
	}
	if p.isVal("implements") {
		p.advance()
		implementsList = p.parseIdentList()
	}

	cl := ClassLike{
		Kind:        kind,
		Name:        name,
		Namespace:   p.namespace,
		Modifiers:   modifiers,
		BackingType: backing,
		Extends:     extends,
		Implements:  implementsList,
		Doc:         p.docAt(startIdx),
		Line:        classLine,
	}

	if p.isPunct("{") {
		p.advance()
		p.parseMembers(&cl)
	}

	return cl
}

func (p *parser) parseMembers(cl *ClassLike) {
	for !p.isPunct("}") && p.cur().Type != TEOF {
		before := p.pos

		switch {
		case p.isVal("use"):
			for !p.isPunct(";") && p.cur().Type != TEOF {
				p.advance()
			}
			if p.isPunct(";") {
				p.advance()
			}

		case p.isVal("case"):
			p.advance()
			caseName := p.advance().Value
			val := ""
			if p.isPunct("=") {
				p.advance()
				val = p.collectExprText([]string{";"})
			}
			if p.isPunct(";") {
				p.advance()
			}
			cl.Cases = append(cl.Cases, EnumCase{Name: caseName, Value: val})

		case p.isVal("const"):
			p.advance()
			for !p.isPunct("=") && !p.isPunct(";") && p.cur().Type != TEOF {
				p.advance()
			}
			if p.isPunct("=") {
				p.advance()
				p.skipExpr([]string{";"})
			}
			if p.isPunct(";") {
				p.advance()
			}

		default:
			p.parseMemberDeclaration(cl)
		}

		if p.pos == before {
			p.advance()
		}
	}
	if p.isPunct("}") {
		p.advance()
	}
}

func (p *parser) parseMemberDeclaration(cl *ClassLike) {
	memberStart := p.pos
	p.skipAttributes()

	var modifiers []string
	for p.cur().Type == TIdent && isMemberModifierValue(p.cur().Value) {
		modifiers = append(modifiers, p.advance().Value)
	}

	if p.isVal("function") {
		funcLine := p.cur().Line
		p.advance()
		if p.isPunct("&") {
			p.advance()
		}
		name := p.advance().Value
		if p.isPunct("(") {
			p.advance()
		}
		params := p.parseParamList()
		if p.isPunct(")") {
			p.advance()
		}

		returnType := ""
		if p.isPunct(":") {
			p.advance()
			returnType = p.parseTypeExpr()
		}

		var returnTupleNames []string
		if p.isPunct(";") {
			p.advance()
		} else if p.isPunct("{") {
			returnTupleNames = p.skipBodyCapturingReturnTuple()
		}

		cl.Methods = append(cl.Methods, Method{
			Name:             name,
			Visibility:       visibilityOf(modifiers),
			Static:           contains(modifiers, "static"),
			Abstract:         contains(modifiers, "abstract"),
			Params:           params,
			ReturnType:       returnType,
			ReturnTupleNames: returnTupleNames,
			Doc:              p.docAt(memberStart),
			Line:             funcLine,
		})
		return
	}

	// property declaration
	typ := p.parseTypeExpr()

	if p.cur().Type != TVariable {
		return // not a recognizable member; caller's stuck-guard will advance
	}

	doc := p.docAt(memberStart)
	name := p.advance().Value
	if p.isPunct("=") {
		p.advance()
		p.skipExpr([]string{";", ","})
	}
	cl.Properties = append(cl.Properties, Property{
		Name:       name,
		Type:       typ,
		Visibility: visibilityOf(modifiers),
		Static:     contains(modifiers, "static"),
		Readonly:   contains(modifiers, "readonly"),
		Doc:        doc,
	})

	for p.isPunct(",") {
		p.advance()
		if p.cur().Type != TVariable {
			break
		}
		n2 := p.advance().Value
		if p.isPunct("=") {
			p.advance()
			p.skipExpr([]string{";", ","})
		}
		cl.Properties = append(cl.Properties, Property{
			Name: n2, Type: typ,
			Visibility: visibilityOf(modifiers),
			Static:     contains(modifiers, "static"),
			Readonly:   contains(modifiers, "readonly"),
		})
	}

	if p.isPunct(";") {
		p.advance()
	}
}

// parseTypeExpr reads an optional "?" plus an identifier chain joined by "|"
// or "&" (union/intersection). It stops as soon as something else appears
// (a variable, "(", etc.) so callers can check what follows.
func (p *parser) parseTypeExpr() string {
	typ := ""
	if p.isPunct("?") {
		typ = "?"
		p.advance()
	}
	if p.cur().Type == TIdent {
		typ += p.advance().Value
		for (p.isPunct("|") || p.isPunct("&")) && p.pos+1 < len(p.toks) && p.toks[p.pos+1].Type == TIdent {
			op := p.advance().Value
			typ += op + p.advance().Value
		}
	}
	return typ
}

func (p *parser) parseParamList() []Param {
	var params []Param
	if p.isPunct(")") {
		return params
	}
	for {
		p.skipAttributes()

		var modifiers []string
		for p.cur().Type == TIdent && isMemberModifierValue(p.cur().Value) {
			modifiers = append(modifiers, p.advance().Value)
		}

		typ := p.parseTypeExpr()

		if p.isPunct("&") {
			p.advance() // by-reference parameter: Type &$name
		}
		if p.isPunct("...") {
			p.advance()
		}

		name := ""
		if p.cur().Type == TVariable {
			name = p.advance().Value
		}

		if p.isPunct("=") {
			p.advance()
			p.skipExpr([]string{",", ")"})
		}

		params = append(params, Param{
			Name:       name,
			Type:       typ,
			Promoted:   len(modifiers) > 0,
			Visibility: visibilityOf(modifiers),
		})

		if p.isPunct(",") {
			p.advance()
			if p.isPunct(")") {
				break
			}
			continue
		}
		break
	}
	return params
}

// skipBodyCapturingReturnTuple is skipBalancedBraces plus a side-channel
// observation: while walking the method body token-by-token (never
// interpreting it), it also watches for a top-level "return [$a, $b, ...];"
// -- directly inside the method, not inside a nested closure/if/etc -- and
// captures the bare variable names if found. This is a pure peek: it never
// changes how tokens are consumed, so it can't desync the brace-depth
// tracking the way misreading an expression would.
func (p *parser) skipBodyCapturingReturnTuple() []string {
	depth := 0
	var names []string
	for p.cur().Type != TEOF {
		if names == nil && depth == 1 {
			if n := p.peekReturnTuple(); n != nil {
				names = n
			}
		}
		if p.isPunct("{") {
			depth++
			p.advance()
			continue
		}
		if p.isPunct("}") {
			depth--
			p.advance()
			if depth == 0 {
				return names
			}
			continue
		}
		p.advance()
	}
	return names
}

// peekReturnTuple checks whether cur() starts "return [$a, $b, ...];" with
// bare variables only, without consuming any tokens -- the caller's normal
// token-by-token walk advances through them regardless.
func (p *parser) peekReturnTuple() []string {
	if !(p.cur().Type == TIdent && p.cur().Value == "return") {
		return nil
	}
	i := p.pos + 1
	if i >= len(p.toks) || p.toks[i].Type != TPunct || p.toks[i].Value != "[" {
		return nil
	}
	i++

	var names []string
	for i < len(p.toks) {
		if p.toks[i].Type != TVariable {
			return nil
		}
		names = append(names, p.toks[i].Value)
		i++
		if i < len(p.toks) && p.toks[i].Type == TPunct && p.toks[i].Value == "," {
			i++
			continue
		}
		break
	}

	if i >= len(p.toks) || p.toks[i].Type != TPunct || p.toks[i].Value != "]" {
		return nil
	}
	i++
	if i < len(p.toks) && p.toks[i].Type == TPunct && p.toks[i].Value == ";" {
		return names
	}
	return nil
}

// skipExpr advances past an expression (e.g. a default value), honoring
// nested (), [], {} so a top-level stop token isn't mistaken for a nested
// one, then stops right before a stop token at depth 0 without consuming it.
func (p *parser) skipExpr(stopAtDepthZero []string) {
	depth := 0
	for p.cur().Type != TEOF {
		if depth == 0 {
			t := p.cur()
			if t.Type == TPunct && contains(stopAtDepthZero, t.Value) {
				return
			}
		}
		switch {
		case p.isPunct("(") || p.isPunct("[") || p.isPunct("{"):
			depth++
		case p.isPunct(")") || p.isPunct("]") || p.isPunct("}"):
			depth--
		}
		p.advance()
	}
}

// collectExprText is like skipExpr but returns the raw text it skipped,
// used for enum case values where we want to show them, not just discard.
func (p *parser) collectExprText(stopAtDepthZero []string) string {
	start := p.pos
	p.skipExpr(stopAtDepthZero)
	var out string
	for i := start; i < p.pos; i++ {
		if p.toks[i].Type == TVariable {
			out += "$" + p.toks[i].Value
		} else {
			out += p.toks[i].Value
		}
	}
	return out
}
