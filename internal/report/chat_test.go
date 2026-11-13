package report

import (
	"strings"
	"testing"

	"fantasy_bot/internal/sleeper"
)

func TestRosterDigest(t *testing.T) {
	users := []sleeper.User{{UserID: "u1", DisplayName: "Alice", Metadata: sleeper.UserMetadata{TeamName: "Gridiron Gang"}}}
	rosters := []sleeper.Roster{{
		RosterID: 1,
		OwnerID:  "u1",
		Players:  []string{"qb", "rb", "wr", "ir"},
		Starters: []string{"qb", "0"},
		Reserve:  []string{"ir"},
	}}
	players := map[string]sleeper.Player{
		"qb": {FullName: "Josh Allen", Position: "QB", Team: "BUF"},
		"rb": {FullName: "Bench Back", Position: "RB", Team: "NYJ", InjuryStatus: "Questionable"},
		"wr": {FullName: "Free Agent Guy", Position: "WR"},
		"ir": {FullName: "Hurt Guy", Position: "TE", Team: "KC", InjuryStatus: "IR"},
	}
	out := NewLeagueContext(sleeper.League{}, rosters, users, players, 5).RosterDigest()

	for _, want := range []string{
		"Gridiron Gang (GG, manager Alice)",
		"Starters: Josh Allen (QB, BUF); (empty slot)",
		"Bench: Bench Back (RB, NYJ, Questionable); Free Agent Guy (WR)",
		"IR: Hurt Guy (TE, KC, IR)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("digest missing %q:\n%s", want, out)
		}
	}
}

func TestTeams(t *testing.T) {
	users := []sleeper.User{
		{UserID: "u1", DisplayName: "alice", Metadata: sleeper.UserMetadata{TeamName: "Gridiron Gang"}},
		{UserID: "u2", DisplayName: "bob"}, // no custom team name: TeamName is already "bob"
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1"},
		{RosterID: 2, OwnerID: "u2"},
		{RosterID: 3, OwnerID: ""}, // orphaned: left out
	}
	got := Teams(rosters, users)
	want := []TeamRef{{OwnerID: "u1", Label: "Gridiron Gang (alice)"}, {OwnerID: "u2", Label: "bob"}}
	if len(got) != len(want) {
		t.Fatalf("Teams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Teams[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
