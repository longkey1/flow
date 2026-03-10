package action

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Action, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var a Action
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("parsing action %s: %w", path, err)
	}

	if err := a.Validate(); err != nil {
		return nil, fmt.Errorf("validating action %s: %w", path, err)
	}

	return &a, nil
}

func Find(actionsDir, name string) (string, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		path := filepath.Join(actionsDir, name, "action"+ext)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("action %q not found in %s", name, actionsDir)
}
