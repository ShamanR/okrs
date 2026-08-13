package domain

import "testing"

func TestIsValidKRHealthStatus(t *testing.T) {
	valid := []string{"not_started", "on_track", "at_risk", "done"}
	for _, s := range valid {
		if !IsValidKRHealthStatus(s) {
			t.Errorf("expected %q to be valid", s)
		}
	}
	invalid := []string{"", "NOT_STARTED", "onTrack", "risk", "completed"}
	for _, s := range invalid {
		if IsValidKRHealthStatus(s) {
			t.Errorf("expected %q to be invalid", s)
		}
	}
}

func TestKRHealthConstsMatchStrings(t *testing.T) {
	if string(KRHealthNotStarted) != "not_started" ||
		string(KRHealthOnTrack) != "on_track" ||
		string(KRHealthAtRisk) != "at_risk" ||
		string(KRHealthDone) != "done" {
		t.Fatal("health status const string values drifted")
	}
}
