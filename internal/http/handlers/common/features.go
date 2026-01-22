package common

import (
	"os"
	"strings"
)

func FeatureEnabled(name string) bool {
	if name == "" {
		return false
	}
	upperName := strings.ToUpper(name)
	upperName = strings.ReplaceAll(upperName, "-", "_")
	value := os.Getenv(upperName)
	if value == "" {
		value = os.Getenv(name)
	}
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}
