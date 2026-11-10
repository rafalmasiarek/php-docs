package main

type Param struct {
	Name       string
	Type       string
	Promoted   bool
	Visibility string
}

type Method struct {
	Name       string
	Visibility string
	Static     bool
	Abstract   bool
	Params     []Param
	ReturnType string
	Doc        DocBlock

	// ReturnTupleNames holds the bare variable names from a top-level
	// "return [$a, $b, ...];" statement in this method's body, if one was
	// found -- a fallback source for naming shaped-array return slots when
	// the docblock description doesn't spell them out. Nil if none found.
	ReturnTupleNames []string

	// Line is the 1-based source line the "function" keyword starts on --
	// only meaningful together with the owning ClassLike's FilePath, used
	// to build a --repo-url source link.
	Line int
}

type Property struct {
	Name       string
	Type       string
	Visibility string
	Static     bool
	Readonly   bool
	Doc        DocBlock
}

type EnumCase struct {
	Name  string
	Value string
}

type ClassLike struct {
	Kind        string // class, interface, trait, enum
	Name        string
	Namespace   string
	Modifiers   []string
	BackingType string
	Extends     []string
	Implements  []string
	Doc         DocBlock
	Properties  []Property
	Methods     []Method
	Cases       []EnumCase

	// FilePath is this class's source file, relative to the lib root
	// (set by main.go, not the parser -- the parser only ever sees one
	// file's tokens and has no notion of a lib root to be relative to).
	// Line is the 1-based source line the class/interface/trait/enum
	// keyword starts on. Both are only meaningful together, for building
	// a --repo-url source link.
	FilePath string
	Line     int
}
