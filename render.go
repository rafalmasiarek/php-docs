package main

import (
	"fmt"
	"regexp"
	"strings"
)

var wsRe = regexp.MustCompile(`\s+`)

func flatten(s string) string {
	return strings.TrimSpace(wsRe.ReplaceAllString(s, " "))
}

// cell escapes "|" so a union type (Type|null) or any free text containing
// a literal pipe can't be mistaken for a GFM table column delimiter -- an
// unescaped "|" inside a cell breaks the table structure, backticks or not.
func cell(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}

// htmlEscape escapes text going inside a raw HTML tag (e.g. <summary>) --
// unlike cell(), markdown table/backtick escaping doesn't apply here, since
// this is literal HTML, not markdown: an unescaped "<" from a generic type
// like list<array{0: string}> would be parsed as the start of an HTML tag.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// visibilityBadge renders @internal/@api as a small marker -- these are
// presence-only tags (no value), so there's nothing to show but the tag
// itself.
func visibilityBadge(doc DocBlock) string {
	switch {
	case doc.Internal && doc.InternalDesc != "":
		return "*`@internal`* — " + doc.InternalDesc
	case doc.Internal:
		return "*`@internal`*"
	case doc.Api:
		return "*`@api`*"
	default:
		return ""
	}
}

// renderDocMetadata writes every "informational" PHPDoc tag that isn't
// already handled by a dedicated code path (deprecated, internal/api,
// var, throws, param/return) -- package/subpackage/category, authorship,
// cross-references, and todos. Shared between class- and method-level
// docblocks since phpDocumentor allows all of these on either.
func renderDocMetadata(b *strings.Builder, doc DocBlock) {
	if doc.Ignore {
		desc := doc.IgnoreDesc
		if desc != "" {
			desc = " — " + desc
		}
		fmt.Fprintf(b, "*`@ignore`*%s\n\n", desc)
	}
	if doc.Final {
		desc := doc.FinalDesc
		if desc != "" {
			desc = " — " + desc
		}
		fmt.Fprintf(b, "*`@final`*%s\n\n", desc)
	}
	if doc.Package != "" {
		fmt.Fprintf(b, "**Package:** `%s`\n\n", doc.Package)
	}
	if doc.Subpackage != "" {
		fmt.Fprintf(b, "**Subpackage:** `%s`\n\n", doc.Subpackage)
	}
	if doc.Category != "" {
		fmt.Fprintf(b, "**Category:** `%s`\n\n", doc.Category)
	}
	if doc.Author != "" {
		fmt.Fprintf(b, "**Author:** %s\n\n", doc.Author)
	}
	if doc.Copyright != "" {
		fmt.Fprintf(b, "**Copyright:** %s\n\n", doc.Copyright)
	}
	if doc.License != "" {
		fmt.Fprintf(b, "**License:** %s\n\n", doc.License)
	}
	if doc.Version != "" {
		fmt.Fprintf(b, "**Version:** %s\n\n", doc.Version)
	}
	if doc.FileSource {
		b.WriteString("*`@filesource`*\n\n")
	}
	if doc.Source != "" {
		fmt.Fprintf(b, "**Source:** %s\n\n", doc.Source)
	}
	for _, see := range doc.See {
		fmt.Fprintf(b, "**See:** %s\n\n", see)
	}
	for _, link := range doc.Link {
		fmt.Fprintf(b, "**Link:** %s\n\n", link)
	}
	for _, uses := range doc.Uses {
		fmt.Fprintf(b, "**Uses:** %s\n\n", uses)
	}
	for _, ex := range doc.Example {
		fmt.Fprintf(b, "**Example:** %s\n\n", ex)
	}
	for _, todo := range doc.Todo {
		fmt.Fprintf(b, "**Todo:** %s\n\n", todo)
	}
}

func findParamDoc(doc DocBlock, name string) (ParamDoc, bool) {
	for _, p := range doc.Params {
		if p.Name == name {
			return p, true
		}
	}
	return ParamDoc{}, false
}

// fqcn is the fully-qualified class name: namespace + class name.
func fqcn(cl ClassLike) string {
	if cl.Namespace == "" {
		return cl.Name
	}
	return cl.Namespace + "\\" + cl.Name
}

