package discordbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestRoastCommandRegistered(t *testing.T) {
	var roast *discordgo.ApplicationCommand
	for _, c := range buildCommands() {
		if c.Name == roastCommandName {
			roast = c
		}
	}
	if roast == nil {
		t.Fatal("/roast not in buildCommands()")
	}
	if roast.DMPermission == nil || *roast.DMPermission {
		t.Error("/roast should be server-only")
	}
	if roast.DefaultMemberPermissions != nil {
		t.Error("/roast should be open to everyone by default")
	}
	for _, o := range roast.Options {
		if o.Required {
			t.Errorf("option %q is required; who and team should each be optional", o.Name)
		}
		if o.Name == "team" && !o.Autocomplete {
			t.Error("team option should autocomplete")
		}
	}
}

func TestFilterTeams(t *testing.T) {
	teams := []string{"Gridiron Gang (Alice)", "Bob's Burgers", "Carl"}
	if got := filterTeams(teams, ""); len(got) != 3 {
		t.Errorf("empty filter = %v, want all teams", got)
	}
	if got := filterTeams(teams, "  aLiCe "); len(got) != 1 || got[0] != "Gridiron Gang (Alice)" {
		t.Errorf("filter by manager = %v", got)
	}
	if got := filterTeams(teams, "zzz"); len(got) != 0 {
		t.Errorf("no-match filter = %v, want none", got)
	}

	many := make([]string, 40)
	for i := range many {
		many[i] = fmt.Sprintf("Team %d", i)
	}
	if got := filterTeams(many, "team"); len(got) != maxAutocompleteChoices {
		t.Errorf("got %d choices, want Discord's cap of %d", len(got), maxAutocompleteChoices)
	}

	long := strings.Repeat("é", 150)
	if got := filterTeams([]string{long}, ""); len([]rune(got[0])) != maxChoiceLen {
		t.Errorf("long team name not trimmed to %d chars", maxChoiceLen)
	}
}

func TestTargetNames(t *testing.T) {
	resolved := &discordgo.ApplicationCommandInteractionDataResolved{
		Users:   map[string]*discordgo.User{"1": {ID: "1", Username: "bob", GlobalName: "Bob"}},
		Members: map[string]*discordgo.Member{"1": {Nick: "Commish Bob"}},
	}
	got := targetNames(resolved, "1")
	// "Bob" and "bob" are the same name; only the first is kept.
	if want := []string{"Commish Bob", "Bob"}; strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("targetNames = %v, want %v", got, want)
	}
	if got := targetNames(resolved, ""); got != nil {
		t.Errorf("no target = %v, want nil", got)
	}
	if got := targetNames(nil, "1"); got != nil {
		t.Errorf("nil resolved = %v, want nil", got)
	}
}

type fakeAI struct {
	teams []string
	err   error
	calls int
}

func (f *fakeAI) Answer(context.Context, string, string, string) (string, error) { return "", nil }
func (f *fakeAI) Roast(context.Context, string, []string, string) (string, error) {
	return "", nil
}
func (f *fakeAI) TeamNames(context.Context) ([]string, error) {
	f.calls++
	return f.teams, f.err
}

func TestTeamNamesCachedAndFallsBackOnError(t *testing.T) {
	ai := &fakeAI{teams: []string{"A", "B"}}
	h := newRoastHandler(ai, newCooldowns(0))

	for range 3 {
		if got, err := h.teamNames(context.Background()); err != nil || len(got) != 2 {
			t.Fatalf("teamNames = %v, %v", got, err)
		}
	}
	if ai.calls != 1 {
		t.Errorf("TeamNames called %d times, want 1 (cached)", ai.calls)
	}

	// Expire the cache and make the refresh fail: the stale list is still
	// better than an empty autocomplete.
	h.teamsFetched = h.teamsFetched.Add(-2 * teamCacheTTL)
	ai.err = errors.New("sleeper down")
	got, err := h.teamNames(context.Background())
	if err == nil || len(got) != 2 {
		t.Errorf("failed refresh = %v, %v; want stale list plus the error", got, err)
	}
}
