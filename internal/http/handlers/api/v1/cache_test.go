package v1

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// Tenant-scoped API responses (hierarchy, periods) must not be publicly cacheable: a
// `public, max-age` response is reused by the browser after a tenant switch, showing the
// previous tenant's data (and is a cross-tenant cache leak). They must be private + revalidated.
func TestAPICacheControlIsPrivateNoCache(t *testing.T) {
	w := httptest.NewRecorder()
	SetAPICacheControl(w)
	got := w.Header().Get("Cache-Control")
	if strings.Contains(got, "public") {
		t.Fatalf("scoped responses must not be public-cacheable, got %q", got)
	}
	if !strings.Contains(got, "private") || !strings.Contains(got, "no-cache") {
		t.Fatalf("Cache-Control = %q, want private + no-cache", got)
	}
}
