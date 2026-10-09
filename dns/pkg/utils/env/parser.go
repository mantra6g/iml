package env

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	DefaultIMLConfigMapPath = "/etc/iml/config"
)

type Config struct {
	DNSZone string
}

func GetGlobalLoomConfig() (*Config, error) {
	get := func(key string) (string, error) {
		data, err := os.ReadFile(filepath.Join(DefaultIMLConfigMapPath, key))
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(string(data)), nil
	}

	dnsZone, err := get("dns-zone")
	if err != nil {
		return nil, err
	}

	return &Config{
		DNSZone: dnsZone,
	}, nil
}
