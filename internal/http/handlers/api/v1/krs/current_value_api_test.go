package krs_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"okrs/internal/http/handlers/api/v1/testutil"
	"okrs/internal/store/grants"
)

// TestNumericalCurrentValueThroughAPI walks the exact path the defect was reported on:
// create a metric KR, edit its start before updating progress, and read the team board.
// It also pins that a current value passed to the definition endpoints is ignored — the only
// way to move it is a check-in.
func TestNumericalCurrentValueThroughAPI(t *testing.T) {
	pool, repo, teardown := setupKRAccessDB(t)
	defer teardown()
	teamID, periodID, goalID, _ := buildKRAccessFixture(t, pool, repo)
	gc := grants.NewGrantsCache(repo.Grants)
	server := httptest.NewServer(testutil.NewAPIV1RouterWithScope(t, repo, gc, []int64{teamID}))
	defer server.Close()

	const title = "Метрика 0 → 200"

	post := func(what, url string, fields map[string]string) {
		t.Helper()
		body, ct := multipartBody(fields)
		resp, err := http.Post(url, ct, body)
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", what, resp.StatusCode)
		}
	}

	// board returns the KR under test as the API renders it on the team board.
	type measuredKR struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		Measure struct {
			Numerical *struct {
				StartValue           float64 `json:"start_value"`
				TargetValue          float64 `json:"target_value"`
				CurrentValue         float64 `json:"current_value"`
				CurrentValueRecorded bool    `json:"current_value_recorded"`
			} `json:"numerical"`
		} `json:"measure"`
		ProgressUpdatedAt *time.Time `json:"progress_updated_at"`
		Progress          int        `json:"progress"`
	}
	board := func(what string) measuredKR {
		t.Helper()
		resp, err := http.Get(fmt.Sprintf("%s/api/v1/teams/%d/okrs?period_id=%d", server.URL, teamID, periodID))
		if err != nil {
			t.Fatalf("%s: get okrs: %v", what, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: expected 200, got %d", what, resp.StatusCode)
		}
		var payload struct {
			Goals []struct {
				KeyResults []measuredKR `json:"key_results"`
			} `json:"goals"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("%s: decode: %v", what, err)
		}
		for _, g := range payload.Goals {
			for _, kr := range g.KeyResults {
				if kr.Title == title {
					if kr.Measure.Numerical == nil {
						t.Fatalf("%s: KR %q came back without a numerical measure", what, title)
					}
					return kr
				}
			}
		}
		t.Fatalf("%s: KR %q not found on the board", what, title)
		return measuredKR{}
	}

	// A current value sent with the definition must be ignored: this is what used to pin
	// current_value to the start value at creation.
	post("create kr", fmt.Sprintf("%s/api/v1/goals/%d/key-results", server.URL, goalID), map[string]string{
		"title": title, "kind": "NUMERICAL", "weight": "50",
		"numerical_unit": "%", "numerical_start": "0", "numerical_target": "200",
		"numerical_current": "30",
	})

	created := board("just created")
	if created.Measure.Numerical.CurrentValue != 0 {
		t.Fatalf("expected the current value to come back as the start value 0, got %v", created.Measure.Numerical.CurrentValue)
	}
	if created.Measure.Numerical.CurrentValueRecorded {
		t.Fatal("expected current_value_recorded=false on a KR nobody checked in on")
	}
	if created.ProgressUpdatedAt != nil {
		t.Fatalf("expected no progress timestamp on a new KR, got %v", created.ProgressUpdatedAt)
	}
	if created.Progress != 0 {
		t.Fatalf("expected progress 0, got %d", created.Progress)
	}
	krURL := fmt.Sprintf("%s/api/v1/krs/%d", server.URL, created.ID)

	// The reported defect: edit the start before any progress update.
	post("edit start", krURL, map[string]string{
		"title": title, "kind": "NUMERICAL", "weight": "50",
		"numerical_unit": "%", "numerical_start": "100", "numerical_target": "200",
	})
	edited := board("start edited before the first check-in")
	if edited.Measure.Numerical.StartValue != 100 || edited.Measure.Numerical.CurrentValue != 100 {
		t.Fatalf("expected start and current to be 100, got start=%v current=%v",
			edited.Measure.Numerical.StartValue, edited.Measure.Numerical.CurrentValue)
	}
	if edited.ProgressUpdatedAt != nil {
		t.Fatalf("editing the definition must not count as updating progress, got %v", edited.ProgressUpdatedAt)
	}

	// A check-in is the only thing that moves the value.
	payload, _ := json.Marshal(map[string]float64{"current_value": 150})
	resp, err := http.Post(fmt.Sprintf("%s/api/v1/krs/%d/progress/numerical", server.URL, created.ID), "application/json", bytes.NewBuffer(payload))
	if err != nil {
		t.Fatalf("post progress: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post progress: expected 200, got %d", resp.StatusCode)
	}
	checked := board("after the check-in")
	if checked.Measure.Numerical.CurrentValue != 150 || checked.Progress != 50 {
		t.Fatalf("expected current 150 at 50%%, got current=%v progress=%d",
			checked.Measure.Numerical.CurrentValue, checked.Progress)
	}
	if !checked.Measure.Numerical.CurrentValueRecorded {
		t.Fatal("expected current_value_recorded=true after the check-in")
	}
	if checked.ProgressUpdatedAt == nil {
		t.Fatal("expected the check-in to stamp progress_updated_at")
	}

	// From now on the value is the KR's own: edits of the definition leave it alone, and a
	// current value smuggled into the edit is still ignored.
	post("edit after check-in", krURL, map[string]string{
		"title": title, "kind": "NUMERICAL", "weight": "50",
		"numerical_unit": "%", "numerical_start": "0", "numerical_target": "300",
		"numerical_current": "7",
	})
	after := board("start edited after the check-in")
	if after.Measure.Numerical.CurrentValue != 150 {
		t.Fatalf("expected the check-in value 150 to survive the edit, got %v", after.Measure.Numerical.CurrentValue)
	}
}
