package main

import "strings"

// isInheritDocOnly reports whether a docblock's entire summary is just the
// {@inheritDoc} inline tag -- the only shape we actually saw in the corpus
// (RecaptchaProvider, HCaptchaProvider, TurnstileProvider all use it bare).
func isInheritDocOnly(summary string) bool {
	s := strings.ToLower(strings.TrimSpace(summary))
	return s == "{@inheritdoc}"
}

func stripLeadingBackslash(s string) string {
	return strings.TrimPrefix(s, "\\")
}

func findMethodByName(methods []Method, name string) *Method {
	for i := range methods {
		if methods[i].Name == name {
			return &methods[i]
		}
	}
	return nil
}

// ResolveInheritance fills in {@inheritDoc} summaries (and, for methods,
// any still-missing params/return/throws) from the matching
// implemented/extended class, when that parent is also part of this same
// documented set and its name is unambiguous. Ambiguous or external
// (out-of-lib) parents are left as literal "{@inheritDoc}" text -- better
// an honest unresolved tag than a guess that links to the wrong class.
func ResolveInheritance(classes []ClassLike) {
	byName := map[string][]int{}
	for i, cl := range classes {
		key := strings.ToLower(cl.Name)
		byName[key] = append(byName[key], i)
	}

	resolve := func(name string) (ClassLike, bool) {
		idxs, ok := byName[strings.ToLower(stripLeadingBackslash(name))]
		if !ok || len(idxs) != 1 {
			return ClassLike{}, false
		}
		return classes[idxs[0]], true
	}

	for i := range classes {
		cl := &classes[i]

		var parents []string
		parents = append(parents, cl.Implements...)
		parents = append(parents, cl.Extends...)

		if isInheritDocOnly(cl.Doc.Summary) {
			for _, pname := range parents {
				if parent, ok := resolve(pname); ok && parent.Doc.Summary != "" {
					cl.Doc.Summary = parent.Doc.Summary
					break
				}
			}
		}

		for mi := range cl.Methods {
			m := &cl.Methods[mi]
			if !isInheritDocOnly(m.Doc.Summary) {
				continue
			}
			for _, pname := range parents {
				parent, ok := resolve(pname)
				if !ok {
					continue
				}
				pm := findMethodByName(parent.Methods, m.Name)
				if pm == nil {
					continue
				}
				m.Doc.Summary = pm.Doc.Summary
				if len(m.Doc.Params) == 0 {
					m.Doc.Params = pm.Doc.Params
				}
				if m.Doc.ReturnType == "" {
					m.Doc.ReturnType = pm.Doc.ReturnType
				}
				if m.Doc.ReturnDesc == "" {
					m.Doc.ReturnDesc = pm.Doc.ReturnDesc
				}
				if len(m.Doc.Throws) == 0 {
					m.Doc.Throws = pm.Doc.Throws
				}
				break
			}
		}
	}
}
