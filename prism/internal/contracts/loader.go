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
