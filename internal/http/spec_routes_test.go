package http

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const structureSpec = "../../specs/070-code-structure.md"

// TestSpecRouteTableMatchesRouter guards the "URI → пакет" map in
// specs/070-code-structure.md against the routes the server actually serves.
//
// §6 of that spec calls itself the conformance check for the one-package-per-URI rule,
// and §7 lists the deliberate exceptions. Neither claim is worth anything if nobody
// verifies it: when the table was written by hand it was already missing six live
// endpoints (the goal/KR move-up/move-down pair and comments resolve/unresolve) plus
// /no-access, and the nineteen shell URIs were undocumented altogether. This test makes
// the spec fail the build instead of drifting quietly.
//
// It reads the golden route list rather than assembling a router, so it stays a pure
// text comparison; TestRoutesGolden is what keeps the golden itself honest.
func TestSpecRouteTableMatchesRouter(t *testing.T) {
	spec, err := os.ReadFile(structureSpec)
	if err != nil {
		t.Fatalf("read %s: %v", structureSpec, err)
	}
	golden, err := os.ReadFile(routesGolden)
	if err != nil {
		t.Fatalf("read %s: %v", routesGolden, err)
	}

	documented := map[string]bool{}
	for _, uri := range specURIs(string(spec)) {
		documented[uri] = true
	}

	routed := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(golden)), "\n") {
		_, uri, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("malformed golden line: %q", line)
		}
		routed[uri] = true
	}

	if missing := diffKeys(routed, documented); len(missing) > 0 {
		t.Errorf("routes served but absent from specs/070-code-structure.md (add a §6 row, or §7 if it is a deliberate exception):\n  %s",
			strings.Join(missing, "\n  "))
	}
	if stale := diffKeys(documented, routed); len(stale) > 0 {
		t.Errorf("URIs documented in specs/070-code-structure.md but not served by the router:\n  %s",
			strings.Join(stale, "\n  "))
	}
}

// specTableRow matches a §6 row: | `/uri` | METHODS | `package` |
var specTableRow = regexp.MustCompile("^\\|\\s*`(/[^`]*)`\\s*\\|")

// specBacktickedURI matches any backticked path, used for the §7 exception section
// where one cell may name several URIs.
var specBacktickedURI = regexp.MustCompile("`(/[^`]*)`")

// specURIs collects every URI the spec accounts for: §6 table rows plus everything
// named in §7 (the exceptions, where one row may carry several URIs and a redirect
// target). Sections other than those two are ignored so that prose elsewhere — the
// package-layout examples in §1–§4 — cannot silently satisfy the check.
func specURIs(spec string) []string {
	var out []string
	section := ""
	for _, line := range strings.Split(spec, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = line
			continue
		}
		switch {
		case strings.HasPrefix(section, "## 6."):
			if m := specTableRow.FindStringSubmatch(line); m != nil {
				out = append(out, m[1])
			}
		case strings.HasPrefix(section, "## 7."):
			for _, m := range specBacktickedURI.FindAllStringSubmatch(line, -1) {
				out = append(out, m[1])
			}
		}
	}
	return out
}

func diffKeys(have, want map[string]bool) []string {
	var out []string
	for k := range have {
		if !want[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
