package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ComposerJSON struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	License     json.RawMessage `json:"license"`
	Autoload    struct {
		PSR4 map[string]string `json:"psr-4"`
	} `json:"autoload"`
}

// LicenseStrings normalizes composer's "license" field, which may be a
// single string or an array of SPDX identifiers.
func (c ComposerJSON) LicenseStrings() []string {
	if len(c.License) == 0 {
		return nil
	}
	var single string
	if err := json.Unmarshal(c.License, &single); err == nil {
		return []string{single}
	}
	var many []string
	if err := json.Unmarshal(c.License, &many); err == nil {
		return many
	}
	return nil
}

func ReadComposerJSON(libDir string) (ComposerJSON, error) {
	var c ComposerJSON
	data, err := os.ReadFile(filepath.Join(libDir, "composer.json"))
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}

// SourceDirs resolves the autoload.psr-4 map to absolute directories under
// libDir -- this is "where the source code is", straight from the lib's own
// declaration, no separate config needed.
func (c ComposerJSON) SourceDirs(libDir string) []string {
	var dirs []string
	for _, rel := range c.Autoload.PSR4 {
		dirs = append(dirs, filepath.Join(libDir, rel))
	}
	return dirs
}

// GlobPHPFiles recursively collects every .php file under the given roots,
// skipping vendor/ and any dotdir.
func GlobPHPFiles(roots []string) ([]string, error) {
	var files []string
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // missing/unreadable dir: skip rather than fail the whole run
			}
			if info.IsDir() {
				base := filepath.Base(path)
				if base == "vendor" || (len(base) > 1 && base[0] == '.') {
					return filepath.SkipDir
				}
				return nil
			}
			if filepath.Ext(path) == ".php" {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return files, nil
}
