package common

import (
	"os"
	"strings"
)

func FeatureEnabled(flag string) bool {
	if flag == "" {
		return false
	}
	value := os.Getenv(strings.ToUpper(flag))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes") || strings.EqualFold(value, "on")
}
