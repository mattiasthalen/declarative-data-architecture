package contracts

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prism-data/prism/internal/naming"
)

// LoadFocal parses a single contracts/dab/<entity>.yml file.
func LoadFocal(path string) (*Focal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read focal contract %s: %w", path, err)
	}
	var f Focal
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse focal contract %s: %w", path, err)
	}
	return &f, nil
}

// FocalBundle is a parsed focal contract plus its filesystem-derived ID.
type FocalBundle struct {
	EntityID string  // snake_case (filename basename), e.g. "customer"
	Path     string  // absolute path
	Focal    *Focal  // parsed contents
}

// LoadAllDab reads every <entity>.yml file directly under dabDir and returns one
// FocalBundle per file. Subdirectories are not allowed (DAB is flat).
// File basenames must be snake_case (per ADR-005).
func LoadAllDab(dabDir string) ([]*FocalBundle, error) {
	if _, err := os.Stat(dabDir); os.IsNotExist(err) {
		return nil, nil // empty / not yet authored — caller treats as no focals
	}
	entries, err := os.ReadDir(dabDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dabDir, err)
	}
	var bundles []*FocalBundle
	for _, e := range entries {
		if e.IsDir() {
			return nil, fmt.Errorf("dab directory %s: nested directory %q not allowed; DAB is flat", dabDir, e.Name())
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml")
		if err := naming.ValidateSnakeCaseIdentifier(base); err != nil {
			return nil, fmt.Errorf("focal file %s: %w", name, err)
		}
		abs, err := filepath.Abs(filepath.Join(dabDir, name))
		if err != nil {
			return nil, err
		}
		f, err := LoadFocal(abs)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, &FocalBundle{
			EntityID: base, Path: abs, Focal: f,
		})
	}
	return bundles, nil
}
