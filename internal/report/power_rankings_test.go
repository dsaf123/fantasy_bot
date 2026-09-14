package report

import (
	"bytes"
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestPowerScoreWeightsRecordAndPoints(t *testing.T) {
	// Perfect record and best PF in the group scores 100.
	if got := powerScore(3, 0, 0, 300, 300); got != 100 {
		t.Errorf("powerScore(perfect) = %v, want 100", got)
	}
	// Winless team with no points scores 0.
	if got := powerScore(0, 3, 0, 0, 300); got != 0 {
		t.Errorf("powerScore(winless, scoreless) = %v, want 0", got)
	}
	// .500 record at half the max PF sits at the midpoint.
	if got := powerScore(1, 1, 0, 150, 300); got != 50 {
		t.Errorf("powerScore(.500, half PF) = %v, want 50", got)
	}
}

func TestPlayoffOddsInsideVsOutsideCutoff(t *testing.T) {
	// 6-team league, top 3 make the playoffs, 1 week left.
	inside := playoffOdds(1, 3, 6, 1)
	onBubble := playoffOdds(3, 3, 6, 1)
	outside := playoffOdds(6, 3, 6, 1)

	if inside <= onBubble || onBubble <= outside {
		t.Errorf("expected odds to decrease with rank: inside=%v onBubble=%v outside=%v", inside, onBubble, outside)
	}
	if outside > 10 {
		t.Errorf("last place with 1 week left should have low odds, got %v", outside)
	}
	if inside < 90 {
		t.Errorf("1st place with 1 week left should have high odds, got %v", inside)
	}
}

func TestPlayoffOddsCompressTowardMiddleWithMoreWeeksRemaining(t *testing.T) {
	// A team just outside the cutoff is less doomed with many weeks left
	// than with the season about to end.
	early := playoffOdds(4, 3, 6, 10)
	late := playoffOdds(4, 3, 6, 0)
	if early <= late {
		t.Errorf("expected early-season odds (%v) to sit closer to 50%% than late-season odds (%v)", early, late)
	}
	if early <= 20 {
		t.Errorf("expected meaningful uncertainty with 10 weeks left, got %v", early)
	}
}

func TestPlayoffOddsEveryoneInWhenSpotsCoverAllTeams(t *testing.T) {
	if got := playoffOdds(4, 4, 4, 3); got != 100 {
		t.Errorf("playoffOdds with playoffSpots == teams = %v, want 100", got)
	}
}

// powerRankingsFixture builds a 4-team, 2-week league context: week 1 sees
// Alice and Carl win, week 2 flips the results and Alice pulls further
// ahead on points, so Alice should rank 1st with an upward trend and Dana
// last with a downward one.
func powerRankingsFixture() (*LeagueContext, map[int][]sleeper.Matchup) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
		{UserID: "u3", DisplayName: "Carl"},
		{UserID: "u4", DisplayName: "Dana"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1", Settings: sleeper.RosterSettings{Wins: 2, Losses: 0, FPTS: 220}},
		{RosterID: 2, OwnerID: "u2", Settings: sleeper.RosterSettings{Wins: 0, Losses: 2, FPTS: 150}},
		{RosterID: 3, OwnerID: "u3", Settings: sleeper.RosterSettings{Wins: 1, Losses: 1, FPTS: 190}},
		{RosterID: 4, OwnerID: "u4", Settings: sleeper.RosterSettings{Wins: 1, Losses: 1, FPTS: 160}},
	}
	league := sleeper.League{Settings: sleeper.LeagueSettings{PlayoffTeams: 2, PlayoffWeekStart: 5}}
	ctx := NewLeagueContext(league, rosters, users, nil, 2)

	history := map[int][]sleeper.Matchup{
		1: {
			{RosterID: 1, MatchupID: 1, Points: 100}, // Alice beats Bob
			{RosterID: 2, MatchupID: 1, Points: 80},
			{RosterID: 3, MatchupID: 2, Points: 90}, // Carl beats Dana
			{RosterID: 4, MatchupID: 2, Points: 70},
		},
		2: {
			{RosterID: 1, MatchupID: 1, Points: 120}, // Alice beats Carl
			{RosterID: 3, MatchupID: 1, Points: 100},
			{RosterID: 2, MatchupID: 2, Points: 70}, // Bob beats Dana
			{RosterID: 4, MatchupID: 2, Points: 90},
		},
	}
	return ctx, history
}

func TestPowerRankingsRanksAndShowsTrend(t *testing.T) {
	ctx, history := powerRankingsFixture()
	out := ctx.PowerRankings(history)

	if !strings.HasPrefix(out, "Power Rankings (Playoff %)\n\n") {
		t.Fatalf("unexpected header, got:\n%s", out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")[2:]
	if len(lines) != 4 {
		t.Fatalf("expected 4 ranked lines, got %d:\n%s", len(lines), out)
	}
	if !strings.HasSuffix(lines[0], "ALIC") {
		t.Errorf("expected Alice ranked 1st, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "🟢") {
		t.Errorf("expected Alice trending up, got: %s", lines[0])
	}
	if !strings.HasSuffix(lines[len(lines)-1], "BOB") {
		t.Errorf("expected Bob ranked last (winless, lowest PF), got:\n%s", out)
	}
	if !strings.HasPrefix(lines[0], "100.00 ") {
		t.Errorf("expected top team rescaled to exactly 100, got: %s", lines[0])
	}
}

func TestPowerRankingsOmitsTrendOnFirstWeek(t *testing.T) {
	ctx, _ := powerRankingsFixture()
	ctx.Week = 1

	out := ctx.PowerRankings(nil)
	if strings.Contains(out, "🟢") || strings.Contains(out, "🔻") {
		t.Errorf("week 1 has no prior week to trend against, got:\n%s", out)
	}
}

func TestPowerRankingsChartNilBeforeTwoWeeks(t *testing.T) {
	ctx, history := powerRankingsFixture()
	ctx.Week = 1

	png, err := ctx.PowerRankingsChart(history)
	if err != nil {
		t.Fatalf("PowerRankingsChart() error = %v", err)
	}
	if png != nil {
		t.Errorf("expected nil chart before week 2, got %d bytes", len(png))
	}
}

func TestPowerRankingsChartRendersPNG(t *testing.T) {
	ctx, history := powerRankingsFixture()

	png, err := ctx.PowerRankingsChart(history)
	if err != nil {
		t.Fatalf("PowerRankingsChart() error = %v", err)
	}
	pngSignature := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if !bytes.HasPrefix(png, pngSignature) {
		t.Errorf("expected PNG output, got %d bytes starting with %v", len(png), png[:min(8, len(png))])
	}
}
