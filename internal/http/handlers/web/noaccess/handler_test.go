package noaccess

// Страница «нет доступа» резолвится через реестр nomembership — это точка
// расширения OSS/SaaS. Проверяется, что берётся реализация под настроенным
// именем, а не первая попавшаяся, и что незарегистрированное имя не даёт
// пустую страницу.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"okrs/internal/platform/nomembership"
)

type stub struct{ body string }

func (s stub) ServeNoMembership(w http.ResponseWriter, _ *http.Request) {
	_, _ = w.Write([]byte(s.body))
}

func get(h *Handler) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	h.Get(w, httptest.NewRequest(http.MethodGet, "/no-access", nil))
	return w
}

// Опечатка в имени должна быть видна как ошибка сервера, а не как пустая
// страница, на которой пользователь застрянет без объяснения.
func TestUnregisteredNameIs500(t *testing.T) {
	w := get(New("нет-такой-реализации"))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("код = %d, want 500", w.Code)
	}
}

func TestServesRegisteredImplementation(t *testing.T) {
	nomembership.Register("noaccess-test-a", stub{body: "страница A"})
	nomembership.Register("noaccess-test-b", stub{body: "страница B"})

	w := get(New("noaccess-test-b"))
	if w.Code != http.StatusOK {
		t.Fatalf("код = %d, want 200", w.Code)
	}
	if got := w.Body.String(); got != "страница B" {
		t.Fatalf("отдана %q — выбрана не та реализация", got)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("Content-Type = %q, want text/html", ct)
	}
}
