package runner

import (
	"bufio"
	"os"
	"strings"
)

func parseOutputFile(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	outputs := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if ok && key != "" {
			outputs[key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return outputs, nil
}
