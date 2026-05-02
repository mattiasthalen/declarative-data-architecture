# Prism M1 (DAS) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a working `prism` Go binary that turns per-entity DAS contracts into typed `__historized` tables + `__current` views in DuckDB, using dlt (in per-source uv venvs) to land raw JSONL. Validate end-to-end against AdventureWorks OData.

**Architecture:** Single Go binary with embedded Python `prism_dlt_runner` (extracted to user cache on first run). Per-source uv venvs run dlt with locked invariants (no normalization beyond `_dlt_id`/`_dlt_load_id`). DuckDB via go-duckdb (cgo) behind an `Engine`/`Dialect` interface. SQL templates render typed historized DDL + append + current view per entity. See `docs/superpowers/specs/2026-05-01-prism-m1-das-design.md` and `docs/adr/0001..0007`.

**Tech Stack:**
- Go 1.22+, [cobra](https://github.com/spf13/cobra) v1.8+, [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) v3.0.1, [jsonschema/v6](https://github.com/santhosh-tekuri/jsonschema), [go-duckdb/v2](https://github.com/marcboeker/go-duckdb), [testify](https://github.com/stretchr/testify) v1.9+
- Python 3.11+, [uv](https://docs.astral.sh/uv/) 0.5+, [dlt](https://dlthub.com) 1.5+ with `[filesystem]` extra, pytest

**Where work happens:** This plan operates in a fresh prism Go repo at `prism/` (created in Task 1). The warehouse repo (`docs/superpowers/specs/...`) is read-only context. Use absolute paths in shell commands; relative paths in code/tests.

---

## File structure (target by end of plan)

```
prism/
├── go.mod, go.sum, .gitignore, LICENSE, README.md, Makefile
├── cmd/prism/main.go                     # CLI entry, wires cobra
├── internal/
│   ├── cli/                              # cobra commands
│   │   ├── root.go, init.go, validate.go, doctor.go
│   │   ├── das_discover.go, das_land.go, das_build.go, das_run.go
│   │   └── run.go
│   ├── config/                           # prism.yml loader + defaults
│   ├── naming/                           # snake_case, identifier helpers
│   ├── types/                            # canonical type system → SQL types
│   ├── contracts/                        # YAML loader + schema validation
│   │   ├── loader.go, source.go, entity.go, validate.go
│   │   └── schemas/  (embed)             # JSON Schema files
│   ├── engine/                           # Engine + Dialect interfaces
│   │   ├── engine.go, dialect.go, spec.go
│   │   └── duckdb/duckdb.go              # DuckDB implementation
│   ├── tmpl/                             # SQL templates (text/template)
│   │   └── duckdb/  (embed)              # *.sql.tmpl files
│   ├── pipeline/                         # uv venv lifecycle + runner invocation
│   │   ├── uv.go, sync.go, run.go
│   │   └── extract.go                    # extract embedded runner to cache
│   ├── events/                           # JSONL event types (Go side)
│   ├── discover/                         # OData $metadata fetcher + scaffolder
│   │   └── odata.go
│   └── drift/                            # drift contract tests
├── runtime/dlt_runner/  (embed)          # Python runner, embedded into binary
│   ├── __main__.py, runner.py, events.py
│   ├── providers/odata.py
│   └── pyproject.toml.tmpl
└── testdata/
    ├── contracts/{valid,invalid}/        # YAML fixtures
    └── jsonl/                            # *.jsonl.gz fixtures for engine tests
```

---

## Phase 0: Repo scaffolding

### Task 1: Initialize the prism Go repo

**Files:**
- Create: `prism/.gitignore`
- Create: `prism/go.mod`
- Create: `prism/LICENSE`
- Create: `prism/README.md`
- Create: `prism/Makefile`

- [ ] **Step 1: Create the directory and init git**

```bash
mkdir -p ~/prism && cd prism && git init -b main
```

- [ ] **Step 2: Create `prism/.gitignore`**

```
# Build artifacts
/dist/
/prism
*.exe
*.test
*.out
coverage.*

# Python venvs (extracted runner cache during dev)
/.cache/
__pycache__/
*.py[cod]

# Editor
.vscode/
.idea/

# OS
.DS_Store
```

- [ ] **Step 3: Create `prism/Makefile`**

```makefile
.PHONY: build test test-unit test-py test-e2e fmt vet lint tidy

build:
	go build -o prism ./cmd/prism

test: test-unit test-py

test-unit:
	go test ./...

test-py:
	cd runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest -v

test-e2e:
	go test -tags=e2e ./...

fmt:
	gofmt -w .

vet:
	go vet ./...

tidy:
	go mod tidy
```

- [ ] **Step 4: Create `prism/LICENSE`** (MIT)

```
MIT License

Copyright (c) 2026 Prism Contributors

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 5: Create `prism/README.md` (stub)**

```markdown
# prism

Declarative data architecture CLI. Drives a DuckDB warehouse from YAML contracts.

See `docs/` in your warehouse repo (the place you put `contracts/das/...`) for the architecture spec and ADRs.

## Status

M1 (DAS) under active development.

## Install

(coming with first release)

## Quickstart

(coming with first release)
```

- [ ] **Step 6: `go mod init` and stage**

```bash
cd prism && go mod init github.com/prism-data/prism
git add .gitignore go.mod LICENSE Makefile README.md
```

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git commit -m "chore: scaffold prism Go module"
```

---

## Phase 1: Pure-Go core (TDD-able)

Tasks 2–9 build the offline core: naming, type system, contracts, engine interface, SQL templates, DuckDB engine. After this phase, `go test ./...` exercises the entire SQL generation + DuckDB round-trip path.

### Task 2: Naming utilities

**Files:**
- Create: `prism/internal/naming/naming.go`
- Create: `prism/internal/naming/naming_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// prism/internal/naming/naming_test.go
package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"Customer":          "customer",
		"SalesOrderHeader":  "sales_order_header",
		"PurchaseOrderID":   "purchase_order_id",
		"already_snake":     "already_snake",
		"HTTPServer":        "http_server",
		"customerID":        "customer_id",
		"":                  "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, ToSnakeCase(in))
		})
	}
}

func TestValidateSnakeCaseIdentifier(t *testing.T) {
	valid := []string{"customer", "sales_order_header", "x", "a1", "snake_case_123"}
	for _, s := range valid {
		t.Run("valid/"+s, func(t *testing.T) {
			require.NoError(t, ValidateSnakeCaseIdentifier(s))
		})
	}
	invalid := []string{"", "Customer", "_leading", "trailing_", "double__underscore", "1leading_digit", "with-dash", "with space"}
	for _, s := range invalid {
		t.Run("invalid/"+s, func(t *testing.T) {
			require.Error(t, ValidateSnakeCaseIdentifier(s))
		})
	}
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/naming/ -v
```

Expected: FAIL — `naming.go` does not exist.

- [ ] **Step 3: Implement**

```go
// prism/internal/naming/naming.go
package naming

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ToSnakeCase converts PascalCase / camelCase to snake_case.
// Sequences of uppercase letters are kept together (HTTPServer -> http_server).
func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			// Insert underscore when transitioning from lower/digit to upper,
			// or from upper to upper-followed-by-lower (HTTPServer split).
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				b.WriteRune('_')
			} else if unicode.IsUpper(prev) && next != 0 && unicode.IsLower(next) {
				b.WriteRune('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

var snakeRe = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// ValidateSnakeCaseIdentifier returns an error if s is not a valid snake_case
// identifier (lowercase, digits, single underscores, leading letter).
func ValidateSnakeCaseIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("identifier is empty")
	}
	if !snakeRe.MatchString(s) {
		return fmt.Errorf("identifier %q is not snake_case (lowercase letters, digits, single underscores; must start with a letter)", s)
	}
	return nil
}
```

- [ ] **Step 4: Add testify dependency**

```bash
cd prism && go get github.com/stretchr/testify@v1.9.0
```

- [ ] **Step 5: Run; verify it passes**

```bash
cd prism && go test ./internal/naming/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/naming/ prism/go.mod prism/go.sum && git commit -m "feat(naming): snake_case conversion + identifier validation"
```

---

### Task 3: Canonical type system

**Files:**
- Create: `prism/internal/types/types.go`
- Create: `prism/internal/types/types_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// prism/internal/types/types_test.go
package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseTypeBasic(t *testing.T) {
	cases := []struct {
		in       string
		wantSQL  string
	}{
		{"STRING", "VARCHAR"},
		{"INTEGER", "INTEGER"},
		{"BIGINT", "BIGINT"},
		{"BOOLEAN", "BOOLEAN"},
		{"DATE", "DATE"},
		{"TIMESTAMP", "TIMESTAMP"},
		{"JSON", "JSON"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			tp, err := Parse(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.wantSQL, tp.DuckDBType())
		})
	}
}

func TestParseDecimal(t *testing.T) {
	tp, err := Parse("DECIMAL(18,4)")
	require.NoError(t, err)
	assert.Equal(t, "DECIMAL(18,4)", tp.DuckDBType())
}

func TestParseDecimalRequiresPrecision(t *testing.T) {
	_, err := Parse("DECIMAL")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "precision")
}

func TestParseUnknown(t *testing.T) {
	_, err := Parse("FLOAT64")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown type")
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/types/ -v
```

Expected: FAIL — `types.go` does not exist.

- [ ] **Step 3: Implement**

```go
// prism/internal/types/types.go
// Package types defines prism's small canonical type system and its mapping to
// engine-specific SQL types. M1 supports DuckDB only.
package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Type represents a prism canonical type.
type Type struct {
	Name      string // STRING, INTEGER, BIGINT, BOOLEAN, DATE, TIMESTAMP, JSON, DECIMAL
	Precision int    // DECIMAL only
	Scale     int    // DECIMAL only
}

var basic = map[string]string{
	"STRING":    "VARCHAR",
	"INTEGER":   "INTEGER",
	"BIGINT":    "BIGINT",
	"BOOLEAN":   "BOOLEAN",
	"DATE":      "DATE",
	"TIMESTAMP": "TIMESTAMP",
	"JSON":      "JSON",
}

var decimalRe = regexp.MustCompile(`^DECIMAL\((\d+),\s*(\d+)\)$`)

// Parse parses a prism type literal (e.g. "STRING", "DECIMAL(18,4)").
func Parse(s string) (Type, error) {
	s = strings.TrimSpace(s)
	if sql, ok := basic[s]; ok {
		return Type{Name: s, Precision: 0, Scale: 0}, parseDuckOK(sql)
	}
	if strings.HasPrefix(s, "DECIMAL") {
		m := decimalRe.FindStringSubmatch(s)
		if m == nil {
			return Type{}, fmt.Errorf("DECIMAL requires precision and scale, e.g. DECIMAL(18,4); got %q", s)
		}
		p, _ := strconv.Atoi(m[1])
		sc, _ := strconv.Atoi(m[2])
		if p <= 0 || sc < 0 || sc > p {
			return Type{}, fmt.Errorf("DECIMAL precision/scale out of range: %q", s)
		}
		return Type{Name: "DECIMAL", Precision: p, Scale: sc}, nil
	}
	return Type{}, fmt.Errorf("unknown type %q (supported: STRING, INTEGER, BIGINT, BOOLEAN, DATE, TIMESTAMP, JSON, DECIMAL(p,s))", s)
}

// parseDuckOK is a noop sentinel; placeholder for future validation hooks.
func parseDuckOK(_ string) error { return nil }

// DuckDBType returns the DuckDB SQL type literal.
func (t Type) DuckDBType() string {
	if t.Name == "DECIMAL" {
		return fmt.Sprintf("DECIMAL(%d,%d)", t.Precision, t.Scale)
	}
	return basic[t.Name]
}
```

- [ ] **Step 4: Run; verify it passes**

```bash
cd prism && go test ./internal/types/ -v
```

Expected: all tests PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/types/ && git commit -m "feat(types): canonical prism type system + DuckDB mapping"
```

---

### Task 4: Contracts package — Source and Entity types + YAML loader

**Files:**
- Create: `prism/internal/contracts/source.go`
- Create: `prism/internal/contracts/entity.go`
- Create: `prism/internal/contracts/loader.go`
- Create: `prism/internal/contracts/loader_test.go`
- Create: `prism/testdata/contracts/valid/adventure_works/_source.yml`
- Create: `prism/testdata/contracts/valid/adventure_works/customer.yml`

- [ ] **Step 1: Add yaml.v3 dependency**

```bash
cd prism && go get gopkg.in/yaml.v3@v3.0.1
```

- [ ] **Step 2: Create fixture `prism/testdata/contracts/valid/adventure_works/_source.yml`**

```yaml
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
```

- [ ] **Step 3: Create fixture `prism/testdata/contracts/valid/adventure_works/customer.yml`**

```yaml
version: 1
entity:
  name: Customer
  description: Customer master data
incremental:
  cursor: ModifiedDate
  strategy: append
schema:
  primary_key:
    - customer_id
  columns:
    - source_path: CustomerID
      target_name: customer_id
      type: BIGINT
      mode: REQUIRED
    - source_path: CompanyName
      target_name: company_name
      type: STRING
      mode: NULLABLE
    - source_path: ModifiedDate
      target_name: modified_date
      type: TIMESTAMP
      mode: REQUIRED
```

- [ ] **Step 4: Write the failing tests**

```go
// prism/internal/contracts/loader_test.go
package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSource(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/_source.yml"))
	require.NoError(t, err)
	assert.Equal(t, 1, src.Version)
	assert.Equal(t, "odata", src.Source.Provider)
	assert.Equal(t, "https://demodata.grapecity.com/adventureworks/odata/v1/", src.Source.BaseURL)
}

func TestLoadEntity(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/customer.yml"))
	require.NoError(t, err)
	assert.Equal(t, "Customer", ent.Entity.Name)
	require.NotNil(t, ent.Incremental)
	assert.Equal(t, "ModifiedDate", ent.Incremental.Cursor)
	assert.Equal(t, []string{"customer_id"}, ent.Schema.PrimaryKey)
	require.Len(t, ent.Schema.Columns, 3)
	assert.Equal(t, "customer_id", ent.Schema.Columns[0].TargetName)
	assert.Equal(t, "BIGINT", ent.Schema.Columns[0].Type)
	assert.True(t, ent.Schema.Columns[0].Mode == "REQUIRED")
}
```

- [ ] **Step 5: Run; verify it fails**

```bash
cd prism && go test ./internal/contracts/ -v
```

Expected: FAIL — package doesn't exist.

- [ ] **Step 6: Implement source/entity types and loaders**

```go
// prism/internal/contracts/source.go
package contracts

// Source is the parsed shape of a `_source.yml` file.
type Source struct {
	Version int          `yaml:"version"`
	Source  SourceConfig `yaml:"source"`
}

type SourceConfig struct {
	Provider string `yaml:"provider"`
	BaseURL  string `yaml:"base_url"`
}
```

```go
// prism/internal/contracts/entity.go
package contracts

// Entity is the parsed shape of a per-entity `<name>.yml` file.
type Entity struct {
	Version     int                 `yaml:"version"`
	Entity      EntityIdent         `yaml:"entity"`
	Incremental *IncrementalConfig  `yaml:"incremental,omitempty"`
	Schema      Schema              `yaml:"schema"`
}

type EntityIdent struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
}

type IncrementalConfig struct {
	Cursor   string `yaml:"cursor"`
	Strategy string `yaml:"strategy"` // "append" | "replace"
}

type Schema struct {
	PrimaryKey []string `yaml:"primary_key"`
	Columns    []Column `yaml:"columns"`
}

type Column struct {
	SourcePath  string `yaml:"source_path"`
	TargetName  string `yaml:"target_name"`
	Type        string `yaml:"type"`
	Mode        string `yaml:"mode"` // "REQUIRED" | "NULLABLE"
	Description string `yaml:"description,omitempty"`
}
```

```go
// prism/internal/contracts/loader.go
package contracts

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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
```

- [ ] **Step 7: Run; verify it passes**

```bash
cd prism && go test ./internal/contracts/ -v
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/contracts/ prism/testdata/contracts/ prism/go.mod prism/go.sum && git commit -m "feat(contracts): Source/Entity types + YAML loaders"
```

---

### Task 5: JSON Schema validation (embedded)

**Files:**
- Create: `prism/internal/contracts/schemas/das_source_v1.json`
- Create: `prism/internal/contracts/schemas/das_entity_v1.json`
- Create: `prism/internal/contracts/validate.go`
- Create: `prism/internal/contracts/validate_test.go`
- Create: `prism/testdata/contracts/invalid/missing_provider/_source.yml`
- Create: `prism/testdata/contracts/invalid/duplicate_target_name/customer.yml`

- [ ] **Step 1: Create JSON Schema for `_source.yml`**

```json
// prism/internal/contracts/schemas/das_source_v1.json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://prism.dev/schemas/das_source_v1.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "source"],
  "properties": {
    "version": {"const": 1},
    "source": {
      "type": "object",
      "additionalProperties": false,
      "required": ["provider", "base_url"],
      "properties": {
        "provider": {"type": "string", "enum": ["odata"]},
        "base_url": {"type": "string", "format": "uri"}
      }
    }
  }
}
```

- [ ] **Step 2: Create JSON Schema for entity contracts**

```json
// prism/internal/contracts/schemas/das_entity_v1.json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://prism.dev/schemas/das_entity_v1.json",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "entity", "schema"],
  "properties": {
    "version": {"const": 1},
    "entity": {
      "type": "object",
      "additionalProperties": false,
      "required": ["name"],
      "properties": {
        "name": {"type": "string", "minLength": 1},
        "description": {"type": "string"}
      }
    },
    "incremental": {
      "type": "object",
      "additionalProperties": false,
      "required": ["cursor", "strategy"],
      "properties": {
        "cursor": {"type": "string", "minLength": 1},
        "strategy": {"type": "string", "enum": ["append", "replace"]}
      }
    },
    "schema": {
      "type": "object",
      "additionalProperties": false,
      "required": ["primary_key", "columns"],
      "properties": {
        "primary_key": {
          "type": "array",
          "minItems": 1,
          "items": {"type": "string", "pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)*$"}
        },
        "columns": {
          "type": "array",
          "minItems": 1,
          "items": {
            "type": "object",
            "additionalProperties": false,
            "required": ["source_path", "target_name", "type", "mode"],
            "properties": {
              "source_path": {"type": "string", "minLength": 1},
              "target_name": {"type": "string", "pattern": "^[a-z][a-z0-9]*(_[a-z0-9]+)*$"},
              "type": {"type": "string", "minLength": 1},
              "mode": {"type": "string", "enum": ["REQUIRED", "NULLABLE"]},
              "description": {"type": "string"}
            }
          }
        }
      }
    }
  }
}
```

- [ ] **Step 3: Add jsonschema/v6 dep**

```bash
cd prism && go get github.com/santhosh-tekuri/jsonschema/v6@latest
```

- [ ] **Step 4: Create invalid fixtures**

`prism/testdata/contracts/invalid/missing_provider/_source.yml`:
```yaml
version: 1
source:
  base_url: https://example.com/
