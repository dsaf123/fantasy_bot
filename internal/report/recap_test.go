package report

import (
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestRecapStreaksSkipsByesAndShortStreaks(t *testing.T) {
	ctx := testContext() // rosters 1-4: Alice, Bob, Carl, Dana
	history := map[int][]sleeper.Matchup{
		// Roster 1 beats roster 2 three weeks running; roster 3/4 split so
		// neither has a 2+ game streak.
		1: {
			{RosterID: 1, MatchupID: 1, Points: 100},
			{RosterID: 2, MatchupID: 1, Points: 90},
			{RosterID: 3, MatchupID: 2, Points: 100},
			{RosterID: 4, MatchupID: 2, Points: 90},
		},
		2: {
			{RosterID: 1, MatchupID: 1, Points: 100},
			{RosterID: 2, MatchupID: 1, Points: 90},
			{RosterID: 3, MatchupID: 2, Points: 80},
			{RosterID: 4, MatchupID: 2, Points: 90},
		},
		3: {
			{RosterID: 1, MatchupID: 1, Points: 100},
			{RosterID: 2, MatchupID: 1, Points: 90},
			// roster 3 has a bye in week 3; roster 4 sits alone too (no
			// effect on either streak).
		},
	}

	out := recapStreaks(history, []int{1, 2, 3, 4}, 3, ctx)

	if !strings.Contains(out, "Alice: 3-game win streak") {
		t.Errorf("expected Alice's 3-game win streak, got:\n%s", out)
	}
	if !strings.Contains(out, "Bob: 3-game loss streak") {
		t.Errorf("expected Bob's 3-game loss streak, got:\n%s", out)
	}
	if strings.Contains(out, "Carl") || strings.Contains(out, "Dana") {
		t.Errorf("Carl/Dana split their games and shouldn't show a streak, got:\n%s", out)
	}
}

func TestRecapHeadToHeadOnlyShowsRepeatMeetings(t *testing.T) {
	ctx := testContext()
	history := map[int][]sleeper.Matchup{
		1: {
			{RosterID: 1, MatchupID: 1, Points: 100},
			{RosterID: 2, MatchupID: 1, Points: 90},
			{RosterID: 3, MatchupID: 2, Points: 100},
			{RosterID: 4, MatchupID: 2, Points: 90},
		},
		2: {
			// 1 vs 2 rematch, this time Bob wins - a real season series.
			// Carl/Dana don't play again this week, so they stay a
			// one-time pairing.
			{RosterID: 2, MatchupID: 1, Points: 95},
			{RosterID: 1, MatchupID: 1, Points: 85},
		},
	}

	out := recapHeadToHead(history, 2, ctx)

	if !strings.Contains(out, "Alice vs Bob: 1-1-0") {
		t.Errorf("expected Alice/Bob 1-1 series, got:\n%s", out)
	}
	if strings.Contains(out, "Carl vs Dana") {
		t.Errorf("Carl/Dana have only played once and shouldn't be listed as a series, got:\n%s", out)
	}
}

func TestRecapHeadToHeadNoneYet(t *testing.T) {
	ctx := testContext()
	history := map[int][]sleeper.Matchup{
		1: testMatchups(),
	}
	out := recapHeadToHead(history, 1, ctx)
	if !strings.Contains(out, "None - every pairing has met at most once") {
		t.Errorf("expected no-series message, got:\n%s", out)
	}
}

func TestRecapPlayoffRaceGamesBack(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
		{UserID: "u3", DisplayName: "Carl"},
		{UserID: "u4", DisplayName: "Dana"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1", Settings: sleeper.RosterSettings{Wins: 5, Losses: 1}},
		{RosterID: 2, OwnerID: "u2", Settings: sleeper.RosterSettings{Wins: 3, Losses: 3}},
		{RosterID: 3, OwnerID: "u3", Settings: sleeper.RosterSettings{Wins: 2, Losses: 4}},
		{RosterID: 4, OwnerID: "u4", Settings: sleeper.RosterSettings{Wins: 1, Losses: 5}},
	}
	league := sleeper.League{Settings: sleeper.LeagueSettings{PlayoffTeams: 2}}
	ctx := NewLeagueContext(league, rosters, users, nil, 6)

	out := ctx.recapPlayoffRace()

	if !strings.Contains(out, "1. Alice (5-1) - IN") {
		t.Errorf("expected Alice comfortably in, got:\n%s", out)
	}
	if !strings.Contains(out, "2. Bob (3-3) - IN (last team in)") {
		t.Errorf("expected Bob marked as last team in, got:\n%s", out)
	}
	if !strings.Contains(out, "3. Carl (2-4) - OUT (1.0 games back)") {
		t.Errorf("expected Carl 1.0 games back, got:\n%s", out)
	}
	if !strings.Contains(out, "4. Dana (1-5) - OUT (2.0 games back)") {
		t.Errorf("expected Dana 2.0 games back, got:\n%s", out)
	}
}

func TestRecapLineupRegretPrefersFlippedResult(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
		{UserID: "u3", DisplayName: "Carl"},
		{UserID: "u4", DisplayName: "Dana"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1"},
		{RosterID: 2, OwnerID: "u2"},
		{RosterID: 3, OwnerID: "u3"},
		{RosterID: 4, OwnerID: "u4"},
	}
	players := map[string]sleeper.Player{
		"a_start": {FullName: "A Starter", Position: "RB"},
		"a_bench": {FullName: "A Bench", Position: "RB"},
		"c_start": {FullName: "C Starter", Position: "RB"},
		"c_bench": {FullName: "C Bench", Position: "RB"},
	}
	league := sleeper.League{RosterPositions: []string{"RB", "BN"}}
	ctx := NewLeagueContext(league, rosters, users, players, 1)

	matchups := []sleeper.Matchup{
		// Alice loses 90-100, but her bench had enough to win (optimal 110).
		{
			RosterID: 1, MatchupID: 1, Points: 90,
			Starters:      []string{"a_start"},
			Players:       []string{"a_start", "a_bench"},
			PlayersPoints: map[string]float64{"a_start": 90, "a_bench": 110},
		},
		{RosterID: 2, MatchupID: 1, Points: 100},
		// Carl loses 40-100 and left points on the bench, but not enough to
		// have won - shouldn't outrank Alice's flip.
		{
			RosterID: 3, MatchupID: 2, Points: 40,
			Starters:      []string{"c_start"},
			Players:       []string{"c_start", "c_bench"},
			PlayersPoints: map[string]float64{"c_start": 40, "c_bench": 55},
		},
		{RosterID: 4, MatchupID: 2, Points: 100},
	}

	out := ctx.recapLineupRegret(matchups)

	if !strings.Contains(out, "Alice lost to Bob") {
		t.Errorf("expected Alice's flip-the-result regret to win out, got:\n%s", out)
	}
	if !strings.Contains(out, "enough to win") {
		t.Errorf("expected the 'would have won' framing, got:\n%s", out)
	}
}

func TestRecapDigestIncludesAllSections(t *testing.T) {
	ctx := testContext()
	ctx.Week = 1
	history := map[int][]sleeper.Matchup{1: testMatchups()}

	out := ctx.RecapDigest(history, 1)

	for _, want := range []string{"Standings:", "Active streaks", "Season head-to-head", "Playoff race", "lineup regret"} {
		if !strings.Contains(out, want) {
			t.Errorf("RecapDigest missing section %q, got:\n%s", want, out)
		}
	}
}
