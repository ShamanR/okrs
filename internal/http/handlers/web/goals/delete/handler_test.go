package delete

// SSR-эндпоинт: ошибки рендерятся страницей через deps.Logger, поэтому логгер
// обязателен даже в тестах гейтов — иначе RenderError разыменует nil.
// Тело шлётся multipart: обработчик зовёт ParseMultipartForm, и на
// urlencoded-теле он отвалится раньше проверяемого гейта.

import (
	"bytes"
	"context"
	"io"
	multipartWriter "mime/multipart"
	"net/http/httptest"

	"github.com/go-chi/chi/v5"

	"log/slog"
	"net/http"
	"testing"

	"okrs/internal/http/handlers/handlertest"
	"okrs/internal/http/handlers/web/common"
)

func handler() *Handler {
	return New(common.Dependencies{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}, nil)
}

const uri = "/goals/1/delete"

func TestBadGoalIDRendersError(t *testing.T) {
	w := multipart(handler().Post, handlertest.Tenant(1), handlertest.URLParam("goalID", "не-число"))
	if w.Code == http.StatusOK {
		t.Fatalf("неразбираемый goalID дал 200, ожидалась страница ошибки (тело: %s)", w.Body.String())
	}
}

// Без активного tenant форма удаления отвечает 403 текстом, а не рендерит страницу.
func TestRequiresTenant(t *testing.T) {
	w := multipart(handler().Post, handlertest.URLParam("goalID", "1"))
	handlertest.Status(t, w, http.StatusForbidden)
}

// multipart собирает запрос с одним полем team_id в том виде, в каком его шлёт форма.
func multipart(h http.HandlerFunc, opts ...handlertest.Option) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	mw := multipartWriter.NewWriter(&buf)
	_ = mw.WriteField("team_id", "1")
	_ = mw.Close()
	r := httptest.NewRequest(http.MethodPost, uri, &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))
	for _, o := range opts {
		r = o(r)
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w
}
