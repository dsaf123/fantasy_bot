package discordbot

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/report"
	"fantasy_bot/internal/settings"
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

func TestRoastTargetUsesLinks(t *testing.T) {
	mgr := newTestManager(t)
	if err := mgr.Save(settings.Settings{TeamLinks: map[string]settings.TeamLink{
		"111": {SleeperUserID: "u1", DiscordName: "Alice", TeamLabel: "Old Name"},
		"222": {SleeperUserID: "u2", DiscordName: "Bob", TeamLabel: "Bob's Burgers"},
		"333": {SleeperUserID: "u2", DiscordName: "Bob's co-manager", TeamLabel: "Bob's Burgers"},
	}}); err != nil {
		t.Fatal(err)
	}
	ai := &fakeAI{teams: []report.TeamRef{{OwnerID: "u1", Label: "Gridiron Gang (alice)"}, {OwnerID: "u2", Label: "Bob's Burgers"}}}
	h := newRoastHandler(ai, newCooldowns(0), newTeamDirectory(ai, mgr))
	ctx := context.Background()

	tests := []struct {
		name, who, team string
		wantTeam        string
		wantPings       []string
	}{
		{"member uses their linked team (current name)", "111", "", "Gridiron Gang (alice)", []string{"111"}},
		{"unlinked member leaves team to the model", "999", "", "", []string{"999"}},
		{"picked team pings all its linked managers", "", "u2", "Bob's Burgers", []string{"222", "333"}},
		{"typed team name matches too", "", "gridiron", "Gridiron Gang (alice)", []string{"111"}},
		{"unknown typed team passes through, pings nobody", "", "Nobody FC", "Nobody FC", nil},
		{"both given: team wins, member pinged once", "222", "u2", "Bob's Burgers", []string{"222", "333"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			team, pings := h.target(ctx, tt.who, tt.team)
			if team != tt.wantTeam {
				t.Errorf("team = %q, want %q", team, tt.wantTeam)
			}
			if strings.Join(pings, ",") != strings.Join(tt.wantPings, ",") {
				t.Errorf("pings = %v, want %v", pings, tt.wantPings)
			}
		})
	}
}

type fakeAI struct {
	teams []report.TeamRef
	err   error
	calls int
}

func (f *fakeAI) Answer(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeAI) Roast(context.Context, string, []string, string) (string, error) {
	return "", nil
}
func (f *fakeAI) Teams(context.Context) ([]report.TeamRef, error) {
	f.calls++
	return f.teams, f.err
}
