package goals

import (
	"bytes"
	"context"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"okrs/internal/http/handlers/web/common"

	"github.com/go-chi/chi/v5"
)

// goalRouteCtx sets chi URL params on the request.
func goalRouteCtx(r *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

// multipartBody builds a multipart form body with the given fields.
func multipartBody(fields map[string][]string) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for key, values := range fields {
		for _, v := range values {
			_ = w.WriteField(key, v)
		}
	}
	w.Close()
	return &buf, w.FormDataContentType()
}

// TestHandleUpdateGoalShare_AllMalformedTeamIDs verifies that when every submitted
// team_id fails to parse as an integer the handler returns an error without calling
// the service, preventing accidental deletion of all goal shares.
func TestHandleUpdateGoalShare_AllMalformedTeamIDs(t *testing.T) {
	body, ct := multipartBody(map[string][]string{
		"team_ids": {"not-a-number", "also-invalid", ""},
	})

	req := httptest.NewRequest(http.MethodPost, "/goals/1/share", body)
	req.Header.Set("Content-Type", ct)
	req = goalRouteCtx(req, map[string]string{"goalID": "1"})

	rec := httptest.NewRecorder()

	// Service is intentionally nil: if the handler reaches the service call it panics,
	// making the test fail and confirming the fix is not in place.
	h := New(common.Dependencies{Logger: slog.Default()})
	h.HandleUpdateGoalShare(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// TestHandleUpdateGoalShare_NoTeamIDs verifies the pre-existing guard for a completely
// absent team_ids field.
func TestHandleUpdateGoalShare_NoTeamIDs(t *testing.T) {
	body, ct := multipartBody(map[string][]string{})

	req := httptest.NewRequest(http.MethodPost, "/goals/1/share", body)
	req.Header.Set("Content-Type", ct)
	req = goalRouteCtx(req, map[string]string{"goalID": "1"})

	rec := httptest.NewRecorder()

	h := New(common.Dependencies{Logger: slog.Default()})
	h.HandleUpdateGoalShare(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}
