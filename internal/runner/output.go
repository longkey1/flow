package runner

import (
	"bufio"
	"os"
	"strings"
)

// parseDelimiterStart checks if a line matches the delimiter start syntax "KEY<<DELIMITER".
// Returns the key, delimiter, and true if matched.
func parseDelimiterStart(line string) (key, delimiter string, ok bool) {
	idx := strings.Index(line, "<<")
	if idx <= 0 {
		return "", "", false
	}
	return line[:idx], line[idx+2:], true
}

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

		// Check for delimiter syntax: KEY<<DELIMITER
		if key, delim, ok := parseDelimiterStart(line); ok {
			var lines []string
			for scanner.Scan() {
				l := scanner.Text()
				if l == delim {
					break
				}
				lines = append(lines, l)
			}
			outputs[key] = strings.Join(lines, "\n")
			continue
		}

		// Simple KEY=VALUE syntax
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
