package store

import (
	"testing"

	"okrs/internal/store/grants"
)

// TestGrantsCacheAliasCompiles verifies that store.NewGrantsCache is a valid alias
// for grants.NewGrantsCache and returns *grants.GrantsCache.
func TestGrantsCacheAliasCompiles(t *testing.T) {
	var _ func(*grants.GrantRepository) *grants.GrantsCache = NewGrantsCache
}