```

`prism/testdata/contracts/invalid/duplicate_target_name/customer.yml`:
```yaml
version: 1
entity:
  name: Customer
schema:
  primary_key: [customer_id]
  columns:
    - source_path: A
      target_name: customer_id
      type: BIGINT
      mode: REQUIRED
    - source_path: B
      target_name: customer_id
      type: STRING
      mode: NULLABLE
```

- [ ] **Step 5: Write the failing tests**

```go
// prism/internal/contracts/validate_test.go
package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateSourceValid(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/_source.yml"))
	require.NoError(t, err)
	require.NoError(t, ValidateSource(src))
}

func TestValidateSourceMissingProvider(t *testing.T) {
	src, err := LoadSource(filepath.FromSlash("../../testdata/contracts/invalid/missing_provider/_source.yml"))
	require.NoError(t, err)
	err = ValidateSource(src)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provider")
}

func TestValidateEntityValid(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/customer.yml"))
	require.NoError(t, err)
	require.NoError(t, ValidateEntity(ent))
}

func TestValidateEntityDuplicateTargetName(t *testing.T) {
	ent, err := LoadEntity(filepath.FromSlash("../../testdata/contracts/invalid/duplicate_target_name/customer.yml"))
	require.NoError(t, err)
	err = ValidateEntity(ent)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate target_name")
}

func TestValidateEntityPKReferencesUnknownColumn(t *testing.T) {
	ent := &Entity{
		Version: 1,
		Entity:  EntityIdent{Name: "X"},
		Schema: Schema{
			PrimaryKey: []string{"missing"},
			Columns: []Column{{
				SourcePath: "A", TargetName: "a", Type: "STRING", Mode: "REQUIRED",
			}},
		},
	}
	require.Error(t, ValidateEntity(ent))
}

func TestValidateEntityUnknownType(t *testing.T) {
	ent := &Entity{
		Version: 1,
		Entity:  EntityIdent{Name: "X"},
		Schema: Schema{
			PrimaryKey: []string{"a"},
			Columns: []Column{{
				SourcePath: "A", TargetName: "a", Type: "FLOAT64", Mode: "REQUIRED",
			}},
		},
	}
	require.Error(t, ValidateEntity(ent))
}
```

- [ ] **Step 6: Implement validator**

```go
// prism/internal/contracts/validate.go
package contracts

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/prism-data/prism/internal/types"
)

//go:embed schemas/*.json
var schemasFS embed.FS

var (
	sourceSchema *jsonschema.Schema
	entitySchema *jsonschema.Schema
)

func init() {
	sourceSchema = mustCompile("schemas/das_source_v1.json")
	entitySchema = mustCompile("schemas/das_entity_v1.json")
}

func mustCompile(path string) *jsonschema.Schema {
	raw, err := schemasFS.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("embed read %s: %v", path, err))
	}
	c := jsonschema.NewCompiler()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		panic(fmt.Sprintf("parse schema %s: %v", path, err))
	}
	if err := c.AddResource(path, doc); err != nil {
		panic(fmt.Sprintf("add schema %s: %v", path, err))
	}
	sch, err := c.Compile(path)
	if err != nil {
		panic(fmt.Sprintf("compile schema %s: %v", path, err))
	}
	return sch
}

func ValidateSource(s *Source) error {
	v, err := toJSONValue(s)
	if err != nil {
		return err
	}
	if err := sourceSchema.Validate(v); err != nil {
		return fmt.Errorf("source schema: %w", err)
	}
	return nil
}

func ValidateEntity(e *Entity) error {
	v, err := toJSONValue(e)
	if err != nil {
		return err
	}
	if err := entitySchema.Validate(v); err != nil {
		return fmt.Errorf("entity schema: %w", err)
	}
	// Cross-field checks beyond JSON Schema:
	seen := map[string]bool{}
	for _, c := range e.Schema.Columns {
		if seen[c.TargetName] {
			return fmt.Errorf("duplicate target_name %q in entity %q", c.TargetName, e.Entity.Name)
		}
		seen[c.TargetName] = true
		if _, err := types.Parse(c.Type); err != nil {
			return fmt.Errorf("entity %q column %q: %w", e.Entity.Name, c.TargetName, err)
		}
	}
	for _, pk := range e.Schema.PrimaryKey {
		if !seen[pk] {
			return fmt.Errorf("primary_key %q in entity %q does not reference any declared column", pk, e.Entity.Name)
		}
	}
	return nil
}