// computeAnchors replicates, in one pass, exactly which heading GitHub will
// assign which slug to -- GitHub ignores any "{#custom-id}" we might write
// (that's a kramdown/Pandoc extension, not GFM) and always derives the
// anchor from the heading's own text, with a page-wide sequential counter
// for collisions (so two different classes' "__construct()" methods don't
// collide with EACH OTHER's links, they collide with every "__construct()"
// anywhere earlier on the page). This function must visit headings in
// exactly the order Render() actually emits them -- index heading, its
// present kind subheadings, then per class: the class heading followed by
// each of its method headings -- or the slugs it predicts won't match
// what GitHub really assigns.
func computeAnchors(classes []ClassLike) (classAnchor map[string]string, methodAnchor map[string]string) {
	slugger := NewGithubSlugger()
	classAnchor = map[string]string{}
	methodAnchor = map[string]string{}

	if len(classes) > 1 {
		slugger.Slug("Index")
		for _, g := range kindGroups {
			for _, cl := range classes {
				if cl.Kind == g.kind {
					slugger.Slug(g.plural)
					break
				}
			}
		}
	}

	for _, cl := range classes {
		classAnchor[fqcn(cl)] = slugger.Slug(cl.Name)
		for _, m := range cl.Methods {
			methodAnchor[fqcn(cl)+"::"+m.Name] = slugger.Slug(m.Name + "()")
		}
	}
	return classAnchor, methodAnchor
}

var scalarTypes = map[string]bool{
	"string": true, "int": true, "float": true, "bool": true, "array": true,
	"mixed": true, "void": true, "callable": true, "object": true,
	"iterable": true, "self": true, "static": true, "parent": true, "null": true,
	"never": true, "false": true, "true": true,
}

// classDependencies lists the non-scalar types a class's constructor takes
// plus any additional non-scalar property types (covering setter/property
// injection, not just constructor injection), in first-appearance order,
// deduplicated, and excluding whatever's already shown via
// implements/extends -- the closest thing to "what does this class need"
// without a full usage-graph analysis.
func classDependencies(cl ClassLike) []string {
	seen := map[string]bool{}
	for _, iface := range cl.Implements {
		seen[stripLeadingBackslash(iface)] = true
	}
	for _, parent := range cl.Extends {
		seen[stripLeadingBackslash(parent)] = true
	}

	var deps []string
	add := func(t string) {
		t = strings.TrimPrefix(t, "?")
		for _, part := range strings.Split(t, "|") {
			part = strings.TrimSpace(part)
			if part == "" || scalarTypes[strings.ToLower(part)] || seen[part] {
				continue
			}
			seen[part] = true
			deps = append(deps, part)
		}
	}

	for i := range cl.Methods {
		if cl.Methods[i].Name == "__construct" {
			for _, p := range cl.Methods[i].Params {
				add(p.Type)
			}
			break
		}
	}
	for _, p := range cl.Properties {
		add(p.Type)
	}

	return deps
}

// buildTypeLinks maps a bare class/interface name (lowercased) to its
// real GitHub anchor, but only when that name is unique across the whole
// documented set -- an ambiguous name (two CaptchaExtension in different
// namespaces) is left unlinked rather than guessed.
func buildTypeLinks(classes []ClassLike, classAnchor map[string]string) map[string]string {
	counts := map[string]int{}
	anchor := map[string]string{}
	for _, cl := range classes {
		key := strings.ToLower(cl.Name)
		counts[key]++
		anchor[key] = classAnchor[fqcn(cl)]
	}
	links := map[string]string{}
	for k, c := range counts {
		if c == 1 {
			links[k] = anchor[k]
		}
	}
	return links
}

