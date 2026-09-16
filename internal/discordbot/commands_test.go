package discordbot

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

func newTestManager(t *testing.T) *settings.Manager {
	t.Helper()
	mgr, err := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return mgr
}

func strOpt(name, value string) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: name, Type: discordgo.ApplicationCommandOptionString, Value: value}
}

func boolOpt(name string, value bool) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: name, Type: discordgo.ApplicationCommandOptionBoolean, Value: value}
}

func sub(name string, opts ...*discordgo.ApplicationCommandInteractionDataOption) *discordgo.ApplicationCommandInteractionDataOption {
	return &discordgo.ApplicationCommandInteractionDataOption{Name: name, Type: discordgo.ApplicationCommandOptionSubCommand, Options: opts}
}

func TestHandleSubcommandView(t *testing.T) {
	cfg := &config.Config{Timezone: "America/New_York", MonitorReport: true}
	mgr := newTestManager(t)

	content, err := handleSubcommand(cfg, mgr, sub("view"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "America/New_York") {
		t.Errorf("view output missing default timezone: %s", content)
	}
}

func TestHandleSubcommandJobTogglePersists(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	content, err := handleSubcommand(cfg, mgr, sub("job", strOpt("name", "monitor"), boolOpt("enabled", false)))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "disabled") {
		t.Errorf("confirmation = %q, want it to mention disabled", content)
	}
	if mgr.Get().JobIsEnabled("monitor", true) {
		t.Error("monitor should be persisted as disabled")
	}
}

// TestHandleSubcommandWaiverDaysInvalidReturnsErrorNotPersisted checks
// handleSubcommand's error path directly; interactionHandler is what turns
// this error into the ⚠️-prefixed ephemeral content shown to the user.
func TestHandleSubcommandWaiverDaysInvalidReturnsErrorNotPersisted(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	if _, err := handleSubcommand(cfg, mgr, sub("waiver-days", strOpt("days", "funday"))); err == nil {
		t.Fatal("expected an error for an unrecognized day token")
	}
	if mgr.Get().WaiverDays != nil {
		t.Error("an invalid waiver-days input must not be persisted")
	}
}

func TestHandleSubcommandWaiverDaysValid(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	if _, err := handleSubcommand(cfg, mgr, sub("waiver-days", strOpt("days", "wed,fri"))); err != nil {
		t.Fatal(err)
	}
	got := mgr.Get().EffectiveWaiverDays(nil)
	if len(got) != 2 || got[0] != 3 || got[1] != 5 {
		t.Errorf("WaiverDays = %v, want [3 5]", got)
	}
}

func TestHandleSubcommandTimezoneInvalidNotPersisted(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	if _, err := handleSubcommand(cfg, mgr, sub("timezone", strOpt("value", "Not/AZone"))); err == nil {
		t.Fatal("expected an error for an invalid IANA timezone")
	}
	if mgr.Get().Timezone != nil {
		t.Error("an invalid timezone must not be persisted")
	}
}

func TestHandleSubcommandRecapPromptOmittedShowsCurrent(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	content, err := handleSubcommand(cfg, mgr, sub("recap-prompt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "ghostwriter") {
		t.Errorf("content should show the default recap prompt, got: %s", content)
	}
}

func TestHandleSubcommandRecapPromptSetAndReset(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	if _, err := handleSubcommand(cfg, mgr, sub("recap-prompt", strOpt("prompt", "Be brief."))); err != nil {
		t.Fatal(err)
	}
	if mgr.Get().EffectiveRecapPrompt("") != "Be brief." {
		t.Fatalf("RecapPrompt = %v, want 'Be brief.'", mgr.Get().RecapPrompt)
	}

	if _, err := handleSubcommand(cfg, mgr, sub("reset", strOpt("scope", "recap_prompt"))); err != nil {
		t.Fatal(err)
	}
	if mgr.Get().RecapPrompt != nil {
		t.Error("reset recap_prompt should clear the override")
	}
}

func TestHandleSubcommandResetAll(t *testing.T) {
	cfg := &config.Config{}
	mgr := newTestManager(t)

	tz := "America/Chicago"
	if err := mgr.Save(settings.Settings{Timezone: &tz}); err != nil {
		t.Fatal(err)
	}

	if _, err := handleSubcommand(cfg, mgr, sub("reset", strOpt("scope", "all"))); err != nil {
		t.Fatal(err)
	}
	if mgr.Get().Timezone != nil {
		t.Error("reset all should clear every override")
	}
}