// toJSONValue marshals a struct via YAML→JSON to feed the JSON Schema validator.
func toJSONValue(v any) (any, error) {
	yb, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(yb, &node); err != nil {
		return nil, err
	}
	jb, err := yamlNodeToJSON(&node)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(jb, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func yamlNodeToJSON(n *yaml.Node) ([]byte, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}
```

- [ ] **Step 7: Run; verify it passes**

```bash
cd prism && go test ./internal/contracts/ -v
```

Expected: all tests PASS.

- [ ] **Step 8: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/contracts/ prism/testdata/contracts/invalid/ prism/go.mod prism/go.sum && git commit -m "feat(contracts): JSON Schema validation + cross-field checks"
```

---

### Task 6: Walk-the-tree contracts loader

**Files:**
- Modify: `prism/internal/contracts/loader.go` (add `LoadAll`)
- Create: `prism/internal/contracts/loader_walk_test.go`

- [ ] **Step 1: Write the failing test**

```go
// prism/internal/contracts/loader_walk_test.go
package contracts

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAll(t *testing.T) {
	bundle, err := LoadAll(filepath.FromSlash("../../testdata/contracts/valid"))
	require.NoError(t, err)
	require.Len(t, bundle, 1, "expect exactly one source under valid/")
	src := bundle[0]
	assert.Equal(t, "adventure_works", src.SourceID)
	require.NotNil(t, src.Source)
	assert.Equal(t, "odata", src.Source.Source.Provider)
	require.Len(t, src.Entities, 1)
	ent := src.Entities[0]
	assert.Equal(t, "customer", ent.EntityID)
	assert.Equal(t, "Customer", ent.Entity.Entity.Name)
}

func TestLoadAllRejectsBadDirName(t *testing.T) {
	// Create a tmp tree with a Bad-Directory name and assert error.
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "BadName")
	require.NoError(t, mkSourceTree(t, bad))
	_, err := LoadAll(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snake_case")
}
```

- [ ] **Step 2: Add the test helper**

```go
// prism/internal/contracts/loader_walk_test.go (continued)
import "os"

func mkSourceTree(t *testing.T, dir string) error {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	src := []byte("version: 1\nsource:\n  provider: odata\n  base_url: https://x.example/\n")
	return os.WriteFile(filepath.Join(dir, "_source.yml"), src, 0o644)
}
```

(Combine the imports — adjust to the canonical Go style: a single `import (...)` block.)

- [ ] **Step 3: Run; verify it fails**

```bash
cd prism && go test ./internal/contracts/ -run LoadAll -v
```

Expected: FAIL — `LoadAll` undefined.

- [ ] **Step 4: Implement `LoadAll`**

Append to `prism/internal/contracts/loader.go`:

```go
import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/prism-data/prism/internal/naming"
)

// SourceBundle is a parsed source plus its parsed entities.
type SourceBundle struct {
	SourceID  string         // snake_case, derived from directory name
	SourceDir string         // absolute path
	Source    *Source        // parsed _source.yml
	Entities  []EntityBundle // parsed <entity>.yml files
}

type EntityBundle struct {
	EntityID  string  // snake_case, derived from filename basename
	Path      string  // absolute path
	Entity    *Entity // parsed contents
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
```

(After editing, run `go fmt` and `go vet` to settle imports.)

- [ ] **Step 5: Run; verify it passes**

```bash
cd prism && gofmt -w internal/contracts/ && go test ./internal/contracts/ -v
```

Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/contracts/ && git commit -m "feat(contracts): LoadAll walks contracts/das/<source>/{_source,<entity>}.yml"
```

---

### Task 7: Engine + Dialect interfaces and spec types

**Files:**
- Create: `prism/internal/engine/engine.go`
- Create: `prism/internal/engine/spec.go`

- [ ] **Step 1: Create the Engine interface**

```go
// prism/internal/engine/engine.go
// Package engine defines prism's storage-engine abstraction. M1 implements
// DuckDB only (see internal/engine/duckdb). Future engines (Postgres,
// BigQuery, Databricks) plug in here behind the same interface — see ADR-004.
package engine

import "context"

// Engine represents a connected warehouse engine.
type Engine interface {
	Close() error
	Exec(ctx context.Context, sql string) error
	Dialect() Dialect
}

// Dialect produces engine-specific SQL strings from spec structs. Pure
// rendering — no IO.
type Dialect interface {
	QuoteIdent(name string) string
	Schema(name string) string

	CreateSchemaIfNotExists(schema string) string
	CreateHistorizedTableIfNotExists(spec HistorizedTableSpec) string
	AppendIntoHistorized(spec HistorizedAppendSpec) string
	CreateOrReplaceCurrentView(spec CurrentViewSpec) string
}
```

- [ ] **Step 2: Create the spec structs**

```go
// prism/internal/engine/spec.go
package engine

// Column describes one declared column in a DAS contract, after type parsing.
type Column struct {
	SourcePath string // e.g. "CustomerID" or "Address.City"
	TargetName string // e.g. "customer_id"
	SQLType    string // dialect-rendered, e.g. "BIGINT", "DECIMAL(18,4)", "VARCHAR"
	NotNull    bool   // true when contract mode is REQUIRED
}

type HistorizedTableSpec struct {
	Schema  string
	Name    string // e.g. "customer__historized"
	Columns []Column
}

type HistorizedAppendSpec struct {
	Schema      string
	Name        string // e.g. "customer__historized"
	LakeGlob    string // e.g. "/abs/path/_lake/das/adventure_works/Customer/**/*.jsonl.gz"
	Compression string // "gzip"
	Columns     []Column
}

type CurrentViewSpec struct {
	Schema           string
	Name             string   // e.g. "customer__current"
	HistorizedTable  string   // e.g. "customer__historized"
	PrimaryKey       []string // target_name list
}
```

- [ ] **Step 3: Verify compiles**

```bash
cd prism && go vet ./internal/engine/
```

Expected: no errors.

- [ ] **Step 4: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/engine/ && git commit -m "feat(engine): Engine + Dialect interfaces and spec structs"
```


---

### Task 8: DuckDB SQL templates (text/template)

**Files:**
- Create: `prism/internal/tmpl/duckdb/create_schema.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/create_historized.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/append_historized.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/create_current_view.sql.tmpl`
- Create: `prism/internal/tmpl/duckdb/render.go`
- Create: `prism/internal/tmpl/duckdb/render_test.go`
- Create: `prism/internal/tmpl/duckdb/testdata/golden/*.sql`

- [ ] **Step 1: Create template files**

`prism/internal/tmpl/duckdb/create_schema.sql.tmpl`:
```
CREATE SCHEMA IF NOT EXISTS {{ quote .Schema }};
```

`prism/internal/tmpl/duckdb/create_historized.sql.tmpl`:
```
CREATE TABLE IF NOT EXISTS {{ quote .Schema }}.{{ quote .Name }} (
    _record_hash  VARCHAR  NOT NULL PRIMARY KEY,
    _dlt_id       VARCHAR  NOT NULL,
    _dlt_load_id  VARCHAR  NOT NULL,
    _loaded_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP{{ range .Columns }},
    {{ quote .TargetName }}  {{ .SQLType }}{{ if .NotNull }}  NOT NULL{{ end }}{{ end }}
);
```

`prism/internal/tmpl/duckdb/append_historized.sql.tmpl`:
```
INSERT INTO {{ quote .Schema }}.{{ quote .Name }} (
    _record_hash, _dlt_id, _dlt_load_id, _loaded_at{{ range .Columns }},
    {{ quote .TargetName }}{{ end }}
)
SELECT
    md5(typed_row::VARCHAR)                 AS _record_hash,
    json_extract_string(json, '$._dlt_id')      AS _dlt_id,
    json_extract_string(json, '$._dlt_load_id') AS _dlt_load_id,
    CURRENT_TIMESTAMP                       AS _loaded_at{{ range .Columns }},
    typed_row.{{ quote .TargetName }}{{ end }}
FROM (
    SELECT
        json,
        struct_pack(
{{- range $i, $c := .Columns }}{{ if $i }},{{ end }}
            {{ quote $c.TargetName }} := CAST(json_extract(json, '$.{{ $c.SourcePath }}') AS {{ $c.SQLType }}){{ end }}
        ) AS typed_row
    FROM read_ndjson_objects(
        '{{ .LakeGlob }}',
        compression = '{{ .Compression }}'
    ) AS t(json)
)
ON CONFLICT (_record_hash) DO NOTHING;
```

`prism/internal/tmpl/duckdb/create_current_view.sql.tmpl`:
```
CREATE OR REPLACE VIEW {{ quote .Schema }}.{{ quote .Name }} AS
SELECT * EXCLUDE (_record_hash, _dlt_id, _dlt_load_id, _loaded_at, _row_num)
FROM (
    SELECT *,
        ROW_NUMBER() OVER (
            PARTITION BY {{ range $i, $pk := .PrimaryKey }}{{ if $i }}, {{ end }}{{ quote $pk }}{{ end }}
            ORDER BY _loaded_at DESC, _record_hash DESC
        ) AS _row_num
    FROM {{ quote .Schema }}.{{ quote .HistorizedTable }}
)
WHERE _row_num = 1;
```

- [ ] **Step 2: Implement render.go**

```go
// prism/internal/tmpl/duckdb/render.go
// Package duckdb renders prism's SQL templates against the DuckDB dialect.
package duckdb

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/prism-data/prism/internal/engine"
)

//go:embed *.sql.tmpl
var tmplFS embed.FS

var funcs = template.FuncMap{
	"quote": quoteIdent,
}

// quoteIdent wraps an identifier in DuckDB's double-quote escaping.
// Example: quoteIdent("das__adventure_works") -> `"das__adventure_works"`.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func render(name string, data any) (string, error) {
	t, err := template.New(name).Funcs(funcs).ParseFS(tmplFS, name)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func RenderCreateSchema(schema string) (string, error) {
	return render("create_schema.sql.tmpl", struct{ Schema string }{schema})
}

func RenderCreateHistorized(spec engine.HistorizedTableSpec) (string, error) {
	return render("create_historized.sql.tmpl", spec)
}

func RenderAppendHistorized(spec engine.HistorizedAppendSpec) (string, error) {
	return render("append_historized.sql.tmpl", spec)
}

func RenderCreateCurrentView(spec engine.CurrentViewSpec) (string, error) {
	return render("create_current_view.sql.tmpl", spec)
}
```

- [ ] **Step 3: Write golden-file tests**

```go
// prism/internal/tmpl/duckdb/render_test.go
package duckdb

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine"
)

var update = flag.Bool("update", false, "regenerate golden files")

func goldenAssert(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".sql")
	if *update {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got+"\n"), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file %s; run `go test ./internal/tmpl/duckdb/ -update`", path)
	assert.Equal(t, string(want), got+"\n")
}

func TestCreateSchema(t *testing.T) {
	got, err := RenderCreateSchema("das__adventure_works")
	require.NoError(t, err)
	goldenAssert(t, "create_schema", got)
}

func sampleColumns() []engine.Column {
	return []engine.Column{
		{SourcePath: "CustomerID", TargetName: "customer_id", SQLType: "BIGINT", NotNull: true},
		{SourcePath: "CompanyName", TargetName: "company_name", SQLType: "VARCHAR", NotNull: false},
		{SourcePath: "ModifiedDate", TargetName: "modified_date", SQLType: "TIMESTAMP", NotNull: true},
	}
}

func TestCreateHistorized(t *testing.T) {
	got, err := RenderCreateHistorized(engine.HistorizedTableSpec{
		Schema:  "das__adventure_works",
		Name:    "customer__historized",
		Columns: sampleColumns(),
	})
	require.NoError(t, err)
	goldenAssert(t, "create_historized", got)
}

func TestAppendHistorized(t *testing.T) {
	got, err := RenderAppendHistorized(engine.HistorizedAppendSpec{
		Schema:      "das__adventure_works",
		Name:        "customer__historized",
		LakeGlob:    "/lake/das/adventure_works/Customer/**/*.jsonl.gz",
		Compression: "gzip",
		Columns:     sampleColumns(),
	})
	require.NoError(t, err)
	goldenAssert(t, "append_historized", got)
}

func TestCreateCurrentView(t *testing.T) {
	got, err := RenderCreateCurrentView(engine.CurrentViewSpec{
		Schema:          "das__adventure_works",
		Name:            "customer__current",
		HistorizedTable: "customer__historized",
		PrimaryKey:      []string{"customer_id"},
	})
	require.NoError(t, err)
	goldenAssert(t, "create_current_view", got)
}
```

- [ ] **Step 4: Generate goldens (first time only)**

```bash
cd prism && go test ./internal/tmpl/duckdb/ -update
```

Inspect the four files in `internal/tmpl/duckdb/testdata/golden/` and confirm they look correct (typed columns, MD5 hash, ON CONFLICT, ROW_NUMBER ordering).

- [ ] **Step 5: Run normally — should pass against goldens**

```bash
cd prism && go test ./internal/tmpl/duckdb/ -v
```

Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/tmpl/ && git commit -m "feat(tmpl/duckdb): SQL templates + golden-file tests"
```


---

### Task 9: DuckDB engine + dialect implementation; round-trip integration test

**Files:**
- Create: `prism/internal/engine/duckdb/duckdb.go`
- Create: `prism/internal/engine/duckdb/duckdb_test.go`
- Create: `prism/testdata/jsonl/customer_v1.jsonl.gz`

- [ ] **Step 1: Add go-duckdb dependency**

```bash
cd prism && go get github.com/marcboeker/go-duckdb/v2@latest
```

(Requires a C toolchain on the build machine. If `go get` fails with cgo errors, document it in `README.md` and proceed in an environment with a working C compiler.)

- [ ] **Step 2: Create the JSONL fixture**

Create raw newline-JSON content with the dlt metadata fields that prism-runner adds. Use `gzip` to compress.

```bash
cd prism && mkdir -p testdata/jsonl && cat > /tmp/customer.jsonl <<'JSONL'
{"CustomerID":1,"CompanyName":"Acme","ModifiedDate":"2026-01-01T00:00:00Z","_dlt_id":"abc1","_dlt_load_id":"L1"}
{"CustomerID":2,"CompanyName":"Beta","ModifiedDate":"2026-01-02T00:00:00Z","_dlt_id":"abc2","_dlt_load_id":"L1"}
{"CustomerID":1,"CompanyName":"Acme Updated","ModifiedDate":"2026-01-03T00:00:00Z","_dlt_id":"abc3","_dlt_load_id":"L2"}
JSONL
gzip -c /tmp/customer.jsonl > testdata/jsonl/customer_v1.jsonl.gz
```

- [ ] **Step 3: Implement the engine**

```go
// prism/internal/engine/duckdb/duckdb.go
// Package duckdb is the DuckDB implementation of the engine.Engine interface.
package duckdb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb/v2"

	"github.com/prism-data/prism/internal/engine"
	tmpl "github.com/prism-data/prism/internal/tmpl/duckdb"
)

type Engine struct {
	db *sql.DB
}

// Open returns a connected DuckDB engine. Path may be a filesystem path or
// `:memory:` for an ephemeral DB.
func Open(path string) (*Engine, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping duckdb %s: %w", path, err)
	}
	return &Engine{db: db}, nil
}

func (e *Engine) Close() error { return e.db.Close() }

func (e *Engine) Exec(ctx context.Context, sql string) error {
	_, err := e.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("exec: %w\n--- sql ---\n%s\n", err, sql)
	}
	return nil
}

// Query is exposed for tests / discovery / future inspection commands.
func (e *Engine) Query(ctx context.Context, q string) (*sql.Rows, error) {
	return e.db.QueryContext(ctx, q)
}

func (e *Engine) Dialect() engine.Dialect { return dialect{} }

type dialect struct{}

func (dialect) QuoteIdent(name string) string { return `"` + name + `"` }
func (dialect) Schema(name string) string     { return name }

func (dialect) CreateSchemaIfNotExists(schema string) string {
	s, err := tmpl.RenderCreateSchema(schema)
	if err != nil {
		panic(err) // template errors are programmer bugs
	}
	return s
}

func (dialect) CreateHistorizedTableIfNotExists(spec engine.HistorizedTableSpec) string {
	s, err := tmpl.RenderCreateHistorized(spec)
	if err != nil {
		panic(err)
	}
	return s
}

func (dialect) AppendIntoHistorized(spec engine.HistorizedAppendSpec) string {
	s, err := tmpl.RenderAppendHistorized(spec)
	if err != nil {
		panic(err)
	}
	return s
}

