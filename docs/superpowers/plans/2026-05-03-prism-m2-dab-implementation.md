# Prism M2 (DAB) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the DAB layer to prism: contracts under `contracts/dab/<entity>.yml` declaratively define focal entities, descriptor groups, and relationships; `prism dab build` reads typed DAS staging tables (M1 output) and populates a conformed `dab` schema in the same DuckDB warehouse with IDFR / Focal / Descriptor / Relationship tables, per-group typed views, and a per-entity `__current` view, all bi-temporal.

**Architecture:** Pure-Go orchestration (no Python this milestone — DAS already landed). New `internal/contracts/` types + JSON Schema for DAB; new `internal/dab/` package builds an execution plan from parsed contracts; `Engine.Dialect()` gains 13 methods rendered from new SQL templates under `internal/tmpl/duckdb/`; CLI gains `prism dab discover/build/run` and integrates `dab build` into `prism run` and `prism validate`. Surrogate keys are deterministic `MD5(canonical_idfr)`. Bi-temporal columns (`EFF_TMSTP`, `VER_TMSTP`, `SEQ_NBR`, `ROW_ST`) are mechanically derived. See `docs/superpowers/specs/2026-05-02-prism-m2-dab-design.md`.

**Tech Stack:**
- Go 1.22+, [cobra](https://github.com/spf13/cobra), [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3), [jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema), [go-duckdb/v2](https://github.com/marcboeker/go-duckdb), [testify](https://github.com/stretchr/testify) — same as M1.
- `crypto/md5` from the Go stdlib for TYPE_KEY hashing.

**Where work happens:** Existing `prism/` Go module. M1 code is intact; M2 is additive. Use absolute paths in shell commands; module-relative paths in Go code.

---

## File structure (target by end of plan)

```
prism/
├── internal/
│   ├── contracts/
│   │   ├── focal.go                              # NEW: parsed-DAB types
│   │   ├── focal_loader.go                       # NEW: LoadAllDab
│   │   ├── focal_loader_test.go                  # NEW
│   │   ├── focal_validate.go                     # NEW: schema + cross-field
│   │   ├── focal_validate_test.go                # NEW
│   │   ├── crosslayer.go                         # NEW: das↔dab reference checks
│   │   ├── crosslayer_test.go                    # NEW
│   │   └── schemas/
│   │       └── dab_entity_v1.json                # NEW: embedded JSON Schema
│   ├── dab/                                      # NEW package
│   │   ├── hash.go                               # NEW: TYPE_KEY + IDFR helpers
│   │   ├── hash_test.go                          # NEW
│   │   ├── plan.go                               # NEW: contracts → execution plan
│   │   ├── plan_test.go                          # NEW
│   │   ├── execute.go                            # NEW: run plan against an engine
│   │   └── execute_test.go                       # NEW: round-trip integration
│   ├── engine/
│   │   ├── engine.go                             # MODIFY: 13 new Dialect methods
│   │   └── spec.go                               # MODIFY: 13 new spec types
│   ├── engine/duckdb/
│   │   └── duckdb.go                             # MODIFY: 13 new method impls
│   ├── tmpl/duckdb/
│   │   ├── render.go                             # MODIFY: new Render funcs
│   │   ├── render_test.go                        # MODIFY: golden snapshot tests
│   │   ├── dab_create_idfr.sql.tmpl              # NEW
│   │   ├── dab_create_focal.sql.tmpl             # NEW
│   │   ├── dab_create_descriptor.sql.tmpl        # NEW
│   │   ├── dab_create_relationship.sql.tmpl     # NEW
│   │   ├── dab_merge_idfr.sql.tmpl               # NEW
│   │   ├── dab_merge_focal.sql.tmpl              # NEW
│   │   ├── dab_merge_descriptor.sql.tmpl         # NEW
│   │   ├── dab_merge_relationship.sql.tmpl       # NEW
│   │   ├── dab_recompute_idfr.sql.tmpl           # NEW
│   │   ├── dab_recompute_descriptor.sql.tmpl     # NEW
│   │   ├── dab_recompute_relationship.sql.tmpl   # NEW
│   │   ├── dab_create_group_view.sql.tmpl        # NEW
│   │   ├── dab_create_entity_current_view.sql.tmpl # NEW
│   │   └── testdata/                             # MODIFY: new golden files
│   ├── cli/
│   │   ├── dab_build.go                          # NEW
│   │   ├── dab_build_test.go                     # NEW
│   │   ├── dab_run.go                            # NEW
│   │   ├── dab_discover.go                       # NEW
│   │   ├── dab_discover_test.go                  # NEW
│   │   ├── run.go                                # MODIFY: chain dab after das
│   │   ├── validate.go                           # MODIFY: also validate DAB
│   │   ├── doctor.go                             # MODIFY: cross-layer probe
│   │   └── root.go                               # MODIFY: wire new commands
└── testdata/
    └── contracts/
        ├── valid/
        │   └── dab/                              # NEW fixtures
        └── invalid/
            └── dab/                              # NEW fixtures
```

---

## Phase 0: JSON Schema + parsed Go types

### Task 1: Embed `dab_entity_v1.json` schema

**Files:**
- Create: `prism/internal/contracts/schemas/dab_entity_v1.json`

- [ ] **Step 1: Write the JSON Schema**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://prism.dev/schemas/dab_entity_v1.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "entity", "attributes", "mapping_groups"],
  "properties": {
    "version": {"const": 1},
    "entity": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "name", "definition"],
      "properties": {
        "id":          {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"},
        "name":        {"type": "string", "minLength": 1},
        "definition":  {"type": "string", "minLength": 1},
        "description": {"type": "string"}
      }
    },
    "attributes": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "definition"],
        "properties": {
          "id":          {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"},
          "definition":  {"type": "string", "minLength": 1},
          "description": {"type": "string"},
          "type": {
            "type": "string",
            "enum": ["STRING", "NUMBER", "UNIT", "START_TIMESTAMP", "END_TIMESTAMP"]
          },
          "effective_timestamp": {"type": "boolean"},
          "group": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["id", "type"],
              "properties": {
                "id":   {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"},
                "type": {"type": "string",
                         "enum": ["STRING", "NUMBER", "UNIT", "START_TIMESTAMP", "END_TIMESTAMP"]}
              }
            }
          }
        },
        "oneOf": [
          {"required": ["type"], "not": {"required": ["group"]}},
          {"required": ["group"], "not": {"required": ["type"]}}
        ]
      }
    },
    "relationships": {
      "type": "array",
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["id", "definition", "target_entity_id"],
        "properties": {
          "id":               {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"},
          "definition":       {"type": "string", "minLength": 1},
          "description":      {"type": "string"},
          "target_entity_id": {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"}
        }
      }
    },
    "mapping_groups": {
      "type": "array",
      "minItems": 1,
      "items": {
        "type": "object",
        "additionalProperties": false,
        "required": ["name", "tables"],
        "properties": {
          "name": {"type": "string", "pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)*$"},
          "allow_multiple_identifiers": {"type": "boolean"},
          "tables": {
            "type": "array",
            "minItems": 1,
            "items": {
              "type": "object",
              "additionalProperties": false,
              "required": ["source", "entity", "primary_keys", "attributes"],
              "properties": {
                "source": {"type": "string", "pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)*$"},
                "entity": {"type": "string", "pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)*$"},
                "from":   {"type": "string", "enum": ["current", "historized"]},
                "primary_keys": {
                  "type": "array", "minItems": 1,
                  "items": {"type": "string", "minLength": 1}
                },
                "entity_effective_timestamp_expression": {"type": "string", "minLength": 1},
                "where": {"type": "string", "minLength": 1},
                "attributes": {
                  "type": "array", "minItems": 1,
                  "items": {
                    "type": "object",
                    "additionalProperties": false,
                    "required": ["id", "transformation_expression"],
                    "properties": {
                      "id": {"type": "string",
                             "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*(\\.[A-Z][A-Z0-9]*(_[A-Z0-9]+)*)?$"},
                      "transformation_expression": {"type": "string", "minLength": 1},
                      "where": {"type": "string", "minLength": 1},
                      "attribute_effective_timestamp_expression": {"type": "string", "minLength": 1}
                    }
                  }
                },
                "relationships": {
                  "type": "array",
                  "items": {
                    "type": "object",
                    "additionalProperties": false,
                    "required": ["id", "target_transformation_expression"],
                    "properties": {
                      "id": {"type": "string", "pattern": "^[A-Z][A-Z0-9]*(_[A-Z0-9]+)*$"},
                      "target_transformation_expression": {"type": "string", "minLength": 1},
                      "where": {"type": "string", "minLength": 1}
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add prism/internal/contracts/schemas/dab_entity_v1.json
git commit -m "feat(contracts): add dab_entity_v1.json JSON Schema"
```

---

### Task 2: Parsed Go types for DAB contract

**Files:**
- Create: `prism/internal/contracts/focal.go`

- [ ] **Step 1: Write the types**

```go
package contracts

// Focal is the parsed shape of a `contracts/dab/<entity>.yml` file.
type Focal struct {
	Version       int                  `yaml:"version"`
	Entity        FocalIdent           `yaml:"entity"`
	Attributes    []FocalAttribute     `yaml:"attributes"`
	Relationships []FocalRelationship  `yaml:"relationships,omitempty"`
	MappingGroups []FocalMappingGroup  `yaml:"mapping_groups"`
}

type FocalIdent struct {
	ID          string `yaml:"id"`          // UPPER_SNAKE
	Name        string `yaml:"name"`
	Definition  string `yaml:"definition"`
	Description string `yaml:"description,omitempty"`
}

// FocalAttribute is either a single-typed attribute (Type set, Group nil) or
// an atomic-context group (Group set, Type empty). Validation enforces the
// xor; the JSON Schema's oneOf catches it at parse-time.
type FocalAttribute struct {
	ID                 string             `yaml:"id"`          // UPPER_SNAKE
	Definition         string             `yaml:"definition"`
	Description        string             `yaml:"description,omitempty"`
	Type               string             `yaml:"type,omitempty"`
	Group              []FocalGroupMember `yaml:"group,omitempty"`
	EffectiveTimestamp bool               `yaml:"effective_timestamp,omitempty"`
}

type FocalGroupMember struct {
	ID   string `yaml:"id"`   // UPPER_SNAKE; scoped to the parent group
	Type string `yaml:"type"` // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

type FocalRelationship struct {
	ID             string `yaml:"id"`               // UPPER_SNAKE
	Definition     string `yaml:"definition"`
	Description    string `yaml:"description,omitempty"`
	TargetEntityID string `yaml:"target_entity_id"` // UPPER_SNAKE
}

type FocalMappingGroup struct {
	Name                      string             `yaml:"name"` // snake_case
	AllowMultipleIdentifiers  bool               `yaml:"allow_multiple_identifiers,omitempty"`
	Tables                    []FocalMappingTable `yaml:"tables"`
}

type FocalMappingTable struct {
	Source                              string                       `yaml:"source"` // DAS source ID
	Entity                              string                       `yaml:"entity"` // DAS entity ID
	From                                string                       `yaml:"from,omitempty"` // "current" (default) | "historized"
	PrimaryKeys                         []string                     `yaml:"primary_keys"`
	EntityEffectiveTimestampExpression  string                       `yaml:"entity_effective_timestamp_expression,omitempty"`
	Where                               string                       `yaml:"where,omitempty"`
	Attributes                          []FocalMappingAttribute      `yaml:"attributes"`
	Relationships                       []FocalMappingRelationship   `yaml:"relationships,omitempty"`
}

type FocalMappingAttribute struct {
	ID                                     string `yaml:"id"`                                 // UPPER_SNAKE; "OUTER" or "OUTER.INNER"
	TransformationExpression               string `yaml:"transformation_expression"`
	Where                                  string `yaml:"where,omitempty"`
	AttributeEffectiveTimestampExpression  string `yaml:"attribute_effective_timestamp_expression,omitempty"`
}

type FocalMappingRelationship struct {
	ID                            string `yaml:"id"`
	TargetTransformationExpression string `yaml:"target_transformation_expression"`
	Where                          string `yaml:"where,omitempty"`
}

// FromOrDefault returns From or "current" when From is unset.
func (t FocalMappingTable) FromOrDefault() string {
	if t.From == "" {
		return "current"
	}
	return t.From
}
```

- [ ] **Step 2: Build to confirm syntax**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./internal/contracts/...
```

Expected: no output (success).

- [ ] **Step 3: Commit**

```bash
git add prism/internal/contracts/focal.go
git commit -m "feat(contracts): add parsed-Go types for DAB focal contracts"
```

---

### Task 3: Loader for DAB contracts

**Files:**
- Create: `prism/internal/contracts/focal_loader.go`
- Create: `prism/internal/contracts/focal_loader_test.go`
- Create: `prism/testdata/contracts/valid/dab/customer.yml`

- [ ] **Step 1: Write a fixture for the test**

Create `prism/testdata/contracts/valid/dab/customer.yml`:

```yaml
version: 1

entity:
  id: CUSTOMER
  name: CUSTOMER
  definition: "A customer account"

attributes:
  - id: CUSTOMER_NAME
    definition: "Display name of the customer"
    type: STRING
    effective_timestamp: true

  - id: CUSTOMER_LIFETIME_VALUE
    definition: "Total monetary value to date"
    effective_timestamp: true
    group:
      - id: AMOUNT
        type: NUMBER
      - id: CURRENCY
        type: UNIT

mapping_groups:
  - name: adventure_works
    tables:
      - source: adventure_works
        entity: customer
        from: current
        primary_keys:
          - "'CUSTOMER:' || CAST(customer_id AS VARCHAR)"
        entity_effective_timestamp_expression: modified_date
        attributes:
          - id: CUSTOMER_NAME
            transformation_expression: company_name
          - id: CUSTOMER_LIFETIME_VALUE.AMOUNT
            transformation_expression: lifetime_value
          - id: CUSTOMER_LIFETIME_VALUE.CURRENCY
            transformation_expression: "'USD'"
```

- [ ] **Step 2: Write the failing test**

Create `prism/internal/contracts/focal_loader_test.go`:

```go
package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestLoadFocal_HappyPath(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab", "customer.yml")
	f, err := contracts.LoadFocal(path)
	require.NoError(t, err)
	require.Equal(t, 1, f.Version)
	require.Equal(t, "CUSTOMER", f.Entity.ID)
	require.Len(t, f.Attributes, 2)
	require.Equal(t, "STRING", f.Attributes[0].Type)
	require.Empty(t, f.Attributes[0].Group)
	require.Empty(t, f.Attributes[1].Type)
	require.Len(t, f.Attributes[1].Group, 2)
	require.Len(t, f.MappingGroups, 1)
	require.Equal(t, "adventure_works", f.MappingGroups[0].Name)
	require.Len(t, f.MappingGroups[0].Tables, 1)
	require.Equal(t, "current", f.MappingGroups[0].Tables[0].FromOrDefault())
}

func TestLoadAllDab_WalksDirectory(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab")
	bs, err := contracts.LoadAllDab(dir)
	require.NoError(t, err)
	require.Len(t, bs, 1)
	require.Equal(t, "customer", bs[0].EntityID)
	require.Equal(t, "CUSTOMER", bs[0].Focal.Entity.ID)
}
```

- [ ] **Step 3: Run the test (expect FAIL — function not defined)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run 'TestLoadFocal_HappyPath|TestLoadAllDab_WalksDirectory' -v
```

Expected: FAIL with `undefined: contracts.LoadFocal` (and `LoadAllDab`).

- [ ] **Step 4: Implement the loader**

Create `prism/internal/contracts/focal_loader.go`:

```go
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
```

- [ ] **Step 5: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run 'TestLoadFocal_HappyPath|TestLoadAllDab_WalksDirectory' -v
```

Expected: PASS for both subtests.

- [ ] **Step 6: Commit**

```bash
git add prism/internal/contracts/focal_loader.go prism/internal/contracts/focal_loader_test.go prism/testdata/contracts/valid/dab/customer.yml
git commit -m "feat(contracts): add LoadFocal and LoadAllDab"
```

---

### Task 4: Schema validation + cross-field rules for DAB contracts

**Files:**
- Create: `prism/internal/contracts/focal_validate.go`
- Create: `prism/internal/contracts/focal_validate_test.go`
- Modify: `prism/internal/contracts/validate.go` (add `init()` registration)
- Create: invalid fixtures under `prism/testdata/contracts/invalid/dab/`

- [ ] **Step 1: Add the schema-load init in `validate.go`**

Modify `prism/internal/contracts/validate.go`. After the existing `entitySchema` declaration and before `init()`, add a focal schema variable and load it:

```go
var (
	sourceSchema *jsonschema.Schema
	entitySchema *jsonschema.Schema
	focalSchema  *jsonschema.Schema
)

func init() {
	sourceSchema = mustCompile("schemas/das_source_v1.json")
	entitySchema = mustCompile("schemas/das_entity_v1.json")
	focalSchema  = mustCompile("schemas/dab_entity_v1.json")
}
```

- [ ] **Step 2: Create the four invalid fixtures**

Create `prism/testdata/contracts/invalid/dab/duplicate_attribute_id.yml`:

```yaml
version: 1
entity: {id: CUSTOMER, name: CUSTOMER, definition: "A customer"}
attributes:
  - {id: NAME, definition: "x", type: STRING}
  - {id: NAME, definition: "y", type: STRING}
mapping_groups:
  - name: aw
    tables:
      - source: adventure_works
        entity: customer
        primary_keys: ["customer_id"]
        attributes:
          - {id: NAME, transformation_expression: company_name}
```

Create `prism/testdata/contracts/invalid/dab/group_partial_binding.yml`:

```yaml
version: 1
entity: {id: CUSTOMER, name: CUSTOMER, definition: "A customer"}
attributes:
  - id: CUSTOMER_LIFETIME_VALUE
    definition: "x"
    group:
      - {id: AMOUNT, type: NUMBER}
      - {id: CURRENCY, type: UNIT}
mapping_groups:
  - name: aw
    tables:
      - source: adventure_works
        entity: customer
        primary_keys: ["customer_id"]
        attributes:
          # binds AMOUNT but not CURRENCY -- partial group is illegal
          - {id: CUSTOMER_LIFETIME_VALUE.AMOUNT, transformation_expression: lifetime_value}
```

Create `prism/testdata/contracts/invalid/dab/group_duplicate_type.yml`:

```yaml
version: 1
entity: {id: CUSTOMER, name: CUSTOMER, definition: "A customer"}
attributes:
  - id: TWO_STRINGS
    definition: "x"
    group:
      - {id: A, type: STRING}
      - {id: B, type: STRING}   # two STRINGs in one group: forbidden
mapping_groups:
  - name: aw
    tables:
      - source: adventure_works
        entity: customer
        primary_keys: ["customer_id"]
        attributes:
          - {id: TWO_STRINGS.A, transformation_expression: a}
          - {id: TWO_STRINGS.B, transformation_expression: b}
```

Create `prism/testdata/contracts/invalid/dab/multi_identifier_unsupported.yml`:

```yaml
version: 1
entity: {id: CUSTOMER, name: CUSTOMER, definition: "A customer"}
attributes:
  - {id: CUSTOMER_NAME, definition: "x", type: STRING}
mapping_groups:
  - name: aw
    allow_multiple_identifiers: true   # M2 rejects this
    tables:
      - source: adventure_works
        entity: customer
        primary_keys: ["customer_id"]
        attributes:
          - {id: CUSTOMER_NAME, transformation_expression: company_name}
```

- [ ] **Step 3: Write the failing test**

Create `prism/internal/contracts/focal_validate_test.go`:

```go
package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestValidateFocal_HappyPath(t *testing.T) {
	f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "valid", "dab", "customer.yml"))
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))
}

func TestValidateFocal_RejectsInvalid(t *testing.T) {
	cases := []struct {
		file string
		want string // substring expected in the error message
	}{
		{"duplicate_attribute_id.yml", "duplicate attribute id"},
		{"group_partial_binding.yml", "partial group binding"},
		{"group_duplicate_type.yml", "duplicate type"},
		{"multi_identifier_unsupported.yml", "allow_multiple_identifiers: true is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "invalid", "dab", tc.file))
			require.NoError(t, err) // parse must succeed; validation rejects
			err = contracts.ValidateFocal(f)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.want)
		})
	}
}
```

- [ ] **Step 4: Run the test (expect FAIL — function not defined)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run TestValidateFocal -v
```

Expected: FAIL with `undefined: contracts.ValidateFocal`.

- [ ] **Step 5: Implement validation**

Create `prism/internal/contracts/focal_validate.go`:

```go
package contracts

import (
	"fmt"
	"strings"
)

// ValidateFocal runs JSON-Schema validation followed by cross-field rules
// (atomic-group completeness, mapping coverage, EFF_TMSTP agreement, etc.).
func ValidateFocal(f *Focal) error {
	v, err := toJSONValue(f)
	if err != nil {
		return err
	}
	if err := focalSchema.Validate(v); err != nil {
		return fmt.Errorf("focal schema: %w", err)
	}
	// Attribute ID uniqueness; group inner-ID uniqueness; group type uniqueness.
	seenAttr := map[string]bool{}
	groupMembers := map[string]map[string]string{} // outerID -> innerID -> type
	for _, a := range f.Attributes {
		if seenAttr[a.ID] {
			return fmt.Errorf("duplicate attribute id %q", a.ID)
		}
		seenAttr[a.ID] = true
		if len(a.Group) > 0 {
			members := map[string]string{}
			seenType := map[string]bool{}
			for _, m := range a.Group {
				if _, ok := members[m.ID]; ok {
					return fmt.Errorf("attribute %q: duplicate inner id %q", a.ID, m.ID)
				}
				if seenType[m.Type] {
					return fmt.Errorf("attribute %q: duplicate type %q in group", a.ID, m.Type)
				}
				members[m.ID] = m.Type
				seenType[m.Type] = true
			}
			groupMembers[a.ID] = members
		} else {
			// Single-type: synthesize a one-member group keyed by the outer ID.
			groupMembers[a.ID] = map[string]string{a.ID: a.Type}
		}
	}
	// Relationship ID uniqueness.
	seenRel := map[string]bool{}
	for _, r := range f.Relationships {
		if seenRel[r.ID] {
			return fmt.Errorf("duplicate relationship id %q", r.ID)
		}
		seenRel[r.ID] = true
	}
	// Mapping group rules.
	for _, mg := range f.MappingGroups {
		if mg.AllowMultipleIdentifiers {
			return fmt.Errorf("mapping group %q: allow_multiple_identifiers: true is not supported in M2", mg.Name)
		}
		for ti, t := range mg.Tables {
			if err := validateTable(f, mg.Name, ti, t, groupMembers, seenRel); err != nil {
				return err
			}
		}
	}
	// Mapping completeness: warn if any model attribute or relationship is unbound.
	// (We only error on partial group binding; full unbound attributes are warnings,
	// surfaced at validate-time via the CLI. Plan-time validation does not lower
	// to warnings here; the CLI prints them.)
	return nil
}

// validateTable enforces per-table cross-field rules.
func validateTable(
	f *Focal,
	groupName string, tableIdx int, t FocalMappingTable,
	groupMembers map[string]map[string]string,
	seenRel map[string]bool,
) error {
	// Bound members per outer-attribute id, plus their attribute_eff_tmstp_expression.
	bound := map[string]map[string]string{}    // outerID -> innerID -> bound (anything truthy)
	effExprs := map[string]map[string]string{} // outerID -> innerID -> per-attr eff_tmstp
	for ai, a := range t.Attributes {
		outer, inner, hasDot := strings.Cut(a.ID, ".")
		if !hasDot {
			inner = outer // single-type alias
		}
		members, ok := groupMembers[outer]
		if !ok {
			return fmt.Errorf("mapping_groups[%s].tables[%d].attributes[%d]: unknown attribute id %q", groupName, tableIdx, ai, a.ID)
		}
		if _, ok := members[inner]; !ok {
			return fmt.Errorf("mapping_groups[%s].tables[%d].attributes[%d]: unknown inner id %q for attribute %q", groupName, tableIdx, ai, inner, outer)
		}
		if bound[outer] == nil {
			bound[outer] = map[string]string{}
			effExprs[outer] = map[string]string{}
		}
		bound[outer][inner] = "y"
		if a.AttributeEffectiveTimestampExpression != "" {
			effExprs[outer][inner] = a.AttributeEffectiveTimestampExpression
		}
	}
	// Atomic-group completeness: for every bound outer, all members must be bound.
	for outer, members := range bound {
		want := groupMembers[outer]
		for innerID := range want {
			if _, ok := members[innerID]; !ok {
				return fmt.Errorf("mapping_groups[%s].tables[%d]: partial group binding of %q — inner id %q is unbound", groupName, tableIdx, outer, innerID)
			}
		}
		// EFF_TMSTP agreement: all per-attribute expressions for the same group must be string-equal.
		var seen string
		for _, expr := range effExprs[outer] {
			if seen == "" {
				seen = expr
				continue
			}
			if expr != seen {
				return fmt.Errorf("mapping_groups[%s].tables[%d]: attribute %q has inconsistent attribute_effective_timestamp_expression across inner members", groupName, tableIdx, outer)
			}
		}
	}
	// Relationships: each bound id must exist in the model relationships.
	for ri, r := range t.Relationships {
		if !seenRel[r.ID] {
			return fmt.Errorf("mapping_groups[%s].tables[%d].relationships[%d]: relationship id %q not declared at model level", groupName, tableIdx, ri, r.ID)
		}
	}
	return nil
}
```

- [ ] **Step 6: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run TestValidateFocal -v
```

Expected: PASS for all five subtests (one happy + four invalid).

- [ ] **Step 7: Commit**

```bash
git add prism/internal/contracts/focal_validate.go prism/internal/contracts/focal_validate_test.go prism/internal/contracts/validate.go prism/testdata/contracts/invalid/dab/
git commit -m "feat(contracts): validate DAB focal contracts (schema + cross-field)"
```

---

### Task 5: Cross-layer validation (DAS source/entity references resolve)

**Files:**
- Create: `prism/internal/contracts/crosslayer.go`
- Create: `prism/internal/contracts/crosslayer_test.go`
- Create: `prism/testdata/contracts/valid/das/adventure_works/_source.yml` (if not present — re-use M1 fixture)

- [ ] **Step 1: Verify the DAS fixture already exists**

```bash
ls /home/user/declarative-data-architecture/prism/testdata/contracts/valid/das/adventure_works/
```

Expected: `_source.yml` and `customer.yml` already present (M1 fixture).

- [ ] **Step 2: Write the failing test**

Create `prism/internal/contracts/crosslayer_test.go`:

```go
package contracts_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
)

func TestCrossLayer_HappyPath(t *testing.T) {
	dasDir := filepath.Join("..", "..", "testdata", "contracts", "valid", "das")
	dabDir := filepath.Join("..", "..", "testdata", "contracts", "valid", "dab")
	dasBs, err := contracts.LoadAll(dasDir)
	require.NoError(t, err)
	dabBs, err := contracts.LoadAllDab(dabDir)
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateCrossLayer(dasBs, dabBs))
}

func TestCrossLayer_RejectsUnknownSource(t *testing.T) {
	dasBs, err := contracts.LoadAll(filepath.Join("..", "..", "testdata", "contracts", "valid", "das"))
	require.NoError(t, err)
	bad := &contracts.FocalBundle{
		EntityID: "customer",
		Path:     "<test>",
		Focal: &contracts.Focal{
			Version: 1,
			Entity:  contracts.FocalIdent{ID: "CUSTOMER", Name: "CUSTOMER", Definition: "x"},
			Attributes: []contracts.FocalAttribute{{ID: "X", Definition: "x", Type: "STRING"}},
			MappingGroups: []contracts.FocalMappingGroup{{
				Name: "aw",
				Tables: []contracts.FocalMappingTable{{
					Source:      "no_such_source",
					Entity:      "customer",
					PrimaryKeys: []string{"customer_id"},
					Attributes:  []contracts.FocalMappingAttribute{{ID: "X", TransformationExpression: "company_name"}},
				}},
			}},
		},
	}
	err = contracts.ValidateCrossLayer(dasBs, []*contracts.FocalBundle{bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown DAS source")
}

func TestCrossLayer_RejectsUnknownTargetEntity(t *testing.T) {
	dasBs, err := contracts.LoadAll(filepath.Join("..", "..", "testdata", "contracts", "valid", "das"))
	require.NoError(t, err)
	bad := &contracts.FocalBundle{
		EntityID: "customer",
		Path:     "<test>",
		Focal: &contracts.Focal{
			Version: 1,
			Entity:  contracts.FocalIdent{ID: "CUSTOMER", Name: "CUSTOMER", Definition: "x"},
			Attributes: []contracts.FocalAttribute{{ID: "X", Definition: "x", Type: "STRING"}},
			Relationships: []contracts.FocalRelationship{
				{ID: "PLACES_GHOST", Definition: "x", TargetEntityID: "GHOST"},
			},
			MappingGroups: []contracts.FocalMappingGroup{{
				Name: "aw",
				Tables: []contracts.FocalMappingTable{{
					Source: "adventure_works", Entity: "customer",
					PrimaryKeys: []string{"customer_id"},
					Attributes:  []contracts.FocalMappingAttribute{{ID: "X", TransformationExpression: "company_name"}},
				}},
			}},
		},
	}
	err = contracts.ValidateCrossLayer(dasBs, []*contracts.FocalBundle{bad})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown target_entity_id")
}
```

- [ ] **Step 3: Run the test (expect FAIL)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run TestCrossLayer -v
```

Expected: FAIL with `undefined: contracts.ValidateCrossLayer`.

- [ ] **Step 4: Implement cross-layer validation**

Create `prism/internal/contracts/crosslayer.go`:

```go
package contracts

import "fmt"

// ValidateCrossLayer enforces references between DAS bundles and DAB focal
// bundles: every mapping_groups[].tables[].source and entity must resolve to a
// loaded DAS contract; every focal relationship's target_entity_id must
// resolve to a loaded focal entity.id.
func ValidateCrossLayer(das []*SourceBundle, dab []*FocalBundle) error {
	dasIdx := map[string]map[string]bool{}
	for _, b := range das {
		ents := map[string]bool{}
		for _, e := range b.Entities {
			ents[e.EntityID] = true
		}
		dasIdx[b.SourceID] = ents
	}
	focalIdx := map[string]bool{}
	for _, b := range dab {
		focalIdx[b.Focal.Entity.ID] = true
	}
	for _, b := range dab {
		f := b.Focal
		for _, mg := range f.MappingGroups {
			for ti, t := range mg.Tables {
				ents, ok := dasIdx[t.Source]
				if !ok {
					return fmt.Errorf("focal %s: mapping_groups[%s].tables[%d]: unknown DAS source %q", b.EntityID, mg.Name, ti, t.Source)
				}
				if !ents[t.Entity] {
					return fmt.Errorf("focal %s: mapping_groups[%s].tables[%d]: DAS source %q has no entity %q", b.EntityID, mg.Name, ti, t.Source, t.Entity)
				}
			}
		}
		for _, r := range f.Relationships {
			if !focalIdx[r.TargetEntityID] {
				return fmt.Errorf("focal %s: relationship %q: unknown target_entity_id %q (no focal declares this id)", b.EntityID, r.ID, r.TargetEntityID)
			}
		}
	}
	return nil
}
```

- [ ] **Step 5: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/contracts/ -run TestCrossLayer -v
```

Expected: PASS for all three subtests.

- [ ] **Step 6: Commit**

```bash
git add prism/internal/contracts/crosslayer.go prism/internal/contracts/crosslayer_test.go
git commit -m "feat(contracts): add ValidateCrossLayer for DAS/DAB references"
```

---

## Phase 1: Pure-Go execution-plan builder (`internal/dab`)

### Task 6: TYPE_KEY hashing + canonical-IDFR helpers

**Files:**
- Create: `prism/internal/dab/hash.go`
- Create: `prism/internal/dab/hash_test.go`

- [ ] **Step 1: Write the failing test**

Create `prism/internal/dab/hash_test.go`:

```go
package dab_test

import (
	"crypto/md5"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/dab"
)

func TestTypeKeyHex_DescriptorType(t *testing.T) {
	got := dab.TypeKeyHex("CUSTOMER", "CUSTOMER_LIFETIME_VALUE")
	want := md5sum("CUSTOMER:CUSTOMER_LIFETIME_VALUE")
	require.Equal(t, want, got)
	require.Len(t, got, 32)
}

func TestTypeKeyHex_RelationshipType(t *testing.T) {
	got := dab.TypeKeyHex("CUSTOMER", "CUSTOMER_PLACES_ORDER")
	want := md5sum("CUSTOMER:CUSTOMER_PLACES_ORDER")
	require.Equal(t, want, got)
}

func TestCanonicalIDFRExpr_SingleKey(t *testing.T) {
	expr := dab.CanonicalIDFRExpr([]string{"customer_id"})
	require.Equal(t, "CAST((customer_id) AS VARCHAR)", expr)
}

func TestCanonicalIDFRExpr_MultiKey(t *testing.T) {
	expr := dab.CanonicalIDFRExpr([]string{"order_id", "line_no"})
	require.Equal(t, "CAST((order_id) AS VARCHAR) || '||__||' || CAST((line_no) AS VARCHAR)", expr)
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
```

- [ ] **Step 2: Run the test (expect FAIL)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run 'TestTypeKeyHex|TestCanonicalIDFRExpr' -v
```

Expected: FAIL with `no such file or directory` or `package dab is not in std` (the package doesn't exist yet).

- [ ] **Step 3: Implement the helpers**

Create `prism/internal/dab/hash.go`:

```go
// Package dab translates parsed DAB focal contracts into engine specs and
// SQL execution plans, and runs them against an engine.Engine.
package dab

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// IDFRSeparator joins multi-part primary_keys[] values into one canonical
// IDFR string. Chosen to avoid collisions with snake_case content and the
// SQL `||` operator characters appearing in raw data.
const IDFRSeparator = "||__||"

// TypeKeyHex returns the 32-char hex MD5 of "<entityID>:<attrOrRelID>".
// Used as the deterministic TYPE_KEY value for descriptors and relationships.
func TypeKeyHex(entityID, attrOrRelID string) string {
	h := md5.Sum([]byte(entityID + ":" + attrOrRelID))
	return hex.EncodeToString(h[:])
}

// CanonicalIDFRExpr produces a SQL expression that evaluates to the canonical
// IDFR string for one DAS row. Each primary_keys[] entry is wrapped in
// parentheses, cast to VARCHAR, and joined with IDFRSeparator. Single-element
// lists do not insert the separator.
func CanonicalIDFRExpr(primaryKeys []string) string {
	parts := make([]string, len(primaryKeys))
	for i, k := range primaryKeys {
		parts[i] = "CAST((" + k + ") AS VARCHAR)"
	}
	return strings.Join(parts, " || '"+IDFRSeparator+"' || ")
}
```

- [ ] **Step 4: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run 'TestTypeKeyHex|TestCanonicalIDFRExpr' -v
```

Expected: PASS for all four subtests.

- [ ] **Step 5: Commit**

```bash
git add prism/internal/dab/hash.go prism/internal/dab/hash_test.go
git commit -m "feat(dab): TYPE_KEY hashing and canonical-IDFR expression helpers"
```

---

### Task 7: Build execution plan from parsed contracts

**Files:**
- Create: `prism/internal/dab/plan.go`
- Create: `prism/internal/dab/plan_test.go`

The execution plan is a tree of pure-data structs that the `execute` step
later renders to SQL via the engine dialect. Building the plan is fully
deterministic and pure-Go (no DB), which makes it cheap to TDD.

- [ ] **Step 1: Write the failing test**

Create `prism/internal/dab/plan_test.go`:

```go
package dab_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
)

func TestBuildPlan_FromCustomerFixture(t *testing.T) {
	f, err := contracts.LoadFocal(filepath.Join("..", "..", "testdata", "contracts", "valid", "dab", "customer.yml"))
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))

	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{
		EntityID: "customer", Path: "ignored", Focal: f,
	})
	require.NoError(t, err)
	require.Equal(t, "customer", plan.Entity)

	// One DDL per table type:
	require.NotZero(t, plan.IDFR)
	require.NotZero(t, plan.Focal)
	require.NotZero(t, plan.Descriptor)
	require.Empty(t, plan.Relationships) // none in fixture

	// One mapping plan per (mapping_group, table) pair.
	require.Len(t, plan.Mappings, 1)
	m := plan.Mappings[0]
	require.Equal(t, "adventure_works", m.MappingGroup)
	require.Equal(t, "adventure_works", m.SourceID)
	require.Equal(t, "customer", m.SourceEntity)
	require.Equal(t, "current", m.From)
	require.Contains(t, m.IDFRExpr, "'CUSTOMER:'")
	require.Contains(t, m.EffTmstpExpr, "modified_date")

	// Descriptor mappings: one per outer attribute (CUSTOMER_NAME single-type;
	// CUSTOMER_LIFETIME_VALUE atomic group with two members).
	require.Len(t, m.Descriptors, 2)
	byAttr := map[string]dab.DescriptorMapping{}
	for _, d := range m.Descriptors {
		byAttr[d.AttrID] = d
	}
	name := byAttr["CUSTOMER_NAME"]
	require.Equal(t, "company_name", name.ValStrExpr)
	require.Empty(t, name.ValNumExpr)

	clv := byAttr["CUSTOMER_LIFETIME_VALUE"]
	require.Equal(t, "lifetime_value", clv.ValNumExpr)
	require.Equal(t, "'USD'", clv.UomExpr)
	require.Empty(t, clv.ValStrExpr)

	// Per-group typed view inputs.
	require.Len(t, plan.GroupViews, 2)
	require.Len(t, plan.EntityCurrentView.Attributes, 2)
}
```

- [ ] **Step 2: Run the test (expect FAIL — function not defined)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestBuildPlan -v
```

Expected: FAIL with `undefined: dab.BuildEntityPlan`.

- [ ] **Step 3: Implement the plan builder**

Create `prism/internal/dab/plan.go`:

```go
package dab

import (
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/naming"
)

// SchemaName is the single conformed schema where all DAB objects live.
const SchemaName = "dab"

// EntityPlan is the full set of operations needed to build one focal entity.
// Operations run in the order: DDLs → per-mapping merges → recompute → views.
type EntityPlan struct {
	Entity        string                 // lower-snake from filename (== focal id lower-cased)
	EntityID      string                 // upper-snake (focal.entity.id, used in TYPE_KEY hashing)
	IDFR          IDFRDDL                // create idfr table
	Focal         FocalDDL               // create focal table
	Descriptor    DescriptorDDL          // create descriptor table
	Relationships []RelationshipDDL      // one per declared relationship
	Mappings      []MappingPlan          // one per (mapping_group, table)
	GroupViews    []GroupViewPlan        // one per outer attribute
	EntityCurrentView EntityCurrentViewPlan
}

type IDFRDDL struct{ Schema, Entity string }
type FocalDDL struct{ Schema, Entity string }
type DescriptorDDL struct{ Schema, Entity string }

type RelationshipDDL struct {
	Schema       string
	Entity       string
	Related      string // lower-snake from focal.relationships[].target_entity_id
	Suffix       string // "" or "_<rel_id_lower>" for disambiguation
}

// MappingPlan describes one (mapping_group, table) contribution. The execute
// step builds a DAS-reading CTE from this plan and then emits idfr/focal/
// descriptor/relationship merges from it.
type MappingPlan struct {
	MappingGroup       string                  // INST_KEY value
	SourceID           string                  // DAS source id (lower-snake)
	SourceEntity       string                  // DAS entity id (lower-snake)
	From               string                  // "current" or "historized"
	InstRowKey         string                  // "<source>.<entity>"
	IDFRExpr           string                  // SQL: canonical IDFR string per row
	EffTmstpExpr       string                  // SQL: TIMESTAMP per row (table-level default)
	Where              string                  // SQL: optional table-level WHERE; empty when none
	Descriptors        []DescriptorMapping     // one per outer attribute bound by this table
	Relationships      []RelationshipMapping   // one per relationship bound by this table
}

type DescriptorMapping struct {
	AttrID         string // upper-snake outer attribute id (used for TYPE_KEY)
	TypeKeyHex     string // 32-char MD5
	EffTmstpExpr   string // SQL: per-group EFF_TMSTP (overrides MappingPlan.EffTmstpExpr if set)
	ValStrExpr     string // SQL or "" if not used in this group
	ValNumExpr     string
	UomExpr        string
	StaTmstpExpr   string
	EndTmstpExpr   string
	Where          string // combined per-attribute WHERE clauses (AND-joined); empty when none
}

type RelationshipMapping struct {
	RelID                string // upper-snake relationship id
	TypeKeyHex           string
	Related              string // lower-snake target focal id
	Suffix               string // "" or "_<rel_id_lower>"
	TargetExpr           string // SQL: target focal's canonical IDFR string
	EffTmstpExpr         string
	Where                string
}

// GroupViewPlan is the input for one per-group typed view.
type GroupViewPlan struct {
	Schema     string
	Entity     string
	AttrID     string // lower-snake (used in view name)
	TypeKeyHex string
	Members    []GroupViewMember
}

type GroupViewMember struct {
	InnerID string // lower-snake (column name in the view)
	Type    string // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

// EntityCurrentViewPlan is the input for the per-entity __current view.
type EntityCurrentViewPlan struct {
	Schema     string
	Entity     string
	Attributes []EntityCurrentAttribute
}

type EntityCurrentAttribute struct {
	AttrID  string            // lower-snake outer
	Members []GroupViewMember // for atomic groups; for single-type, one member with InnerID == AttrID
}

// BuildEntityPlan compiles a parsed focal contract into an EntityPlan.
func BuildEntityPlan(b *contracts.FocalBundle) (*EntityPlan, error) {
	ent := b.EntityID
	entUp := b.Focal.Entity.ID

	plan := &EntityPlan{
		Entity:     ent,
		EntityID:   entUp,
		IDFR:       IDFRDDL{Schema: SchemaName, Entity: ent},
		Focal:      FocalDDL{Schema: SchemaName, Entity: ent},
		Descriptor: DescriptorDDL{Schema: SchemaName, Entity: ent},
		EntityCurrentView: EntityCurrentViewPlan{Schema: SchemaName, Entity: ent},
	}

	// Build a quick model index: outer attribute id -> list of (inner id, type).
	model := buildAttributeIndex(b.Focal.Attributes)

	// Group views + current view: one entry per outer attribute regardless of binding.
	for _, a := range b.Focal.Attributes {
		gv := GroupViewPlan{
			Schema:     SchemaName,
			Entity:     ent,
			AttrID:     strings.ToLower(a.ID),
			TypeKeyHex: TypeKeyHex(entUp, a.ID),
		}
		ec := EntityCurrentAttribute{AttrID: strings.ToLower(a.ID)}
		if len(a.Group) == 0 {
			gv.Members = []GroupViewMember{{InnerID: strings.ToLower(a.ID), Type: a.Type}}
			ec.Members = gv.Members
		} else {
			for _, m := range a.Group {
				gm := GroupViewMember{InnerID: strings.ToLower(m.ID), Type: m.Type}
				gv.Members = append(gv.Members, gm)
				ec.Members = append(ec.Members, gm)
			}
		}
		plan.GroupViews = append(plan.GroupViews, gv)
		plan.EntityCurrentView.Attributes = append(plan.EntityCurrentView.Attributes, ec)
	}

	// Relationships: one DDL per declared relationship; the suffix disambiguates
	// when one focal has multiple relationships to the same target.
	relCountByTarget := map[string]int{}
	for _, r := range b.Focal.Relationships {
		relCountByTarget[r.TargetEntityID]++
	}
	for _, r := range b.Focal.Relationships {
		related := strings.ToLower(r.TargetEntityID)
		suffix := ""
		if relCountByTarget[r.TargetEntityID] > 1 {
			suffix = "_" + strings.ToLower(r.ID)
		}
		plan.Relationships = append(plan.Relationships, RelationshipDDL{
			Schema: SchemaName, Entity: ent, Related: related, Suffix: suffix,
		})
	}

	// Mappings: one MappingPlan per (mapping_group, table).
	for _, mg := range b.Focal.MappingGroups {
		for _, t := range mg.Tables {
			mp, err := buildMappingPlan(entUp, mg.Name, t, model, b.Focal.Relationships, relCountByTarget)
			if err != nil {
				return nil, err
			}
			plan.Mappings = append(plan.Mappings, mp)
		}
	}
	return plan, nil
}

type modelIndex struct {
	outers map[string]struct {
		Attr    contracts.FocalAttribute
		Members map[string]string // inner id -> type (single-type uses one member with id=outer.id)
	}
}

func buildAttributeIndex(attrs []contracts.FocalAttribute) modelIndex {
	idx := modelIndex{outers: map[string]struct {
		Attr    contracts.FocalAttribute
		Members map[string]string
	}{}}
	for _, a := range attrs {
		members := map[string]string{}
		if len(a.Group) > 0 {
			for _, m := range a.Group {
				members[m.ID] = m.Type
			}
		} else {
			members[a.ID] = a.Type
		}
		idx.outers[a.ID] = struct {
			Attr    contracts.FocalAttribute
			Members map[string]string
		}{a, members}
	}
	return idx
}

func buildMappingPlan(
	entUp, groupName string,
	t contracts.FocalMappingTable,
	model modelIndex,
	rels []contracts.FocalRelationship,
	relCountByTarget map[string]int,
) (MappingPlan, error) {
	mp := MappingPlan{
		MappingGroup: groupName,
		SourceID:     t.Source,
		SourceEntity: t.Entity,
		From:         t.FromOrDefault(),
		InstRowKey:   t.Source + "." + t.Entity,
		IDFRExpr:     CanonicalIDFRExpr(t.PrimaryKeys),
		Where:        t.Where,
	}
	if t.EntityEffectiveTimestampExpression != "" {
		mp.EffTmstpExpr = t.EntityEffectiveTimestampExpression
	} else {
		mp.EffTmstpExpr = "_loaded_at"
	}

	// Group bindings: collect per outer attribute.
	type groupBinding struct {
		Outer            string
		Members          map[string]string // inner -> transformation_expression
		Where            []string          // per-inner where clauses
		EffTmstp         string            // per-group EFF_TMSTP override (or "")
	}
	bindings := map[string]*groupBinding{}
	for _, a := range t.Attributes {
		outer, inner, hasDot := strings.Cut(a.ID, ".")
		if !hasDot {
			inner = outer
		}
		if _, ok := model.outers[outer]; !ok {
			return mp, fmt.Errorf("mapping_groups[%s].tables[%s.%s].attributes: unknown attribute id %q", groupName, t.Source, t.Entity, a.ID)
		}
		gb := bindings[outer]
		if gb == nil {
			gb = &groupBinding{Outer: outer, Members: map[string]string{}}
			bindings[outer] = gb
		}
		gb.Members[inner] = a.TransformationExpression
		if a.Where != "" {
			gb.Where = append(gb.Where, a.Where)
		}
		if a.AttributeEffectiveTimestampExpression != "" {
			gb.EffTmstp = a.AttributeEffectiveTimestampExpression
		}
	}

	for _, gb := range bindings {
		entry := model.outers[gb.Outer]
		dm := DescriptorMapping{
			AttrID:     gb.Outer,
			TypeKeyHex: TypeKeyHex(entUp, gb.Outer),
		}
		if gb.EffTmstp != "" {
			dm.EffTmstpExpr = gb.EffTmstp
		} else {
			dm.EffTmstpExpr = mp.EffTmstpExpr
		}
		for innerID, typ := range entry.Members {
			expr, ok := gb.Members[innerID]
			if !ok {
				return mp, fmt.Errorf("internal: partial group %q (validate should have caught this)", gb.Outer)
			}
			switch typ {
			case "STRING":
				dm.ValStrExpr = expr
			case "NUMBER":
				dm.ValNumExpr = expr
			case "UNIT":
				dm.UomExpr = expr
			case "START_TIMESTAMP":
				dm.StaTmstpExpr = expr
			case "END_TIMESTAMP":
				dm.EndTmstpExpr = expr
			default:
				return mp, fmt.Errorf("attribute %s.%s: unknown type %q", gb.Outer, innerID, typ)
			}
		}
		if len(gb.Where) > 0 {
			dm.Where = "(" + strings.Join(gb.Where, ") AND (") + ")"
		}
		mp.Descriptors = append(mp.Descriptors, dm)
	}

	// Relationship bindings.
	relIdx := map[string]contracts.FocalRelationship{}
	for _, r := range rels {
		relIdx[r.ID] = r
	}
	for _, r := range t.Relationships {
		def, ok := relIdx[r.ID]
		if !ok {
			return mp, fmt.Errorf("mapping_groups[%s].tables[%s.%s].relationships: %q not declared", groupName, t.Source, t.Entity, r.ID)
		}
		related := strings.ToLower(def.TargetEntityID)
		suffix := ""
		if relCountByTarget[def.TargetEntityID] > 1 {
			suffix = "_" + strings.ToLower(r.ID)
		}
		mp.Relationships = append(mp.Relationships, RelationshipMapping{
			RelID:        r.ID,
			TypeKeyHex:   TypeKeyHex(entUp, r.ID),
			Related:      related,
			Suffix:       suffix,
			TargetExpr:   r.TargetTransformationExpression,
			EffTmstpExpr: mp.EffTmstpExpr,
			Where:        r.Where,
		})
	}
	_ = naming.ValidateSnakeCaseIdentifier // imported elsewhere; suppress unused if no further use here
	return mp, nil
}
```

- [ ] **Step 4: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestBuildPlan -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add prism/internal/dab/plan.go prism/internal/dab/plan_test.go
git commit -m "feat(dab): build execution plan from parsed focal contracts"
```

---

## Phase 2: Engine spec types + Dialect interface extensions

### Task 8: Add 13 new spec types (+ 2 helper structs) to `engine/spec.go`

**Files:**
- Modify: `prism/internal/engine/spec.go`

- [ ] **Step 1: Append the new spec types**

Open `prism/internal/engine/spec.go`. Append the following at the end of the file (keep M1 types untouched):

```go
// --- M2 (DAB) specs ---------------------------------------------------------

// IdfrTableSpec describes the IDFR audit table for one focal entity.
type IdfrTableSpec struct {
	Schema string // "dab"
	Entity string // lower-snake; produces dab.<entity>__idfr
}

// FocalTableSpec describes the focal table (one row per surrogate key).
type FocalTableSpec struct {
	Schema string
	Entity string // lower-snake; produces dab.<entity>
}

// DescriptorTableSpec describes the generic-EAV descriptor table for a focal.
type DescriptorTableSpec struct {
	Schema string
	Entity string // lower-snake; produces dab.<entity>__descriptor
}

// RelationshipTableSpec describes one relationship table on a focal.
// Suffix is empty unless the same focal has multiple relationships to the same
// target, in which case Suffix is "_<rel_id_lower>".
type RelationshipTableSpec struct {
	Schema  string
	Entity  string // lower-snake source focal
	Related string // lower-snake target focal
	Suffix  string // e.g. "" or "_places_order_alt"
}

// MergeIdfrSpec is one IDFR insert from a (mapping_group, table) contribution.
// SourceCTE is a SELECT that produces, at minimum, columns:
//   <entity>_idfr  VARCHAR
//   eff_tmstp      TIMESTAMP
type MergeIdfrSpec struct {
	Schema       string
	Entity       string
	MappingGroup string // INST_KEY
	InstRowKey   string // INST_ROW_KEY ("<source>.<entity>")
	SourceCTE    string // SELECT expression body, no leading SELECT keyword required
}

// MergeFocalSpec inserts/upserts the focal row for every key in
// dab.<entity>__idfr, refreshing eff_tmstp = MIN(idfr.eff_tmstp).
type MergeFocalSpec struct {
	Schema string
	Entity string
}

// MergeDescriptorSpec inserts descriptor rows for one (mapping_group, table)
// + one outer attribute. SourceCTE columns:
//   <entity>_key   VARCHAR
//   type_key       VARCHAR
//   eff_tmstp      TIMESTAMP
//   val_str        VARCHAR
//   val_num        DOUBLE
//   uom            VARCHAR
//   sta_tmstp      TIMESTAMP
//   end_tmstp      TIMESTAMP
type MergeDescriptorSpec struct {
	Schema       string
	Entity       string
	MappingGroup string
	InstRowKey   string
	SourceCTE    string
}

// MergeRelationshipSpec inserts relationship rows. SourceCTE columns:
//   <entity>_key   VARCHAR
//   <related>_key  VARCHAR
//   type_key       VARCHAR
//   eff_tmstp      TIMESTAMP
type MergeRelationshipSpec struct {
	Schema       string
	Entity       string
	Related      string
	Suffix       string
	MappingGroup string
	InstRowKey   string
	SourceCTE    string
}

// RecomputeIdfrRowStSpec recomputes ROW_ST and SEQ_NBR over the full IDFR
// table. Partition: (entity_key, idfr); order: (eff_tmstp, ver_tmstp).
type RecomputeIdfrRowStSpec struct {
	Schema string
	Entity string
}

// RecomputeDescriptorRowStSpec recomputes over descriptor.
// Partition: (entity_key, type_key); order: (eff_tmstp, ver_tmstp).
type RecomputeDescriptorRowStSpec struct {
	Schema string
	Entity string
}

// RecomputeRelationshipRowStSpec recomputes over a relationship table.
// Partition: (entity_key, related_key, type_key).
type RecomputeRelationshipRowStSpec struct {
	Schema  string
	Entity  string
	Related string
	Suffix  string
}

// GroupViewSpec is one per-group typed view. The view exposes one column per
// member (named by InnerID, lower-snake), projected from the corresponding
// type slot in the descriptor table. Filtered to the matching TYPE_KEY hex.
type GroupViewSpec struct {
	Schema     string
	Entity     string
	AttrID     string // lower-snake (used in view name dab.<entity>__<attrid>)
	TypeKeyHex string // 32-char MD5 hex
	Members    []GroupViewMember
}

type GroupViewMember struct {
	InnerID string // lower-snake; column name in the view
	Type    string // STRING|NUMBER|UNIT|START_TIMESTAMP|END_TIMESTAMP
}

// EntityCurrentViewSpec is the per-entity __current view: focal joined with
// per-group views, ROW_ST='Y' only.
type EntityCurrentViewSpec struct {
	Schema     string
	Entity     string
	Attributes []EntityCurrentAttribute
}

type EntityCurrentAttribute struct {
	AttrID  string            // lower-snake outer
	Members []GroupViewMember // 1+ inner members
}
```

- [ ] **Step 2: Build to confirm**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./internal/engine/...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add prism/internal/engine/spec.go
git commit -m "feat(engine): add DAB spec types (IDFR/Focal/Descriptor/Relationship/views)"
```

---

### Task 9: Extend the Dialect interface with 13 new methods (and wire DuckDB stubs that delegate to renderers)

**Files:**
- Modify: `prism/internal/engine/engine.go`
- Modify: `prism/internal/engine/duckdb/duckdb.go`

- [ ] **Step 1: Add new methods to the Dialect interface**

Open `prism/internal/engine/engine.go`. Replace the `Dialect` interface with:

```go
// Dialect produces engine-specific SQL strings from spec structs. Pure
// rendering — no IO.
type Dialect interface {
	QuoteIdent(name string) string
	Schema(name string) string

	CreateSchemaIfNotExists(schema string) string
	CreateHistorizedTableIfNotExists(spec HistorizedTableSpec) string
	AppendIntoHistorized(spec HistorizedAppendSpec) string
	CreateOrReplaceCurrentView(spec CurrentViewSpec) string

	// M2 — DAB:
	CreateIdfrTableIfNotExists(spec IdfrTableSpec) string
	CreateFocalTableIfNotExists(spec FocalTableSpec) string
	CreateDescriptorTableIfNotExists(spec DescriptorTableSpec) string
	CreateRelationshipTableIfNotExists(spec RelationshipTableSpec) string

	MergeIdfr(spec MergeIdfrSpec) string
	MergeFocal(spec MergeFocalSpec) string
	MergeDescriptor(spec MergeDescriptorSpec) string
	MergeRelationship(spec MergeRelationshipSpec) string

	RecomputeIdfrRowSt(spec RecomputeIdfrRowStSpec) string
	RecomputeDescriptorRowSt(spec RecomputeDescriptorRowStSpec) string
	RecomputeRelationshipRowSt(spec RecomputeRelationshipRowStSpec) string

	CreateOrReplaceGroupView(spec GroupViewSpec) string
	CreateOrReplaceEntityCurrentView(spec EntityCurrentViewSpec) string
}
```

- [ ] **Step 2: Add panic stubs to the DuckDB dialect**

Open `prism/internal/engine/duckdb/duckdb.go`. Below the existing `func (dialect) CreateOrReplaceCurrentView(...) string { ... }`, append:

```go
// --- M2 (DAB) — panic-stubs until templates are wired in Phase 3 -----------

func (dialect) CreateIdfrTableIfNotExists(spec engine.IdfrTableSpec) string {
	s, err := tmpl.RenderCreateIdfr(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateFocalTableIfNotExists(spec engine.FocalTableSpec) string {
	s, err := tmpl.RenderCreateFocal(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateDescriptorTableIfNotExists(spec engine.DescriptorTableSpec) string {
	s, err := tmpl.RenderCreateDescriptor(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateRelationshipTableIfNotExists(spec engine.RelationshipTableSpec) string {
	s, err := tmpl.RenderCreateRelationship(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeIdfr(spec engine.MergeIdfrSpec) string {
	s, err := tmpl.RenderMergeIdfr(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeFocal(spec engine.MergeFocalSpec) string {
	s, err := tmpl.RenderMergeFocal(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeDescriptor(spec engine.MergeDescriptorSpec) string {
	s, err := tmpl.RenderMergeDescriptor(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeRelationship(spec engine.MergeRelationshipSpec) string {
	s, err := tmpl.RenderMergeRelationship(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeIdfrRowSt(spec engine.RecomputeIdfrRowStSpec) string {
	s, err := tmpl.RenderRecomputeIdfrRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeDescriptorRowSt(spec engine.RecomputeDescriptorRowStSpec) string {
	s, err := tmpl.RenderRecomputeDescriptorRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeRelationshipRowSt(spec engine.RecomputeRelationshipRowStSpec) string {
	s, err := tmpl.RenderRecomputeRelationshipRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateOrReplaceGroupView(spec engine.GroupViewSpec) string {
	s, err := tmpl.RenderCreateGroupView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateOrReplaceEntityCurrentView(spec engine.EntityCurrentViewSpec) string {
	s, err := tmpl.RenderCreateEntityCurrentView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
```

- [ ] **Step 3: Build (expect FAIL — Render funcs don't exist yet)**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./internal/engine/...
```

Expected: FAIL with `undefined: tmpl.RenderCreateIdfr` (and 12 more). This is the intended state heading into Phase 3 — Render funcs land in Tasks 10-13.

- [ ] **Step 4: Do NOT commit yet**

The interface is dangling; commit happens after Task 13 when the templates exist and the package builds. Move to Task 10.

---

## Phase 3: SQL templates + Render funcs

### Task 10: DDL templates (4 tables) + Render funcs + golden tests

**Files:**
- Create: `prism/internal/tmpl/duckdb/dab_create_idfr.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_create_focal.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_create_descriptor.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_create_relationship.sql.tmpl`
- Modify: `prism/internal/tmpl/duckdb/render.go` (add 4 Render funcs)
- Modify: `prism/internal/tmpl/duckdb/render_test.go` (add 4 golden tests)
- Create: `prism/internal/tmpl/duckdb/testdata/dab_create_idfr.sql.golden`
- Create: `prism/internal/tmpl/duckdb/testdata/dab_create_focal.sql.golden`
- Create: `prism/internal/tmpl/duckdb/testdata/dab_create_descriptor.sql.golden`
- Create: `prism/internal/tmpl/duckdb/testdata/dab_create_relationship.sql.golden`

- [ ] **Step 1: Write the four DDL templates**

`prism/internal/tmpl/duckdb/dab_create_idfr.sql.tmpl`:

```
CREATE TABLE IF NOT EXISTS {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}   VARCHAR   NOT NULL,
    {{ quote .IdfrCol }}  VARCHAR   NOT NULL,
    eff_tmstp             TIMESTAMP NOT NULL,
    ver_tmstp             TIMESTAMP NOT NULL,
    seq_nbr               BIGINT    NOT NULL,
    row_st                CHAR(1)   NOT NULL,
    data_key              VARCHAR   NOT NULL,
    inst_key              VARCHAR   NOT NULL,
    inst_row_key          VARCHAR   NOT NULL,
    popln_tmstp           TIMESTAMP NOT NULL,
    PRIMARY KEY ({{ quote .KeyCol }}, {{ quote .IdfrCol }}, ver_tmstp)
);
```

`prism/internal/tmpl/duckdb/dab_create_focal.sql.tmpl`:

```
CREATE TABLE IF NOT EXISTS {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}  VARCHAR   NOT NULL PRIMARY KEY,
    eff_tmstp            TIMESTAMP NOT NULL,
    ver_tmstp            TIMESTAMP NOT NULL,
    row_st               CHAR(1)   NOT NULL,
    data_key             VARCHAR   NOT NULL,
    popln_tmstp          TIMESTAMP NOT NULL
);
```

`prism/internal/tmpl/duckdb/dab_create_descriptor.sql.tmpl`:

```
CREATE TABLE IF NOT EXISTS {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}  VARCHAR   NOT NULL,
    type_key             VARCHAR   NOT NULL,
    eff_tmstp            TIMESTAMP NOT NULL,
    ver_tmstp            TIMESTAMP NOT NULL,
    seq_nbr              BIGINT    NOT NULL,
    row_st               CHAR(1)   NOT NULL,
    sta_tmstp            TIMESTAMP,
    end_tmstp            TIMESTAMP,
    val_str              VARCHAR,
    val_num              DOUBLE,
    uom                  VARCHAR,
    data_key             VARCHAR   NOT NULL,
    inst_key             VARCHAR   NOT NULL,
    inst_row_key         VARCHAR   NOT NULL,
    popln_tmstp          TIMESTAMP NOT NULL,
    PRIMARY KEY ({{ quote .KeyCol }}, type_key, eff_tmstp, ver_tmstp)
);
```

`prism/internal/tmpl/duckdb/dab_create_relationship.sql.tmpl`:

```
CREATE TABLE IF NOT EXISTS {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}      VARCHAR   NOT NULL,
    {{ quote .RelatedCol }}  VARCHAR   NOT NULL,
    type_key                 VARCHAR   NOT NULL,
    eff_tmstp                TIMESTAMP NOT NULL,
    ver_tmstp                TIMESTAMP NOT NULL,
    seq_nbr                  BIGINT    NOT NULL,
    row_st                   CHAR(1)   NOT NULL,
    data_key                 VARCHAR   NOT NULL,
    inst_key                 VARCHAR   NOT NULL,
    inst_row_key             VARCHAR   NOT NULL,
    popln_tmstp              TIMESTAMP NOT NULL,
    PRIMARY KEY ({{ quote .KeyCol }}, {{ quote .RelatedCol }}, type_key, eff_tmstp, ver_tmstp)
);
```

- [ ] **Step 2: Add Render funcs to `render.go`**

Open `prism/internal/tmpl/duckdb/render.go`. Append at the end:

```go
// --- M2 (DAB) renderers ----------------------------------------------------

func RenderCreateIdfr(spec engine.IdfrTableSpec) (string, error) {
	return render("dab_create_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
	})
}

func RenderCreateFocal(spec engine.FocalTableSpec) (string, error) {
	return render("dab_create_focal.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity,
		spec.Entity + "_key",
	})
}

func RenderCreateDescriptor(spec engine.DescriptorTableSpec) (string, error) {
	return render("dab_create_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
	})
}

func RenderCreateRelationship(spec engine.RelationshipTableSpec) (string, error) {
	return render("dab_create_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
	})
}
```

- [ ] **Step 3: Add the four golden snapshot files**

Create `prism/internal/tmpl/duckdb/testdata/dab_create_idfr.sql.golden` with the rendered output as expected (no trailing newline differences; `render()` already TrimSpaces):

```
CREATE TABLE IF NOT EXISTS "dab"."customer__idfr" (
    "customer_key"   VARCHAR   NOT NULL,
    "customer_idfr"  VARCHAR   NOT NULL,
    eff_tmstp             TIMESTAMP NOT NULL,
    ver_tmstp             TIMESTAMP NOT NULL,
    seq_nbr               BIGINT    NOT NULL,
    row_st                CHAR(1)   NOT NULL,
    data_key              VARCHAR   NOT NULL,
    inst_key              VARCHAR   NOT NULL,
    inst_row_key          VARCHAR   NOT NULL,
    popln_tmstp           TIMESTAMP NOT NULL,
    PRIMARY KEY ("customer_key", "customer_idfr", ver_tmstp)
);
```

Create `prism/internal/tmpl/duckdb/testdata/dab_create_focal.sql.golden`:

```
CREATE TABLE IF NOT EXISTS "dab"."customer" (
    "customer_key"  VARCHAR   NOT NULL PRIMARY KEY,
    eff_tmstp            TIMESTAMP NOT NULL,
    ver_tmstp            TIMESTAMP NOT NULL,
    row_st               CHAR(1)   NOT NULL,
    data_key             VARCHAR   NOT NULL,
    popln_tmstp          TIMESTAMP NOT NULL
);
```

Create `prism/internal/tmpl/duckdb/testdata/dab_create_descriptor.sql.golden`:

```
CREATE TABLE IF NOT EXISTS "dab"."customer__descriptor" (
    "customer_key"  VARCHAR   NOT NULL,
    type_key             VARCHAR   NOT NULL,
    eff_tmstp            TIMESTAMP NOT NULL,
    ver_tmstp            TIMESTAMP NOT NULL,
    seq_nbr              BIGINT    NOT NULL,
    row_st               CHAR(1)   NOT NULL,
    sta_tmstp            TIMESTAMP,
    end_tmstp            TIMESTAMP,
    val_str              VARCHAR,
    val_num              DOUBLE,
    uom                  VARCHAR,
    data_key             VARCHAR   NOT NULL,
    inst_key             VARCHAR   NOT NULL,
    inst_row_key         VARCHAR   NOT NULL,
    popln_tmstp          TIMESTAMP NOT NULL,
    PRIMARY KEY ("customer_key", type_key, eff_tmstp, ver_tmstp)
);
```

Create `prism/internal/tmpl/duckdb/testdata/dab_create_relationship.sql.golden`:

```
CREATE TABLE IF NOT EXISTS "dab"."customer__order__rel" (
    "customer_key"      VARCHAR   NOT NULL,
    "order_key"  VARCHAR   NOT NULL,
    type_key                 VARCHAR   NOT NULL,
    eff_tmstp                TIMESTAMP NOT NULL,
    ver_tmstp                TIMESTAMP NOT NULL,
    seq_nbr                  BIGINT    NOT NULL,
    row_st                   CHAR(1)   NOT NULL,
    data_key                 VARCHAR   NOT NULL,
    inst_key                 VARCHAR   NOT NULL,
    inst_row_key             VARCHAR   NOT NULL,
    popln_tmstp              TIMESTAMP NOT NULL,
    PRIMARY KEY ("customer_key", "order_key", type_key, eff_tmstp, ver_tmstp)
);
```

(Note: the indentation of the rendered output exactly matches what the Go `text/template` whitespace rules produce when rendering with `quote` substitutions. If the actual rendered string differs, **update the golden file** to match — the rendered output is the source of truth, the golden is the snapshot.)

- [ ] **Step 4: Add the golden tests to `render_test.go`**

Open `prism/internal/tmpl/duckdb/render_test.go`. Add:

```go
func TestRenderCreateIdfr_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateIdfr(engine.IdfrTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_idfr.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateFocal_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateFocal(engine.FocalTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_focal.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateDescriptor_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateDescriptor(engine.DescriptorTableSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_descriptor.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateRelationship_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateRelationship(engine.RelationshipTableSpec{
		Schema: "dab", Entity: "customer", Related: "order",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_relationship.sql.golden")
	require.Equal(t, want, got)
}
```

(If `readGolden` doesn't exist yet from M1, define it in the same file:)

```go
func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return strings.TrimSpace(string(b))
}
```

(Add the imports `os`, `path/filepath`, `strings` if not already present.)

- [ ] **Step 5: Run the golden tests; reconcile any whitespace mismatch**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/tmpl/duckdb/ -run 'TestRenderCreate(Idfr|Focal|Descriptor|Relationship)_Golden' -v
```

If a test fails on whitespace, diff the actual vs expected and **rewrite the golden file** to match the actual output. The render output is canonical; the golden is the snapshot. Re-run until all four pass.

- [ ] **Step 6: Commit**

```bash
git add prism/internal/tmpl/duckdb/dab_create_*.sql.tmpl \
        prism/internal/tmpl/duckdb/render.go \
        prism/internal/tmpl/duckdb/render_test.go \
        prism/internal/tmpl/duckdb/testdata/dab_create_*.sql.golden
git commit -m "feat(tmpl): DAB DDL templates (idfr, focal, descriptor, relationship)"
```

---

### Task 11: Merge templates (4 merges) + Render funcs + golden tests

**Files:**
- Create: `prism/internal/tmpl/duckdb/dab_merge_idfr.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_merge_focal.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_merge_descriptor.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_merge_relationship.sql.tmpl`
- Modify: `prism/internal/tmpl/duckdb/render.go` (4 more Render funcs)
- Modify: `prism/internal/tmpl/duckdb/render_test.go` (4 more golden tests)
- Create: 4 corresponding `testdata/dab_merge_*.sql.golden`

- [ ] **Step 1: Write the merge templates**

`prism/internal/tmpl/duckdb/dab_merge_idfr.sql.tmpl`:

```
INSERT INTO {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}, {{ quote .IdfrCol }},
    eff_tmstp, ver_tmstp, seq_nbr, row_st,
    data_key, inst_key, inst_row_key, popln_tmstp
)
SELECT
    md5(src.{{ quote .IdfrCol }}) AS {{ quote .KeyCol }},
    src.{{ quote .IdfrCol }},
    src.eff_tmstp,
    CURRENT_TIMESTAMP             AS ver_tmstp,
    1                             AS seq_nbr,
    'Y'                           AS row_st,
    'dab'                         AS data_key,
    '{{ .MappingGroup }}'         AS inst_key,
    '{{ .InstRowKey }}'           AS inst_row_key,
    CURRENT_TIMESTAMP             AS popln_tmstp
FROM (
{{ .SourceCTE }}
) src
WHERE NOT EXISTS (
    SELECT 1 FROM {{ quote .Schema }}.{{ quote .Table }} d
    WHERE d.{{ quote .IdfrCol }} = src.{{ quote .IdfrCol }}
      AND d.eff_tmstp = src.eff_tmstp
);
```

`prism/internal/tmpl/duckdb/dab_merge_focal.sql.tmpl`:

```
INSERT INTO {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}, eff_tmstp, ver_tmstp, row_st, data_key, popln_tmstp
)
SELECT
    {{ quote .KeyCol }},
    MIN(eff_tmstp)    AS eff_tmstp,
    CURRENT_TIMESTAMP AS ver_tmstp,
    'Y'               AS row_st,
    'dab'             AS data_key,
    CURRENT_TIMESTAMP AS popln_tmstp
FROM {{ quote .Schema }}.{{ quote .IdfrTable }}
GROUP BY {{ quote .KeyCol }}
ON CONFLICT ({{ quote .KeyCol }}) DO UPDATE
SET eff_tmstp   = LEAST(eff_tmstp,  EXCLUDED.eff_tmstp),
    ver_tmstp   = EXCLUDED.ver_tmstp,
    popln_tmstp = EXCLUDED.popln_tmstp;
```

`prism/internal/tmpl/duckdb/dab_merge_descriptor.sql.tmpl`:

```
INSERT INTO {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}, type_key,
    eff_tmstp, ver_tmstp, seq_nbr, row_st,
    sta_tmstp, end_tmstp, val_str, val_num, uom,
    data_key, inst_key, inst_row_key, popln_tmstp
)
SELECT
    src.{{ quote .KeyCol }},
    src.type_key,
    src.eff_tmstp,
    CURRENT_TIMESTAMP AS ver_tmstp,
    1                 AS seq_nbr,
    'Y'               AS row_st,
    src.sta_tmstp, src.end_tmstp,
    src.val_str, src.val_num, src.uom,
    'dab'                 AS data_key,
    '{{ .MappingGroup }}' AS inst_key,
    '{{ .InstRowKey }}'   AS inst_row_key,
    CURRENT_TIMESTAMP     AS popln_tmstp
FROM (
{{ .SourceCTE }}
) src
WHERE NOT EXISTS (
    SELECT 1 FROM {{ quote .Schema }}.{{ quote .Table }} d
    WHERE d.{{ quote .KeyCol }} = src.{{ quote .KeyCol }}
      AND d.type_key            = src.type_key
      AND d.eff_tmstp           = src.eff_tmstp
      AND coalesce(d.val_str, '')                    = coalesce(src.val_str, '')
      AND coalesce(CAST(d.val_num   AS VARCHAR), '') = coalesce(CAST(src.val_num   AS VARCHAR), '')
      AND coalesce(d.uom, '')                        = coalesce(src.uom, '')
      AND coalesce(CAST(d.sta_tmstp AS VARCHAR), '') = coalesce(CAST(src.sta_tmstp AS VARCHAR), '')
      AND coalesce(CAST(d.end_tmstp AS VARCHAR), '') = coalesce(CAST(src.end_tmstp AS VARCHAR), '')
);
```

`prism/internal/tmpl/duckdb/dab_merge_relationship.sql.tmpl`:

```
INSERT INTO {{ quote .Schema }}.{{ quote .Table }} (
    {{ quote .KeyCol }}, {{ quote .RelatedCol }}, type_key,
    eff_tmstp, ver_tmstp, seq_nbr, row_st,
    data_key, inst_key, inst_row_key, popln_tmstp
)
SELECT
    src.{{ quote .KeyCol }},
    src.{{ quote .RelatedCol }},
    src.type_key,
    src.eff_tmstp,
    CURRENT_TIMESTAMP AS ver_tmstp,
    1                 AS seq_nbr,
    'Y'               AS row_st,
    'dab'                 AS data_key,
    '{{ .MappingGroup }}' AS inst_key,
    '{{ .InstRowKey }}'   AS inst_row_key,
    CURRENT_TIMESTAMP     AS popln_tmstp
FROM (
{{ .SourceCTE }}
) src
WHERE NOT EXISTS (
    SELECT 1 FROM {{ quote .Schema }}.{{ quote .Table }} d
    WHERE d.{{ quote .KeyCol }}     = src.{{ quote .KeyCol }}
      AND d.{{ quote .RelatedCol }} = src.{{ quote .RelatedCol }}
      AND d.type_key                = src.type_key
      AND d.eff_tmstp               = src.eff_tmstp
);
```

- [ ] **Step 2: Add Render funcs**

Append to `prism/internal/tmpl/duckdb/render.go`:

```go
func RenderMergeIdfr(spec engine.MergeIdfrSpec) (string, error) {
	return render("dab_merge_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}

func RenderMergeFocal(spec engine.MergeFocalSpec) (string, error) {
	return render("dab_merge_focal.sql.tmpl", struct {
		Schema, Table, IdfrTable, KeyCol string
	}{
		spec.Schema,
		spec.Entity,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
	})
}

func RenderMergeDescriptor(spec engine.MergeDescriptorSpec) (string, error) {
	return render("dab_merge_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}

func RenderMergeRelationship(spec engine.MergeRelationshipSpec) (string, error) {
	return render("dab_merge_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}
```

- [ ] **Step 3: Add four golden tests**

Append to `render_test.go`:

```go
func TestRenderMergeIdfr_Golden(t *testing.T) {
	got, err := tmpl.RenderMergeIdfr(engine.MergeIdfrSpec{
		Schema: "dab", Entity: "customer",
		MappingGroup: "adventure_works",
		InstRowKey:   "adventure_works.customer",
		SourceCTE: "    SELECT 'CUSTOMER:' || CAST(customer_id AS VARCHAR) AS \"customer_idfr\",\n           modified_date AS eff_tmstp\n    FROM \"das__adventure_works\".\"customer__current\"",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_merge_idfr.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderMergeFocal_Golden(t *testing.T) {
	got, err := tmpl.RenderMergeFocal(engine.MergeFocalSpec{Schema: "dab", Entity: "customer"})
	require.NoError(t, err)
	want := readGolden(t, "dab_merge_focal.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderMergeDescriptor_Golden(t *testing.T) {
	got, err := tmpl.RenderMergeDescriptor(engine.MergeDescriptorSpec{
		Schema: "dab", Entity: "customer",
		MappingGroup: "adventure_works", InstRowKey: "adventure_works.customer",
		SourceCTE: "    SELECT * FROM \"das__adventure_works\".\"customer__current\"",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_merge_descriptor.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderMergeRelationship_Golden(t *testing.T) {
	got, err := tmpl.RenderMergeRelationship(engine.MergeRelationshipSpec{
		Schema: "dab", Entity: "customer", Related: "order",
		MappingGroup: "adventure_works", InstRowKey: "adventure_works.customer",
		SourceCTE: "    SELECT * FROM \"das__adventure_works\".\"customer__current\"",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_merge_relationship.sql.golden")
	require.Equal(t, want, got)
}
```

- [ ] **Step 4: Run tests; capture rendered output as golden snapshots**

First run will fail because the goldens don't exist:

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/tmpl/duckdb/ -run 'TestRenderMerge.*_Golden' -v
```

Expected: FAIL (file not found). For each failing test, run a small Go program (or use a temporary `t.Logf("%s", got)` line) to print the actual rendered output and write that into the corresponding `testdata/dab_merge_*.sql.golden` file. Re-run; all four should now PASS.

The simplest way: temporarily add `t.Logf("\n%s\n", got)` before each `require.Equal`, run with `-v`, copy the printed output verbatim into the golden, remove the `t.Logf` line.

- [ ] **Step 5: Verify all four pass**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/tmpl/duckdb/ -run 'TestRenderMerge.*_Golden' -v
```

Expected: PASS for all four.

- [ ] **Step 6: Commit**

```bash
git add prism/internal/tmpl/duckdb/dab_merge_*.sql.tmpl \
        prism/internal/tmpl/duckdb/render.go \
        prism/internal/tmpl/duckdb/render_test.go \
        prism/internal/tmpl/duckdb/testdata/dab_merge_*.sql.golden
git commit -m "feat(tmpl): DAB merge templates (idfr, focal, descriptor, relationship)"
```

---

### Task 12: Recompute ROW_ST templates (3) + Render funcs + golden tests

**Files:**
- Create: `prism/internal/tmpl/duckdb/dab_recompute_idfr.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_recompute_descriptor.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_recompute_relationship.sql.tmpl`
- Modify: `prism/internal/tmpl/duckdb/render.go`
- Modify: `prism/internal/tmpl/duckdb/render_test.go`
- Create: 3 corresponding `testdata/*.golden`

- [ ] **Step 1: Write the three templates**

`prism/internal/tmpl/duckdb/dab_recompute_idfr.sql.tmpl`:

```
WITH ranked AS (
    SELECT
        {{ quote .KeyCol }},
        {{ quote .IdfrCol }},
        eff_tmstp,
        ver_tmstp,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, {{ quote .IdfrCol }}
            ORDER BY eff_tmstp ASC, ver_tmstp ASC
        ) AS new_seq_nbr,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, {{ quote .IdfrCol }}
            ORDER BY eff_tmstp DESC, ver_tmstp DESC
        ) AS inv_rnk
    FROM {{ quote .Schema }}.{{ quote .Table }}
)
UPDATE {{ quote .Schema }}.{{ quote .Table }} d
SET row_st  = CASE WHEN r.inv_rnk = 1 THEN 'Y' ELSE 'N' END,
    seq_nbr = r.new_seq_nbr
FROM ranked r
WHERE d.{{ quote .KeyCol }}  = r.{{ quote .KeyCol }}
  AND d.{{ quote .IdfrCol }} = r.{{ quote .IdfrCol }}
  AND d.eff_tmstp           = r.eff_tmstp
  AND d.ver_tmstp           = r.ver_tmstp;
```

`prism/internal/tmpl/duckdb/dab_recompute_descriptor.sql.tmpl`:

```
WITH ranked AS (
    SELECT
        {{ quote .KeyCol }},
        type_key,
        eff_tmstp,
        ver_tmstp,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, type_key
            ORDER BY eff_tmstp ASC, ver_tmstp ASC
        ) AS new_seq_nbr,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, type_key
            ORDER BY eff_tmstp DESC, ver_tmstp DESC
        ) AS inv_rnk
    FROM {{ quote .Schema }}.{{ quote .Table }}
)
UPDATE {{ quote .Schema }}.{{ quote .Table }} d
SET row_st  = CASE WHEN r.inv_rnk = 1 THEN 'Y' ELSE 'N' END,
    seq_nbr = r.new_seq_nbr
FROM ranked r
WHERE d.{{ quote .KeyCol }} = r.{{ quote .KeyCol }}
  AND d.type_key           = r.type_key
  AND d.eff_tmstp          = r.eff_tmstp
  AND d.ver_tmstp          = r.ver_tmstp;
```

`prism/internal/tmpl/duckdb/dab_recompute_relationship.sql.tmpl`:

```
WITH ranked AS (
    SELECT
        {{ quote .KeyCol }},
        {{ quote .RelatedCol }},
        type_key,
        eff_tmstp,
        ver_tmstp,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, {{ quote .RelatedCol }}, type_key
            ORDER BY eff_tmstp ASC, ver_tmstp ASC
        ) AS new_seq_nbr,
        ROW_NUMBER() OVER (
            PARTITION BY {{ quote .KeyCol }}, {{ quote .RelatedCol }}, type_key
            ORDER BY eff_tmstp DESC, ver_tmstp DESC
        ) AS inv_rnk
    FROM {{ quote .Schema }}.{{ quote .Table }}
)
UPDATE {{ quote .Schema }}.{{ quote .Table }} d
SET row_st  = CASE WHEN r.inv_rnk = 1 THEN 'Y' ELSE 'N' END,
    seq_nbr = r.new_seq_nbr
FROM ranked r
WHERE d.{{ quote .KeyCol }}     = r.{{ quote .KeyCol }}
  AND d.{{ quote .RelatedCol }} = r.{{ quote .RelatedCol }}
  AND d.type_key                = r.type_key
  AND d.eff_tmstp               = r.eff_tmstp
  AND d.ver_tmstp               = r.ver_tmstp;
```

- [ ] **Step 2: Add Render funcs**

Append to `render.go`:

```go
func RenderRecomputeIdfrRowSt(spec engine.RecomputeIdfrRowStSpec) (string, error) {
	return render("dab_recompute_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
	})
}

func RenderRecomputeDescriptorRowSt(spec engine.RecomputeDescriptorRowStSpec) (string, error) {
	return render("dab_recompute_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
	})
}

func RenderRecomputeRelationshipRowSt(spec engine.RecomputeRelationshipRowStSpec) (string, error) {
	return render("dab_recompute_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
	})
}
```

- [ ] **Step 3: Add golden tests**

Append to `render_test.go`:

```go
func TestRenderRecomputeIdfr_Golden(t *testing.T) {
	got, err := tmpl.RenderRecomputeIdfrRowSt(engine.RecomputeIdfrRowStSpec{
		Schema: "dab", Entity: "customer",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_recompute_idfr.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderRecomputeDescriptor_Golden(t *testing.T) {
	got, err := tmpl.RenderRecomputeDescriptorRowSt(engine.RecomputeDescriptorRowStSpec{
		Schema: "dab", Entity: "customer",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_recompute_descriptor.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderRecomputeRelationship_Golden(t *testing.T) {
	got, err := tmpl.RenderRecomputeRelationshipRowSt(engine.RecomputeRelationshipRowStSpec{
		Schema: "dab", Entity: "customer", Related: "order",
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_recompute_relationship.sql.golden")
	require.Equal(t, want, got)
}
```

- [ ] **Step 4: Run tests; capture goldens (same procedure as Task 11)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/tmpl/duckdb/ -run 'TestRenderRecompute.*_Golden' -v
```

First run: FAIL (file missing). Capture rendered output, write to `testdata/*.golden`. Re-run: PASS.

- [ ] **Step 5: Commit**

```bash
git add prism/internal/tmpl/duckdb/dab_recompute_*.sql.tmpl \
        prism/internal/tmpl/duckdb/render.go \
        prism/internal/tmpl/duckdb/render_test.go \
        prism/internal/tmpl/duckdb/testdata/dab_recompute_*.sql.golden
git commit -m "feat(tmpl): DAB recompute ROW_ST templates"
```

---

### Task 13: View templates (group view + entity-current view) + Render funcs + golden tests

**Files:**
- Create: `prism/internal/tmpl/duckdb/dab_create_group_view.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/dab_create_entity_current_view.sql.tmpl`
- Modify: `prism/internal/tmpl/duckdb/render.go`
- Modify: `prism/internal/tmpl/duckdb/render_test.go`
- Create: corresponding `testdata/*.golden`

- [ ] **Step 1: Write the group-view template**

`prism/internal/tmpl/duckdb/dab_create_group_view.sql.tmpl`:

```
CREATE OR REPLACE VIEW {{ quote .Schema }}.{{ quote .ViewName }} AS
SELECT
    {{ quote .KeyCol }},
    eff_tmstp,
    ver_tmstp,
    row_st{{ range .Members }},
    {{ .Slot }} AS {{ quote .InnerID }}{{ end }}
FROM {{ quote .Schema }}.{{ quote .DescTable }}
WHERE type_key = '{{ .TypeKeyHex }}';
```

- [ ] **Step 2: Write the entity-current template**

`prism/internal/tmpl/duckdb/dab_create_entity_current_view.sql.tmpl`:

```
CREATE OR REPLACE VIEW {{ quote .Schema }}.{{ quote .ViewName }} AS
SELECT
    f.{{ quote .KeyCol }}{{ range .Columns }},
    {{ .ViewAlias }}.{{ quote .InnerCol }} AS {{ quote .OutputCol }}{{ end }}
FROM {{ quote .Schema }}.{{ quote .FocalTable }} f{{ range .Joins }}
LEFT JOIN {{ quote $.Schema }}.{{ quote .ViewName }} {{ .Alias }}
       ON {{ .Alias }}.{{ quote $.KeyCol }} = f.{{ quote $.KeyCol }}
      AND {{ .Alias }}.row_st = 'Y'{{ end }}
WHERE f.row_st = 'Y';
```

- [ ] **Step 3: Add Render funcs**

Append to `render.go`:

```go
// slotForType maps an attribute type to its descriptor-table column.
func slotForType(t string) string {
	switch t {
	case "STRING":
		return "val_str"
	case "NUMBER":
		return "val_num"
	case "UNIT":
		return "uom"
	case "START_TIMESTAMP":
		return "sta_tmstp"
	case "END_TIMESTAMP":
		return "end_tmstp"
	}
	return "/* unknown type " + t + " */"
}

func RenderCreateGroupView(spec engine.GroupViewSpec) (string, error) {
	type member struct{ InnerID, Slot string }
	ms := make([]member, len(spec.Members))
	for i, m := range spec.Members {
		ms[i] = member{InnerID: m.InnerID, Slot: slotForType(m.Type)}
	}
	return render("dab_create_group_view.sql.tmpl", struct {
		Schema, ViewName, KeyCol, DescTable, TypeKeyHex string
		Members                                         []member
	}{
		spec.Schema,
		spec.Entity + "__" + spec.AttrID,
		spec.Entity + "_key",
		spec.Entity + "__descriptor",
		spec.TypeKeyHex,
		ms,
	})
}

// RenderCreateEntityCurrentView builds the per-entity __current view, joining
// the focal table with each per-group view and projecting one output column
// per inner member of each attribute.
func RenderCreateEntityCurrentView(spec engine.EntityCurrentViewSpec) (string, error) {
	type joinT struct{ Alias, ViewName string }
	type colT struct{ ViewAlias, InnerCol, OutputCol string }
	var joins []joinT
	var cols []colT
	for i, a := range spec.Attributes {
		alias := fmt.Sprintf("a%d", i)
		joins = append(joins, joinT{Alias: alias, ViewName: spec.Entity + "__" + a.AttrID})
		// Single-type attribute (one member with InnerID == AttrID): output name is just AttrID.
		// Atomic group (>=1 member with distinct InnerID): output name is AttrID__InnerID.
		single := len(a.Members) == 1 && a.Members[0].InnerID == a.AttrID
		for _, m := range a.Members {
			out := a.AttrID + "__" + m.InnerID
			if single {
				out = a.AttrID
			}
			cols = append(cols, colT{ViewAlias: alias, InnerCol: m.InnerID, OutputCol: out})
		}
	}
	return render("dab_create_entity_current_view.sql.tmpl", struct {
		Schema, ViewName, KeyCol, FocalTable string
		Joins                                []joinT
		Columns                              []colT
	}{
		spec.Schema,
		spec.Entity + "__current",
		spec.Entity + "_key",
		spec.Entity,
		joins, cols,
	})
}
```

(Add `"fmt"` to the imports of `render.go` if not already present.)

- [ ] **Step 4: Add golden tests**

Append to `render_test.go`:

```go
func TestRenderCreateGroupView_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateGroupView(engine.GroupViewSpec{
		Schema: "dab", Entity: "customer", AttrID: "customer_lifetime_value",
		TypeKeyHex: "deadbeef00000000000000000000beef",
		Members: []engine.GroupViewMember{
			{InnerID: "amount", Type: "NUMBER"},
			{InnerID: "currency", Type: "UNIT"},
		},
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_group_view.sql.golden")
	require.Equal(t, want, got)
}

func TestRenderCreateEntityCurrentView_Golden(t *testing.T) {
	got, err := tmpl.RenderCreateEntityCurrentView(engine.EntityCurrentViewSpec{
		Schema: "dab", Entity: "customer",
		Attributes: []engine.EntityCurrentAttribute{
			{
				AttrID:  "customer_name",
				Members: []engine.GroupViewMember{{InnerID: "customer_name", Type: "STRING"}},
			},
			{
				AttrID: "customer_lifetime_value",
				Members: []engine.GroupViewMember{
					{InnerID: "amount", Type: "NUMBER"},
					{InnerID: "currency", Type: "UNIT"},
				},
			},
		},
	})
	require.NoError(t, err)
	want := readGolden(t, "dab_create_entity_current_view.sql.golden")
	require.Equal(t, want, got)
}
```

- [ ] **Step 5: Run tests; capture goldens; re-run**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/tmpl/duckdb/ -run 'TestRenderCreate(GroupView|EntityCurrentView)_Golden' -v
```

First run: FAIL. Capture, write goldens, re-run: PASS.

- [ ] **Step 6: Build the whole tree to confirm Phase 2 hookup also works now**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./...
```

Expected: success — Phase 2's `engine.Dialect` interface and DuckDB stubs now have all 13 Render funcs they reference.

- [ ] **Step 7: Run the full test suite**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./...
```

Expected: PASS for everything (M1 tests should be unaffected; M2 unit tests + golden tests pass).

- [ ] **Step 8: Commit**

```bash
git add prism/internal/tmpl/duckdb/dab_create_group_view.sql.tmpl \
        prism/internal/tmpl/duckdb/dab_create_entity_current_view.sql.tmpl \
        prism/internal/tmpl/duckdb/render.go \
        prism/internal/tmpl/duckdb/render_test.go \
        prism/internal/tmpl/duckdb/testdata/dab_create_group_view.sql.golden \
        prism/internal/tmpl/duckdb/testdata/dab_create_entity_current_view.sql.golden \
        prism/internal/engine/engine.go \
        prism/internal/engine/duckdb/duckdb.go
git commit -m "feat(tmpl,engine): DAB view templates + wire 13 new Dialect methods"
```

---

## Phase 4: `dab.Execute` + DuckDB round-trip integration tests

### Task 14: `dab.Execute` — run an EntityPlan against an engine

**Files:**
- Create: `prism/internal/dab/execute.go`

This is **infrastructure** (no test of its own — Tasks 15-19 exercise it via round-trip). The function is small and deterministic.

- [ ] **Step 1: Implement `Execute`**

Create `prism/internal/dab/execute.go`:

```go
package dab

import (
	"context"
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/engine"
)

// DASSchemaPrefix is the prefix used by M1 for DAS staging schemas.
const DASSchemaPrefix = "das__"

// Execute runs an EntityPlan against an engine: ensures schema, creates DDL,
// runs all merges in plan-order, recomputes ROW_ST, then renders views.
func Execute(ctx context.Context, eng engine.Engine, plan *EntityPlan) error {
	d := eng.Dialect()

	if err := eng.Exec(ctx, d.CreateSchemaIfNotExists(plan.IDFR.Schema)); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateIdfrTableIfNotExists(engine.IdfrTableSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create idfr: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateFocalTableIfNotExists(engine.FocalTableSpec{
		Schema: plan.Focal.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create focal: %w", err)
	}
	if err := eng.Exec(ctx, d.CreateDescriptorTableIfNotExists(engine.DescriptorTableSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("create descriptor: %w", err)
	}
	for _, r := range plan.Relationships {
		if err := eng.Exec(ctx, d.CreateRelationshipTableIfNotExists(engine.RelationshipTableSpec{
			Schema: r.Schema, Entity: r.Entity, Related: r.Related, Suffix: r.Suffix,
		})); err != nil {
			return fmt.Errorf("create relationship %s__%s: %w", r.Entity, r.Related, err)
		}
	}

	// Per-mapping merges.
	for _, m := range plan.Mappings {
		dasTable := fmt.Sprintf(`"%s%s"."%s__%s"`, DASSchemaPrefix, m.SourceID, m.SourceEntity, m.From)
		if err := mergeIDFRForMapping(ctx, eng, plan, m, dasTable); err != nil {
			return err
		}
		for _, dm := range m.Descriptors {
			if err := mergeDescriptorForMapping(ctx, eng, plan, m, dm, dasTable); err != nil {
				return err
			}
		}
		for _, rm := range m.Relationships {
			if err := mergeRelationshipForMapping(ctx, eng, plan, m, rm, dasTable); err != nil {
				return err
			}
		}
	}

	// Focal merge AFTER all IDFR rows are in.
	if err := eng.Exec(ctx, d.MergeFocal(engine.MergeFocalSpec{
		Schema: plan.Focal.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("merge focal: %w", err)
	}

	// Recompute ROW_ST / SEQ_NBR.
	if err := eng.Exec(ctx, d.RecomputeIdfrRowSt(engine.RecomputeIdfrRowStSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("recompute idfr row_st: %w", err)
	}
	if err := eng.Exec(ctx, d.RecomputeDescriptorRowSt(engine.RecomputeDescriptorRowStSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
	})); err != nil {
		return fmt.Errorf("recompute descriptor row_st: %w", err)
	}
	for _, r := range plan.Relationships {
		if err := eng.Exec(ctx, d.RecomputeRelationshipRowSt(engine.RecomputeRelationshipRowStSpec{
			Schema: r.Schema, Entity: r.Entity, Related: r.Related, Suffix: r.Suffix,
		})); err != nil {
			return fmt.Errorf("recompute relationship row_st: %w", err)
		}
	}

	// Views.
	for _, gv := range plan.GroupViews {
		members := make([]engine.GroupViewMember, len(gv.Members))
		for i, m := range gv.Members {
			members[i] = engine.GroupViewMember{InnerID: m.InnerID, Type: m.Type}
		}
		if err := eng.Exec(ctx, d.CreateOrReplaceGroupView(engine.GroupViewSpec{
			Schema: gv.Schema, Entity: gv.Entity, AttrID: gv.AttrID,
			TypeKeyHex: gv.TypeKeyHex, Members: members,
		})); err != nil {
			return fmt.Errorf("create group view %s__%s: %w", gv.Entity, gv.AttrID, err)
		}
	}
	cv := plan.EntityCurrentView
	attrs := make([]engine.EntityCurrentAttribute, len(cv.Attributes))
	for i, a := range cv.Attributes {
		ms := make([]engine.GroupViewMember, len(a.Members))
		for j, m := range a.Members {
			ms[j] = engine.GroupViewMember{InnerID: m.InnerID, Type: m.Type}
		}
		attrs[i] = engine.EntityCurrentAttribute{AttrID: a.AttrID, Members: ms}
	}
	if err := eng.Exec(ctx, d.CreateOrReplaceEntityCurrentView(engine.EntityCurrentViewSpec{
		Schema: cv.Schema, Entity: cv.Entity, Attributes: attrs,
	})); err != nil {
		return fmt.Errorf("create entity current view: %w", err)
	}
	return nil
}

func mergeIDFRForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, dasTable string) error {
	whereClause := ""
	if m.Where != "" {
		whereClause = "    WHERE " + m.Where + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n        %s AS \"%s_idfr\",\n        CAST((%s) AS TIMESTAMP) AS eff_tmstp\n    FROM %s\n%s",
		m.IDFRExpr, plan.Entity, m.EffTmstpExpr, dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeIdfr(engine.MergeIdfrSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

func mergeDescriptorForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, dm DescriptorMapping, dasTable string) error {
	wheres := []string{}
	if m.Where != "" {
		wheres = append(wheres, "("+m.Where+")")
	}
	if dm.Where != "" {
		wheres = append(wheres, "("+dm.Where+")")
	}
	whereClause := ""
	if len(wheres) > 0 {
		whereClause = "    WHERE " + strings.Join(wheres, " AND ") + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n"+
			"        md5(%s) AS \"%s_key\",\n"+
			"        '%s' AS type_key,\n"+
			"        CAST((%s) AS TIMESTAMP) AS eff_tmstp,\n"+
			"        %s AS val_str,\n"+
			"        %s AS val_num,\n"+
			"        %s AS uom,\n"+
			"        %s AS sta_tmstp,\n"+
			"        %s AS end_tmstp\n"+
			"    FROM %s\n%s",
		m.IDFRExpr, plan.Entity,
		dm.TypeKeyHex,
		dm.EffTmstpExpr,
		nullIfEmpty(dm.ValStrExpr, "VARCHAR"),
		nullIfEmpty(dm.ValNumExpr, "DOUBLE"),
		nullIfEmpty(dm.UomExpr, "VARCHAR"),
		nullIfEmpty(dm.StaTmstpExpr, "TIMESTAMP"),
		nullIfEmpty(dm.EndTmstpExpr, "TIMESTAMP"),
		dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeDescriptor(engine.MergeDescriptorSpec{
		Schema: plan.Descriptor.Schema, Entity: plan.Entity,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

func mergeRelationshipForMapping(ctx context.Context, eng engine.Engine, plan *EntityPlan, m MappingPlan, rm RelationshipMapping, dasTable string) error {
	wheres := []string{}
	if m.Where != "" {
		wheres = append(wheres, "("+m.Where+")")
	}
	if rm.Where != "" {
		wheres = append(wheres, "("+rm.Where+")")
	}
	whereClause := ""
	if len(wheres) > 0 {
		whereClause = "    WHERE " + strings.Join(wheres, " AND ") + "\n"
	}
	cte := fmt.Sprintf(
		"    SELECT\n"+
			"        md5(%s) AS \"%s_key\",\n"+
			"        md5(CAST((%s) AS VARCHAR)) AS \"%s_key\",\n"+
			"        '%s' AS type_key,\n"+
			"        CAST((%s) AS TIMESTAMP) AS eff_tmstp\n"+
			"    FROM %s\n%s",
		m.IDFRExpr, plan.Entity,
		rm.TargetExpr, rm.Related,
		rm.TypeKeyHex,
		rm.EffTmstpExpr,
		dasTable, whereClause,
	)
	return eng.Exec(ctx, eng.Dialect().MergeRelationship(engine.MergeRelationshipSpec{
		Schema: plan.IDFR.Schema, Entity: plan.Entity,
		Related: rm.Related, Suffix: rm.Suffix,
		MappingGroup: m.MappingGroup, InstRowKey: m.InstRowKey,
		SourceCTE: strings.TrimRight(cte, "\n"),
	}))
}

// nullIfEmpty returns "NULL" cast to t when expr is empty, otherwise the expr
// cast to t. Empty expressions arise when a group doesn't bind a particular
// type slot (e.g. a STRING-only group leaves val_num/uom/sta/end as NULL).
func nullIfEmpty(expr, t string) string {
	if expr == "" {
		return "CAST(NULL AS " + t + ")"
	}
	return "CAST((" + expr + ") AS " + t + ")"
}
```

- [ ] **Step 2: Build to confirm**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./internal/dab/...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add prism/internal/dab/execute.go
git commit -m "feat(dab): Execute() — run an EntityPlan against an engine"
```

---

### Task 15: Round-trip — single source, single attribute, simple group

**Files:**
- Create: `prism/internal/dab/execute_test.go`

This test populates a DAS staging table directly via `INSERT`, runs `dab.Execute`, and asserts the resulting `dab.*` rows. We don't need real JSONL — Engine round-trip means Engine → SQL → Engine, no filesystem.

- [ ] **Step 1: Write the failing test**

Create `prism/internal/dab/execute_test.go`:

```go
package dab_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// fixtureDASCustomerCurrent creates a typed DAS staging table for the
// adventure_works.customer__current entity matching the fixture contract.
const fixtureDASCustomerCurrent = `
CREATE SCHEMA IF NOT EXISTS das__adventure_works;
CREATE TABLE das__adventure_works.customer__current (
    customer_id     BIGINT    NOT NULL,
    company_name    VARCHAR,
    lifetime_value  DOUBLE,
    modified_date   TIMESTAMP NOT NULL,
    _loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func TestExecute_SingleSourceSingleAttribute(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date)
		VALUES
		    (1, 'Acme',     1000.0, TIMESTAMP '2024-01-15 10:00:00'),
		    (2, 'Globex',    500.0, TIMESTAMP '2024-02-20 11:00:00');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	require.NoError(t, contracts.ValidateFocal(f))

	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{
		EntityID: "customer", Path: "ignored", Focal: f,
	})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// Two distinct customers → two focal rows.
	rows, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows.Close()
	rows.Next()
	var n int
	require.NoError(t, rows.Scan(&n))
	require.Equal(t, 2, n)

	// IDFR table: two rows, both 'Y'.
	rows2, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__idfr WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows2.Close()
	rows2.Next()
	require.NoError(t, rows2.Scan(&n))
	require.Equal(t, 2, n)

	// Descriptor table: 4 rows (2 customers × 2 outer attributes).
	rows3, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__descriptor WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows3.Close()
	rows3.Next()
	require.NoError(t, rows3.Scan(&n))
	require.Equal(t, 4, n)

	// __current view: typed columns by attribute.
	rows4, err := eng.Query(ctx, `
		SELECT customer_name, customer_lifetime_value__amount, customer_lifetime_value__currency
		FROM dab.customer__current
		ORDER BY customer_name;
	`)
	require.NoError(t, err)
	defer rows4.Close()
	type row struct {
		Name     string
		Amount   float64
		Currency string
	}
	var got []row
	for rows4.Next() {
		var r row
		require.NoError(t, rows4.Scan(&r.Name, &r.Amount, &r.Currency))
		got = append(got, r)
	}
	require.Equal(t, []row{
		{Name: "Acme", Amount: 1000.0, Currency: "USD"},
		{Name: "Globex", Amount: 500.0, Currency: "USD"},
	}, got)
}
```

- [ ] **Step 2: Run the test**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestExecute_SingleSourceSingleAttribute -v
```

Expected on first run: PASS (Tasks 6-14 implemented all the building blocks). If it fails, the failure tells you which template's SQL DuckDB rejects — fix the template (whitespace, quoting, column names) and re-run.

- [ ] **Step 3: Commit**

```bash
git add prism/internal/dab/execute_test.go
git commit -m "test(dab): single-source round-trip — assert focal/idfr/desc/view rows"
```

---

### Task 16: Round-trip — atomic-context group with multiple inner members

This is already partially exercised by Task 15 (CUSTOMER_LIFETIME_VALUE has AMOUNT + CURRENCY). Add a test that covers a group with START_TIMESTAMP + END_TIMESTAMP members (validity window).

**Files:**
- Modify: `prism/testdata/contracts/valid/dab/customer.yml` (extend fixture)
- Modify: `prism/internal/dab/execute_test.go`

- [ ] **Step 1: Extend the fixture with a window group**

Edit `prism/testdata/contracts/valid/dab/customer.yml`. Add to `attributes:`:

```yaml
  - id: CUSTOMER_ACTIVE_WINDOW
    definition: "Window during which the customer is active"
    effective_timestamp: true
    group:
      - id: START
        type: START_TIMESTAMP
      - id: END
        type: END_TIMESTAMP
```

Add to the table's `attributes:`:

```yaml
          - id: CUSTOMER_ACTIVE_WINDOW.START
            transformation_expression: created_at
          - id: CUSTOMER_ACTIVE_WINDOW.END
            transformation_expression: deactivated_at
```

- [ ] **Step 2: Extend the DAS fixture DDL in the test**

Modify `fixtureDASCustomerCurrent` to add columns:

```
CREATE TABLE das__adventure_works.customer__current (
    customer_id     BIGINT    NOT NULL,
    company_name    VARCHAR,
    lifetime_value  DOUBLE,
    created_at      TIMESTAMP,
    deactivated_at  TIMESTAMP,
    modified_date   TIMESTAMP NOT NULL,
    _loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

Update the INSERT in Task 15's test:

```sql
INSERT INTO das__adventure_works.customer__current
(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
VALUES
    (1, 'Acme',     1000.0, TIMESTAMP '2023-06-01', NULL,                       TIMESTAMP '2024-01-15 10:00:00'),
    (2, 'Globex',    500.0, TIMESTAMP '2023-09-01', TIMESTAMP '2024-04-01',     TIMESTAMP '2024-02-20 11:00:00');
```

Add a new subtest:

```go
func TestExecute_AtomicGroup_WindowMembers(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES
		    (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL,                       TIMESTAMP '2024-01-15 10:00:00'),
		    (2, 'Globex', 500.0, TIMESTAMP '2023-09-01', TIMESTAMP '2024-04-01',    TIMESTAMP '2024-02-20 11:00:00');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	rows, err := eng.Query(ctx, `
		SELECT customer_name,
		       customer_active_window__start,
		       customer_active_window__end
		FROM dab.customer__current
		ORDER BY customer_name;
	`)
	require.NoError(t, err)
	defer rows.Close()

	var got []string
	for rows.Next() {
		var name string
		var sta, end interface{}
		require.NoError(t, rows.Scan(&name, &sta, &end))
		got = append(got, name)
	}
	require.Equal(t, []string{"Acme", "Globex"}, got)

	// Spot-check the typed view exposes the window columns.
	rows2, err := eng.Query(ctx, `
		SELECT count(*) FROM dab.customer__customer_active_window WHERE row_st = 'Y';
	`)
	require.NoError(t, err)
	defer rows2.Close()
	rows2.Next()
	var n int
	require.NoError(t, rows2.Scan(&n))
	require.Equal(t, 2, n)
}
```

- [ ] **Step 3: Run both tests**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestExecute -v
```

Expected: PASS for both.

- [ ] **Step 4: Commit**

```bash
git add prism/testdata/contracts/valid/dab/customer.yml prism/internal/dab/execute_test.go
git commit -m "test(dab): atomic-group with START/END_TIMESTAMP members round-trip"
```

---

### Task 17: Round-trip — relationships

**Files:**
- Create: `prism/testdata/contracts/valid/dab/order.yml` (a second focal so target_entity_id resolves)
- Modify: `prism/testdata/contracts/valid/dab/customer.yml` (add CUSTOMER_PLACES_ORDER relationship + binding)
- Create: `prism/testdata/contracts/valid/das/adventure_works/order.yml` (DAS contract for cross-layer validate)
- Modify: `prism/internal/dab/execute_test.go`

- [ ] **Step 1: Add the order DAS contract**

Create `prism/testdata/contracts/valid/das/adventure_works/order.yml`:

```yaml
version: 1
entity:
  name: Order
  description: Sales order header
incremental:
  cursor: ModifiedDate
  strategy: append
schema:
  primary_key: [order_id]
  columns:
    - {source_path: OrderID,      target_name: order_id,      type: BIGINT,    mode: REQUIRED}
    - {source_path: CustomerID,   target_name: customer_id,   type: BIGINT,    mode: REQUIRED}
    - {source_path: ModifiedDate, target_name: modified_date, type: TIMESTAMP, mode: REQUIRED}
```

- [ ] **Step 2: Add the order focal contract**

Create `prism/testdata/contracts/valid/dab/order.yml`:

```yaml
version: 1
entity:
  id: ORDER
  name: ORDER
  definition: "A sales order"

attributes:
  - id: ORDER_NUMBER
    definition: "Numeric order identifier"
    type: NUMBER
    effective_timestamp: true

mapping_groups:
  - name: adventure_works
    tables:
      - source: adventure_works
        entity: order
        primary_keys:
          - "'ORDER:' || CAST(order_id AS VARCHAR)"
        entity_effective_timestamp_expression: modified_date
        attributes:
          - id: ORDER_NUMBER
            transformation_expression: order_id
```

- [ ] **Step 3: Add the relationship to customer.yml**

Modify `prism/testdata/contracts/valid/dab/customer.yml`. Add a top-level section:

```yaml
relationships:
  - id: CUSTOMER_PLACES_ORDER
    definition: "Customer placed this order"
    target_entity_id: ORDER
```

(The CUSTOMER side maps the relationship via the order table — easier than putting the binding on customer's existing table since a single AW customer row doesn't carry all their order ids. We'll add a second mapping_groups entry that reads from the order DAS to populate the relationship rows.)

Add a second `mapping_groups[]` entry:

```yaml
  - name: adventure_works_orders
    tables:
      - source: adventure_works
        entity: order
        from: current
        primary_keys:
          - "'CUSTOMER:' || CAST(customer_id AS VARCHAR)"
        entity_effective_timestamp_expression: modified_date
        attributes:
          # No attribute bindings — this contribution exists purely to populate
          # the IDFR side and the relationship table. We must bind at least one
          # attribute (mapping schema requires attributes[] minItems: 1), so we
          # bind CUSTOMER_NAME from the order's customer_id-side proxy: empty.
          # WORKAROUND: bind CUSTOMER_NAME with a where:false so it never produces
          # a descriptor row (preserves model-binding completeness).
          - id: CUSTOMER_NAME
            transformation_expression: "CAST(NULL AS VARCHAR)"
            where: "FALSE"
        relationships:
          - id: CUSTOMER_PLACES_ORDER
            target_transformation_expression: "'ORDER:' || CAST(order_id AS VARCHAR)"
```

(That `where: FALSE` placeholder is a knowing M2 limitation — every mapping table must bind at least one attribute. The cleaner alternative is to allow `attributes:[]` empty when only relationships are bound; we accept the workaround for now and note it in the spec's "What M2 does not include".)

- [ ] **Step 4: Update validate test if needed**

The new `customer.yml` has 2 mapping groups; the Task 7 plan test asserts `len(plan.Mappings) == 1`. Update that assertion to `>= 1` or write a new fixture for plan_test.go and use a different fixture for execute_test.go.

Simpler: split fixtures. Create `prism/testdata/contracts/valid/dab_simple/customer.yml` (the original simple version used by plan_test.go) and keep `valid/dab/customer.yml` as the relationship-extended version. Update `plan_test.go` to load from `valid/dab_simple/customer.yml` instead.

```bash
mkdir -p /home/user/declarative-data-architecture/prism/testdata/contracts/valid/dab_simple
```

Move (copy) the **pre-relationship** customer.yml to `valid/dab_simple/customer.yml`. Adjust `plan_test.go` and the `TestLoadFocal_HappyPath` / `TestLoadAllDab_WalksDirectory` paths to use `valid/dab_simple/`. Keep the directory `valid/dab/` for the relationship-bearing fixtures.

(`TestValidateCrossLayer_HappyPath` should now load `valid/dab/` — the order.yml fixture is needed for `target_entity_id: ORDER` to resolve.)

- [ ] **Step 5: Add the relationship round-trip test**

Append to `prism/internal/dab/execute_test.go`:

```go
const fixtureDASOrderCurrent = `
CREATE TABLE das__adventure_works.order__current (
    order_id        BIGINT    NOT NULL,
    customer_id     BIGINT    NOT NULL,
    modified_date   TIMESTAMP NOT NULL,
    _loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func TestExecute_Relationships(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASOrderCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL, TIMESTAMP '2024-01-15');
	`))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.order__current
		(order_id, customer_id, modified_date)
		VALUES
		    (101, 1, TIMESTAMP '2024-01-20'),
		    (102, 1, TIMESTAMP '2024-02-05');
	`))

	custFocal, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	orderFocal, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/order.yml")
	require.NoError(t, err)

	custPlan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: custFocal})
	require.NoError(t, err)
	orderPlan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "order", Focal: orderFocal})
	require.NoError(t, err)

	require.NoError(t, dab.Execute(ctx, eng, custPlan))
	require.NoError(t, dab.Execute(ctx, eng, orderPlan))

	// Two rows in the relationship table, ROW_ST = 'Y'.
	row, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__order__rel WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer row.Close()
	row.Next()
	var n int
	require.NoError(t, row.Scan(&n))
	require.Equal(t, 2, n)

	// The customer_key on the relationship table matches the focal table.
	row2, err := eng.Query(ctx, `
		SELECT count(*)
		FROM dab.customer__order__rel r
		JOIN dab.customer c ON c.customer_key = r.customer_key
		JOIN dab.order    o ON o.order_key    = r.order_key
		WHERE r.row_st = 'Y';
	`)
	require.NoError(t, err)
	defer row2.Close()
	row2.Next()
	require.NoError(t, row2.Scan(&n))
	require.Equal(t, 2, n)
}
```

- [ ] **Step 6: Run tests**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestExecute_Relationships -v
```

Expected: PASS.

- [ ] **Step 7: Run all tests to confirm nothing regressed**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./...
```

Expected: PASS across all packages.

- [ ] **Step 8: Commit**

```bash
git add prism/testdata/contracts/valid/dab/order.yml \
        prism/testdata/contracts/valid/dab/customer.yml \
        prism/testdata/contracts/valid/dab_simple/ \
        prism/testdata/contracts/valid/das/adventure_works/order.yml \
        prism/internal/dab/execute_test.go \
        prism/internal/dab/plan_test.go \
        prism/internal/contracts/focal_loader_test.go
git commit -m "test(dab): relationship round-trip + ORDER focal fixture"
```

---

### Task 18: Round-trip — multi-source unification (same surrogate from two DAS sources)

**Files:**
- Create: `prism/testdata/contracts/valid/das/stripe/_source.yml`
- Create: `prism/testdata/contracts/valid/das/stripe/customer.yml`
- Modify: `prism/testdata/contracts/valid/dab/customer.yml` (add stripe mapping group)
- Modify: `prism/internal/dab/execute_test.go`

- [ ] **Step 1: Add Stripe DAS contracts**

Create `prism/testdata/contracts/valid/das/stripe/_source.yml`:

```yaml
version: 1
source:
  provider: rest
  base_url: "https://api.stripe.com/v1"
```

Create `prism/testdata/contracts/valid/das/stripe/customer.yml`:

```yaml
version: 1
entity:
  name: customer
schema:
  primary_key: [stripe_id]
  columns:
    - {source_path: id,                 target_name: stripe_id,        type: STRING, mode: REQUIRED}
    - {source_path: name,               target_name: name,             type: STRING, mode: NULLABLE}
    - {source_path: total_revenue,      target_name: total_revenue,    type: BIGINT, mode: NULLABLE}
    - {source_path: currency,           target_name: currency,         type: STRING, mode: NULLABLE}
    - {source_path: aw_customer_id,     target_name: aw_customer_id,   type: BIGINT, mode: REQUIRED}
    - {source_path: updated,            target_name: updated,          type: TIMESTAMP, mode: REQUIRED}
```

- [ ] **Step 2: Add the stripe mapping group to customer.yml**

Append to `prism/testdata/contracts/valid/dab/customer.yml`:

```yaml
  - name: stripe
    tables:
      - source: stripe
        entity: customer
        from: current
        primary_keys:
          - "'CUSTOMER:' || CAST(aw_customer_id AS VARCHAR)"
        entity_effective_timestamp_expression: updated
        attributes:
          - id: CUSTOMER_NAME
            transformation_expression: name
          - id: CUSTOMER_LIFETIME_VALUE.AMOUNT
            transformation_expression: total_revenue / 100.0
          - id: CUSTOMER_LIFETIME_VALUE.CURRENCY
            transformation_expression: currency
          # Window members not bound from stripe; binding-completeness is per-table,
          # so the active window simply isn't contributed by the stripe table.
```

Wait — this fails plan validation (atomic-group completeness requires every member to be bound when ANY member is bound by a table). This stripe contribution doesn't bind any CUSTOMER_ACTIVE_WINDOW member, so the rule is satisfied. Good — completeness is per-table, partial bindings are illegal but **zero** bindings of a group from a particular table are fine.

- [ ] **Step 3: Add the multi-source round-trip test**

Append to `execute_test.go`:

```go
const fixtureDASStripeCustomerCurrent = `
CREATE SCHEMA IF NOT EXISTS das__stripe;
CREATE TABLE das__stripe.customer__current (
    stripe_id        VARCHAR  NOT NULL,
    name             VARCHAR,
    total_revenue    BIGINT,
    currency         VARCHAR,
    aw_customer_id   BIGINT   NOT NULL,
    updated          TIMESTAMP NOT NULL,
    _loaded_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
`

func TestExecute_MultiSourceUnification(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, fixtureDASStripeCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, created_at, deactivated_at, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2023-06-01', NULL, TIMESTAMP '2024-01-15');
	`))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__stripe.customer__current
		(stripe_id, name, total_revenue, currency, aw_customer_id, updated)
		VALUES ('cus_xyz', 'Acme Corporation', 250000, 'USD', 1, TIMESTAMP '2024-03-01');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// One focal row — same canonical IDFR string from both sources collapses to one surrogate.
	row, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer;`)
	require.NoError(t, err)
	defer row.Close()
	row.Next()
	var n int
	require.NoError(t, row.Scan(&n))
	require.Equal(t, 1, n)

	// IDFR table: two rows (one per source contribution; same key, same idfr — but
	// different ver_tmstp won't apply because both rows are inserted in the same
	// build... so actually they collide on (key, idfr, eff_tmstp) only if eff_tmstp
	// matches. Because eff_tmstps differ (modified_date vs updated), we get 2 rows).
	row2, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__idfr;`)
	require.NoError(t, err)
	defer row2.Close()
	row2.Next()
	require.NoError(t, row2.Scan(&n))
	require.Equal(t, 2, n)

	// Both descriptor sources contributed CUSTOMER_NAME and CUSTOMER_LIFETIME_VALUE.
	// ROW_ST = 'Y' selects the latest per (customer_key, type_key) by eff_tmstp.
	// Stripe's updated 2024-03-01 > AW's modified_date 2024-01-15, so stripe wins.
	row3, err := eng.Query(ctx, `
		SELECT customer_name, customer_lifetime_value__amount
		FROM dab.customer__current;
	`)
	require.NoError(t, err)
	defer row3.Close()
	row3.Next()
	var name string
	var amount float64
	require.NoError(t, row3.Scan(&name, &amount))
	require.Equal(t, "Acme Corporation", name)
	require.Equal(t, 2500.0, amount) // 250000 / 100.0 from stripe
}
```

- [ ] **Step 4: Run the test**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run TestExecute_MultiSourceUnification -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add prism/testdata/contracts/valid/das/stripe/ \
        prism/testdata/contracts/valid/dab/customer.yml \
        prism/internal/dab/execute_test.go
git commit -m "test(dab): multi-source unification round-trip (AW + stripe)"
```

---

### Task 19: Round-trip — bi-temporal ROW_ST flip + idempotency

**Files:**
- Modify: `prism/internal/dab/execute_test.go`

- [ ] **Step 1: Add the test**

Append to `execute_test.go`:

```go
func TestExecute_BiTemporalRowStFlip(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab_simple/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)

	// Build #1: customer_id=1 with company_name='Acme', modified_date=2024-01-15
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2024-01-15');
	`))
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// Build #2: same customer, but DAS now reflects a rename (Acme -> AcmeCorp, modified_date=2024-06-01).
	require.NoError(t, eng.Exec(ctx, `
		UPDATE das__adventure_works.customer__current
		SET company_name = 'AcmeCorp', modified_date = TIMESTAMP '2024-06-01'
		WHERE customer_id = 1;
	`))
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// Two descriptor rows for CUSTOMER_NAME on this customer; latest is 'Y',
	// older is 'N'.
	row, err := eng.Query(ctx, `
		SELECT row_st, val_str
		FROM dab.customer__descriptor
		WHERE type_key = (SELECT md5('CUSTOMER:CUSTOMER_NAME'))
		ORDER BY eff_tmstp ASC;
	`)
	require.NoError(t, err)
	defer row.Close()
	type r struct {
		Status string
		Value  string
	}
	var got []r
	for row.Next() {
		var s, v string
		require.NoError(t, row.Scan(&s, &v))
		got = append(got, r{s, v})
	}
	require.Equal(t, []r{{"N", "Acme"}, {"Y", "AcmeCorp"}}, got)

	// __current view returns the latest only.
	row2, err := eng.Query(ctx, `SELECT customer_name FROM dab.customer__current;`)
	require.NoError(t, err)
	defer row2.Close()
	row2.Next()
	var name string
	require.NoError(t, row2.Scan(&name))
	require.Equal(t, "AcmeCorp", name)
}

func TestExecute_Idempotency(t *testing.T) {
	ctx := context.Background()
	eng, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer eng.Close()

	require.NoError(t, eng.Exec(ctx, fixtureDASCustomerCurrent))
	require.NoError(t, eng.Exec(ctx, `
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date)
		VALUES (1, 'Acme', 1000.0, TIMESTAMP '2024-01-15');
	`))

	f, err := contracts.LoadFocal("../../testdata/contracts/valid/dab_simple/customer.yml")
	require.NoError(t, err)
	plan, err := dab.BuildEntityPlan(&contracts.FocalBundle{EntityID: "customer", Focal: f})
	require.NoError(t, err)

	require.NoError(t, dab.Execute(ctx, eng, plan))
	require.NoError(t, dab.Execute(ctx, eng, plan))

	// After two builds with no DAS changes, descriptor table should have
	// exactly the rows from build #1 (idempotent NOT EXISTS guard).
	row, err := eng.Query(ctx, `SELECT count(*) FROM dab.customer__descriptor;`)
	require.NoError(t, err)
	defer row.Close()
	row.Next()
	var n int
	require.NoError(t, row.Scan(&n))
	// 1 customer × 2 outer attributes (CUSTOMER_NAME, CUSTOMER_LIFETIME_VALUE) = 2.
	require.Equal(t, 2, n)
}
```

- [ ] **Step 2: Run the tests**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -run 'TestExecute_(BiTemporalRowStFlip|Idempotency)' -v
```

Expected: PASS for both.

- [ ] **Step 3: Run the full DAB suite to confirm everything still passes**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/dab/ -v
```

Expected: PASS for all subtests in this file.

- [ ] **Step 4: Commit**

```bash
git add prism/internal/dab/execute_test.go
git commit -m "test(dab): bi-temporal ROW_ST flip + idempotency"
```

---

## Phase 5: CLI integration

### Task 20: `prism dab build` command

**Files:**
- Create: `prism/internal/cli/dab_build.go`
- Create: `prism/internal/cli/dab_build_test.go`
- Modify: `prism/internal/cli/root.go`

- [ ] **Step 1: Write the command**

Create `prism/internal/cli/dab_build.go`:

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/dab"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// addDab attaches and returns (idempotently) the `prism dab` parent command.
func addDab(root *cobra.Command) *cobra.Command {
	for _, c := range root.Commands() {
		if c.Use == "dab" {
			return c
		}
	}
	d := &cobra.Command{Use: "dab", Short: "DAB layer commands (focal entities, descriptors, relationships)"}
	root.AddCommand(d)
	return d
}

func addDabBuild(root *cobra.Command) {
	var contractsRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "build [<entity>]",
		Short: "Generate SQL from contracts/dab/; populate dab.* tables and views",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer eng.Close()
			if all || len(args) == 0 {
				return RunDabBuildAll(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot)
			}
			return RunDabBuild(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "build all focal entities")
	addDab(root).AddCommand(cmd)
}

func RunDabBuildAll(ctx context.Context, eng *duckdb.Engine, out io.Writer, contractsRoot string) error {
	dabBs, err := loadAndValidateDab(contractsRoot)
	if err != nil {
		return err
	}
	for _, b := range dabBs {
		if err := executeFocal(ctx, eng, out, b); err != nil {
			return err
		}
	}
	return nil
}

func RunDabBuild(ctx context.Context, eng *duckdb.Engine, out io.Writer, contractsRoot, entityID string) error {
	dabBs, err := loadAndValidateDab(contractsRoot)
	if err != nil {
		return err
	}
	for _, b := range dabBs {
		if b.EntityID == entityID {
			return executeFocal(ctx, eng, out, b)
		}
	}
	return fmt.Errorf("focal entity %q not found under %s", entityID, filepath.Join(contractsRoot, "dab"))
}

func loadAndValidateDab(contractsRoot string) ([]*contracts.FocalBundle, error) {
	dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return nil, err
	}
	dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
	if err != nil {
		return nil, err
	}
	for _, b := range dabBs {
		if err := contracts.ValidateFocal(b.Focal); err != nil {
			return nil, fmt.Errorf("focal %s: %w", b.EntityID, err)
		}
	}
	if err := contracts.ValidateCrossLayer(dasBs, dabBs); err != nil {
		return nil, err
	}
	return dabBs, nil
}

func executeFocal(ctx context.Context, eng *duckdb.Engine, out io.Writer, b *contracts.FocalBundle) error {
	plan, err := dab.BuildEntityPlan(b)
	if err != nil {
		return fmt.Errorf("focal %s: %w", b.EntityID, err)
	}
	if err := dab.Execute(ctx, eng, plan); err != nil {
		return fmt.Errorf("focal %s: %w", b.EntityID, err)
	}
	fmt.Fprintf(out, "  built dab.%s + dab.%s__idfr + dab.%s__descriptor (+ %d rel) + views\n",
		b.EntityID, b.EntityID, b.EntityID, len(plan.Relationships))
	return nil
}
```

- [ ] **Step 2: Wire into root**

Modify `prism/internal/cli/root.go`. After `addDasRun(root)` add `addDabBuild(root)`:

```go
	addDasRun(root)
	addDabBuild(root)
	addRun(root)
```

- [ ] **Step 3: Write a smoke test**

Create `prism/internal/cli/dab_build_test.go`:

```go
package cli_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/cli"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestDabBuild_AgainstSimpleFixture: end-to-end CLI invocation against the
// dab_simple fixture. Validates that loadAndValidate + Execute work via
// the public command path and that messages print to stdout.
func TestDabBuild_AgainstSimpleFixture(t *testing.T) {
	tmpDir := t.TempDir()
	contractsRoot := filepath.Join(tmpDir, "contracts")
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "das"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "dab"), 0o755))
	require.NoError(t, copyDir(t,
		"../../testdata/contracts/valid/das/adventure_works",
		filepath.Join(contractsRoot, "das", "adventure_works")))
	require.NoError(t, copyFile(t,
		"../../testdata/contracts/valid/dab_simple/customer.yml",
		filepath.Join(contractsRoot, "dab", "customer.yml")))

	wh := filepath.Join(tmpDir, "warehouse.duckdb")
	eng, err := duckdb.Open(wh)
	require.NoError(t, err)
	defer eng.Close()

	// Seed DAS staging table directly (skip M1 land step).
	_ = eng.Exec(context.Background(), `
		CREATE SCHEMA IF NOT EXISTS das__adventure_works;
		CREATE TABLE das__adventure_works.customer__current (
			customer_id     BIGINT    NOT NULL,
			company_name    VARCHAR,
			lifetime_value  DOUBLE,
			modified_date   TIMESTAMP NOT NULL,
			_loaded_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);
		INSERT INTO das__adventure_works.customer__current
		(customer_id, company_name, lifetime_value, modified_date)
		VALUES (1, 'Acme', 100.0, TIMESTAMP '2024-01-01');
	`)

	var out bytes.Buffer
	require.NoError(t, cli.RunDabBuild(context.Background(), eng, &out, contractsRoot, "customer"))
	require.Contains(t, out.String(), "built dab.customer")
}

func copyDir(t *testing.T, src, dst string) error {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(t *testing.T, src, dst string) error {
	t.Helper()
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
```

- [ ] **Step 4: Run the test**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/cli/ -run TestDabBuild_AgainstSimpleFixture -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add prism/internal/cli/dab_build.go prism/internal/cli/dab_build_test.go prism/internal/cli/root.go
git commit -m "feat(cli): prism dab build — generate SQL from contracts/dab; populate warehouse"
```

---

### Task 21: `prism dab run` (alias for build in M2)

**Files:**
- Create: `prism/internal/cli/dab_run.go`
- Modify: `prism/internal/cli/root.go`

- [ ] **Step 1: Write the alias**

Create `prism/internal/cli/dab_run.go`:

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addDabRun(root *cobra.Command) {
	var contractsRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "run [<entity>]",
		Short: "Alias for `prism dab build` (M2 has no separate land step for DAB)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			eng, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer eng.Close()
			if all || len(args) == 0 {
				return RunDabBuildAll(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot)
			}
			return RunDabBuild(cmd.Context(), eng, cmd.OutOrStdout(), contractsRoot, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "run all focal entities")
	addDab(root).AddCommand(cmd)
}
```

- [ ] **Step 2: Wire into root**

In `root.go`, add `addDabRun(root)` after `addDabBuild(root)`.

- [ ] **Step 3: Quick build check**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./...
```

Expected: success.

- [ ] **Step 4: Commit**

```bash
git add prism/internal/cli/dab_run.go prism/internal/cli/root.go
git commit -m "feat(cli): prism dab run (alias for build)"
```

---

### Task 22: `prism run` chains DAS + DAB

**Files:**
- Modify: `prism/internal/cli/run.go`

- [ ] **Step 1: Update the run command to chain both layers**

Replace the body of `run.go`:

```go
package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the entire warehouse end-to-end (DAS + DAB)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			if err := runDasRunAll(cmd.Context(), out, contractsRoot, lakeRoot, pipelinesRoot, warehousePath); err != nil {
				return err
			}
			eng, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer eng.Close()
			return RunDabBuildAll(cmd.Context(), eng, out, contractsRoot)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}
```

- [ ] **Step 2: Build to verify**

```bash
cd /home/user/declarative-data-architecture/prism && go build ./...
```

Expected: success.

- [ ] **Step 3: Commit**

```bash
git add prism/internal/cli/run.go
git commit -m "feat(cli): prism run chains DAS run-all + DAB build-all"
```

---

### Task 23: `prism validate` extended to DAB contracts

**Files:**
- Modify: `prism/internal/cli/validate.go`

- [ ] **Step 1: Update the validator**

Replace `validate.go`:

```go
package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
)

func addValidate(root *cobra.Command) {
	var contractsRoot string
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate all contracts under contracts/das and contracts/dab",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
			if err != nil {
				return err
			}
			for _, b := range dasBs {
				fmt.Fprintf(out, "OK das/%s (%d entities)\n", b.SourceID, len(b.Entities))
			}
			dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
			if err != nil {
				return err
			}
			for _, b := range dabBs {
				if err := contracts.ValidateFocal(b.Focal); err != nil {
					return fmt.Errorf("focal %s: %w", b.EntityID, err)
				}
				fmt.Fprintf(out, "OK dab/%s\n", b.EntityID)
			}
			if err := contracts.ValidateCrossLayer(dasBs, dabBs); err != nil {
				return err
			}
			fmt.Fprintln(out, "OK cross-layer references")
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	root.AddCommand(cmd)
}
```

- [ ] **Step 2: Smoke test the binary**

```bash
cd /home/user/declarative-data-architecture/prism && go build -o /tmp/prism ./cmd/prism && /tmp/prism validate --contracts ./testdata/contracts/valid
```

Expected output (DAS already there from M1; DAB now too):
```
OK das/adventure_works (N entities)
OK das/stripe (1 entities)
OK dab/customer
OK dab/order
OK cross-layer references
```

- [ ] **Step 3: Commit**

```bash
git add prism/internal/cli/validate.go
git commit -m "feat(cli): prism validate extended to DAB contracts + cross-layer"
```

---

### Task 24: `prism doctor` extended (DAS staging tables exist for referenced sources)

**Files:**
- Modify: `prism/internal/cli/doctor.go`

- [ ] **Step 1: Add the cross-layer probe**

Modify `prism/internal/cli/doctor.go`. After the existing contracts check, add:

```go
			// DAB cross-layer probe: every contract under contracts/dab references a known DAS contract,
			// and (warning, not error) the corresponding DAS staging table exists in DuckDB.
			dabBs, err := contracts.LoadAllDab(filepath.Join(contractsRoot, "dab"))
			if err != nil {
				fmt.Fprintf(out, "  dab:          ✗ %s\n", err)
				return err
			}
			if err := contracts.ValidateCrossLayer(bundles, dabBs); err != nil {
				fmt.Fprintf(out, "  dab:          ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  dab:          ✓ %d focal(s)\n", len(dabBs))

			// Probe DAS staging tables (warn-only).
			eng, openErr := duckdb.Open(warehousePath)
			if openErr == nil {
				defer eng.Close()
				missing := 0
				for _, b := range dabBs {
					for _, mg := range b.Focal.MappingGroups {
						for _, t := range mg.Tables {
							qschema := "das__" + t.Source
							qtable := t.Entity + "__" + t.FromOrDefault()
							rows, err := eng.Query(context.Background(),
								fmt.Sprintf("SELECT 1 FROM information_schema.tables WHERE table_schema='%s' AND table_name='%s' LIMIT 1;", qschema, qtable))
							if err != nil {
								continue
							}
							ok := rows.Next()
							rows.Close()
							if !ok {
								fmt.Fprintf(out, "  warning: focal %s references %s.%s — not present yet (run prism das build first)\n",
									b.EntityID, qschema, qtable)
								missing++
							}
						}
					}
				}
				if missing == 0 {
					fmt.Fprintf(out, "  staging:      ✓ all DAS staging tables present\n")
				}
			}
			return nil
```

(Add imports `context` and `github.com/prism-data/prism/internal/engine/duckdb` to doctor.go.)

- [ ] **Step 2: Build and smoke test**

```bash
cd /home/user/declarative-data-architecture/prism && go build -o /tmp/prism ./cmd/prism && /tmp/prism doctor --contracts ./testdata/contracts/valid --warehouse /tmp/test-doctor.duckdb
```

Expected: success with the new `dab` and `staging` lines printed.

- [ ] **Step 3: Commit**

```bash
git add prism/internal/cli/doctor.go
git commit -m "feat(cli): prism doctor extended — DAB references + DAS staging probe"
```

---

### Task 25: `prism dab discover`

**Files:**
- Create: `prism/internal/cli/dab_discover.go`
- Create: `prism/internal/cli/dab_discover_test.go`
- Modify: `prism/internal/cli/root.go`

`dab discover` scaffolds one `contracts/dab/<entity>.yml` per DAS entity that doesn't already have a corresponding focal contract. It does NOT propose atomic-context groups, relationships, or cross-source unification — those are business decisions.

- [ ] **Step 1: Write the failing test**

Create `prism/internal/cli/dab_discover_test.go`:

```go
package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/cli"
)

func TestDabDiscover_ScaffoldsFromDAS(t *testing.T) {
	tmp := t.TempDir()
	contractsRoot := filepath.Join(tmp, "contracts")
	dasDir := filepath.Join(contractsRoot, "das", "adventure_works")
	require.NoError(t, os.MkdirAll(dasDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "_source.yml"), []byte(`
version: 1
source:
  provider: odata
  base_url: "https://example/odata"
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "customer.yml"), []byte(`
version: 1
entity:
  name: Customer
schema:
  primary_key: [customer_id]
  columns:
    - {source_path: CustomerID, target_name: customer_id, type: BIGINT, mode: REQUIRED}
    - {source_path: Name, target_name: name, type: STRING, mode: NULLABLE}
    - {source_path: ModifiedDate, target_name: modified_date, type: TIMESTAMP, mode: REQUIRED}
`), 0o644))

	require.NoError(t, cli.RunDabDiscover(context.Background(), os.Stderr, contractsRoot))

	body, err := os.ReadFile(filepath.Join(contractsRoot, "dab", "customer.yml"))
	require.NoError(t, err)
	got := string(body)
	require.Contains(t, got, "id: CUSTOMER")
	require.Contains(t, got, "id: NAME")
	require.Contains(t, got, "type: STRING")
	require.Contains(t, got, "id: CUSTOMER_ID")
	require.Contains(t, got, "type: NUMBER")
	require.Contains(t, got, "source: adventure_works")
	require.Contains(t, got, "entity: customer")
}

func TestDabDiscover_DoesNotOverwriteExisting(t *testing.T) {
	tmp := t.TempDir()
	contractsRoot := filepath.Join(tmp, "contracts")
	dasDir := filepath.Join(contractsRoot, "das", "adventure_works")
	require.NoError(t, os.MkdirAll(dasDir, 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(contractsRoot, "dab"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "_source.yml"), []byte("version: 1\nsource: {provider: odata, base_url: x}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dasDir, "customer.yml"), []byte(`
version: 1
entity: {name: Customer}
schema:
  primary_key: [customer_id]
  columns:
    - {source_path: CustomerID, target_name: customer_id, type: BIGINT, mode: REQUIRED}
`), 0o644))
	existing := []byte("version: 1\nentity: {id: CUSTOMER, name: CUSTOMER, definition: x}\nattributes: [{id: X, definition: x, type: STRING}]\nmapping_groups: []\n")
	require.NoError(t, os.WriteFile(filepath.Join(contractsRoot, "dab", "customer.yml"), existing, 0o644))

	require.NoError(t, cli.RunDabDiscover(context.Background(), os.Stderr, contractsRoot))

	got, err := os.ReadFile(filepath.Join(contractsRoot, "dab", "customer.yml"))
	require.NoError(t, err)
	require.Equal(t, existing, got)
}
```

- [ ] **Step 2: Run the test (expect FAIL)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/cli/ -run TestDabDiscover -v
```

Expected: FAIL with `undefined: cli.RunDabDiscover`.

- [ ] **Step 3: Implement discover**

Create `prism/internal/cli/dab_discover.go`:

```go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/types"
)

func addDabDiscover(root *cobra.Command) {
	var contractsRoot string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "Scaffold contracts/dab/<entity>.yml for each DAS entity not yet covered",
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunDabDiscover(cmd.Context(), cmd.OutOrStdout(), contractsRoot)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	addDab(root).AddCommand(cmd)
}

// RunDabDiscover reads contracts/das/* and writes one focal scaffold to
// contracts/dab/<entity>.yml for each DAS entity that doesn't already have one.
func RunDabDiscover(ctx context.Context, out io.Writer, contractsRoot string) error {
	dasBs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	dabDir := filepath.Join(contractsRoot, "dab")
	if err := os.MkdirAll(dabDir, 0o755); err != nil {
		return err
	}
	existing := map[string]bool{}
	if entries, err := os.ReadDir(dabDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yml") {
				existing[strings.TrimSuffix(e.Name(), ".yml")] = true
			}
		}
	}
	for _, b := range dasBs {
		for _, ent := range b.Entities {
			if existing[ent.EntityID] {
				continue
			}
			body := scaffoldFocalYAML(b.SourceID, ent.EntityID, ent.Entity)
			path := filepath.Join(dabDir, ent.EntityID+".yml")
			if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
				return err
			}
			fmt.Fprintf(out, "scaffolded contracts/dab/%s.yml from das/%s/%s\n",
				ent.EntityID, b.SourceID, ent.EntityID)
		}
	}
	return nil
}

// scaffoldFocalYAML builds a 1:1 DAB scaffold from a DAS entity.
// Type mapping:
//
//	STRING/VARCHAR  -> STRING
//	NUMBER/INT/...  -> NUMBER
//	TIMESTAMP/DATE  -> STRING (with `# TODO:` comment)
//	BOOLEAN         -> STRING (with `# TODO:` comment)
func scaffoldFocalYAML(sourceID, entityID string, das *contracts.Entity) string {
	upperID := strings.ToUpper(entityID)
	var sb strings.Builder
	sb.WriteString("version: 1\n\n")
	fmt.Fprintf(&sb, "entity:\n  id: %s\n  name: %s\n  definition: \"%s\"\n\n", upperID, upperID, das.Entity.Name)
	sb.WriteString("attributes:\n")
	cols := append([]contracts.Column{}, das.Schema.Columns...)
	sort.SliceStable(cols, func(i, j int) bool { return cols[i].TargetName < cols[j].TargetName })
	for _, c := range cols {
		t := scaffoldType(c.Type)
		todo := ""
		if t == "STRING" && (c.Type == "TIMESTAMP" || c.Type == "DATE" || c.Type == "BOOLEAN") {
			todo = "  # TODO: " + c.Type + " — wrap in START_TIMESTAMP/END_TIMESTAMP group or change type"
		}
		fmt.Fprintf(&sb, "  - id: %s\n    definition: \"%s\"\n    type: %s%s\n",
			strings.ToUpper(c.TargetName), c.TargetName, t, todo)
	}
	sb.WriteString("\nmapping_groups:\n")
	fmt.Fprintf(&sb, "  - name: %s\n", sourceID)
	sb.WriteString("    tables:\n")
	fmt.Fprintf(&sb, "      - source: %s\n", sourceID)
	fmt.Fprintf(&sb, "        entity: %s\n", entityID)
	sb.WriteString("        from: current\n")
	sb.WriteString("        primary_keys:\n")
	for _, pk := range das.Schema.PrimaryKey {
		fmt.Fprintf(&sb, "          - %s\n", pk)
	}
	sb.WriteString("        attributes:\n")
	for _, c := range cols {
		fmt.Fprintf(&sb, "          - id: %s\n            transformation_expression: %s\n",
			strings.ToUpper(c.TargetName), c.TargetName)
	}
	return sb.String()
}

func scaffoldType(t string) string {
	parsed, err := types.Parse(t)
	if err != nil {
		return "STRING"
	}
	sql := parsed.DuckDBType()
	switch {
	case strings.HasPrefix(sql, "VARCHAR"), strings.HasPrefix(sql, "TEXT"):
		return "STRING"
	case strings.HasPrefix(sql, "BIGINT"), strings.HasPrefix(sql, "INTEGER"),
		strings.HasPrefix(sql, "DOUBLE"), strings.HasPrefix(sql, "DECIMAL"):
		return "NUMBER"
	case strings.HasPrefix(sql, "TIMESTAMP"), strings.HasPrefix(sql, "DATE"):
		return "STRING"
	case strings.HasPrefix(sql, "BOOLEAN"):
		return "STRING"
	}
	return "STRING"
}
```

- [ ] **Step 4: Wire into root**

In `root.go`, add `addDabDiscover(root)` after `addDabRun(root)`.

- [ ] **Step 5: Run the test (expect PASS)**

```bash
cd /home/user/declarative-data-architecture/prism && go test ./internal/cli/ -run TestDabDiscover -v
```

Expected: PASS for both subtests.

- [ ] **Step 6: Commit**

```bash
git add prism/internal/cli/dab_discover.go prism/internal/cli/dab_discover_test.go prism/internal/cli/root.go
git commit -m "feat(cli): prism dab discover — scaffold focal contracts from DAS entities"
```

---

## Phase 6: E2E + polish

### Task 26: AdventureWorks E2E test (gated by tag)

**Files:**
- Create: `prism/internal/dab/e2e_test.go` (build-tag-gated)
- Modify: `prism/Makefile` (extend `test-e2e` if needed)

The M1 e2e covers `prism das discover/run`. M2 e2e adds `prism dab discover` then a hand-edited (or fixture-supplied) focal contract, then `prism dab build`, then assertions.

- [ ] **Step 1: Write the e2e test**

Create `prism/internal/dab/e2e_test.go`:

```go
//go:build e2e

package dab_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestE2E_AdventureWorks_DAB exercises the full DAS+DAB pipeline against the
// AdventureWorks OData feed (same env as M1's e2e). Requires uv and network.
func TestE2E_AdventureWorks_DAB(t *testing.T) {
	if os.Getenv("PRISM_E2E") == "" {
		t.Skip("PRISM_E2E not set; skipping network-bound e2e")
	}
	tmp := t.TempDir()
	prismBin, err := exec.LookPath("prism")
	if err != nil {
		// Fall back to building locally.
		prismBin = filepath.Join(tmp, "prism")
		out, err := exec.Command("go", "build", "-o", prismBin, "../../cmd/prism").CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run := func(name string, args ...string) {
		cmd := exec.Command(prismBin, args...)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s: %s", name, string(out))
		t.Logf("%s:\n%s", name, string(out))
	}
	run("init", "init", "--dir", ".")
	run("das discover", "das", "discover",
		"--source", "adventure_works",
		"--metadata-url", "https://services.odata.org/V4/AdventureWorksV3/AdventureWorks.svc/$metadata")
	run("das run", "das", "run", "--all")
	run("dab discover", "dab", "discover")
	// Inspect: at minimum, there should now be a contracts/dab/customer.yml.
	_, err = os.Stat(filepath.Join(tmp, "contracts", "dab", "customer.yml"))
	require.NoError(t, err)

	run("validate", "validate")
	run("dab run", "dab", "run", "--all")

	// Open the warehouse and assert at least one focal row was produced.
	eng, err := duckdb.Open(filepath.Join(tmp, "warehouse.duckdb"))
	require.NoError(t, err)
	defer eng.Close()
	rows, err := eng.Query(context.Background(),
		`SELECT count(*) FROM dab.customer WHERE row_st = 'Y';`)
	require.NoError(t, err)
	defer rows.Close()
	rows.Next()
	var n int
	require.NoError(t, rows.Scan(&n))
	require.Greater(t, n, 0, "no rows in dab.customer after e2e build")
}
```

- [ ] **Step 2: Confirm Makefile already has `test-e2e`**

```bash
grep -n test-e2e /home/user/declarative-data-architecture/prism/Makefile
```

Expected: `test-e2e: go test -tags=e2e ./...` (M1 already wired this).

- [ ] **Step 3: Smoke-run the e2e (locally, with PRISM_E2E set)**

```bash
cd /home/user/declarative-data-architecture/prism && PRISM_E2E=1 go test -tags=e2e ./internal/dab/ -run TestE2E_AdventureWorks_DAB -v
```

Expected: PASS within ~30-60s (depends on network). If it fails on `dab discover` because the discovered contract has a column type not yet handled by `scaffoldType`, extend `scaffoldType` and re-run. If it fails on `dab run` because a discovered contract has invalid bindings, the test surface tells you which contract — fix in the test or in `scaffoldFocalYAML`.

- [ ] **Step 4: Commit**

```bash
git add prism/internal/dab/e2e_test.go
git commit -m "test(dab): AdventureWorks e2e — DAS+DAB end-to-end (gated by PRISM_E2E)"
```

---

### Task 27: README quickstart additions

**Files:**
- Modify: `prism/README.md`

- [ ] **Step 1: Append a DAB section to the quickstart**

Add to README.md, after the existing M1 quickstart:

```markdown
## DAB (Data According to Business)

After DAS lands and types your raw data, DAB layers focal entities,
descriptors, and relationships on top — bi-temporal, conformed across
sources.

```bash
prism dab discover                 # scaffold contracts/dab/*.yml from DAS
# (edit contracts/dab/customer.yml: organize attributes into atomic-context
#  groups, declare relationships to other focals, etc.)
prism validate                     # checks DAS + DAB + cross-layer references
prism dab run --all                # populate dab.* tables and views
duckdb warehouse.duckdb -c "FROM dab.customer__current LIMIT 5;"
```

`prism run` does the whole chain (`das run --all` then `dab build --all`).

See `docs/superpowers/specs/2026-05-02-prism-m2-dab-design.md` for the
full DAB design.
```

- [ ] **Step 2: Commit**

```bash
git add prism/README.md
git commit -m "docs(prism): README quickstart for DAB layer"
```

---

## Done

When all 27 tasks are complete:

- `go test ./...` passes from a fresh checkout (modulo gated e2e).
- `prism --help` shows `dab discover/build/run`.
- A repo with both `contracts/das/` and `contracts/dab/` builds end-to-end via `prism run`.
- The AdventureWorks e2e (with `PRISM_E2E=1`) passes nightly.
- The DAB design spec at `docs/superpowers/specs/2026-05-02-prism-m2-dab-design.md` matches the implemented behavior.

Push the feature branch when ready.




