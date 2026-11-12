package domain

import (
	"strings"
	"testing"
)

func TestTenantSlugValid(t *testing.T) {
	cases := map[string]bool{
		"default": true,
		"acme":    true,
		"acme-eu": true,
		"a":       false, // too short (min 2)
		"Acme":    false, // uppercase
		"-acme":   false, // leading dash
		"acme-":   false, // trailing dash
		"www":     false, // reserved
		"api":     false, // reserved
	}
	cases["a"+strings.Repeat("1", 31)] = true  // 32 chars — old ceiling
	cases["a"+strings.Repeat("1", 32)] = true  // 33 chars — beyond the old limit
	cases["a"+strings.Repeat("1", 62)] = true  // 63 chars — DNS label limit
	cases["a"+strings.Repeat("1", 63)] = false // 64 chars — too long
	for slug, want := range cases {
		if got := ValidTenantSlug(slug); got != want {
			t.Errorf("ValidTenantSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}
