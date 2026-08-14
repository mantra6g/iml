package devicemap

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ParseIfExists(path string) (map[uint64]string, error) {
	result, err := Parse(path)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		return map[uint64]string{}, nil
	}
	return result, err
}

func Parse(path string) (map[uint64]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open device-map file %q: %w", path, err)
	}
	defer file.Close()

	result := make(map[uint64]string)
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("device-map file %q: malformed line %d: %q (expected id=p4target-name)",
				path, lineNumber, line)
		}

		idString := strings.TrimSpace(parts[0])
		name := strings.TrimSpace(parts[1])
		if name == "" {
			return nil, fmt.Errorf("device-map file %q: empty p4target name on line %d", path, lineNumber)
		}

		id, err := strconv.ParseUint(idString, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("device-map file %q: invalid device_id on line %d: %q: %w",
				path, lineNumber, idString, err)
		}

		if previousName, exists := result[id]; exists {
			return nil, fmt.Errorf("device-map file %q: duplicate device_id %d on line %d (already mapped to %q)",
				path, id, lineNumber, previousName)
		}
		result[id] = name
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to read device-map file %q: %w", path, err)
	}
	return result, nil
}