// linkType renders a type as a link to its own section when it's a single,
// unambiguous identifier we also documented -- union types, nullable
// markers, and generics (list<T>, array{...}) are left as plain code since
// linking one member of a compound type inside one backtick span isn't
// representable in Markdown.
func linkType(links map[string]string, t string) string {
	t = normalizeShapeSpacing(t)
	nullable := strings.HasPrefix(t, "?")
	bare := stripLeadingBackslash(strings.TrimPrefix(t, "?"))
	if anchor, ok := links[strings.ToLower(bare)]; ok {
		prefix := ""
		if nullable {
			prefix = "?"
		}
		return prefix + "[`" + bare + "`](#" + anchor + ")"
	}
	return "`" + cell(t) + "`"
}

var kindGroups = []struct {
	kind, plural string
}{
	{"class", "Classes"},
	{"interface", "Interfaces"},
	{"trait", "Traits"},
	{"enum", "Enums"},
}

// renderIndex groups the discovered classes by kind so a lib with many
// files (e.g. 23 classes in php-captcha) is navigable from the top instead
// of forcing a scroll through file-discovery order.
func renderIndex(classes []ClassLike, classAnchor map[string]string) string {
	var b strings.Builder
	b.WriteString("## Index\n\n")
	for _, g := range kindGroups {
		var items []ClassLike
		for _, cl := range classes {
			if cl.Kind == g.kind {
				items = append(items, cl)
			}
		}
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "### %s\n\n", g.plural)
		for _, cl := range items {
			ns := cl.Namespace
			if ns == "" {
				ns = "—"
			}
			fmt.Fprintf(&b, "- [`%s`](#%s) — `%s`\n", cl.Name, classAnchor[fqcn(cl)], ns)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// sourceLink builds a "[source](.../blob/REF/path#Lline)" link, or "" when
// repoURL is empty -- the whole feature is opt-in, exactly one flag away
// from not existing at all in the output.
func sourceLink(repoURL, ref, filePath string, line int) string {
	if repoURL == "" || filePath == "" || line == 0 {
		return ""
	}
	base := strings.TrimSuffix(repoURL, "/")
	return fmt.Sprintf("[source](%s/blob/%s/%s#L%d)", base, ref, filePath, line)
}

func Render(classes []ClassLike, repoURL, ref string) string {
	var b strings.Builder
	classAnchor, methodAnchor := computeAnchors(classes)
	links := buildTypeLinks(classes, classAnchor)

	if len(classes) > 1 {
		b.WriteString(renderIndex(classes, classAnchor))
	}

	for _, cl := range classes {
		b.WriteString(fmt.Sprintf("## `%s`\n\n", cl.Name))

		if cl.Doc.Deprecated != "" {
			b.WriteString(fmt.Sprintf("> **Deprecated** — %s\n\n", cl.Doc.Deprecated))
		}
		if badge := visibilityBadge(cl.Doc); badge != "" {
			b.WriteString(badge + "\n\n")
		}

		subtitle := cl.Kind
		if len(cl.Modifiers) > 0 {
			subtitle = strings.Join(cl.Modifiers, " ") + " " + subtitle
		}
		fmt.Fprintf(&b, "*%s* — `%s`", subtitle, fqcn(cl))
		if link := sourceLink(repoURL, ref, cl.FilePath, cl.Line); link != "" {
			fmt.Fprintf(&b, " · %s", link)
		}
		b.WriteString("\n\n")

		if cl.Doc.Summary != "" {
			b.WriteString(cl.Doc.Summary + "\n\n")
		}

		linkJoin := func(names []string) string {
			linked := make([]string, len(names))
			for i, n := range names {
				linked[i] = linkType(links, n)
			}
			return strings.Join(linked, ", ")
		}

		var meta []string
		if len(cl.Implements) > 0 {
			meta = append(meta, "implements "+linkJoin(cl.Implements))
		}
		if len(cl.Extends) > 0 {
			meta = append(meta, "extends "+linkJoin(cl.Extends))
		}
		if cl.BackingType != "" {
			meta = append(meta, "backed by "+linkType(links, cl.BackingType))
		}
		if len(meta) > 0 {
			b.WriteString(strings.Join(meta, " · ") + "\n\n")
		}

		if deps := classDependencies(cl); len(deps) > 0 {
			fmt.Fprintf(&b, "**Depends on:** %s\n\n", linkJoin(deps))
		}

		// @package is only worth showing when it disagrees with the real
		// namespace -- otherwise it's pure restatement of what's already
		// in the subtitle above.
		docForMeta := cl.Doc
		if docForMeta.Package == cl.Namespace {
			docForMeta.Package = ""
		}
		renderDocMetadata(&b, docForMeta)

		if len(cl.Doc.MagicProperties) > 0 {
			b.WriteString("| Magic Property | Type | Mode | Description |\n|---|---|---|---|\n")
			for _, mp := range cl.Doc.MagicProperties {
				desc := mp.Description
				if desc == "" {
					desc = "—"
				}
				fmt.Fprintf(&b, "| `$%s` | %s | %s | %s |\n", mp.Name, linkType(links, mp.Type), mp.Mode, cell(desc))
			}
			b.WriteString("\n")
		}

		if len(cl.Doc.MagicMethods) > 0 {
			b.WriteString("| Magic Method | Returns | Description |\n|---|---|---|\n")
			for _, mm := range cl.Doc.MagicMethods {
				var params []string
				for _, p := range mm.Params {
					params = append(params, strings.TrimSpace(p.Type+" $"+p.Name))
				}
				static := ""
				if mm.Static {
					static = "static "
				}
				sig := fmt.Sprintf("%s%s(%s)", static, mm.Name, strings.Join(params, ", "))
				desc := mm.Description
				if desc == "" {
					desc = "—"
				}
				fmt.Fprintf(&b, "| `%s` | %s | %s |\n", cell(sig), linkType(links, mm.ReturnType), cell(desc))
			}
			b.WriteString("\n")
		}

		if len(cl.Cases) > 0 {
			b.WriteString("| Case | Value |\n|---|---|\n")
			for _, c := range cl.Cases {
				val := c.Value
				if val == "" {
					val = "—"
				}
				b.WriteString(fmt.Sprintf("| `%s` | `%s` |\n", c.Name, cell(val)))
			}
			b.WriteString("\n")
		}

		if len(cl.Properties) > 0 {
			b.WriteString("| Property | Type | Visibility | Description |\n|---|---|---|---|\n")
			for _, prop := range cl.Properties {
				typ := prop.Type
				if prop.Doc.VarType != "" {
					typ = prop.Doc.VarType
				}
				rawDesc := prop.Doc.VarDesc
				if rawDesc == "" {
					rawDesc = prop.Doc.Summary
				}
				typ = humanizeType(typ, []string{prop.Doc.VarDesc, prop.Doc.Summary}, nil)
				desc := rawDesc
				if desc == "" {
					desc = "—"
				}
				vis := prop.Visibility
				if prop.Static {
					vis += " static"
				}
				if prop.Readonly {
					vis += " readonly"
				}
				b.WriteString(fmt.Sprintf("| `$%s` | %s | %s | %s |\n", prop.Name, linkType(links, typ), vis, cell(flatten(desc))))
			}
			b.WriteString("\n")
		}

		if len(cl.Methods) > 0 {
			b.WriteString("| Method | Visibility | Returns | Summary |\n|---|---|---|---|\n")
			for _, m := range cl.Methods {
				anchor := methodAnchor[fqcn(cl)+"::"+m.Name]
				summary := flatten(m.Doc.Summary)
				if summary == "" {
					summary = "—"
				}
				vis := m.Visibility
				if m.Static {
					vis += " static"
				}
				ret := m.ReturnType
				if m.Doc.ReturnType != "" {
					ret = m.Doc.ReturnType
				}
				ret = humanizeType(ret, []string{m.Doc.ReturnDesc, m.Doc.Summary}, m.ReturnTupleNames)
				retCol := linkType(links, ret)
				if m.Name == "__construct" {
					retCol = "—"
				} else if ret == "" {
					retCol = "`mixed`"
				}
				b.WriteString(fmt.Sprintf("| [`%s()`](#%s) | %s | %s | %s |\n", m.Name, anchor, vis, retCol, cell(summary)))
			}
			b.WriteString("\n")

			for _, m := range cl.Methods {
				renderMethodDetail(&b, m, links, sourceLink(repoURL, ref, cl.FilePath, m.Line))
			}
		}
	}

	return b.String()
}

func renderMethodDetail(b *strings.Builder, m Method, links map[string]string, srcLink string) {
	// Collapsed by default: a class with a dozen-plus methods would
	// otherwise turn the whole document into a wall of h4 headings. The
	// heading still lives inside -- GitHub parses markdown normally
	// inside <details> as long as blank lines surround the tags, so the
	// index table's links down to here keep working. The heading carries
	// no explicit id: GitHub ignores "{#id}" (that's a kramdown/Pandoc
	// extension, not GFM) and always derives the anchor from the heading
	// text itself, which is exactly what computeAnchors() predicts.
	//
	// Just the name here -- visibility and return type are already in the
	// index table one row above, repeating them in the teaser too was
	// pure restatement.
	fmt.Fprintf(b, "<details>\n<summary><code>%s()</code></summary>\n\n", htmlEscape(m.Name))

	fmt.Fprintf(b, "#### `%s()`\n\n", m.Name)

	if srcLink != "" {
		fmt.Fprintf(b, "%s\n\n", srcLink)
	}

	if m.Doc.Deprecated != "" {
		fmt.Fprintf(b, "> **Deprecated** — %s\n\n", m.Doc.Deprecated)
	}
	if badge := visibilityBadge(m.Doc); badge != "" {
		fmt.Fprintf(b, "%s\n\n", badge)
	}

	// The fenced block below must stay valid, real PHP -- native AST types
	// only. PHPDoc-refined types (literal unions, list<T>, shaped arrays)
	// aren't legal type syntax and belong in the prose tables instead.
	var sigParams []string
	for _, prm := range m.Params {
		sigParams = append(sigParams, fmt.Sprintf("%s $%s", prm.Type, prm.Name))
	}
	vis := m.Visibility
	if m.Static {
		vis += " static "
	} else {
		vis += " "
	}
	if m.Name == "__construct" {
		fmt.Fprintf(b, "```php\n%sfunction %s(%s)\n```\n\n", vis, m.Name, strings.Join(sigParams, ", "))
	} else {
		fmt.Fprintf(b, "```php\n%sfunction %s(%s): %s\n```\n\n", vis, m.Name, strings.Join(sigParams, ", "), m.ReturnType)
	}

	// From here on, doc-refined types are fine -- this is prose, not code.
	ret := m.ReturnType
	if m.Doc.ReturnType != "" {
		ret = m.Doc.ReturnType
	}

	if m.Doc.Summary != "" {
		fmt.Fprintf(b, "%s\n\n", m.Doc.Summary)
	}

	if len(m.Params) > 0 {
		b.WriteString("| Param | Type | Description |\n|---|---|---|\n")
		for _, prm := range m.Params {
			t := prm.Type
			rawDesc := ""
			if d, ok := findParamDoc(m.Doc, prm.Name); ok {
				if d.Type != "" {
					t = d.Type
				}
				rawDesc = d.Description
			}
			t = humanizeType(t, []string{rawDesc, m.Doc.Summary}, nil)
			desc := rawDesc
			if desc == "" {
				desc = "—"
			}
			fmt.Fprintf(b, "| `$%s` | %s | %s |\n", prm.Name, linkType(links, t), cell(desc))
		}
		b.WriteString("\n")
	}

	ret = humanizeType(ret, []string{m.Doc.ReturnDesc, m.Doc.Summary}, m.ReturnTupleNames)
	if ret != "" && ret != "void" {
		desc := ""
		if m.Doc.ReturnDesc != "" {
			desc = " — " + m.Doc.ReturnDesc
		}
		fmt.Fprintf(b, "**Returns:** %s%s\n\n", linkType(links, ret), desc)
	}

	if len(m.Doc.Throws) > 0 {
		b.WriteString("| Throws | When |\n|---|---|\n")
		for _, th := range m.Doc.Throws {
			desc := th.Description
			if desc == "" {
				desc = "—"
			}
			fmt.Fprintf(b, "| %s | %s |\n", linkType(links, th.Type), cell(desc))
		}
		b.WriteString("\n")
	}

	renderDocMetadata(b, m.Doc)

	b.WriteString("</details>\n\n")
}