func (dialect) CreateOrReplaceCurrentView(spec engine.CurrentViewSpec) string {
	s, err := tmpl.RenderCreateCurrentView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
```

- [ ] **Step 4: Write the round-trip test**

```go
// prism/internal/engine/duckdb/duckdb_test.go
package duckdb

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	e, err := Open(":memory:")
	require.NoError(t, err)
	defer e.Close()
	d := e.Dialect()

	cols := []engine.Column{
		{SourcePath: "CustomerID", TargetName: "customer_id", SQLType: "BIGINT", NotNull: true},
		{SourcePath: "CompanyName", TargetName: "company_name", SQLType: "VARCHAR"},
		{SourcePath: "ModifiedDate", TargetName: "modified_date", SQLType: "TIMESTAMP", NotNull: true},
	}

	require.NoError(t, e.Exec(ctx, d.CreateSchemaIfNotExists("das__adventure_works")))
	require.NoError(t, e.Exec(ctx, d.CreateHistorizedTableIfNotExists(engine.HistorizedTableSpec{
		Schema: "das__adventure_works", Name: "customer__historized", Columns: cols,
	})))

	abs, err := filepath.Abs("../../../testdata/jsonl/customer_v1.jsonl.gz")
	require.NoError(t, err)
	require.NoError(t, e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: "das__adventure_works", Name: "customer__historized",
		LakeGlob: abs, Compression: "gzip", Columns: cols,
	})))

	// Three rows in fixture, one is a true update of customer_id=1 → all three are unique by hash.
	row := e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 3, n)

	// Idempotency: re-run append, count still 3.
	require.NoError(t, e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: "das__adventure_works", Name: "customer__historized",
		LakeGlob: abs, Compression: "gzip", Columns: cols,
	})))
	row = e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 3, n)

	// Build current view; expect one row per PK.
	require.NoError(t, e.Exec(ctx, d.CreateOrReplaceCurrentView(engine.CurrentViewSpec{
		Schema: "das__adventure_works", Name: "customer__current",
		HistorizedTable: "customer__historized", PrimaryKey: []string{"customer_id"},
	})))
	row = e.db.QueryRowContext(ctx, `SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 2, n, "two distinct customer_id values → two rows in __current")

	// The latest row for customer_id=1 should be "Acme Updated".
	row = e.db.QueryRowContext(ctx, `SELECT company_name FROM "das__adventure_works"."customer__current" WHERE customer_id = 1`)
	var name string
	require.NoError(t, row.Scan(&name))
	assert.Equal(t, "Acme Updated", name)
}
```

- [ ] **Step 5: Run; verify it passes**

```bash
cd prism && go test ./internal/engine/duckdb/ -v
```

Expected: PASS. If cgo errors occur, install gcc (Linux) / Xcode CLT (macOS) / TDM-GCC (Windows) and retry.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/engine/duckdb/ prism/testdata/jsonl/ prism/go.mod prism/go.sum && git commit -m "feat(engine/duckdb): DuckDB engine + dialect + round-trip test"
```


---

## Phase 2: Embedded Python dlt runner

Tasks 10–13 build the Python module that runs inside per-source uv venvs. The runner is later embedded into the Go binary (Phase 3, Task 17). Lives at `prism/runtime/dlt_runner/`.

### Task 10: Runtime scaffold — events.py, pyproject.toml.tmpl

**Files:**
- Create: `prism/runtime/dlt_runner/__init__.py`
- Create: `prism/runtime/dlt_runner/events.py`
- Create: `prism/runtime/dlt_runner/pyproject.toml.tmpl`
- Create: `prism/runtime/dlt_runner/tests/__init__.py`
- Create: `prism/runtime/dlt_runner/tests/test_events.py`

- [ ] **Step 1: Empty package marker**

```python
# prism/runtime/dlt_runner/__init__.py
__version__ = "0.1.0"
```

- [ ] **Step 2: pyproject template**

```
# prism/runtime/dlt_runner/pyproject.toml.tmpl
[project]
name = "prism-pipeline-{{source_id}}"
version = "0.0.0"
description = "prism-managed dlt pipeline for source {{source_id}}"
requires-python = ">=3.11"
dependencies = [
{{#each dlt_extras}}
    "dlt[{{this}}]>=1.5",
{{/each}}
    "pyyaml>=6.0",
]

[tool.uv.sources]
prism-dlt-runner = { path = "{{runner_path}}" }

[project.optional-dependencies]
prism-runner = ["prism-dlt-runner"]
```

(The Go side does the `{{...}}` substitutions before writing — see Task 15. The template uses literal mustache-like markers for clarity; Go's `text/template` will be wired with custom delimiters or simple string replace, depending on what Task 15 ends up choosing.)

- [ ] **Step 3: Write events.py with failing test**

```python
# prism/runtime/dlt_runner/tests/test_events.py
import io
import json

from prism_dlt_runner import events


def test_emit_writes_jsonl(tmp_path):
    buf = io.StringIO()
    em = events.Emitter(stream=buf)
    em.source_start("adventure_works")
    em.entity_start("Customer")
    em.entity_progress("Customer", rows=100)
    em.entity_end("Customer", rows=123, load_id="L1", files=2)
    em.source_end("adventure_works", entities=1, duration_ms=42)
    em.error("Customer", kind="http_404", message="not found")

    lines = [json.loads(line) for line in buf.getvalue().splitlines()]
    assert lines[0] == {"event": "source.start", "source": "adventure_works"}
    assert lines[1] == {"event": "entity.start", "entity": "Customer"}
    assert lines[2] == {"event": "entity.progress", "entity": "Customer", "rows": 100}
    assert lines[3] == {
        "event": "entity.end", "entity": "Customer", "rows": 123, "load_id": "L1", "files": 2
    }
    assert lines[4] == {
        "event": "source.end", "source": "adventure_works", "entities": 1, "duration_ms": 42
    }
    assert lines[5] == {
        "event": "error", "entity": "Customer", "kind": "http_404", "message": "not found"
    }
```

- [ ] **Step 4: Run; verify it fails**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml pytest tests/test_events.py -v
```

Expected: FAIL — `events` module does not exist.

- [ ] **Step 5: Implement events.py**

```python
# prism/runtime/dlt_runner/events.py
"""Structured stdout event emitter for the prism dlt runner.

Events are JSON objects, one per line, written to a configurable stream
(stdout in production, StringIO in tests). See ADR-006 and the design spec
section on IPC for the full event vocabulary.
"""

from __future__ import annotations

import json
import sys
from typing import IO, Any


class Emitter:
    def __init__(self, stream: IO[str] | None = None) -> None:
        self._stream = stream if stream is not None else sys.stdout

    def _emit(self, payload: dict[str, Any]) -> None:
        self._stream.write(json.dumps(payload, separators=(",", ":")) + "\n")
        self._stream.flush()

    def source_start(self, source: str) -> None:
        self._emit({"event": "source.start", "source": source})

    def source_end(self, source: str, entities: int, duration_ms: int) -> None:
        self._emit({
            "event": "source.end", "source": source,
            "entities": entities, "duration_ms": duration_ms,
        })

    def entity_start(self, entity: str) -> None:
        self._emit({"event": "entity.start", "entity": entity})

    def entity_progress(self, entity: str, rows: int) -> None:
        self._emit({"event": "entity.progress", "entity": entity, "rows": rows})

    def entity_end(self, entity: str, rows: int, load_id: str, files: int) -> None:
        self._emit({
            "event": "entity.end", "entity": entity,
            "rows": rows, "load_id": load_id, "files": files,
        })

    def error(self, entity: str, kind: str, message: str) -> None:
        self._emit({
            "event": "error", "entity": entity,
            "kind": kind, "message": message,
        })
```

- [ ] **Step 6: Run; verify it passes**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml pytest tests/test_events.py -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/runtime/dlt_runner/ && git commit -m "feat(runner): events.py + pyproject.toml.tmpl scaffold"
```


---

### Task 11: providers/odata.py — OData source factory with locked invariants

**Files:**
- Create: `prism/runtime/dlt_runner/providers/__init__.py`
- Create: `prism/runtime/dlt_runner/providers/odata.py`
- Create: `prism/runtime/dlt_runner/tests/test_providers_odata.py`

- [ ] **Step 1: Empty providers package marker**

```python
# prism/runtime/dlt_runner/providers/__init__.py
```

- [ ] **Step 2: Write the failing test**

```python
# prism/runtime/dlt_runner/tests/test_providers_odata.py
"""Verify the OData provider builds a dlt source with prism's invariants."""

from unittest.mock import MagicMock, patch

from prism_dlt_runner.providers import odata


def test_build_source_passes_base_url():
    src_cfg = {"provider": "odata", "base_url": "https://api.example/odata/v1/"}
    entities = [{"name": "Customer"}, {"name": "Product"}]
    with patch.object(odata, "_dlt_rest_api_source") as factory:
        factory.return_value = MagicMock(name="dlt.source")
        odata.build_source(src_cfg, entities)
        factory.assert_called_once()
        kwargs = factory.call_args.kwargs
        assert kwargs["base_url"] == "https://api.example/odata/v1/"
        # Two entity resources requested:
        assert {r["name"] for r in kwargs["resources"]} == {"Customer", "Product"}


def test_invariants_are_returned():
    invariants = odata.PRISM_INVARIANTS
    assert invariants["max_table_nesting"] == 0
    assert invariants["naming_convention"] == "direct"
    assert invariants["loader_file_format"] == "jsonl"
    assert invariants["add_dlt_id"] is True
    assert invariants["add_dlt_load_id"] is True
```

- [ ] **Step 3: Run; verify it fails**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest tests/test_providers_odata.py -v
```

Expected: FAIL — `providers.odata` does not exist.

- [ ] **Step 4: Implement providers/odata.py**

```python
# prism/runtime/dlt_runner/providers/odata.py
"""OData source factory for the prism dlt runner.

Wraps dlt's REST API helper with OData defaults (paging via @odata.nextLink,
JSON envelope under "value"). Returns a dlt.Source ready to pass to
pipeline.run() with PRISM_INVARIANTS.
"""

from __future__ import annotations

from typing import Any

# Lazy import shim so tests can patch the factory without dlt at import time.
def _dlt_rest_api_source(**kwargs: Any) -> Any:
    from dlt.sources.rest_api import rest_api_source  # type: ignore
    return rest_api_source(**kwargs)


PRISM_INVARIANTS: dict[str, Any] = {
    "write_disposition":  "append",
    "loader_file_format": "jsonl",
    "max_table_nesting":  0,
    "naming_convention":  "direct",
    "add_dlt_id":         True,
    "add_dlt_load_id":    True,
}


def build_source(src_cfg: dict, entities: list[dict]):
    """Construct a dlt.Source for the given OData endpoint and entity list.

    src_cfg keys: provider (must be "odata"), base_url
    entities: list of {"name": str, "incremental": {...}?} dicts
    """
    if src_cfg.get("provider") != "odata":
        raise ValueError(f"odata.build_source called with provider={src_cfg.get('provider')!r}")
    base_url = src_cfg["base_url"]

    resources: list[dict] = []
    for ent in entities:
        name = ent["name"]
        resource: dict[str, Any] = {
            "name": name,
            "endpoint": {
                "path": name,
                "data_selector": "value",
                "paginator": {
                    "type": "json_response",
                    "next_url_path": "@odata.nextLink",
                },
            },
        }
        inc = ent.get("incremental")
        if inc:
            resource["endpoint"]["incremental"] = {
                "cursor_path": inc["cursor"],
            }
        resources.append(resource)

    return _dlt_rest_api_source(
        base_url=base_url,
        resources=resources,
    )
```

- [ ] **Step 5: Run; verify it passes**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest tests/test_providers_odata.py -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/runtime/dlt_runner/providers/ prism/runtime/dlt_runner/tests/test_providers_odata.py && git commit -m "feat(runner/providers): odata source factory with locked invariants"
```


---

### Task 12: runner.py — load contracts, dispatch, run dlt with invariants

**Files:**
- Create: `prism/runtime/dlt_runner/runner.py`
- Create: `prism/runtime/dlt_runner/__main__.py`
- Create: `prism/runtime/dlt_runner/tests/test_runner.py`
- Create: `prism/runtime/dlt_runner/tests/fixtures/source.yml`
- Create: `prism/runtime/dlt_runner/tests/fixtures/customer.yml`

- [ ] **Step 1: Test fixtures**

`prism/runtime/dlt_runner/tests/fixtures/source.yml`:
```yaml
version: 1
source:
  provider: odata
  base_url: https://api.example/odata/v1/
```

`prism/runtime/dlt_runner/tests/fixtures/customer.yml`:
```yaml
version: 1
entity:
  name: Customer
incremental:
  cursor: ModifiedDate
  strategy: append
schema:
  primary_key: [customer_id]
  columns:
    - {source_path: CustomerID, target_name: customer_id, type: BIGINT, mode: REQUIRED}
```

- [ ] **Step 2: Write failing test**

```python
# prism/runtime/dlt_runner/tests/test_runner.py
import io
import json
from pathlib import Path
from unittest.mock import MagicMock, patch

from prism_dlt_runner import runner


FIX = Path(__file__).parent / "fixtures"


def test_load_contracts():
    src, ents = runner.load_contracts(FIX / "source.yml", [FIX / "customer.yml"])
    assert src["source"]["provider"] == "odata"
    assert ents[0]["entity"]["name"] == "Customer"
    assert ents[0]["incremental"]["cursor"] == "ModifiedDate"


def test_run_invokes_pipeline_with_invariants(tmp_path):
    buf = io.StringIO()
    fake_pipeline = MagicMock()
    fake_pipeline.run.return_value = MagicMock(loads_ids=["L1"])
    with patch.object(runner, "_make_pipeline", return_value=fake_pipeline) as mp:
        with patch("prism_dlt_runner.providers.odata.build_source") as bs:
            bs.return_value = MagicMock(name="dlt.source")
            runner.run(
                source_path=FIX / "source.yml",
                entity_paths=[FIX / "customer.yml"],
                lake_dir=tmp_path,
                stream=buf,
            )
            bs.assert_called_once()
            mp.assert_called_once()
            fake_pipeline.run.assert_called_once()
            kwargs = fake_pipeline.run.call_args.kwargs
            assert kwargs["loader_file_format"] == "jsonl"
            assert kwargs["write_disposition"] == "append"

    events = [json.loads(l) for l in buf.getvalue().splitlines()]
    kinds = [e["event"] for e in events]
    assert "source.start" in kinds
    assert "source.end" in kinds


def test_unknown_provider_errors():
    src = {"source": {"provider": "mysql", "base_url": "x"}}
    ents = []
    try:
        runner._dispatch_source(src, ents)
    except ValueError as e:
        assert "mysql" in str(e)
    else:
        raise AssertionError("expected ValueError")
```

- [ ] **Step 3: Run; verify failures**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest tests/test_runner.py -v
```

Expected: FAIL — `runner` module missing.

- [ ] **Step 4: Implement runner.py**

```python
# prism/runtime/dlt_runner/runner.py
"""Entry point logic for the prism dlt runner.

Reads one `_source.yml` and N `<entity>.yml` files; constructs a dlt.Source
via the appropriate provider factory; runs a dlt.pipeline that writes JSONL
to <lake_dir>/<source_id>/<entity>/. All transformation invariants live here
(see ADR-002, ADR-006).
"""

from __future__ import annotations

import time
from pathlib import Path
from typing import IO, Any

import yaml

from . import events
from .providers import odata as odata_provider


PRISM_INVARIANTS: dict[str, Any] = odata_provider.PRISM_INVARIANTS  # shared


def load_contracts(source_path: Path, entity_paths: list[Path]):
    src = yaml.safe_load(Path(source_path).read_text())
    ents = [yaml.safe_load(Path(p).read_text()) for p in entity_paths]
    return src, ents


def _dispatch_source(src: dict, entities: list[dict]):
    provider = src["source"]["provider"]
    if provider == "odata":
        return odata_provider.build_source(src["source"], [e["entity"] | {"incremental": e.get("incremental")} for e in entities])
    raise ValueError(f"unknown provider {provider!r}; supported: odata")


def _make_pipeline(source_id: str, lake_dir: Path):
    import dlt  # local import; the venv has it
    return dlt.pipeline(
        pipeline_name=f"prism_{source_id}",
        destination=dlt.destinations.filesystem(bucket_url=str(lake_dir.resolve())),
        dataset_name=source_id,
    )


def run(
    *,
    source_path: Path,
    entity_paths: list[Path],
    lake_dir: Path,
    stream: IO[str] | None = None,
) -> None:
    em = events.Emitter(stream=stream)
    src, ents = load_contracts(source_path, entity_paths)
    source_id = Path(source_path).parent.name  # contracts/das/<source_id>/_source.yml

    em.source_start(source_id)
    started = time.monotonic()

    try:
        dlt_source = _dispatch_source(src, ents)
        pipeline = _make_pipeline(source_id, Path(lake_dir))
        for ent in ents:
            em.entity_start(ent["entity"]["name"])
        info = pipeline.run(dlt_source, **PRISM_INVARIANTS)
        load_id = info.loads_ids[0] if getattr(info, "loads_ids", None) else "unknown"
        for ent in ents:
            em.entity_end(ent["entity"]["name"], rows=-1, load_id=load_id, files=-1)
    except Exception as exc:  # pragma: no cover — surface the error to Go side
        em.error(entity="(source)", kind=type(exc).__name__, message=str(exc))
        raise

    em.source_end(
        source_id,
        entities=len(ents),
        duration_ms=int((time.monotonic() - started) * 1000),
    )
```

- [ ] **Step 5: Implement __main__.py**

```python
# prism/runtime/dlt_runner/__main__.py
"""CLI entry: `python -m prism_dlt_runner --source <yaml> --entity <yaml> ... --lake <dir>`."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from . import runner


def main(argv: list[str] | None = None) -> int:
    p = argparse.ArgumentParser("prism_dlt_runner")
    p.add_argument("--source", required=True, type=Path,
                   help="path to _source.yml")
    p.add_argument("--entity", action="append", default=[], type=Path,
                   help="path to one entity contract (repeatable)")
    p.add_argument("--lake", required=True, type=Path,
                   help="root lake directory")
    args = p.parse_args(argv)
    if not args.entity:
        print('{"event":"error","entity":"(source)","kind":"NoEntities","message":"at least one --entity required"}', file=sys.stdout, flush=True)
        return 2
    try:
        runner.run(
            source_path=args.source, entity_paths=args.entity, lake_dir=args.lake,
        )
    except Exception:
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
```

- [ ] **Step 6: Run; verify passes**

```bash
cd prism/runtime/dlt_runner && uv run --with pytest --with pyyaml --with dlt[filesystem] pytest tests/ -v
```

Expected: all PASS (events + odata + runner).

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/runtime/dlt_runner/ && git commit -m "feat(runner): runner.py + __main__.py with invariants and event emission"
```


---

## Phase 3: uv subprocess orchestration + IPC parser

Tasks 13–16 wire Go to the Python runner. After this phase, `prism` (programmatically, not yet via CLI) can ensure a per-source venv exists, invoke the runner, and parse its event stream.

### Task 13: uv detection + version check

**Files:**
- Create: `prism/internal/pipeline/uv.go`
- Create: `prism/internal/pipeline/uv_test.go`

- [ ] **Step 1: Write failing test**

```go
// prism/internal/pipeline/uv_test.go
package pipeline

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseUVVersion(t *testing.T) {
	cases := []struct {
		out  string
		want string
	}{
		{"uv 0.5.10\n", "0.5.10"},
		{"uv 0.6.0 (Homebrew 2026-01-01)\n", "0.6.0"},
	}
	for _, c := range cases {
		got, err := parseUVVersion(c.out)
		require.NoError(t, err, c.out)
		assert.Equal(t, c.want, got)
	}
}

func TestVersionAtLeast(t *testing.T) {
	assert.True(t, versionAtLeast("0.5.0", "0.5.0"))
	assert.True(t, versionAtLeast("0.5.10", "0.5.0"))
	assert.True(t, versionAtLeast("1.0.0", "0.5.0"))
	assert.False(t, versionAtLeast("0.4.99", "0.5.0"))
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/pipeline/ -v
```

Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement uv.go**

```go
// prism/internal/pipeline/uv.go
// Package pipeline orchestrates per-source uv venvs and invokes the embedded
// dlt runner. See ADR-001 and ADR-006.
package pipeline

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

const minUVVersion = "0.5.0"

// FindUV locates the `uv` binary on PATH and verifies its version.
func FindUV(ctx context.Context) (path, version string, err error) {
	path, err = exec.LookPath("uv")
	if err != nil {
		return "", "", fmt.Errorf("uv not found on PATH (install via https://docs.astral.sh/uv/): %w", err)
	}
	out, err := exec.CommandContext(ctx, path, "--version").Output()
	if err != nil {
		return path, "", fmt.Errorf("`uv --version` failed: %w", err)
	}
	version, err = parseUVVersion(string(out))
	if err != nil {
		return path, "", err
	}
	if !versionAtLeast(version, minUVVersion) {
		return path, version, fmt.Errorf("uv %s is older than required minimum %s", version, minUVVersion)
	}
	return path, version, nil
}

func parseUVVersion(out string) (string, error) {
	out = strings.TrimSpace(out)
	if !strings.HasPrefix(out, "uv ") {
		return "", fmt.Errorf("unexpected uv version output: %q", out)
	}
	rest := strings.TrimPrefix(out, "uv ")
	v := strings.SplitN(rest, " ", 2)[0]
	return v, nil
}

func versionAtLeast(have, want string) bool {
	hp := splitVersion(have)
	wp := splitVersion(want)
	for i := 0; i < 3; i++ {
		switch {
		case hp[i] > wp[i]:
			return true
		case hp[i] < wp[i]:
			return false
		}
	}
	return true
}

func splitVersion(v string) [3]int {
	var out [3]int
	parts := strings.Split(v, ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}
```

- [ ] **Step 4: Run; verify it passes**

```bash
cd prism && go test ./internal/pipeline/ -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/pipeline/ && git commit -m "feat(pipeline): uv detection + version floor check"
```


---

### Task 14: Embed and extract the Python runner

**Files:**
- Create: `prism/internal/pipeline/extract.go`
- Create: `prism/internal/pipeline/extract_test.go`
- Modify: `prism/runtime/dlt_runner/__init__.py` (add a sentinel constant for version inspection)

- [ ] **Step 1: Sketch the embed root**

The runner directory tree (`prism/runtime/dlt_runner/`) is embedded into the Go binary using `go:embed` directives in `extract.go`. We exclude `tests/` and `__pycache__/`.

- [ ] **Step 2: Write failing test**

```go
// prism/internal/pipeline/extract_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractRunner(t *testing.T) {
	dest := t.TempDir()
	dir, err := ExtractRunner(dest)
	require.NoError(t, err)
	assert.True(t, filepath.IsAbs(dir))

	// Spot-check expected files exist.
	for _, p := range []string{
		"__init__.py",
		"events.py",
		"runner.py",
		"__main__.py",
		"providers/__init__.py",
		"providers/odata.py",
		"pyproject.toml.tmpl",
	} {
		_, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err, p)
	}
}

func TestExtractRunnerIdempotent(t *testing.T) {
	dest := t.TempDir()
	d1, err := ExtractRunner(dest)
	require.NoError(t, err)
	d2, err := ExtractRunner(dest)
	require.NoError(t, err)
	assert.Equal(t, d1, d2)
}
```

- [ ] **Step 3: Run; verify failure**

```bash
cd prism && go test ./internal/pipeline/ -run Extract -v
```

Expected: FAIL.

- [ ] **Step 4: Implement extract.go**

```go
// prism/internal/pipeline/extract.go
package pipeline

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:runtime_dlt_runner
var runnerFS embed.FS

// embedRoot is the directory inside runnerFS that contains the runner package.
const embedRoot = "runtime_dlt_runner"

// runnerVersion bumps when the embedded runner is changed; used in the cache path.
const runnerVersion = "0.1.0"

// ExtractRunner copies the embedded runner tree into <cacheDir>/<runnerVersion>/dlt_runner/.
// Returns the absolute path of the package directory (suitable for use as a uv path source).
// Re-extraction is a no-op if the destination already exists with the right marker.
func ExtractRunner(cacheDir string) (string, error) {
	root := filepath.Join(cacheDir, runnerVersion, "dlt_runner")
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	marker := filepath.Join(abs, ".prism_runner_version")
	if data, err := os.ReadFile(marker); err == nil && string(data) == runnerVersion {
		return abs, nil
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	err = fs.WalkDir(runnerFS, embedRoot, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(embedRoot, p)
		if rel == "." {
			return nil
		}
		out := filepath.Join(abs, rel)
		if d.IsDir() {
			return os.MkdirAll(out, 0o755)
		}
		data, err := runnerFS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(out, data, 0o644)
	})
	if err != nil {
		return "", fmt.Errorf("extract runner: %w", err)
	}
	if err := os.WriteFile(marker, []byte(runnerVersion), 0o644); err != nil {
		return "", err
	}
	return abs, nil
}
```

- [ ] **Step 5: Symlink (or copy) the runner tree under the embed-friendly name**

`go:embed` paths are relative to the Go file. We use a symlink (one-time setup) so the runner lives at `prism/runtime/dlt_runner/` (matching the project structure) but is also embed-addressable from `internal/pipeline/`:

```bash
cd prism/internal/pipeline && ln -s ../../runtime/dlt_runner runtime_dlt_runner
```

If symlinks aren't desired (Windows CI, etc.), an alternative is to use `//go:embed all:../../runtime/dlt_runner` — but Go disallows `..` in embed paths. The clean fix is to keep the runner under `internal/pipeline/runtime_dlt_runner/` directly. **Choose one** in the executor's environment and document it. The test below assumes the symlink is in place.

- [ ] **Step 6: Run; verify it passes**

```bash
cd prism && go test ./internal/pipeline/ -run Extract -v
```

Expected: PASS. If embed errors say "no matching files" check the symlink target.

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/pipeline/extract.go prism/internal/pipeline/extract_test.go prism/internal/pipeline/runtime_dlt_runner && git commit -m "feat(pipeline): embed + extract Python runner to cache dir"
```


---

### Task 15: Synthesize per-source `pyproject.toml`; `uv sync` wrapper

**Files:**
- Create: `prism/internal/pipeline/sync.go`
- Create: `prism/internal/pipeline/sync_test.go`

- [ ] **Step 1: Write the failing test**

```go
// prism/internal/pipeline/sync_test.go
package pipeline

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderPyproject(t *testing.T) {
	out, err := renderPyproject("adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.Contains(t, out, `name = "prism-pipeline-adventure_works"`)
	assert.Contains(t, out, `"dlt[filesystem]>=1.5"`)
	assert.Contains(t, out, `path = "/abs/runner"`)
}

func TestEnsurePyprojectWritesAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	changed, err := EnsurePyproject(dir, "adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.True(t, changed)
	// second run with same inputs is a no-op
	changed, err = EnsurePyproject(dir, "adventure_works", []string{"filesystem"}, "/abs/runner")
	require.NoError(t, err)
	assert.False(t, changed)
	// different extras → rewritten
	changed, err = EnsurePyproject(dir, "adventure_works", []string{"filesystem", "sql_database"}, "/abs/runner")
	require.NoError(t, err)
	assert.True(t, changed)

	body, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(body), `"dlt[sql_database]>=1.5"`)
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/pipeline/ -run Pyproject -v
```

Expected: FAIL.

- [ ] **Step 3: Implement sync.go**

```go
// prism/internal/pipeline/sync.go
package pipeline

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// providerExtras is the static map from contract `provider:` to dlt extras
// names installed in that source's venv. See ADR-002.
var providerExtras = map[string][]string{
	"odata": {"filesystem"},
	// M2/M3:
	// "rest_api":     {"filesystem"},
	// "sql_database": {"filesystem", "sql_database"},
}

// ExtrasFor returns the dlt extras list for the given provider.
func ExtrasFor(provider string) ([]string, error) {
	x, ok := providerExtras[provider]
	if !ok {
		return nil, fmt.Errorf("no extras mapping for provider %q (M1 supports: odata)", provider)
	}
	return x, nil
}

func renderPyproject(sourceID string, extras []string, runnerPath string) (string, error) {
	var deps strings.Builder
	for _, x := range extras {
		fmt.Fprintf(&deps, "    \"dlt[%s]>=1.5\",\n", x)
	}
	tpl := `[project]
name = "prism-pipeline-%s"
version = "0.0.0"
description = "prism-managed dlt pipeline for source %s"
requires-python = ">=3.11"
dependencies = [
%s    "pyyaml>=6.0",
    "prism-dlt-runner",
]

[tool.uv.sources]
prism-dlt-runner = { path = "%s" }
`
	return fmt.Sprintf(tpl, sourceID, sourceID, deps.String(), runnerPath), nil
}

// EnsurePyproject writes pyproject.toml under dir if missing or if its content
// differs from the rendered output. Returns (changed, error).
func EnsurePyproject(dir, sourceID string, extras []string, runnerPath string) (bool, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, err
	}
	want, err := renderPyproject(sourceID, extras, runnerPath)
	if err != nil {
		return false, err
	}
	path := filepath.Join(dir, "pyproject.toml")
	have, err := os.ReadFile(path)
	if err == nil && string(have) == want {
		return false, nil
	}
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		return false, fmt.Errorf("write %s: %w", path, err)
	}
	return true, nil
}

// UVSync runs `uv sync --project <dir>` and returns its combined output on error.
func UVSync(ctx context.Context, uvPath, projectDir string) error {
	cmd := exec.CommandContext(ctx, uvPath, "sync", "--project", projectDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("uv sync %s: %w\n%s", projectDir, err, string(out))
	}
	return nil
}
```

- [ ] **Step 4: Run; verify it passes**

```bash
cd prism && go test ./internal/pipeline/ -v
```

Expected: PASS (UVSync isn't tested here — that requires a real uv venv; covered indirectly by Phase 5 E2E).

- [ ] **Step 5: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/pipeline/sync.go prism/internal/pipeline/sync_test.go && git commit -m "feat(pipeline): synthesize pyproject.toml + uv sync wrapper"
```


---

### Task 16: Run the runner; parse JSONL events

**Files:**
- Create: `prism/internal/events/events.go`
- Create: `prism/internal/events/events_test.go`
- Create: `prism/internal/pipeline/run.go`
- Create: `prism/internal/pipeline/run_test.go`

- [ ] **Step 1: Define the event types**

```go
// prism/internal/events/events.go
// Package events models the JSONL event stream emitted by the prism dlt runner.
// See ADR-006 and the design spec section on IPC.
package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type Event struct {
	Event    string `json:"event"`
	Source   string `json:"source,omitempty"`
	Entity   string `json:"entity,omitempty"`
	Rows     int    `json:"rows,omitempty"`
	LoadID   string `json:"load_id,omitempty"`
	Files    int    `json:"files,omitempty"`
	Entities int    `json:"entities,omitempty"`
	Duration int    `json:"duration_ms,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Parse reads JSONL events from r, calling fn for each. Non-JSON lines are
// reported via fn as Event{Event:"runner.warn", Message: line}. Returns the
// first hard error (read failure or fn-returned error).
func Parse(r io.Reader, fn func(Event) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			if err := fn(Event{Event: "runner.warn", Message: line}); err != nil {
				return err
			}
			continue
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read events: %w", err)
	}
	return nil
}
```

- [ ] **Step 2: Test event parsing**

```go
// prism/internal/events/events_test.go
package events

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	in := `{"event":"source.start","source":"adventure_works"}
{"event":"entity.start","entity":"Customer"}
not-json garbage line
{"event":"entity.end","entity":"Customer","rows":12,"load_id":"L1","files":2}
`
	var got []Event
	err := Parse(strings.NewReader(in), func(e Event) error {
		got = append(got, e)
		return nil
	})
	require.NoError(t, err)
	require.Len(t, got, 4)
	assert.Equal(t, "source.start", got[0].Event)
	assert.Equal(t, "Customer", got[1].Entity)
	assert.Equal(t, "runner.warn", got[2].Event)
	assert.Equal(t, "not-json garbage line", got[2].Message)
	assert.Equal(t, 12, got[3].Rows)
}
```

- [ ] **Step 3: Run; verify**

```bash
cd prism && go test ./internal/events/ -v
```

Expected: PASS.

- [ ] **Step 4: Implement pipeline/run.go**

```go
// prism/internal/pipeline/run.go
package pipeline

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/prism-data/prism/internal/events"
)

