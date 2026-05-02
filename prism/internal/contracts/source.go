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
