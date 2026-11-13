package discordbot

import (
	"context"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/report"
	"fantasy_bot/internal/settings"
)

func userOpt(name, id string) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: name, Type: discordgo.ApplicationCommandOptionUser, Value: id}
}

func TestLinkAndUnlink(t *testing.T) {
	mgr := newTestManager(t)
	ai := &fakeAI{teams: []report.TeamRef{{OwnerID: "u1", Label: "Gridiron Gang (alice)"}}}
	teams := newTeamDirectory(ai, mgr)
	resolved := &discordgo.ApplicationCommandInteractionDataResolved{
		Users:   map[string]*discordgo.User{"111": {ID: "111", Username: "alice"}},
		Members: map[string]*discordgo.Member{"111": {Nick: "Commish Alice"}},
	}
	ctx := context.Background()

	content, err := handleLinkSubcommand(ctx, mgr, teams, sub("link", userOpt("who", "111"), strOpt("team", "u1")), resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "Commish Alice") || !strings.Contains(content, "Gridiron Gang (alice)") {
		t.Errorf("link reply = %q", content)
	}
	want := settings.TeamLink{SleeperUserID: "u1", DiscordName: "Commish Alice", TeamLabel: "Gridiron Gang (alice)"}
	if got := mgr.Get().TeamLinks["111"]; got != want {
		t.Errorf("stored link = %+v, want %+v", got, want)
	}
	if view := buildView(&config.Config{}, mgr.Get()); !strings.Contains(view, "• Commish Alice → Gridiron Gang (alice)") {
		t.Errorf("view missing link:\n%s", view)
	}

	if _, err := handleLinkSubcommand(ctx, mgr, teams, sub("link", userOpt("who", "111"), strOpt("team", "Nobody FC")), resolved); err == nil {
		t.Error("linking to an unknown team should fail")
	}

	// A full reset keeps links: they're league data, not .env overrides.
	if _, err := handleSubcommand(&config.Config{}, mgr, sub("reset", strOpt("scope", "all"))); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.Get().TeamLinks["111"]; !ok {
		t.Error("reset all dropped the team link")
	}

	if _, err := handleLinkSubcommand(ctx, mgr, teams, sub("unlink", userOpt("who", "111")), resolved); err != nil {
		t.Fatal(err)
	}
	if got := mgr.Get().TeamLinks; got != nil {
		t.Errorf("TeamLinks after unlink = %v, want nil", got)
	}
	if _, err := handleLinkSubcommand(ctx, mgr, teams, sub("unlink", userOpt("who", "111")), resolved); err == nil {
		t.Error("unlinking someone who isn't linked should fail")
	}
}

func TestSettingsCommandIncludesLinkSubcommands(t *testing.T) {
	var got []string
	for _, c := range buildCommands() {
		if c.Name != rootCommandName {
			continue
		}
		for _, o := range c.Options {
			got = append(got, o.Name)
		}
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "link") || !strings.Contains(joined, "unlink") {
		t.Errorf("/settings subcommands = %v, want link and unlink", got)
	}
}
