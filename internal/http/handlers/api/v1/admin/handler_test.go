package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/auth"
	"okrs/internal/domain"
)

func TestHandleMeReturns401WhenNoUser(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	w := httptest.NewRecorder()
	HandleMe(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandleMeReturnsUserJSON(t *testing.T) {
	u := &domain.User{
		ID:          99,
		DisplayName: "Alice",
		Email:       "alice@example.com",
		AvatarURL:   "https://example.com/avatar.png",
		Provider:    "google",
		IsAdmin:     true,
	}
	r := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	r = r.WithContext(auth.WithUser(r.Context(), u))
	w := httptest.NewRecorder()
	HandleMe(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var got meResponse
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if got.ID != 99 {
		t.Errorf("id: want 99, got %d", got.ID)
	}
	if got.DisplayName != "Alice" {
		t.Errorf("display_name: want Alice, got %s", got.DisplayName)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email: want alice@example.com, got %s", got.Email)
	}
	if got.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("avatar_url: want https://example.com/avatar.png, got %s", got.AvatarURL)
	}
	if got.Provider != "google" {
		t.Errorf("provider: want google, got %s", got.Provider)
	}
	if !got.IsAdmin {
		t.Errorf("is_admin: want true, got false")
	}
}
