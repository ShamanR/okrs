package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeSettings struct {
	data map[string]json.RawMessage
}

func (f *fakeSettings) GetSetting(_ context.Context, key string) (json.RawMessage, error) {
	return f.data[key], nil
}

func TestHandleConfigReturnsDocumentationURL(t *testing.T) {
	raw, _ := json.Marshal("https://example.com/wiki")
	h := New(&fakeSettings{data: map[string]json.RawMessage{"documentation_url": raw}})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got configResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.DocumentationURL != "https://example.com/wiki" {
		t.Errorf("documentation_url: want https://example.com/wiki, got %q", got.DocumentationURL)
	}
}

func TestHandleConfigEmptyWhenUnset(t *testing.T) {
	h := New(&fakeSettings{data: map[string]json.RawMessage{}})

	r := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	w := httptest.NewRecorder()
	h.HandleConfig(w, r)

	var got configResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.DocumentationURL != "" {
		t.Errorf("documentation_url: want empty, got %q", got.DocumentationURL)
	}
}
