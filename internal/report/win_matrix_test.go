package report

import (
	"fmt"
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestWinMatrixNoHistoryReturnsNoMatchupData(t *testing.T) {
	ctx := testContext()
	if got := ctx.WinMatrix(nil); got != NoMatchupData {
		t.Errorf("WinMatrix(nil) = %q, want %q", got, NoMatchupData)
	}
}

func TestWinMatrixHasTitleAndCodeBlock(t *testing.T) {
	ctx := testContext()
	out := ctx.WinMatrix(map[int][]sleeper.Matchup{1: testMatchups()})

	if !strings.HasPrefix(out, "**Standings if everyone played every team every week**\n```\n") {
		t.Fatalf("unexpected header, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "```") {
		t.Fatalf("expected output to end with a closing code fence, got:\n%s", out)
	}
}

// TestWinMatrixTalliesAllPlayRecordAcrossWeeks builds a 4-team, 2-week
// history where Carl (roster 3) outscores everyone both weeks and Dana
// (roster 4) is outscored by everyone both weeks, so Carl should finish
// 6-0 and Dana 0-6 (3 opponents x 2 weeks each) regardless of who Sleeper
// actually paired them against.
func TestWinMatrixTalliesAllPlayRecordAcrossWeeks(t *testing.T) {
	ctx := testContext() // Alice=1, Bob=2, Carl=3, Dana=4
	history := map[int][]sleeper.Matchup{
		1: testMatchups(), // Alice 100, Bob 90, Carl 150, Dana 50
		2: testMatchups(),
	}

	out := ctx.WinMatrix(history)

	if !strings.Contains(out, "1. Carl") {
		t.Errorf("expected Carl ranked 1st (6-0), got:\n%s", out)
	}
	if !strings.Contains(out, "(6-0)") {
		t.Errorf("expected Carl's record to be 6-0, got:\n%s", out)
	}
	if !strings.Contains(out, "(0-6)") {
		t.Errorf("expected Dana's record to be 0-6, got:\n%s", out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := lines[len(lines)-2] // last team row, before the closing ```
	if !strings.Contains(last, "Dana") {
		t.Errorf("expected Dana ranked last, got:\n%s", out)
	}
}

func TestWinMatrixBreaksTiesByPointsFor(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1"},
		{RosterID: 2, OwnerID: "u2"},
	}
	ctx := NewLeagueContext(sleeper.League{}, rosters, users, nil, 1)

	// Both teams go 1-1 across two weeks (each beats the other once), so
	// wins are tied 2-2 and the tiebreak must fall to total points scored:
	// Bob out-scores Alice overall (140+80=220 vs 120+90=210).
	history := map[int][]sleeper.Matchup{
		1: {
			{RosterID: 1, MatchupID: 1, Points: 120},
			{RosterID: 2, MatchupID: 1, Points: 80},
		},
		2: {
			{RosterID: 1, MatchupID: 1, Points: 90},
			{RosterID: 2, MatchupID: 1, Points: 140},
		},
	}

	out := ctx.WinMatrix(history)
	bobIdx := strings.Index(out, "Bob")
	aliceIdx := strings.Index(out, "Alice")
	if bobIdx == -1 || aliceIdx == -1 || bobIdx > aliceIdx {
		t.Errorf("expected Bob ranked above Alice on points-for tiebreak, got:\n%s", out)
	}
}

// TestWinMatrixAlignsRankAcrossDoubleDigits builds a 10-team, 1-week league
// so ranks span 1 through 10, and checks that every row's record column
// starts at the same offset - a single-digit rank ("1. ") is one character
// shorter than a double-digit one ("10. "), so the rank itself needs
// width-padding or every row after rank 9 shifts right.
func TestWinMatrixAlignsRankAcrossDoubleDigits(t *testing.T) {
	users := make([]sleeper.User, 10)
	rosters := make([]sleeper.Roster, 10)
	matchups := make([]sleeper.Matchup, 10)
	for i := 0; i < 10; i++ {
		id := i + 1
		users[i] = sleeper.User{UserID: fmt.Sprintf("u%d", id), DisplayName: fmt.Sprintf("Team%d", id)}
		rosters[i] = sleeper.Roster{RosterID: id, OwnerID: fmt.Sprintf("u%d", id)}
		// Descending scores so team N's rank is exactly N.
		matchups[i] = sleeper.Matchup{RosterID: id, MatchupID: id, Points: float64(100 - id)}
	}
	ctx := NewLeagueContext(sleeper.League{}, rosters, users, nil, 1)
	out := ctx.WinMatrix(map[int][]sleeper.Matchup{1: matchups})

	lines := strings.Split(strings.TrimSpace(out), "\n")
	rows := lines[2 : len(lines)-1] // drop the title line, opening ```, and closing ```
	if len(rows) != 10 {
		t.Fatalf("expected 10 rows, got %d:\n%s", len(rows), out)
	}
	firstParen := strings.Index(rows[0], "(")
	for _, row := range rows {
		if idx := strings.Index(row, "("); idx != firstParen {
			t.Errorf("record column misaligned: row %q has '(' at %d, want %d (row 1's position)", row, idx, firstParen)
		}
	}
}

func TestWinMatrixRecordAlignsDashAcrossVaryingWidths(t *testing.T) {
	ctx := testContext()
	// 6 weeks x 3 opponents = 18 games per team, enough for a double-digit
	// win/loss split so the dash-alignment padding actually kicks in.
	history := make(map[int][]sleeper.Matchup, 6)
	for week := 1; week <= 6; week++ {
		history[week] = testMatchups()
	}

	out := ctx.WinMatrix(history)
	for _, want := range []string{"(18-0 )", "(12-6 )", "( 6-12)", "( 0-18)"} {
		if !strings.Contains(out, want) {
			t.Errorf("WinMatrix output missing aligned record %q, got:\n%s", want, out)
		}
	}
}
