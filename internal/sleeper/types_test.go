package sleeper

import "testing"

func TestPlayerProjectionPointsForSettings(t *testing.T) {
	// A TE projected for 4 receptions/40 yards/0 TDs in a league running
	// full PPR plus a 0.5 TE premium - the custom rule Sleeper's
	// precomputed pts_ppr total leaves out, which is what made the bot's
	// old approximation diverge from Sleeper's own in-app projection.
	proj := PlayerProjection{
		PlayerID: "te1",
		Stats: map[string]float64{
			"rec":          4,
			"rec_yd":       40,
			"bonus_rec_te": 4,
			"pts_ppr":      10.5, // Sleeper's precomputed total, ignored by PointsForSettings
		},
	}
	scoringSettings := map[string]float64{
		"rec":          1,
		"rec_yd":       0.1,
		"bonus_rec_te": 0.5,
	}

	got := proj.PointsForSettings(scoringSettings)
	want := 4*1 + 40*0.1 + 4*0.5 // = 10.0
	if got != want {
		t.Errorf("PointsForSettings() = %v, want %v", got, want)
	}
}

func TestPlayerProjectionPointsForSettingsMissingStatIsZero(t *testing.T) {
	proj := PlayerProjection{PlayerID: "p1", Stats: map[string]float64{"rec": 3}}
	scoringSettings := map[string]float64{"rec": 1, "pass_td": 4}

	if got := proj.PointsForSettings(scoringSettings); got != 3 {
		t.Errorf("PointsForSettings() = %v, want 3", got)
	}
}

func TestScheduledGameFinal(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"complete", true},
		{"canceled", true},
		{"pre_game", false},
		{"in_game", false},
		{"", false},
	}
	for _, tc := range cases {
		g := ScheduledGame{Status: tc.status}
		if got := g.Final(); got != tc.want {
			t.Errorf("ScheduledGame{Status: %q}.Final() = %v, want %v", tc.status, got, tc.want)
		}
	}
}
