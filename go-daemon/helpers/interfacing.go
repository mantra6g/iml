package helpers

import (
	"fmt"
)

func GenerateUniqueInterfaceName(containerID string) (string, error) {
	// Generate a unique interface name based on the container ID
	if containerID == "" {
		return "", fmt.Errorf("containerID cannot be empty")
	}
	return fmt.Sprintf("nfr-%s", containerID[len(containerID)-6:]), nil
}

