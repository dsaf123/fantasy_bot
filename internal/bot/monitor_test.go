package bot

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"fantasy_bot/internal/report"
	"fantasy_bot/internal/settings"
	"fantasy_bot/internal/sleeper"
)

// recordingSender captures what a report posts instead of sending it.
type recordingSender struct {
	plain []string
	rich  []string
}

func (r *recordingSender) Send(_ context.Context, text string) error {
	r.plain = append(r.plain, text)
	return nil
}

func (r *recordingSender) SendRich(_ context.Context, text string) error {
	r.rich = append(r.rich, text)
	return nil
}

func (r *recordingSender) SendImage(context.Context, string, string, []byte) error { return nil }

const (
	kyleDiscordID   = "111111111111111111"
	andrewDiscordID = "222222222222222222"
)

func monitorTestBot(t *testing.T) (*Bot, *recordingSender) {
	t.Helper()
	mgr, err := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Save(settings.Settings{TeamLinks: map[string]settings.TeamLink{
		kyleDiscordID:   {SleeperUserID: "u1"},
		andrewDiscordID: {SleeperUserID: "u2"},
	}}); err != nil {
		t.Fatal(err)
	}
	sender := &recordingSender{}
	return &Bot{sender: sender, settingsMgr: mgr}, sender
}

func monitorTestContext(players map[string]sleeper.Player) *report.LeagueContext {
	users := []sleeper.User{
		{UserID: "u1", Metadata: sleeper.UserMetadata{TeamName: "Kyle's Football Club"}},
		{UserID: "u2", Metadata: sleeper.UserMetadata{TeamName: "Andrew's Ass-Kickers"}},
		{UserID: "u3", Metadata: sleeper.UserMetadata{TeamName: "Unlinked"}},
	}
	rosters := []sleeper.Roster{
		{RosterID: 1, OwnerID: "u1", Starters: []string{"higgins"}},
		{RosterID: 2, OwnerID: "u2", Starters: []string{"kelce"}},
		{RosterID: 3, OwnerID: "u3", Starters: []string{"addison"}},
	}
	return report.NewLeagueContext(sleeper.League{}, rosters, users, players, 10)
}

func TestRunMonitorPingsLinkedOwnersOfFlaggedTeams(t *testing.T) {
	b, sender := monitorTestBot(t)
	ctx := monitorTestContext(map[string]sleeper.Player{
		"higgins": {FullName: "Tee Higgins", Position: "WR", Team: "CIN", InjuryStatus: "Questionable"},
		"kelce":   {FullName: "Travis Kelce", Position: "TE", Team: "KC"},
		"addison": {FullName: "Jordan Addison", Position: "WR", Team: "MIN", InjuryStatus: "Out"},
	})

	if err := b.runMonitor(context.Background(), ctx, nil, 10, false); err != nil {
		t.Fatal(err)
	}
	if len(sender.plain) != 1 || !strings.HasPrefix(sender.plain[0], report.MonitorHeader+"\n") {
		t.Fatalf("plain posts = %q, want one monitor report", sender.plain)
	}
	// Kyle's team is flagged and linked; Andrew's isn't flagged; the
	// flagged Unlinked team has nobody to ping.
	if len(sender.rich) != 1 || !strings.HasPrefix(sender.rich[0], "<@"+kyleDiscordID+"> - ") {
		t.Errorf("mention posts = %q, want just Kyle pinged", sender.rich)
	}
}

func TestRunMonitorUpdatePostsOnlyWhenSomethingIsNew(t *testing.T) {
	b, sender := monitorTestBot(t)
	players := map[string]sleeper.Player{
		"higgins": {FullName: "Tee Higgins", Position: "WR", Team: "CIN", InjuryStatus: "Questionable"},
		"kelce":   {FullName: "Travis Kelce", Position: "TE", Team: "KC"},
		"addison": {FullName: "Jordan Addison", Position: "WR", Team: "MIN"},
	}
	if err := b.runMonitor(context.Background(), monitorTestContext(players), nil, 10, false); err != nil {
		t.Fatal(err)
	}
	sender.plain, sender.rich = nil, nil

	// Nothing changed: the update stays quiet.
	if err := b.runMonitor(context.Background(), monitorTestContext(players), nil, 10, true); err != nil {
		t.Fatal(err)
	}
	if len(sender.plain)+len(sender.rich) != 0 {
		t.Fatalf("update posted %q / %q with nothing new", sender.plain, sender.rich)
	}

	// Kelce newly ruled out: the update posts the full list, pinging only
	// the manager with something new.
	players["kelce"] = sleeper.Player{FullName: "Travis Kelce", Position: "TE", Team: "KC", InjuryStatus: "Out"}
	if err := b.runMonitor(context.Background(), monitorTestContext(players), nil, 10, true); err != nil {
		t.Fatal(err)
	}
	if len(sender.plain) != 1 || !strings.HasPrefix(sender.plain[0], report.MonitorUpdateHeader+"\n") ||
		!strings.Contains(sender.plain[0], "WR Tee Higgins - Questionable") {
		t.Fatalf("update report = %q, want the full current list under the update header", sender.plain)
	}
	if len(sender.rich) != 1 || !strings.HasPrefix(sender.rich[0], "<@"+andrewDiscordID+"> - ") {
		t.Errorf("update mentions = %q, want just Andrew pinged", sender.rich)
	}
}

func TestRunMonitorUpdateWithoutMorningRunPostsEverything(t *testing.T) {
	b, sender := monitorTestBot(t)
	players := map[string]sleeper.Player{
		"higgins": {FullName: "Tee Higgins", Position: "WR", Team: "CIN", InjuryStatus: "Out"},
	}
	// A morning run from last week doesn't count as this week's baseline.
	b.lastMonitor = monitorSnapshot{week: 9, teams: monitorTestContext(players).MonitorFlags(nil)}

	if err := b.runMonitor(context.Background(), monitorTestContext(players), nil, 10, true); err != nil {
		t.Fatal(err)
	}
	if len(sender.plain) != 1 || len(sender.rich) != 1 {
		t.Errorf("posts = %q / %q, want the report plus pings", sender.plain, sender.rich)
	}
}
