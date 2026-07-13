package http

import (
	"bytes"
	"strings"
	"testing"
)

// renderShell executes a SPA shell template with the given shellData and returns the HTML.
func renderShell(t *testing.T, name string, data shellData) string {
	t.Helper()
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatalf("execute %s: %v", name, err)
	}
	return buf.String()
}

func TestStubShellRenders(t *testing.T) {
	out := renderShell(t, "stub-shell", shellData{})
	for _, want := range []string{`/static/sidebar.js`, `/static/stub.js`, `id="root"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stub-shell output missing %q", want)
		}
	}
}

// TestShellSharedPartials verifies every SPA shell pulls in the shared head partial
// (tokens + shell stylesheets) and the shared vendor block, so the DRY extraction stays wired.
func TestShellSharedPartials(t *testing.T) {
	shells := []string{"tracker-shell", "settings-shell", "admin-shell", "system-shell", "stub-shell"}
	for _, name := range shells {
		out := renderShell(t, name, shellData{})
		for _, want := range []string{`/static/tokens.css`, `/static/shell.css`, `/static/vendor/babel.min.js`, `class="loading-screen"`} {
			if !strings.Contains(out, want) {
				t.Errorf("%s missing shared partial marker %q", name, want)
			}
		}
	}
}

// TestShellReactBuildSwitch verifies WEB_ASSETS_DEV (shellData.Dev) selects the
// development vs production vendored React build.
func TestShellReactBuildSwitch(t *testing.T) {
	prod := renderShell(t, "tracker-shell", shellData{Dev: false})
	if !strings.Contains(prod, "/static/vendor/react.production.min.js") {
		t.Error("production shell must load react.production.min.js")
	}
	if strings.Contains(prod, "react.development.js") {
		t.Error("production shell must not load the development React build")
	}

	dev := renderShell(t, "tracker-shell", shellData{Dev: true})
	if !strings.Contains(dev, "/static/vendor/react.development.js") {
		t.Error("dev shell must load react.development.js")
	}
	if strings.Contains(dev, "react.production.min.js") {
		t.Error("dev shell must not load the production React build")
	}
}
