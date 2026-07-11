package http

import (
	"bytes"
	"strings"
	"testing"
)

func TestStubShellRenders(t *testing.T) {
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates: %v", err)
	}
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, "stub-shell", nil); err != nil {
		t.Fatalf("execute stub-shell: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`/static/sidebar.js`, `/static/stub.js`, `id="root"`} {
		if !strings.Contains(out, want) {
			t.Errorf("stub-shell output missing %q", want)
		}
	}
}
