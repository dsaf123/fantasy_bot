package discordbot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/report"
)

func TestFilterTeams(t *testing.T) {
	teams := []report.TeamRef{{OwnerID: "u1", Label: "Gridiron Gang (Alice)"}, {OwnerID: "u2", Label: "Bob's Burgers"}, {OwnerID: "u3", Label: "Carl"}}
	if got := filterTeams(teams, ""); len(got) != 3 {
		t.Errorf("empty filter = %v, want all teams", got)
	}
	if got := filterTeams(teams, "  aLiCe "); len(got) != 1 || got[0].OwnerID != "u1" {
		t.Errorf("filter by manager = %v", got)
	}
	if got := filterTeams(teams, "zzz"); len(got) != 0 {
		t.Errorf("no-match filter = %v, want none", got)
	}

	many := make([]report.TeamRef, 40)
	for i := range many {
		many[i] = report.TeamRef{OwnerID: fmt.Sprint(i), Label: fmt.Sprintf("Team %d", i)}
	}
	if got := filterTeams(many, "team"); len(got) != maxAutocompleteChoices {
		t.Errorf("got %d choices, want Discord's cap of %d", len(got), maxAutocompleteChoices)
	}

	long := report.TeamRef{OwnerID: "x", Label: strings.Repeat("é", 150)}
	if got := filterTeams([]report.TeamRef{long}, ""); len([]rune(got[0].Label)) != maxChoiceLen {
		t.Errorf("long team name not trimmed to %d chars", maxChoiceLen)
	}
}

func TestTeamDirectoryCachedAndFallsBackOnError(t *testing.T) {
	ai := &fakeAI{teams: []report.TeamRef{{OwnerID: "u1", Label: "A"}, {OwnerID: "u2", Label: "B"}}}
	d := newTeamDirectory(ai, newTestManager(t))

	for range 3 {
		if got, err := d.list(context.Background()); err != nil || len(got) != 2 {
			t.Fatalf("list = %v, %v", got, err)
		}
	}
	if ai.calls != 1 {
		t.Errorf("Teams called %d times, want 1 (cached)", ai.calls)
	}

	// Expire the cache and make the refresh fail: the stale list is still
	// better than nothing.
	d.fetched = d.fetched.Add(-2 * teamCacheTTL)
	ai.err = errors.New("sleeper down")
	got, err := d.list(context.Background())
	if err == nil || len(got) != 2 {
		t.Errorf("failed refresh = %v, %v; want stale list plus the error", got, err)
	}
}

func TestTeamDirectoryResolve(t *testing.T) {
	ai := &fakeAI{teams: []report.TeamRef{{OwnerID: "u1", Label: "Gridiron Gang (alice)"}, {OwnerID: "u2", Label: "Gridiron Gods (bob)"}, {OwnerID: "u3", Label: "Carl"}}}
	d := newTeamDirectory(ai, newTestManager(t))
	ctx := context.Background()

	for input, want := range map[string]string{
		"u2":                    "u2", // autocomplete pick sends the owner ID
		"carl":                  "u3", // exact label, any case
		"alice":                 "u1", // the one label containing it
		"gridiron gang (alice)": "u1",
	} {
		if got, ok := d.resolve(ctx, input); !ok || got.OwnerID != want {
			t.Errorf("resolve(%q) = %v, %v; want %s", input, got, ok, want)
		}
	}
	for _, input := range []string{"", "gridiron", "nobody"} { // empty, ambiguous, unknown
		if got, ok := d.resolve(ctx, input); ok {
			t.Errorf("resolve(%q) = %v, want no match", input, got)
		}
	}
}

func TestFocusedValueFindsNestedOption(t *testing.T) {
	opts := []*discordgo.ApplicationCommandInteractionDataOption{{
		Name: "link",
		Options: []*discordgo.ApplicationCommandInteractionDataOption{
			{Name: "who", Value: "123"},
			{Name: "team", Value: "grid", Focused: true},
		},
	}}
	if got := focusedValue(opts); got != "grid" {
		t.Errorf("focusedValue = %q, want grid", got)
	}
}
