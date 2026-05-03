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
	focalSchema  *jsonschema.Schema
)

func init() {
	sourceSchema = mustCompile("schemas/das_source_v1.json")
	entitySchema = mustCompile("schemas/das_entity_v1.json")
	focalSchema  = mustCompile("schemas/dab_entity_v1.json")
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
