package teams

import "testing"

func TestBuildTeamOKRURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		teamID   int64
		periodID int64
		goalID   int64
		want     string
	}{
		{
			name:     "without goal anchor",
			teamID:   12,
			periodID: 34,
			goalID:   0,
			want:     "/teams/12/okr?period_id=34",
		},
		{
			name:     "with goal anchor",
			teamID:   12,
			periodID: 34,
			goalID:   56,
			want:     "/teams/12/okr?period_id=34#goal-56",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := buildTeamOKRURL(tc.teamID, tc.periodID, tc.goalID)
			if got != tc.want {
				t.Fatalf("unexpected url: got %q want %q", got, tc.want)
			}
		})
	}
}
