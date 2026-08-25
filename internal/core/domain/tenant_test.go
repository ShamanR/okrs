package domain

import "testing"

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
	for slug, want := range cases {
		if got := ValidTenantSlug(slug); got != want {
			t.Errorf("ValidTenantSlug(%q) = %v, want %v", slug, got, want)
		}
	}
}
