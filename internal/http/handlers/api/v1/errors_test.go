package v1

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteError(t *testing.T) {
	recorder := httptest.NewRecorder()
	writeError(recorder, http.StatusBadRequest, "VALIDATION_ERROR", "invalid", map[string]string{"field": "required"})

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	var response ErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if response.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected code VALIDATION_ERROR")
	}
	if response.Error.Fields["field"] != "required" {
		t.Fatalf("expected field error")
	}
}