// RunRunner invokes `uv run --project <pipelineDir> python -m prism_dlt_runner
// --source <sourceYAML> --entity <entityYAML>... --lake <lakeDir>`. Stdout
// JSONL events are parsed and dispatched to handler. Stderr is forwarded to
// stderrSink (typically os.Stderr). Returns nil iff the subprocess exited 0
// AND no error event was observed.
func RunRunner(
	ctx context.Context,
	uvPath, pipelineDir string,
	sourceYAML string, entityYAMLs []string,
	lakeDir string,
	handler func(events.Event) error,
	stderrSink io.Writer,
) error {
	args := []string{"run", "--project", pipelineDir, "python", "-m", "prism_dlt_runner",
		"--source", sourceYAML, "--lake", lakeDir,
	}
	for _, e := range entityYAMLs {
		args = append(args, "--entity", e)
	}
	cmd := exec.CommandContext(ctx, uvPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = stderrSink
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start runner: %w", err)
	}

	var sawError bool
	parseErr := events.Parse(bufio.NewReader(stdout), func(e events.Event) error {
		if e.Event == "error" {
			sawError = true
		}
		return handler(e)
	})

	waitErr := cmd.Wait()

	abs, _ := filepath.Abs(pipelineDir)
	switch {
	case parseErr != nil:
		return fmt.Errorf("event parse: %w (pipeline %s)", parseErr, abs)
	case waitErr != nil:
		return fmt.Errorf("runner exit: %w (pipeline %s)", waitErr, abs)
	case sawError:
		return fmt.Errorf("runner emitted error event (pipeline %s)", abs)
	}
	return nil
}
```

- [ ] **Step 5: Wiring test (no real subprocess)**

```go
// prism/internal/pipeline/run_test.go
package pipeline

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/events"
)

// TestRunRunnerWithFakeBinary uses /bin/sh as a stand-in for `uv` to verify
// the orchestration code parses stdout and propagates errors.
func TestRunRunnerWithFakeBinary(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-uv.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"event":"source.start","source":"x"}'
echo '{"event":"entity.end","entity":"E","rows":3,"load_id":"L1","files":1}'
echo '{"event":"source.end","source":"x","entities":1,"duration_ms":10}'
exit 0
`), 0o755))

	var got []events.Event
	var stderr bytes.Buffer
	err := RunRunner(context.Background(), scriptPath, dir, "/dev/null", []string{"/dev/null"}, dir,
		func(e events.Event) error { got = append(got, e); return nil },
		&stderr,
	)
	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "source.start", got[0].Event)
}

func TestRunRunnerErrorEvent(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "fake-uv.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte(`#!/bin/sh
echo '{"event":"error","entity":"E","kind":"X","message":"boom"}'
exit 0
`), 0o755))
	err := RunRunner(context.Background(), scriptPath, dir, "/dev/null", []string{"/dev/null"}, dir,
		func(events.Event) error { return nil }, &bytes.Buffer{},
	)
	require.Error(t, err)
}
```

- [ ] **Step 6: Run; verify both pass**

