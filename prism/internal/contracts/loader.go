package contracts

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/prism-data/prism/internal/naming"
)

func LoadSource(path string) (*Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read source contract %s: %w", path, err)
	}
	var s Source
	if err := yaml.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse source contract %s: %w", path, err)
	}
	return &s, nil
}

func LoadEntity(path string) (*Entity, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read entity contract %s: %w", path, err)
	}
	var e Entity
	if err := yaml.Unmarshal(data, &e); err != nil {
		return nil, fmt.Errorf("parse entity contract %s: %w", path, err)
	}
	return &e, nil
}

// SourceBundle is a parsed source plus its parsed entities.
type SourceBundle struct {
	SourceID  string         // snake_case, derived from directory name
	SourceDir string         // absolute path
	Source    *Source        // parsed _source.yml
	Entities  []EntityBundle // parsed <entity>.yml files
}

type EntityBundle struct {
	EntityID string  // snake_case, derived from filename basename
	Path     string  // absolute path
	Entity   *Entity // parsed contents
}

// LoadAll walks dasDir (typically `contracts/das`) and returns one SourceBundle
// per immediate subdirectory. Each subdirectory must contain `_source.yml` and
// zero or more `<entity>.yml` files. Subdirectory and entity-file basenames
// must be snake_case (per ADR-005, ADR-007).
func LoadAll(dasDir string) ([]*SourceBundle, error) {
	entries, err := os.ReadDir(dasDir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dasDir, err)
	}
	var bundles []*SourceBundle
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		if err := naming.ValidateSnakeCaseIdentifier(id); err != nil {
			return nil, fmt.Errorf("source directory %q: %w", id, err)
		}
		srcDir, err := filepath.Abs(filepath.Join(dasDir, id))
		if err != nil {
			return nil, err
		}
		bundle, err := loadOneSource(id, srcDir)
		if err != nil {
			return nil, err
		}
		bundles = append(bundles, bundle)
	}
	return bundles, nil
}

func loadOneSource(id, srcDir string) (*SourceBundle, error) {
	srcPath := filepath.Join(srcDir, "_source.yml")
	src, err := LoadSource(srcPath)
	if err != nil {
		return nil, err
	}
	if err := ValidateSource(src); err != nil {
		return nil, fmt.Errorf("source %s: %w", id, err)
	}
	files, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, err
	}
	var ents []EntityBundle
	for _, f := range files {
		if f.IsDir() {
			return nil, fmt.Errorf("source %s: nested directory %q not allowed", id, f.Name())
		}
		name := f.Name()
		if name == "_source.yml" {
			continue
		}
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		base := strings.TrimSuffix(strings.TrimSuffix(name, ".yml"), ".yaml")
		if err := naming.ValidateSnakeCaseIdentifier(base); err != nil {
			return nil, fmt.Errorf("entity file %s/%s: %w", id, name, err)
		}
		entPath := filepath.Join(srcDir, name)
		ent, err := LoadEntity(entPath)
		if err != nil {
			return nil, err
		}
		if err := ValidateEntity(ent); err != nil {
			return nil, fmt.Errorf("entity %s/%s: %w", id, base, err)
		}
		ents = append(ents, EntityBundle{
			EntityID: base, Path: entPath, Entity: ent,
		})
	}
	if len(ents) == 0 {
		// Allowed: a source with only _source.yml (e.g. immediately after `discover`).
		_ = errors.New
	}
	return &SourceBundle{
		SourceID: id, SourceDir: srcDir, Source: src, Entities: ents,
	}, nil
}
