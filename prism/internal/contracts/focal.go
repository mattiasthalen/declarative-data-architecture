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
