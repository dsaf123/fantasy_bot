package report

import (
	"bytes"
	"image/png"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestBadManagementChartNoMatchupsReturnsNil(t *testing.T) {
	ctx := testContext()
	img, err := ctx.BadManagementChart(nil)
	if err != nil {
		t.Fatalf("BadManagementChart returned error: %v", err)
	}
	if img != nil {
		t.Errorf("BadManagementChart(nil) = non-nil image, want nil")
	}
}

func TestBadManagementChartProducesValidPNGSizedForRosterCount(t *testing.T) {
	ctx := testContext() // 4 rosters
	data, err := ctx.BadManagementChart(testMatchups())
	if err != nil {
		t.Fatalf("BadManagementChart returned error: %v", err)
	}
	if data == nil {
		t.Fatal("BadManagementChart returned nil image with non-empty matchups")
	}

	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("returned data isn't a valid PNG: %v", err)
	}

	n := len(testMatchups())
	wantWidth := bmMargin*2 + bmYAxisW + n*bmBarWidth + (n-1)*bmBarGap
	// headerH is the taller of the title/subtitle block and the two-row
	// (Points Scored / Points left on bench) legend block - see
	// drawBadManagementChart.
	const legendRows = 2
	wantHeaderH := max(bmTitleLineH+4+bmSubLineH, legendRows*bmLegendRowH) + bmHeaderGap
	wantHeight := bmMargin*2 + wantHeaderH + bmPlotHeight + bmXLabelH
	if cfg.Width != wantWidth {
		t.Errorf("image width = %d, want %d (one bar per matchup)", cfg.Width, wantWidth)
	}
	if cfg.Height != wantHeight {
		t.Errorf("image height = %d, want %d", cfg.Height, wantHeight)
	}
}

func TestBadManagementChartClampsNegativeBenchLeftToZero(t *testing.T) {
	// testContext's league has no RosterPositions configured, so every
	// matchup's optimal lineup is 0 (see optimalLineupPoints' startingSlots)
	// - which would otherwise make benchLeft go negative once actual points
	// are subtracted from it. This should clamp to 0 and render fine rather
	// than producing a negative-height bar segment.
	ctx := testContext()
	data, err := ctx.BadManagementChart([]sleeper.Matchup{
		{RosterID: 1, MatchupID: 1, Points: 100},
	})
	if err != nil {
		t.Fatalf("BadManagementChart returned error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil image")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("returned data isn't a valid PNG: %v", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		t.Errorf("expected positive image dimensions, got %dx%d", cfg.Width, cfg.Height)
	}
}

func TestNiceAxisMaxRoundsUpToARoundStep(t *testing.T) {
	cases := []struct {
		dataMax                float64
		wantChartMax, wantStep float64
	}{
		{0, 25, 5},
		{42, 50, 10},
		{100, 100, 20},
		{101, 125, 25},
	}
	for _, tc := range cases {
		gotMax, gotStep := niceAxisMax(tc.dataMax)
		if gotMax != tc.wantChartMax || gotStep != tc.wantStep {
			t.Errorf("niceAxisMax(%v) = (%v, %v), want (%v, %v)", tc.dataMax, gotMax, gotStep, tc.wantChartMax, tc.wantStep)
		}
	}
}

func TestMaxStackedTotal(t *testing.T) {
	teams := []teamBadManagement{
		{name: "A", actual: 100, benchLeft: 10},
		{name: "B", actual: 150, benchLeft: 0},
		{name: "C", actual: 90, benchLeft: 65},
	}
	if got := maxStackedTotal(teams); got != 155 {
		t.Errorf("maxStackedTotal() = %v, want 155", got)
	}
}
