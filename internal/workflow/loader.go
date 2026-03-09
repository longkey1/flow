package workflow

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func Load(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow %s: %w", path, err)
	}

	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("validating workflow %s: %w", path, err)
	}

	return &wf, nil
}

func Find(workflowsDir, name string) (string, error) {
	for _, ext := range []string{".yaml", ".yml"} {
		path := workflowsDir + "/" + name + ext
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("workflow %q not found in %s", name, workflowsDir)
}
