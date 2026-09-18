package report

import (
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func testContext() *LeagueContext {
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
	return NewLeagueContext(sleeper.League{}, rosters, users, nil, 1)
}

func testMatchups() []sleeper.Matchup {
	return []sleeper.Matchup{
		{RosterID: 1, MatchupID: 1, Points: 100},
		{RosterID: 2, MatchupID: 1, Points: 90}, // close game, margin 10
		{RosterID: 3, MatchupID: 2, Points: 150},
		{RosterID: 4, MatchupID: 2, Points: 50}, // blowout, margin 100
	}
}

func TestPairGames(t *testing.T) {
	games := pairGames(testMatchups())
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2", len(games))
	}
	if games[0].Margin() != 10 {
		t.Errorf("game 1 margin = %v, want 10", games[0].Margin())
	}
	if games[1].Margin() != 100 {
		t.Errorf("game 2 margin = %v, want 100", games[1].Margin())
	}
}

func TestCloseScores(t *testing.T) {
	ctx := testContext()
	out := ctx.CloseScores(testMatchups(), 15)

	if !strings.Contains(out, "ALIC") || !strings.Contains(out, "BOB") {
		t.Errorf("expected Alice/Bob close game in output, got: %s", out)
	}
	if strings.Contains(out, "CARL") || strings.Contains(out, "DANA") {
		t.Errorf("blowout game should not appear in close scores, got: %s", out)
	}
}

func TestCloseScoresNoneWithinThreshold(t *testing.T) {
	ctx := testContext()
	out := ctx.CloseScores(testMatchups(), 1)

	if out != "No close scores this week." {
		t.Errorf("got %q, want no-close-scores message", out)
	}
}