```bash
cd prism && go test ./internal/events/ ./internal/pipeline/ -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/events/ prism/internal/pipeline/run.go prism/internal/pipeline/run_test.go && git commit -m "feat(pipeline,events): runner subprocess + JSONL event parser"
```


---

## Phase 4: CLI surface

Tasks 17–24 wire the components built in Phases 1–3 to user-facing cobra commands.

### Task 17: cobra scaffold + `prism --version`

**Files:**
- Create: `prism/cmd/prism/main.go`
- Create: `prism/internal/cli/root.go`
- Create: `prism/internal/version/version.go`
- Create: `prism/internal/cli/root_test.go`

- [ ] **Step 1: Add cobra**

```bash
cd prism && go get github.com/spf13/cobra@v1.8.0
```

- [ ] **Step 2: version package**

```go
// prism/internal/version/version.go
package version

// Version is set by the linker via -ldflags "-X .../version.Version=...".
// Defaults to "dev" for local builds.
var Version = "dev"
```

- [ ] **Step 3: root.go**

```go
// prism/internal/cli/root.go
// Package cli wires the cobra command tree.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/version"
)

// NewRoot returns the top-level cobra command. Sub-commands are added via
// init() functions in their own files.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "prism",
		Short:         "Declarative data architecture CLI",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	return root
}
```

- [ ] **Step 4: main.go**

```go
// prism/cmd/prism/main.go
package main

import (
	"fmt"
	"os"

	"github.com/prism-data/prism/internal/cli"
)

func main() {
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Test that --version works**

```go
// prism/internal/cli/root_test.go
package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootVersion(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf)
	r.SetErr(&buf)
	r.SetArgs([]string{"--version"})
	require.NoError(t, r.Execute())
	assert.True(t, strings.Contains(buf.String(), "prism"), buf.String())
}
```

- [ ] **Step 6: Run; build + test**

```bash
cd prism && go build ./... && go test ./internal/cli/ ./internal/version/ -v
```

Expected: PASS, builds clean.

- [ ] **Step 7: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/cmd/ prism/internal/cli/root.go prism/internal/cli/root_test.go prism/internal/version/ prism/go.mod prism/go.sum && git commit -m "feat(cli): cobra scaffold + --version"
```


---

### Task 18: `prism init`

**Files:**
- Create: `prism/internal/cli/init.go`
- Create: `prism/internal/cli/init_test.go`

- [ ] **Step 1: Test the scaffold**

```go
// prism/internal/cli/init_test.go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit(t *testing.T) {
	dir := t.TempDir()
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf); r.SetErr(&buf)
	r.SetArgs([]string{"init", "--dir", dir})
	require.NoError(t, r.Execute(), buf.String())

	for _, p := range []string{
		"prism.yml",
		".gitignore",
		"contracts/das/.gitkeep",
	} {
		_, err := os.Stat(filepath.Join(dir, p))
		require.NoError(t, err, p)
	}

	gi, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gi), "_lake/")
	assert.Contains(t, string(gi), "_pipelines/")
	assert.Contains(t, string(gi), "warehouse.duckdb")
}

func TestInitRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "prism.yml"), []byte("x"), 0o644))
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf); r.SetErr(&buf)
	r.SetArgs([]string{"init", "--dir", dir})
	require.Error(t, r.Execute())
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/cli/ -run Init -v
```

Expected: FAIL — `init` subcommand not registered.

- [ ] **Step 3: Implement init.go**

```go
// prism/internal/cli/init.go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func init() {
	NewRoot() // ensure root is reachable for static analysis; actual wiring below
}

// addInit attaches the `init` subcommand to the root. Called from NewRoot via
// the registry pattern below.
func addInit(root *cobra.Command) {
	var dir string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Scaffold a new prism warehouse repo (prism.yml, contracts/, .gitignore)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(dir)
		},
	}
	cmd.Flags().StringVar(&dir, "dir", ".", "directory to initialize")
	root.AddCommand(cmd)
}

func runInit(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if err := refuseIfHasFile(abs, "prism.yml"); err != nil {
		return err
	}
	files := map[string]string{
		"prism.yml": `version: 1
warehouse:
  duckdb_path: ./warehouse.duckdb
paths:
  contracts: ./contracts
  lake:      ./_lake
  pipelines: ./_pipelines
`,
		".gitignore": `# prism warehouse
_lake/
_pipelines/
warehouse.duckdb
*.duckdb.wal
.cache/
`,
		"contracts/das/.gitkeep": "",
	}
	for rel, body := range files {
		full := filepath.Join(abs, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func refuseIfHasFile(dir, name string) error {
	if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
		return fmt.Errorf("%s already exists in %s; refusing to overwrite", name, dir)
	}
	return nil
}
```

- [ ] **Step 4: Wire `addInit` into the root command**

Edit `prism/internal/cli/root.go`:

```go
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "prism",
		Short:         "Declarative data architecture CLI",
		Version:       version.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	addInit(root)
	return root
}
```

- [ ] **Step 5: Run; verify passes**

```bash
cd prism && go test ./internal/cli/ -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/ && git commit -m "feat(cli): prism init"
```


---

### Task 19: `prism validate`

**Files:**
- Create: `prism/internal/cli/validate.go`
- Create: `prism/internal/cli/validate_test.go`

- [ ] **Step 1: Write failing test**

```go
// prism/internal/cli/validate_test.go
package cli

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateValid(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf); r.SetErr(&buf)
	r.SetArgs([]string{"validate", "--contracts", filepath.FromSlash("../../testdata/contracts/valid")})
	require.NoError(t, r.Execute(), buf.String())
	assert.Contains(t, buf.String(), "OK")
}

func TestValidateInvalid(t *testing.T) {
	r := NewRoot()
	var buf bytes.Buffer
	r.SetOut(&buf); r.SetErr(&buf)
	r.SetArgs([]string{"validate", "--contracts", filepath.FromSlash("../../testdata/contracts/invalid")})
	err := r.Execute()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run; verify failures**

```bash
cd prism && go test ./internal/cli/ -run Validate -v
```

Expected: FAIL — `validate` not registered.

- [ ] **Step 3: Implement validate.go**

```go
// prism/internal/cli/validate.go
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
		Short: "Validate all contracts under contracts/das/ against the embedded schema",
		RunE: func(cmd *cobra.Command, args []string) error {
			dasDir := filepath.Join(contractsRoot, "das")
			bundles, err := contracts.LoadAll(dasDir)
			if err != nil {
				return err
			}
			for _, b := range bundles {
				fmt.Fprintf(cmd.OutOrStdout(), "OK %s (%d entities)\n", b.SourceID, len(b.Entities))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root (containing das/)")
	root.AddCommand(cmd)
}
```

- [ ] **Step 4: Wire it**

In `root.go` `NewRoot()`, append `addValidate(root)`.

- [ ] **Step 5: Run; verify passes**

```bash
cd prism && go test ./internal/cli/ -v
```

Expected: PASS for `valid`; PASS (i.e., asserts the error) for `invalid`.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/validate.go prism/internal/cli/validate_test.go prism/internal/cli/root.go && git commit -m "feat(cli): prism validate"
```


---

### Task 20: `prism doctor`

**Files:**
- Create: `prism/internal/cli/doctor.go`

- [ ] **Step 1: Implement doctor**

```go
// prism/internal/cli/doctor.go
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/pipeline"
)

func addDoctor(root *cobra.Command) {
	var contractsRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Verify environment: uv, DuckDB writability, contracts parse",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Checking environment…")

			uvPath, version, err := pipeline.FindUV(context.Background())
			if err != nil {
				fmt.Fprintf(out, "  uv:           ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  uv:           ✓ %s (%s)\n", version, uvPath)

			if err := writableCheck(warehousePath); err != nil {
				fmt.Fprintf(out, "  warehouse:    ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  warehouse:    ✓ %s writable\n", warehousePath)

			dasDir := filepath.Join(contractsRoot, "das")
			bundles, err := contracts.LoadAll(dasDir)
			if err != nil {
				fmt.Fprintf(out, "  contracts:    ✗ %s\n", err)
				return err
			}
			fmt.Fprintf(out, "  contracts:    ✓ %d source(s)\n", len(bundles))
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}

func writableCheck(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
```

- [ ] **Step 2: Wire it (in `NewRoot()`, add `addDoctor(root)`)**

- [ ] **Step 3: Smoke test manually**

```bash
cd prism && go build -o prism ./cmd/prism && (cd /tmp && rm -rf prism-doctor-tmp && mkdir prism-doctor-tmp && cd prism-doctor-tmp && prism/prism init --dir . && prism/prism doctor)
```

Expected output:
```
Checking environment…
  uv:           ✓ ...
  warehouse:    ✓ ./warehouse.duckdb writable
  contracts:    ✓ 0 source(s)
```

- [ ] **Step 4: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/doctor.go prism/internal/cli/root.go && git commit -m "feat(cli): prism doctor"
```


---

### Task 21: `prism das discover` — OData $metadata → entity contract scaffolds

**Files:**
- Create: `prism/internal/discover/odata.go`
- Create: `prism/internal/discover/odata_test.go`
- Create: `prism/internal/cli/das_discover.go`
- Create: `prism/testdata/odata/$metadata.xml` (small fixture)

- [ ] **Step 1: Tiny `$metadata` fixture (XML)**

`prism/testdata/odata/$metadata.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<edmx:Edmx xmlns:edmx="http://docs.oasis-open.org/odata/ns/edmx" Version="4.0">
  <edmx:DataServices>
    <Schema xmlns="http://docs.oasis-open.org/odata/ns/edm" Namespace="AdventureWorks">
      <EntityType Name="Customer">
        <Key><PropertyRef Name="CustomerID"/></Key>
        <Property Name="CustomerID"  Type="Edm.Int64"  Nullable="false"/>
        <Property Name="CompanyName" Type="Edm.String" Nullable="true"/>
        <Property Name="ModifiedDate" Type="Edm.DateTimeOffset" Nullable="false"/>
      </EntityType>
      <EntityContainer Name="Container">
        <EntitySet Name="Customer" EntityType="AdventureWorks.Customer"/>
      </EntityContainer>
    </Schema>
  </edmx:DataServices>
</edmx:Edmx>
```

- [ ] **Step 2: Failing test for the discover/scaffolder**

```go
// prism/internal/discover/odata_test.go
package discover

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMetadata(t *testing.T) {
	data, err := os.ReadFile(filepath.FromSlash("../../testdata/odata/$metadata.xml"))
	require.NoError(t, err)
	entities, err := ParseODataMetadata(data)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	c := entities[0]
	assert.Equal(t, "Customer", c.Name)
	assert.Equal(t, []string{"CustomerID"}, c.Keys)
	require.Len(t, c.Properties, 3)
	assert.Equal(t, "Edm.Int64", c.Properties[0].EDMType)
	assert.False(t, c.Properties[0].Nullable)
}

func TestRenderScaffold(t *testing.T) {
	ent := EntityType{
		Name: "Customer",
		Keys: []string{"CustomerID"},
		Properties: []Property{
			{Name: "CustomerID", EDMType: "Edm.Int64", Nullable: false},
			{Name: "CompanyName", EDMType: "Edm.String", Nullable: true},
		},
	}
	yaml, err := RenderEntityScaffold(ent)
	require.NoError(t, err)
	assert.Contains(t, yaml, "name: Customer")
	assert.Contains(t, yaml, "primary_key:\n    - customer_id")
	assert.Contains(t, yaml, "type: BIGINT")
	assert.Contains(t, yaml, "type: STRING")
	assert.Contains(t, yaml, "mode: REQUIRED")
}
```

- [ ] **Step 3: Implement discover/odata.go**

```go
// prism/internal/discover/odata.go
// Package discover fetches upstream metadata and scaffolds prism entity
// contracts. M1 supports OData $metadata; future providers add their own
// implementations. See spec section "CLI surface" / `prism das discover`.
package discover

import (
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/prism-data/prism/internal/naming"
)

type EntityType struct {
	Name       string
	Keys       []string
	Properties []Property
}

type Property struct {
	Name     string
	EDMType  string
	Nullable bool
}

// ParseODataMetadata parses an OData v4 $metadata XML document and returns
// one EntityType per declared <EntityType>.
func ParseODataMetadata(data []byte) ([]EntityType, error) {
	var doc struct {
		XMLName      xml.Name `xml:"Edmx"`
		DataServices struct {
			Schemas []struct {
				EntityTypes []struct {
					Name string `xml:"Name,attr"`
					Key  struct {
						PropertyRefs []struct {
							Name string `xml:"Name,attr"`
						} `xml:"PropertyRef"`
					} `xml:"Key"`
					Properties []struct {
						Name     string `xml:"Name,attr"`
						Type     string `xml:"Type,attr"`
						Nullable string `xml:"Nullable,attr"`
					} `xml:"Property"`
				} `xml:"EntityType"`
			} `xml:"Schema"`
		} `xml:"DataServices"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse $metadata: %w", err)
	}
	var out []EntityType
	for _, sch := range doc.DataServices.Schemas {
		for _, et := range sch.EntityTypes {
			ent := EntityType{Name: et.Name}
			for _, k := range et.Key.PropertyRefs {
				ent.Keys = append(ent.Keys, k.Name)
			}
			for _, p := range et.Properties {
				ent.Properties = append(ent.Properties, Property{
					Name: p.Name, EDMType: p.Type, Nullable: p.Nullable != "false",
				})
			}
			out = append(out, ent)
		}
	}
	return out, nil
}

var edmToPrism = map[string]string{
	"Edm.String":         "STRING",
	"Edm.Int16":          "INTEGER",
	"Edm.Int32":          "INTEGER",
	"Edm.Int64":          "BIGINT",
	"Edm.Boolean":        "BOOLEAN",
	"Edm.Date":           "DATE",
	"Edm.DateTimeOffset": "TIMESTAMP",
	"Edm.Guid":           "STRING",
	"Edm.Decimal":        "DECIMAL(38,9)",
}

// RenderEntityScaffold produces a draft entity contract YAML for ent.
func RenderEntityScaffold(ent EntityType) (string, error) {
	var b strings.Builder
	fmt.Fprintln(&b, "version: 1")
	fmt.Fprintln(&b, "entity:")
	fmt.Fprintf(&b,  "  name: %s\n", ent.Name)
	fmt.Fprintln(&b, "schema:")
	fmt.Fprintln(&b, "  primary_key:")
	for _, k := range ent.Keys {
		fmt.Fprintf(&b, "    - %s\n", naming.ToSnakeCase(k))
	}
	fmt.Fprintln(&b, "  columns:")
	for _, p := range ent.Properties {
		typ, ok := edmToPrism[p.EDMType]
		if !ok {
			typ = "STRING" // safe fallback; user can refine
		}
		mode := "REQUIRED"
		if p.Nullable {
			mode = "NULLABLE"
		}
		fmt.Fprintf(&b, "    - source_path: %s\n", p.Name)
		fmt.Fprintf(&b, "      target_name: %s\n", naming.ToSnakeCase(p.Name))
		fmt.Fprintf(&b, "      type: %s\n", typ)
		fmt.Fprintf(&b, "      mode: %s\n", mode)
	}
	return b.String(), nil
}
```

- [ ] **Step 4: Run; expect PASS**

```bash
cd prism && go test ./internal/discover/ -v
```

- [ ] **Step 5: Implement the CLI command**

```go
// prism/internal/cli/das_discover.go
package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/discover"
	"github.com/prism-data/prism/internal/naming"
)

func addDasDiscover(root *cobra.Command) {
	var contractsRoot string
	var update bool
	cmd := &cobra.Command{
		Use:   "discover <source>",
		Short: "Generate (or refresh) per-entity contract scaffolds from upstream metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceID := args[0]
			if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
				return err
			}
			srcDir := filepath.Join(contractsRoot, "das", sourceID)
			srcPath := filepath.Join(srcDir, "_source.yml")
			src, err := contracts.LoadSource(srcPath)
			if err != nil {
				return err
			}
			if src.Source.Provider != "odata" {
				return fmt.Errorf("discover not implemented for provider %q (M1 supports: odata)", src.Source.Provider)
			}
			data, err := fetchMetadata(cmd.Context(), src.Source.BaseURL)
			if err != nil {
				return err
			}
			ents, err := discover.ParseODataMetadata(data)
			if err != nil {
				return err
			}
			for _, ent := range ents {
				yaml, err := discover.RenderEntityScaffold(ent)
				if err != nil {
					return err
				}
				dest := filepath.Join(srcDir, naming.ToSnakeCase(ent.Name)+".yml")
				if _, err := os.Stat(dest); err == nil && !update {
					fmt.Fprintf(cmd.OutOrStdout(), "skip (exists): %s\n", dest)
					continue
				}
				if err := os.WriteFile(dest, []byte(yaml), 0o644); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "wrote: %s\n", dest)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().BoolVar(&update, "update", false, "overwrite existing entity files")
	addDas(root).AddCommand(cmd)
}

// dasGroupOnce ensures `prism das` is added exactly once even if multiple
// `addDas*` registrars are called.
var dasGroup *cobra.Command

func addDas(root *cobra.Command) *cobra.Command {
	if dasGroup != nil {
		return dasGroup
	}
	dasGroup = &cobra.Command{
		Use:   "das",
		Short: "DAS layer subcommands (discover/land/build/run)",
	}
	root.AddCommand(dasGroup)
	return dasGroup
}

func fetchMetadata(ctx context.Context, baseURL string) ([]byte, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = filepath.Join(u.Path, "$metadata")
	req, _ := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	req.Header.Set("Accept", "application/xml")
	cli := &http.Client{Timeout: 30 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("$metadata: HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
```

- [ ] **Step 6: Wire it (in `NewRoot()`, add `addDasDiscover(root)`)**

- [ ] **Step 7: Build + commit (skip live network test in CI)**

```bash
cd prism && go build ./... && go test ./internal/discover/ ./internal/cli/ -v
git add internal/discover/ internal/cli/das_discover.go internal/cli/root.go testdata/odata/ && git commit -m "feat(cli): prism das discover (OData \$metadata → entity scaffolds)"
```


---

### Task 22: `prism das build` — apply DDL/append/views per entity

**Files:**
- Create: `prism/internal/cli/das_build.go`
- Create: `prism/internal/cli/das_build_test.go`

- [ ] **Step 1: Failing integration test (uses :memory: DuckDB + JSONL fixture)**

```go
// prism/internal/cli/das_build_test.go
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/engine/duckdb"
)

// TestDasBuildAgainstFixtureLake builds the DAS layer for the valid fixture
// against an in-memory DuckDB, using a tiny `_lake/` populated from the JSONL fixture.
func TestDasBuildAgainstFixtureLake(t *testing.T) {
	repoRoot := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(repoRoot, "contracts/das/adventure_works"), 0o755))
	// copy the valid fixture into the temp repo
	for _, p := range []string{"_source.yml", "customer.yml"} {
		src := filepath.FromSlash("../../testdata/contracts/valid/adventure_works/" + p)
		data, err := os.ReadFile(src)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(
			filepath.Join(repoRoot, "contracts/das/adventure_works", p),
			data, 0o644))
	}
	// stage the JSONL under <lake>/<source>/<entity-name>/
	lakeDir := filepath.Join(repoRoot, "_lake/das/adventure_works/Customer")
	require.NoError(t, os.MkdirAll(lakeDir, 0o755))
	jsonl, err := os.ReadFile(filepath.FromSlash("../../testdata/jsonl/customer_v1.jsonl.gz"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(lakeDir, "00000.jsonl.gz"), jsonl, 0o644))

	// open in-memory DuckDB and run the build directly (bypass the cobra layer
	// for unit-test speed; we exercise the cobra layer in the E2E test).
	e, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	defer e.Close()

	var buf bytes.Buffer
	err = RunDasBuild(context.Background(), e, &buf,
		filepath.Join(repoRoot, "contracts"),
		filepath.Join(repoRoot, "_lake"),
		"adventure_works", false,
	)
	require.NoError(t, err, buf.String())

	// Quick sanity: __current returns 2 distinct customer_id values
	row := e.Query(context.Background(), `SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	defer row.Close()
	require.True(t, row.Next())
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 2, n)
}
```

- [ ] **Step 2: Run; verify it fails**

```bash
cd prism && go test ./internal/cli/ -run DasBuild -v
```

Expected: FAIL — `RunDasBuild` undefined.

- [ ] **Step 3: Implement das_build.go**

```go
// prism/internal/cli/das_build.go
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
	"github.com/prism-data/prism/internal/naming"
	"github.com/prism-data/prism/internal/types"
)

