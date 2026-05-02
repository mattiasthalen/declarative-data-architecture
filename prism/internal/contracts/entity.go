package contracts

// Entity is the parsed shape of a per-entity `<name>.yml` file.
type Entity struct {
	Version     int                `yaml:"version"`
	Entity      EntityIdent        `yaml:"entity"`
	Incremental *IncrementalConfig `yaml:"incremental,omitempty"`
	Schema      Schema             `yaml:"schema"`
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
