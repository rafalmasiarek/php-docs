package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// version and commit are set via -ldflags at release build time (see
// .goreleaser.yml); "dev"/"none" is what a plain `go build` produces.
var version = "dev"
var commit = "none"

func main() {
	noHeader := flag.Bool("no-header", false, "skip the name/description/license header from composer.json")
	out := flag.String("out", "", "write the generated markdown to this file instead of stdout")
	outputMode := flag.String("output-mode", "replace", `how to write --out: "replace" overwrites the whole file, "inject" updates only the content between `+beginMarker+` and `+endMarker+` markers, seeding the file from a minimal template if it doesn't exist yet`)
	repoURL := flag.String("repo-url", "", "repo URL (e.g. https://github.com/owner/repo) -- when set, adds a source link with file+line next to every class and method; omitted entirely when this flag is empty")
	ref := flag.String("ref", "main", "branch or tag to link to in --repo-url source links")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: php-docs [--no-header] [--out FILE] [--output-mode replace|inject] [--repo-url URL] [--ref REF] <lib-dir>")
	}
	flag.Parse()

	if *showVersion {
		fmt.Printf("php-docs %s (%s)\n", version, commit)
		return
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}
	libDir := flag.Arg(0)

	info, err := os.Stat(libDir)
	if err != nil || !info.IsDir() {
		fmt.Fprintf(os.Stderr, "error: %s is not a directory (pass a library root, not a single file)\n", libDir)
		os.Exit(1)
	}

	composer, err := ReadComposerJSON(libDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error reading composer.json in %s: %v\n", libDir, err)
		os.Exit(1)
	}

	sourceDirs := composer.SourceDirs(libDir)
	if len(sourceDirs) == 0 {
		fmt.Fprintln(os.Stderr, "error: composer.json has no autoload.psr-4 mapping, nothing to document")
		os.Exit(1)
	}

	files, err := GlobPHPFiles(sourceDirs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error walking source dirs: %v\n", err)
		os.Exit(1)
	}

	var allClasses []ClassLike
	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", path, err)
			continue
		}
		toks, docFor := Lex(string(src))
		classes := Parse(toks, docFor)

		relPath, err := filepath.Rel(libDir, path)
		if err != nil {
			relPath = path
		}
		for i := range classes {
			classes[i].FilePath = relPath
		}
		allClasses = append(allClasses, classes...)
	}

	ResolveInheritance(allClasses)

	doc := RenderDocument(composer, allClasses, !*noHeader, *repoURL, *ref)

	if *out == "" {
		fmt.Print(doc)
		return
	}

	finalContent := doc
	if *outputMode == "inject" {
		existing, readErr := os.ReadFile(*out)
		target := defaultTemplate
		if readErr == nil {
			target = string(existing)
		} else if !os.IsNotExist(readErr) {
			fmt.Fprintf(os.Stderr, "error reading %s: %v\n", *out, readErr)
			os.Exit(1)
		}
		finalContent, err = injectBetweenMarkers(target, doc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
	}

	if err := os.WriteFile(*out, []byte(finalContent), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error writing %s: %v\n", *out, err)
		os.Exit(1)
	}
}

func renderHeader(c ComposerJSON) string {
	var b strings.Builder
	if c.Name != "" {
		b.WriteString(fmt.Sprintf("# `%s`\n\n", c.Name))
	}
	if c.Description != "" {
		b.WriteString(c.Description + "\n\n")
	}
	if lic := c.LicenseStrings(); len(lic) > 0 {
		b.WriteString("**License:** " + strings.Join(lic, ", ") + "\n\n")
	}
	return b.String()
}

func RenderDocument(c ComposerJSON, classes []ClassLike, withHeader bool, repoURL, ref string) string {
	if !withHeader {
		return Render(classes, repoURL, ref)
	}
	return renderHeader(c) + Render(classes, repoURL, ref)
}