func addDasBuild(root *cobra.Command) {
	var contractsRoot, lakeRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "build [<source>]",
		Short: "Generate SQL from contracts; create __historized tables and __current views",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := duckdb.Open(warehousePath)
			if err != nil {
				return err
			}
			defer e.Close()
			if all || len(args) == 0 {
				return RunDasBuildAll(cmd.Context(), e, cmd.OutOrStdout(), contractsRoot, lakeRoot)
			}
			return RunDasBuild(cmd.Context(), e, cmd.OutOrStdout(),
				contractsRoot, lakeRoot, args[0], false,
			)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "build all sources")
	addDas(root).AddCommand(cmd)
}

func RunDasBuildAll(ctx context.Context, e *duckdb.Engine, out io.Writer, contractsRoot, lakeRoot string) error {
	dasDir := filepath.Join(contractsRoot, "das")
	bundles, err := contracts.LoadAll(dasDir)
	if err != nil {
		return err
	}
	for _, b := range bundles {
		if err := RunDasBuild(ctx, e, out, contractsRoot, lakeRoot, b.SourceID, true); err != nil {
			return err
		}
	}
	return nil
}

func RunDasBuild(ctx context.Context, e *duckdb.Engine, out io.Writer, contractsRoot, lakeRoot, sourceID string, alreadyLoaded bool) error {
	if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
		return err
	}
	dasDir := filepath.Join(contractsRoot, "das")
	bundles, err := contracts.LoadAll(dasDir)
	if err != nil {
		return err
	}
	var bundle *contracts.SourceBundle
	for _, b := range bundles {
		if b.SourceID == sourceID {
			bundle = b
			break
		}
	}
	if bundle == nil {
		return fmt.Errorf("source %s: contract directory not found under %s", sourceID, dasDir)
	}
	d := e.Dialect()
	schema := "das__" + sourceID
	if err := e.Exec(ctx, d.CreateSchemaIfNotExists(schema)); err != nil {
		return err
	}
	for _, ent := range bundle.Entities {
		if err := buildOneEntity(ctx, e, schema, sourceID, lakeRoot, ent); err != nil {
			return fmt.Errorf("entity %s: %w", ent.EntityID, err)
		}
		fmt.Fprintf(out, "  built %s.%s__historized + %s__current\n", schema, ent.EntityID, ent.EntityID)
	}
	return nil
}

func buildOneEntity(ctx context.Context, e *duckdb.Engine, schema, sourceID, lakeRoot string, ent contracts.EntityBundle) error {
	d := e.Dialect()
	cols, err := toEngineColumns(ent.Entity.Schema.Columns)
	if err != nil {
		return err
	}
	tabName := ent.EntityID + "__historized"
	viewName := ent.EntityID + "__current"
	upstream := ent.Entity.Entity.Name // e.g. "Customer"
	lakeGlob, err := filepath.Abs(filepath.Join(lakeRoot, "das", sourceID, upstream, "**/*.jsonl.gz"))
	if err != nil {
		return err
	}
	if err := e.Exec(ctx, d.CreateHistorizedTableIfNotExists(engine.HistorizedTableSpec{
		Schema: schema, Name: tabName, Columns: cols,
	})); err != nil {
		return err
	}
	// Append only if there is at least one JSONL file (DuckDB errors if the
	// glob matches nothing). Use a quick filesystem probe.
	if any, _ := filepath.Glob(filepath.Join(lakeRoot, "das", sourceID, upstream, "*.jsonl.gz")); len(any) == 0 {
		// Try recursive (one-level) probe; treat empty as a no-op append.
		if any2, _ := filepath.Glob(filepath.Join(lakeRoot, "das", sourceID, upstream, "*", "*.jsonl.gz")); len(any2) == 0 {
			// no files yet; skip append, still create the view
		}
	}
	if err := e.Exec(ctx, d.AppendIntoHistorized(engine.HistorizedAppendSpec{
		Schema: schema, Name: tabName, LakeGlob: lakeGlob, Compression: "gzip", Columns: cols,
	})); err != nil {
		return err
	}
	return e.Exec(ctx, d.CreateOrReplaceCurrentView(engine.CurrentViewSpec{
		Schema: schema, Name: viewName, HistorizedTable: tabName,
		PrimaryKey: ent.Entity.Schema.PrimaryKey,
	}))
}

func toEngineColumns(cs []contracts.Column) ([]engine.Column, error) {
	var out []engine.Column
	for _, c := range cs {
		t, err := types.Parse(c.Type)
		if err != nil {
			return nil, fmt.Errorf("column %s: %w", c.TargetName, err)
		}
		out = append(out, engine.Column{
			SourcePath: c.SourcePath,
			TargetName: c.TargetName,
			SQLType:    t.DuckDBType(),
			NotNull:    c.Mode == "REQUIRED",
		})
	}
	return out, nil
}
```

- [ ] **Step 4: Wire it (in `NewRoot()`, add `addDasBuild(root)`)**

- [ ] **Step 5: Run; verify passes**

```bash
cd prism && go test ./internal/cli/ -run DasBuild -v
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/das_build.go prism/internal/cli/das_build_test.go prism/internal/cli/root.go && git commit -m "feat(cli): prism das build (DDL + append + current view per entity)"
```


---

### Task 23: `prism das land` — invoke runner via uv venv

**Files:**
- Create: `prism/internal/cli/das_land.go`

- [ ] **Step 1: Implement das_land.go**

```go
// prism/internal/cli/das_land.go
package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/events"
	"github.com/prism-data/prism/internal/naming"
	"github.com/prism-data/prism/internal/pipeline"
)