func TestTrophiesHighLowClosestBlowout(t *testing.T) {
	ctx := testContext()
	out := ctx.Trophies(testMatchups(), nil)

	cases := []string{
		"👑️ High score 👑️ \nCarl with 150.00 points",
		"💩️ Low score 💩️ \nDana with 50.00 points",
		"😅️ Close win 😅️ \nAlice barely beat Bob by 10.00 points",
		"😱️ Blow out 😱️ \nCarl blew out Dana by 100.00 points",
	}
	for _, want := range cases {
		if !strings.Contains(out, want) {
			t.Errorf("Trophies output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestTrophiesLuckAndUnluck(t *testing.T) {
	ctx := testContext()
	out := ctx.Trophies(testMatchups(), nil)

	// Carl (150) wins by blowing everyone out - not lucky, just dominant.
	// Alice (100) beats only Bob and Dana, not Carl - the "backed into it" win.
	if !strings.Contains(out, "Alice was 2-1 against the league, but still got the win") {
		t.Errorf("expected Alice to be the lucky winner, got:\n%s", out)
	}
	// Dana (50) lost a blowout, beating nobody - not unlucky, just outscored.
	// Bob (90) lost a close game but still outscored Dana - the unlucky one.
	if !strings.Contains(out, "Bob was 1-2 against the league, but still took an L") {
		t.Errorf("expected Bob to be the unlucky loser, got:\n%s", out)
	}
}

func TestTrophiesAchieversAndManagers(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Alice"},
		{UserID: "u2", DisplayName: "Bob"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1"},
		{RosterID: 2, OwnerID: "u2"},
	}
	players := map[string]sleeper.Player{
		"qb1": {FullName: "QB One", Position: "QB"},
		"rb1": {FullName: "RB One", Position: "RB"},
		"rb2": {FullName: "RB Two", Position: "RB"},
		"wr1": {FullName: "WR One", Position: "WR"},
		"qb2": {FullName: "QB Two", Position: "QB"},
		"rb3": {FullName: "RB Three", Position: "RB"},
		"wr2": {FullName: "WR Two", Position: "WR"},
	}
	league := sleeper.League{
		RosterPositions: []string{"QB", "RB", "WR", "BN"},
		ScoringSettings: map[string]float64{"rec_yd": 1}, // 1 pt/yd, so stats below equal the intended projections
	}
	ctx := NewLeagueContext(league, rosters, users, players, 1)

	matchups := []sleeper.Matchup{
		{
			// Alice started rb1 (5 pts) over the better rb2 (12 pts) on her
			// bench, so her optimal lineup is 10+12+8=30 against an actual
			// (and above-projection) 23.
			RosterID: 1, MatchupID: 1, Points: 23,
			Starters:      []string{"qb1", "rb1", "wr1"},
			Players:       []string{"qb1", "rb1", "wr1", "rb2"},
			PlayersPoints: map[string]float64{"qb1": 10, "rb1": 5, "wr1": 8, "rb2": 12},
		},
		{
			// Bob started his optimal lineup already, but well under his
			// projection.
			RosterID: 2, MatchupID: 1, Points: 45,
			Starters:      []string{"qb2", "rb3", "wr2"},
			Players:       []string{"qb2", "rb3", "wr2"},
			PlayersPoints: map[string]float64{"qb2": 20, "rb3": 15, "wr2": 10},
		},
	}
	projections := []sleeper.PlayerProjection{
		{PlayerID: "qb1", Stats: map[string]float64{"rec_yd": 8}},
		{PlayerID: "rb1", Stats: map[string]float64{"rec_yd": 6}},
		{PlayerID: "wr1", Stats: map[string]float64{"rec_yd": 6}}, // Alice projected 20, scored 23: +3
		{PlayerID: "qb2", Stats: map[string]float64{"rec_yd": 25}},
		{PlayerID: "rb3", Stats: map[string]float64{"rec_yd": 15}},
		{PlayerID: "wr2", Stats: map[string]float64{"rec_yd": 10}}, // Bob projected 50, scored 45: -5
	}

	out := ctx.Trophies(matchups, projections)

	cases := []string{
		"📈️ Overachiever 📈️ \nAlice was 3.00 points over their projection",
		"📉️ Underachiever 📉️ \nBob was 5.00 points under their projection",
		"🤖️ Best Manager 🤖️ \nBob scored 100.00% of their optimal score!",
		"🤡️ Worst Manager 🤡️ \nAlice left 7.00 points on their bench. Only scoring 76.67% of their optimal score.",
	}
	for _, want := range cases {
		if !strings.Contains(out, want) {
			t.Errorf("Trophies output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestStandingsSortsByWinsThenPF(t *testing.T) {
	users := []sleeper.User{{UserID: "u1", DisplayName: "Alice"}, {UserID: "u2", DisplayName: "Bob"}}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1", Settings: sleeper.RosterSettings{Wins: 1, Losses: 2, FPTS: 100}},
		{RosterID: 2, OwnerID: "u2", Settings: sleeper.RosterSettings{Wins: 2, Losses: 1, FPTS: 90}},
	}
	ctx := NewLeagueContext(sleeper.League{}, rosters, users, nil, 1)

	out := ctx.Standings()
	bobIdx := strings.Index(out, "Bob")
	aliceIdx := strings.Index(out, "Alice")
	if bobIdx == -1 || aliceIdx == -1 || bobIdx > aliceIdx {
		t.Errorf("expected Bob (2 wins) ranked above Alice (1 win), got:\n%s", out)
	}
}

func TestTeamNameFallsBackForUnknownRoster(t *testing.T) {
	ctx := NewLeagueContext(sleeper.League{}, nil, nil, nil, 1)
	if got := ctx.TeamName(7); got != "Team 7" {
		t.Errorf("TeamName(7) = %q, want %q", got, "Team 7")
	}
}

func TestTeamAbbrevDerivesFromName(t *testing.T) {
	ctx := testContext()
	if got := ctx.TeamAbbrev(1); got != "ALIC" {
		t.Errorf("TeamAbbrev(1) = %q, want %q", got, "ALIC")
	}
	if got := ctx.TeamAbbrev(2); got != "BOB" {
		t.Errorf("TeamAbbrev(2) = %q, want %q", got, "BOB")
	}
}

func TestTeamAbbrevWordInitials(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Andrew's Ass-Kickers", "AAK"},
		{"Kyle's Football Club", "KFC"},
		{"The Wolfpack", "TW"},
		{"andrewtheman123", "ANDR"},
		{"The Greatest Team Of All Time", "TGTO"}, // capped at 4 words
	}
	for _, tc := range cases {
		users := []sleeper.User{{UserID: "u1", DisplayName: tc.name}}
		rosters := []sleeper.Roster{{RosterID: 1, OwnerID: "u1"}}
		ctx := NewLeagueContext(sleeper.League{}, rosters, users, nil, 1)
		if got := ctx.TeamAbbrev(1); got != tc.want {
			t.Errorf("TeamAbbrev() for %q = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTeamAbbrevOverride(t *testing.T) {
	ctx := testContext()
	ctx.SetAbbreviations(map[int]string{1: "DYNK"})

	if got := ctx.TeamAbbrev(1); got != "DYNK" {
		t.Errorf("TeamAbbrev(1) = %q, want override %q", got, "DYNK")
	}
	// Roster 2 has no override, so it still falls back to the derived code.
	if got := ctx.TeamAbbrev(2); got != "BOB" {
		t.Errorf("TeamAbbrev(2) = %q, want %q", got, "BOB")
	}
}

func TestProjectedScoreboardUsesActualOverProjectedForScoredStarters(t *testing.T) {
	ctx := testContext()
	ctx.League.ScoringSettings = map[string]float64{"rec_yd": 1} // 1 pt/yd, so stats below equal the intended projections
	ctx.SetAbbreviations(map[int]string{1: "DYNK", 2: "ALMO"})

	matchups := []sleeper.Matchup{
		{
			RosterID: 1, MatchupID: 1, Points: 20,
			Starters:       []string{"p1", "p2"},
			StartersPoints: []float64{20, 0}, // p2 hasn't scored yet
		},
		{
			RosterID: 2, MatchupID: 1, Points: 5,
			Starters:       []string{"p3"},
			StartersPoints: []float64{5},
		},
	}
	projections := []sleeper.PlayerProjection{
		{PlayerID: "p1", Stats: map[string]float64{"rec_yd": 999}}, // already scored, ignored
		{PlayerID: "p2", Stats: map[string]float64{"rec_yd": 12.5}},
		{PlayerID: "p3", Stats: map[string]float64{"rec_yd": 999}}, // already scored, ignored
	}

	// No schedule info: falls back to the nonzero-actual-else-projected heuristic.
	out := ctx.ProjectedScoreboard(matchups, projections, nil)

	want := "DYNK  32.50 -   5.00 ALMO\n"
	if !strings.Contains(out, want) {
		t.Errorf("ProjectedScoreboard output missing %q, got:\n%s", want, out)
	}
	if !strings.HasPrefix(out, "Approximate Projected Scores\n") {
		t.Errorf("expected header line, got:\n%s", out)
	}
}

func TestProjectedScoreboardTrustsConfirmedFinalZeroOverProjection(t *testing.T) {
	ctx := testContext()
	ctx.League.ScoringSettings = map[string]float64{"rec_yd": 1} // 1 pt/yd, so stats below equal the intended projections
	ctx.Players = map[string]sleeper.Player{
		"p1": {Team: "KC"},  // game final; a 0 here is a real zero
		"p2": {Team: "BUF"}, // game not yet final; a 0 here just means "hasn't played"
	}
	ctx.SetAbbreviations(map[int]string{1: "DYNK", 2: "ALMO"})

	matchups := []sleeper.Matchup{
		{
			RosterID: 1, MatchupID: 1,
			Starters:       []string{"p1"},
			StartersPoints: []float64{0},
		},
		{
			RosterID: 2, MatchupID: 1,
			Starters:       []string{"p2"},
			StartersPoints: []float64{0},
		},
	}
	projections := []sleeper.PlayerProjection{
		{PlayerID: "p1", Stats: map[string]float64{"rec_yd": 20}}, // must NOT be used: KC is final
		{PlayerID: "p2", Stats: map[string]float64{"rec_yd": 8}},
	}
	schedule := []sleeper.ScheduledGame{
		{Week: 1, Home: "KC", Away: "LV", Status: "complete"},
		{Week: 1, Home: "BUF", Away: "MIA", Status: "pre_game"},
	}

	out := ctx.ProjectedScoreboard(matchups, projections, schedule)

	want := "DYNK   0.00 -   8.00 ALMO\n"
	if !strings.Contains(out, want) {
		t.Errorf("ProjectedScoreboard output missing %q, got:\n%s", want, out)
	}
}

func TestCompletedTeamsForWeek(t *testing.T) {
	schedule := []sleeper.ScheduledGame{
		{Week: 1, Home: "KC", Away: "LV", Status: "complete"},
		{Week: 1, Home: "BUF", Away: "MIA", Status: "pre_game"},
		{Week: 1, Home: "SF", Away: "SEA", Status: "canceled"},
		{Week: 2, Home: "KC", Away: "DEN", Status: "complete"}, // different week, shouldn't count
	}

	complete := completedTeamsForWeek(schedule, 1)

	for _, team := range []string{"KC", "LV", "SF", "SEA"} {
		if !complete[team] {
			t.Errorf("expected %s to be marked complete for week 1", team)
		}
	}
	for _, team := range []string{"BUF", "MIA", "DEN"} {
		if complete[team] {
			t.Errorf("expected %s to not be marked complete for week 1", team)
		}
	}
}

func TestWeekdayScoreboardCombinesCurrentAndProjected(t *testing.T) {
	ctx := testContext()
	ctx.League.ScoringSettings = map[string]float64{"rec_yd": 1} // 1 pt/yd, so stats below equal the intended projections
	ctx.SetAbbreviations(map[int]string{1: "DYNK", 2: "ALMO"})

	matchups := []sleeper.Matchup{
		{
			RosterID: 1, MatchupID: 1, Points: 20,
			Starters:       []string{"p1"},
			StartersPoints: []float64{20},
		},
		{
			RosterID: 2, MatchupID: 1, Points: 0,
			Starters:       []string{"p2"},
			StartersPoints: []float64{0},
		},
	}
	projections := []sleeper.PlayerProjection{
		{PlayerID: "p2", Stats: map[string]float64{"rec_yd": 15}},
	}

	out := ctx.WeekdayScoreboard(matchups, projections, nil)

	if !strings.HasPrefix(out, "Score Update\n") {
		t.Errorf("expected leading Score Update header, got:\n%s", out)
	}
	if !strings.Contains(out, "DYNK  20.00 -   0.00 ALMO\n") {
		t.Errorf("expected current score line, got:\n%s", out)
	}
	if !strings.Contains(out, "Approximate Projected Scores\nDYNK  20.00 -  15.00 ALMO\n") {
		t.Errorf("expected projected score line, got:\n%s", out)
	}
}

func TestMonitorGroupsStartersAndReserveByTeam(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", Metadata: sleeper.UserMetadata{TeamName: "Kyle's Football Club"}},
		{UserID: "u2", Metadata: sleeper.UserMetadata{TeamName: "Andrew's Ass-Kickers"}},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1", Starters: []string{"tee"}},
		{RosterID: 2, OwnerID: "u2", Reserve: []string{"bam", "kincaid"}},
	}
	players := map[string]sleeper.Player{
		"tee":     {FullName: "Tee Higgins", Position: "WR", InjuryStatus: "Questionable"},
		"bam":     {FullName: "Bam Knight", Position: "RB", InjuryStatus: "IR"},
		"kincaid": {FullName: "Dalton Kincaid", Position: "TE", InjuryStatus: ""},
	}
	ctx := NewLeagueContext(sleeper.League{}, rosters, users, players, 1)

	want := "Starting Players to Monitor\n" +
		"Kyle's Football Club: \n" +
		"WR Tee Higgins - Questionable\n" +
		"\n" +
		"Andrew's Ass-Kickers: \n" +
		"RB Bam Knight - Injury Reserve\n" +
		"TE Dalton Kincaid - Not IR eligible\n"

	if got := ctx.Monitor(); got != want {
		t.Errorf("Monitor() = %q, want %q", got, want)
	}
}

func TestMonitorNoneFlagged(t *testing.T) {
	ctx := testContext()
	if got := ctx.Monitor(); got != "No starters flagged with an injury designation." {
		t.Errorf("Monitor() = %q, want no-flags message", got)
	}
}

func TestScoreboardShortFormatsAlignedAbbreviations(t *testing.T) {
	ctx := testContext()
	ctx.SetAbbreviations(map[int]string{1: "DYNK", 2: "ALMO"})
	out := ctx.ScoreboardShort([]sleeper.Matchup{
		{RosterID: 1, MatchupID: 1, Points: 121.06},
		{RosterID: 2, MatchupID: 1, Points: 88.42},
	})

	want := "DYNK 121.06 -  88.42 ALMO\n"
	if !strings.Contains(out, want) {
		t.Errorf("ScoreboardShort output missing aligned line %q, got:\n%s", want, out)
	}
}
