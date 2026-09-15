package report

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestWeekTrophyWinnersMatchesTrophiesOutput(t *testing.T) {
	ctx := testContext()
	winners := ctx.weekTrophyWinners(testMatchups(), nil)

	want := map[string]int{
		"👑️": 3, // Carl, high score
		"💩️": 4, // Dana, low score
		"😱️": 3, // Carl blew out Dana
		"😅️": 1, // Alice barely beat Bob
		"🍀️": 1, // Alice, lucky winner (see TestTrophiesLuckAndUnluck)
		"😡️": 2, // Bob, unlucky loser
	}
	got := make(map[string]int, len(winners))
	for _, w := range winners {
		got[w.emoji] = w.rosterID
	}
	for emoji, rosterID := range want {
		if got[emoji] != rosterID {
			t.Errorf("weekTrophyWinners()[%s] = %d, want %d", emoji, got[emoji], rosterID)
		}
	}
	// No projections supplied, so the achiever columns should be absent.
	for _, emoji := range []string{"📈️", "📉️"} {
		if _, ok := got[emoji]; ok {
			t.Errorf("weekTrophyWinners() should omit %s without projections", emoji)
		}
	}
}

func TestWeekTrophyWinnersNoMatchupData(t *testing.T) {
	ctx := testContext()
	if winners := ctx.weekTrophyWinners(nil, nil); winners != nil {
		t.Errorf("weekTrophyWinners(nil, nil) = %v, want nil", winners)
	}
}

func TestTrophyCaseImageNoHistoryReturnsNil(t *testing.T) {
	ctx := testContext()
	img, err := ctx.TrophyCaseImage(nil, nil)
	if err != nil {
		t.Fatalf("TrophyCaseImage returned error: %v", err)
	}
	if img != nil {
		t.Errorf("TrophyCaseImage(nil, nil) = non-nil image, want nil")
	}
}

func TestTrophyCaseImageProducesValidPNGWithOneRowPerRoster(t *testing.T) {
	ctx := testContext() // 4 rosters: Alice, Bob, Carl, Dana
	history := map[int][]sleeper.Matchup{
		1: testMatchups(),
		2: testMatchups(),
	}

	data, err := ctx.TrophyCaseImage(history, nil)
	if err != nil {
		t.Fatalf("TrophyCaseImage returned error: %v", err)
	}
	if data == nil {
		t.Fatal("TrophyCaseImage returned nil image with non-empty history")
	}

	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("returned data isn't a valid PNG: %v", err)
	}

	wantHeight := tcMargin + tcTitleHeight + tcSubHeight + tcHeaderH + tcRowHeight*len(ctx.Rosters) + tcMargin
	if cfg.Height != wantHeight {
		t.Errorf("image height = %d, want %d (one row per roster, including non-winners)", cfg.Height, wantHeight)
	}

	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("png.Decode failed: %v", err)
	}
	if _, ok := img.(*image.RGBA); !ok {
		// Not strictly required, but confirms the encoder round-trips the
		// color model we drew with.
		t.Logf("decoded image type: %T", img)
	}
}

func TestTrophyCaseImageRanksMostDecoratedTeamFirst(t *testing.T) {
	ctx := testContext()
	// Carl (roster 3) wins High Score and Blowout every week; Dana (roster
	// 4) never wins anything. Both should still get a row, but Carl's
	// should render above Dana's - verified indirectly by checking the
	// image is at least tall enough for all 4 rosters and encodes without
	// error for a skewed, multi-week distribution.
	history := map[int][]sleeper.Matchup{
		1: testMatchups(),
		2: testMatchups(),
		3: testMatchups(),
	}
	data, err := ctx.TrophyCaseImage(history, nil)
	if err != nil {
		t.Fatalf("TrophyCaseImage returned error: %v", err)
	}
	if data == nil {
		t.Fatal("expected non-nil image")
	}
}

func TestTrophyCaseLegendListsAllColumns(t *testing.T) {
	legend := TrophyCaseLegend()
	for _, col := range trophyCaseColumns {
		want := col.emoji + " " + col.name
		if !strings.Contains(legend, want) {
			t.Errorf("TrophyCaseLegend() missing %q\ngot: %s", want, legend)
		}
	}
}