func addDasLand(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot string
	var all bool
	cmd := &cobra.Command{
		Use:   "land [<source>]",
		Short: "Run dlt to land raw JSONL into _lake/das/<source>/<entity>/",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all || len(args) == 0 {
				return runDasLandAll(cmd.Context(), cmd.OutOrStdout(),
					contractsRoot, lakeRoot, pipelinesRoot)
			}
			return runDasLand(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().BoolVar(&all, "all", false, "land all sources")
	addDas(root).AddCommand(cmd)
}

func runDasLandAll(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot string) error {
	bundles, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	for _, b := range bundles {
		if err := runDasLand(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, b.SourceID); err != nil {
			return err
		}
	}
	return nil
}

func runDasLand(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, sourceID string) error {
	if err := naming.ValidateSnakeCaseIdentifier(sourceID); err != nil {
		return err
	}
	uvPath, _, err := pipeline.FindUV(ctx)
	if err != nil {
		return err
	}
	bundle, err := loadOneBundle(contractsRoot, sourceID)
	if err != nil {
		return err
	}
	if len(bundle.Entities) == 0 {
		fmt.Fprintf(out, "no entity contracts in %s; nothing to land\n", bundle.SourceDir)
		return nil
	}
	cacheDir := defaultCacheDir()
	runnerPath, err := pipeline.ExtractRunner(cacheDir)
	if err != nil {
		return err
	}
	extras, err := pipeline.ExtrasFor(bundle.Source.Source.Provider)
	if err != nil {
		return err
	}
	pipelineDir, err := filepath.Abs(filepath.Join(pipelinesRoot, sourceID))
	if err != nil {
		return err
	}
	if _, err := pipeline.EnsurePyproject(pipelineDir, sourceID, extras, runnerPath); err != nil {
		return err
	}
	if err := pipeline.UVSync(ctx, uvPath, pipelineDir); err != nil {
		return err
	}
	srcYAML := filepath.Join(bundle.SourceDir, "_source.yml")
	var entYAMLs []string
	for _, ent := range bundle.Entities {
		entYAMLs = append(entYAMLs, ent.Path)
	}
	lakeAbs, _ := filepath.Abs(lakeRoot)
	return pipeline.RunRunner(ctx, uvPath, pipelineDir, srcYAML, entYAMLs, lakeAbs,
		func(e events.Event) error {
			fmt.Fprintf(out, "[%s] %+v\n", e.Event, e)
			return nil
		},
		os.Stderr,
	)
}

func loadOneBundle(contractsRoot, sourceID string) (*contracts.SourceBundle, error) {
	bs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if b.SourceID == sourceID {
			return b, nil
		}
	}
	return nil, fmt.Errorf("source %s: not found", sourceID)
}

func defaultCacheDir() string {
	if cd, err := os.UserCacheDir(); err == nil {
		return filepath.Join(cd, "prism")
	}
	return filepath.Join(os.TempDir(), "prism-cache")
}
```

- [ ] **Step 2: Wire it (in `NewRoot()`, add `addDasLand(root)`)**

- [ ] **Step 3: Build verification**

```bash
cd prism && go build ./...
```

Expected: clean build. (No automated test for `land` — covered by E2E in Task 27.)

- [ ] **Step 4: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/das_land.go prism/internal/cli/root.go && git commit -m "feat(cli): prism das land (uv venv + dlt runner orchestration)"
```


---

### Task 24: `prism das run` and `prism run`

**Files:**
- Create: `prism/internal/cli/das_run.go`
- Create: `prism/internal/cli/run.go`

- [ ] **Step 1: das_run.go (composition of land + build)**

```go
// prism/internal/cli/das_run.go
package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

func addDasRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	var all bool
	cmd := &cobra.Command{
		Use:   "run [<source>]",
		Short: "Convenience: prism das land then prism das build",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all || len(args) == 0 {
				return runDasRunAll(cmd.Context(), cmd.OutOrStdout(),
					contractsRoot, lakeRoot, pipelinesRoot, warehousePath)
			}
			return runDasRun(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, warehousePath, args[0])
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	cmd.Flags().BoolVar(&all, "all", false, "run all sources")
	addDas(root).AddCommand(cmd)
}

func runDasRunAll(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, warehousePath string) error {
	bs, err := contracts.LoadAll(filepath.Join(contractsRoot, "das"))
	if err != nil {
		return err
	}
	for _, b := range bs {
		if err := runDasRun(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, warehousePath, b.SourceID); err != nil {
			return err
		}
	}
	return nil
}

func runDasRun(ctx context.Context, out io.Writer, contractsRoot, lakeRoot, pipelinesRoot, warehousePath, sourceID string) error {
	fmt.Fprintf(out, "==> %s: land\n", sourceID)
	if err := runDasLand(ctx, out, contractsRoot, lakeRoot, pipelinesRoot, sourceID); err != nil {
		return err
	}
	fmt.Fprintf(out, "==> %s: build\n", sourceID)
	e, err := duckdb.Open(warehousePath)
	if err != nil {
		return err
	}
	defer e.Close()
	return RunDasBuild(ctx, e, out, contractsRoot, lakeRoot, sourceID, true)
}
```

- [ ] **Step 2: run.go (top-level alias)**

```go
// prism/internal/cli/run.go
package cli

import "github.com/spf13/cobra"

func addRun(root *cobra.Command) {
	var contractsRoot, lakeRoot, pipelinesRoot, warehousePath string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run the entire warehouse end-to-end (M1: equivalent to `prism das run --all`)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDasRunAll(cmd.Context(), cmd.OutOrStdout(),
				contractsRoot, lakeRoot, pipelinesRoot, warehousePath)
		},
	}
	cmd.Flags().StringVar(&contractsRoot, "contracts", "./contracts", "contracts root")
	cmd.Flags().StringVar(&lakeRoot, "lake", "./_lake", "lake root")
	cmd.Flags().StringVar(&pipelinesRoot, "pipelines", "./_pipelines", "uv venvs root")
	cmd.Flags().StringVar(&warehousePath, "warehouse", "./warehouse.duckdb", "DuckDB warehouse file")
	root.AddCommand(cmd)
}
```

- [ ] **Step 3: Wire both (in `NewRoot()`, add `addDasRun(root)` and `addRun(root)`)**

- [ ] **Step 4: Build verification**

```bash
cd prism && go build ./... && ./prism --help
```

Expected: help output lists `init`, `validate`, `doctor`, `das`, `run`. `prism das --help` lists `discover`, `land`, `build`, `run`.

- [ ] **Step 5: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/cli/das_run.go prism/internal/cli/run.go prism/internal/cli/root.go && git commit -m "feat(cli): prism das run + prism run"
```


---

## Phase 5: Drift detection + E2E + polish

Tasks 25–28 close the loop: drift detection runs after build, an E2E test exercises the full pipeline against AdventureWorks, and we wire up release engineering.

### Task 25: Drift contract test — NULL-in-REQUIRED detector

**Files:**
- Create: `prism/internal/drift/drift.go`
- Create: `prism/internal/drift/drift_test.go`
- Modify: `prism/internal/cli/das_build.go` (run drift checks after build)

- [ ] **Step 1: Failing test**

```go
// prism/internal/drift/drift_test.go
package drift

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/contracts"
	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

func TestDetectNoDrift(t *testing.T) {
	e, abs := setupCustomerHistorized(t, false /* no drift */)
	defer e.Close()
	res, err := DetectNullsInRequired(context.Background(), e, "das__adventure_works", "customer__historized", reqColumns())
	require.NoError(t, err)
	assert.Empty(t, res, abs)
}

func TestDetectDriftRequiredNull(t *testing.T) {
	e, _ := setupCustomerHistorized(t, true /* inject NULL */)
	defer e.Close()
	res, err := DetectNullsInRequired(context.Background(), e, "das__adventure_works", "customer__historized", reqColumns())
	require.NoError(t, err)
	require.Len(t, res, 1)
	assert.Equal(t, "modified_date", res[0].Column)
	assert.Greater(t, res[0].NullCount, 0)
}

func reqColumns() []engine.Column {
	cs, _ := loadContractCols(filepath.FromSlash("../../testdata/contracts/valid/adventure_works/customer.yml"))
	return cs
}

func loadContractCols(path string) ([]engine.Column, error) {
	ent, err := contracts.LoadEntity(path)
	if err != nil {
		return nil, err
	}
	var out []engine.Column
	for _, c := range ent.Schema.Columns {
		out = append(out, engine.Column{
			TargetName: c.TargetName, NotNull: c.Mode == "REQUIRED",
		})
	}
	return out, nil
}

// setupCustomerHistorized creates a tiny historized table; injectDrift adds a
// row whose REQUIRED column modified_date is NULL (simulating a source path
// gone missing).
func setupCustomerHistorized(t *testing.T, injectDrift bool) (*duckdb.Engine, string) {
	t.Helper()
	e, err := duckdb.Open(":memory:")
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, e.Exec(ctx, `CREATE SCHEMA "das__adventure_works"`))
	require.NoError(t, e.Exec(ctx, `CREATE TABLE "das__adventure_works"."customer__historized" (
		_record_hash VARCHAR PRIMARY KEY,
		_dlt_id VARCHAR, _dlt_load_id VARCHAR, _loaded_at TIMESTAMP,
		customer_id BIGINT NOT NULL,
		company_name VARCHAR,
		modified_date TIMESTAMP
	)`))
	require.NoError(t, e.Exec(ctx, `INSERT INTO "das__adventure_works"."customer__historized"
		VALUES ('h1','d1','L1', CURRENT_TIMESTAMP, 1, 'Acme', '2026-01-01')`))
	if injectDrift {
		require.NoError(t, e.Exec(ctx, `INSERT INTO "das__adventure_works"."customer__historized"
			VALUES ('h2','d2','L2', CURRENT_TIMESTAMP, 2, 'Beta', NULL)`))
	}
	return e, "ok"
}
```

- [ ] **Step 2: Run; verify failure**

```bash
cd prism && go test ./internal/drift/ -v
```

Expected: FAIL — package not implemented.

- [ ] **Step 3: Implement drift.go**

```go
// prism/internal/drift/drift.go
// Package drift runs lightweight contract assertions against materialized
// historized tables. M1 implements the NULL-in-REQUIRED check (see ADR-003).
package drift

import (
	"context"
	"fmt"

	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

type Result struct {
	Column    string
	NullCount int
}

// DetectNullsInRequired returns one Result per REQUIRED column with at least
// one NULL value in the given historized table.
func DetectNullsInRequired(
	ctx context.Context, e *duckdb.Engine,
	schema, table string, cols []engine.Column,
) ([]Result, error) {
	var results []Result
	for _, c := range cols {
		if !c.NotNull {
			continue
		}
		q := fmt.Sprintf(
			`SELECT count(*) FROM "%s"."%s" WHERE "%s" IS NULL`,
			schema, table, c.TargetName,
		)
		row := e.Query(ctx, q)
		// best-effort close even on error path
		var n int
		if row.Next() {
			if err := row.Scan(&n); err != nil {
				row.Close()
				return nil, err
			}
		}
		row.Close()
		if n > 0 {
			results = append(results, Result{Column: c.TargetName, NullCount: n})
		}
	}
	return results, nil
}
```

(Note: the engine's `Query` currently returns `*sql.Rows`. The test code closes via `defer row.Close()` in `Result` callsites; this implementation uses immediate close after scan. If `engine.Engine.Query` is renamed, adjust this file in lockstep.)

- [ ] **Step 4: Run; verify passes**

```bash
cd prism && go test ./internal/drift/ -v
```

Expected: PASS.

- [ ] **Step 5: Wire drift check into `RunDasBuild`**

Edit `prism/internal/cli/das_build.go` — after the `CreateOrReplaceCurrentView` exec in `buildOneEntity`, add:

```go
import "github.com/prism-data/prism/internal/drift"

// ... at the end of buildOneEntity, before `return nil`:
results, err := drift.DetectNullsInRequired(ctx, e, schema, tabName, cols)
if err != nil {
	return fmt.Errorf("drift detection: %w", err)
}
for _, r := range results {
	return fmt.Errorf("DRIFT: required column %q has %d NULLs in %s.%s — check contract source_path", r.Column, r.NullCount, schema, tabName)
}
```

(For M1 we fail-fast on first drift result. Future enhancement: collect all drift across entities and report once.)

- [ ] **Step 6: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/internal/drift/ prism/internal/cli/das_build.go && git commit -m "feat(drift): NULL-in-REQUIRED detector + post-build invocation"
```


---

### Task 26: AdventureWorks E2E test (gated)

**Files:**
- Create: `prism/cmd/prism/e2e_test.go`

- [ ] **Step 1: Write the gated E2E test**

```go
//go:build e2e

// prism/cmd/prism/e2e_test.go
package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/marcboeker/go-duckdb/v2"
)

const adventureWorksURL = "https://demodata.grapecity.com/adventureworks/odata/v1/"

// TestE2EAdventureWorks runs the entire DAS pipeline end-to-end against the
// public AdventureWorks OData endpoint. Requires `uv` on PATH and network
// access. Run with: `go test -tags=e2e -run E2E -v ./cmd/prism/`.
func TestE2EAdventureWorks(t *testing.T) {
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not available")
	}

	repo := t.TempDir()
	bin := filepath.Join(repo, "prism")

	// Build prism binary into the temp repo.
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/prism")
	cmd.Dir = projectRoot(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	run := func(args ...string) string {
		t.Helper()
		c := exec.Command(bin, args...)
		c.Dir = repo
		var buf bytes.Buffer
		c.Stdout = &buf
		c.Stderr = &buf
		require.NoError(t, c.Run(), buf.String())
		return buf.String()
	}

	// Initialize warehouse repo
	run("init", "--dir", repo)

	// Create the source declaration manually (discover requires network too,
	// but doing it explicitly here lets us scope the entities).
	srcDir := filepath.Join(repo, "contracts", "das", "adventure_works")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "_source.yml"), []byte(
		"version: 1\nsource:\n  provider: odata\n  base_url: "+adventureWorksURL+"\n",
	), 0o644))

	// Discover then keep just `customer`, `product`, `sales_order_header`.
	run("das", "discover", "adventure_works")
	keep := map[string]bool{"_source.yml": true, "customer.yml": true, "product.yml": true, "sales_order_header.yml": true}
	dir, err := os.ReadDir(srcDir)
	require.NoError(t, err)
	for _, e := range dir {
		if !keep[e.Name()] {
			require.NoError(t, os.Remove(filepath.Join(srcDir, e.Name())))
		}
	}

	run("doctor")

	// Land + build (timeout-bounded)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_ = ctx // for future cancellation hooks
	run("das", "land", "adventure_works")
	run("das", "build", "adventure_works")

	// Open the warehouse and assert presence + non-empty current view.
	dbPath := filepath.Join(repo, "warehouse.duckdb")
	db, err := sql.Open("duckdb", dbPath)
	require.NoError(t, err)
	defer db.Close()

	row := db.QueryRow(`SELECT count(*) FROM "das__adventure_works"."customer__current"`)
	var n int
	require.NoError(t, row.Scan(&n))
	assert.Greater(t, n, 0, "customer__current should have rows")

	row = db.QueryRow(`SELECT count(*) FROM "das__adventure_works"."customer__historized"`)
	require.NoError(t, row.Scan(&n))
	assert.Greater(t, n, 0)

	// Each PK appears at most once in __current.
	row = db.QueryRow(`
		SELECT count(*) FROM (
			SELECT customer_id, count(*) c FROM "das__adventure_works"."customer__current" GROUP BY customer_id
		) WHERE c > 1`)
	require.NoError(t, row.Scan(&n))
	assert.Equal(t, 0, n)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil && strings.HasSuffix(d, "prism") {
			return d
		}
	}
	t.Fatalf("could not find project root from %s", wd)
	return ""
}
```

- [ ] **Step 2: Run (only when explicitly requested)**

```bash
cd prism && go test -tags=e2e -run E2E -v ./cmd/prism/
```

Expected: PASS, takes 1–5 minutes against the live endpoint. If the AdventureWorks OData service is unreachable, the test fails with a clear network error — that's expected.

- [ ] **Step 3: Commit**

```bash
cd /home/user/declarative-data-architecture && git add prism/cmd/prism/e2e_test.go && git commit -m "test(e2e): AdventureWorks end-to-end pipeline (gated by -tags=e2e)"
```


---

### Task 27: README quickstart + CI workflow

**Files:**
- Modify: `prism/README.md` (replace stub)
- Create: `prism/.github/workflows/ci.yml`

- [ ] **Step 1: Replace `prism/README.md`**

```markdown
# prism

Declarative data architecture CLI. Drives a DuckDB warehouse from YAML contracts.

For the architecture spec and ADRs, see your warehouse repo's `docs/superpowers/specs/2026-05-01-prism-m1-das-design.md` and `docs/adr/`.

## Status

M1 (DAS layer) — usable end-to-end against AdventureWorks OData. M2 (DAB) and M3 (DAR) on the roadmap.

## Install

### From release binary (recommended)

Download the appropriate binary from the [Releases](https://github.com/prism-data/prism/releases) page.

### From source

Requires Go 1.22+ and a C toolchain (cgo for go-duckdb).

```bash
go install github.com/prism-data/prism/cmd/prism@latest
```

## External dependencies

`prism` shells out to **`uv`** (Astral's Python package manager) to manage per-source dlt venvs. Install it once:

```bash
curl -LsSf https://astral.sh/uv/install.sh | sh
```

## Quickstart

```bash
mkdir my-warehouse && cd my-warehouse
prism init
cat > contracts/das/adventure_works/_source.yml <<'YAML'
version: 1
source:
  provider: odata
  base_url: https://demodata.grapecity.com/adventureworks/odata/v1/
YAML
mkdir -p contracts/das/adventure_works
prism das discover adventure_works                 # generates per-entity scaffolds
prism doctor                                       # verify environment
prism run                                          # land + build all sources
duckdb warehouse.duckdb -c "FROM das__adventure_works.customer__current LIMIT 5;"
```

## Commands

| Command | Purpose |
|---|---|
| `prism init` | Scaffold a warehouse repo |
| `prism validate` | Lint contracts (no DB, no network) |
| `prism doctor` | Verify uv, DuckDB, contracts |
| `prism das discover <source>` | Generate per-entity contract scaffolds from upstream metadata |
| `prism das land [<source>]` | Run dlt → JSONL into `_lake/` |
| `prism das build [<source>]` | SQL: typed historized + current |
| `prism das run [<source>]` | land + build |
| `prism run` | M1 alias for `prism das run --all` |

## License

MIT.
```

- [ ] **Step 2: Add CI workflow**

```yaml
# prism/.github/workflows/ci.yml
name: ci

on:
  push: { branches: [main] }
  pull_request:

jobs:
  go-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - name: Install build deps for go-duckdb (cgo)
        run: sudo apt-get update && sudo apt-get install -y build-essential
      - name: Build
        run: go build ./...
      - name: Test
        run: go test ./...

  python-runner-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: astral-sh/setup-uv@v3
      - name: Run pytest
        working-directory: runtime/dlt_runner
        run: uv run --with pytest --with pyyaml --with dlt[filesystem] pytest -v
```

- [ ] **Step 3: Build verification & commit**

```bash
cd prism && go build ./... && git add README.md .github/ && git commit -m "docs(README): quickstart + CI workflow"
```

