package report

import (
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

// fortuneIndexFixture builds a 4-team, 1-week league where the two winners
// have very different all-play strength: Beth barely outscored the league's
// bottom team but still won, while Al crushed everyone and also won. So Beth
// should land luckiest (positive) and her victim Cara, who outscored half
// the league but still lost, should land unluckiest (negative).
func fortuneIndexFixture() (*LeagueContext, map[int][]sleeper.Matchup) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "Al"},
		{UserID: "u2", DisplayName: "Beth"},
		{UserID: "u3", DisplayName: "Cara"},
		{UserID: "u4", DisplayName: "Deb"},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1"},
		{RosterID: 2, OwnerID: "u2"},
		{RosterID: 3, OwnerID: "u3"},
		{RosterID: 4, OwnerID: "u4"},
	}
	league := sleeper.League{}
	ctx := NewLeagueContext(league, rosters, users, nil, 1)

	history := map[int][]sleeper.Matchup{
		1: {
			{RosterID: 1, MatchupID: 1, Points: 100}, // Al beats Deb
			{RosterID: 4, MatchupID: 1, Points: 40},
			{RosterID: 2, MatchupID: 2, Points: 90}, // Beth beats Cara
			{RosterID: 3, MatchupID: 2, Points: 80},
		},
	}
	return ctx, history
}

func TestFortuneIndexRanksLuckiestAndUnluckiest(t *testing.T) {
	ctx, history := fortuneIndexFixture()
	out := ctx.FortuneIndex(history)

	if !strings.HasPrefix(out, "Fortune Index - Week 1\n\n") {
		t.Fatalf("unexpected header, got:\n%s", out)
	}

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")[2:]
	if len(lines) != 4 {
		t.Fatalf("expected 4 ranked lines, got %d:\n%s", len(lines), out)
	}

	// Beth won despite only beating 2/3 of the league that week (Al and Deb
	// both scored under her, Al doesn't count against her, so her all-play
	// rate is 2/3) - she should rank luckiest, crowned, and positive.
	if !strings.Contains(lines[0], "👑") || !strings.Contains(lines[0], "Beth") {
		t.Errorf("expected Beth ranked 1st and crowned, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "+33") {
		t.Errorf("expected Beth at +33, got: %s", lines[0])
	}

	// Cara lost despite outscoring half the league (Deb) - unluckiest.
	if !strings.Contains(lines[3], "💀") || !strings.Contains(lines[3], "Cara") {
		t.Errorf("expected Cara ranked last and skulled, got:\n%s", out)
	}
	if !strings.Contains(lines[3], "-33") {
		t.Errorf("expected Cara at -33, got: %s", lines[3])
	}
}

func TestFortuneIndexSkipsByes(t *testing.T) {
	ctx, _ := fortuneIndexFixture()
	history := map[int][]sleeper.Matchup{
		1: {
			{RosterID: 1, MatchupID: 1, Points: 100}, // bye: no opponent
		},
	}
	out := ctx.FortuneIndex(history)
	if !strings.Contains(out, "+0") && !strings.Contains(out, " 0\n") {
		t.Errorf("expected a bye week to contribute no fortune, got:\n%s", out)
	}
}
