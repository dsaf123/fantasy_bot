package config

import "testing"

func TestLoadRequiresLeagueID(t *testing.T) {
	t.Setenv("LEAGUE_ID", "")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/x/y")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when LEAGUE_ID is unset")
	}
}

func TestLoadRequiresDiscordWebhook(t *testing.T) {
	t.Setenv("LEAGUE_ID", "123")
	t.Setenv("DISCORD_WEBHOOK_URL", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when DISCORD_WEBHOOK_URL is unset")
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("LEAGUE_ID", "123")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/x/y")
	t.Setenv("TIMEZONE", "")
	t.Setenv("CLOSE_SCORES_THRESHOLD", "")
	t.Setenv("DAILY_WAIVER", "")
	t.Setenv("MONITOR_REPORT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Timezone != "America/New_York" {
		t.Errorf("Timezone default = %q, want America/New_York", cfg.Timezone)
	}
	if cfg.CloseScoresThreshold != 15 {
		t.Errorf("CloseScoresThreshold default = %v, want 15", cfg.CloseScoresThreshold)
	}
	if cfg.DailyWaiver {
		t.Error("DailyWaiver default = true, want false")
	}
	if !cfg.MonitorReport {
		t.Error("MonitorReport default = false, want true")
	}
}

func TestLoadTeamAbbreviations(t *testing.T) {
	t.Setenv("LEAGUE_ID", "123")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/x/y")
	t.Setenv("TEAM_ABBREVIATIONS", `{"1":"DYNK","2":"ALMO"}`)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TeamAbbreviations[1] != "DYNK" || cfg.TeamAbbreviations[2] != "ALMO" {
		t.Errorf("TeamAbbreviations = %v, want {1:DYNK, 2:ALMO}", cfg.TeamAbbreviations)
	}
}

func TestLoadTeamAbbreviationsInvalidKey(t *testing.T) {
	t.Setenv("LEAGUE_ID", "123")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/x/y")
	t.Setenv("TEAM_ABBREVIATIONS", `{"not-a-roster-id":"DYNK"}`)

	if _, err := Load(); err == nil {
		t.Fatal("expected error for non-numeric TEAM_ABBREVIATIONS key")
	}
}

func TestLoadOverrides(t *testing.T) {
	t.Setenv("LEAGUE_ID", "123")
	t.Setenv("DISCORD_WEBHOOK_URL", "https://discord.com/api/webhooks/x/y")
	t.Setenv("TIMEZONE", "America/Chicago")
	t.Setenv("CLOSE_SCORES_THRESHOLD", "8.5")
	t.Setenv("DAILY_WAIVER", "true")
	t.Setenv("MONITOR_REPORT", "false")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Timezone != "America/Chicago" {
		t.Errorf("Timezone = %q, want America/Chicago", cfg.Timezone)
	}
	if cfg.CloseScoresThreshold != 8.5 {
		t.Errorf("CloseScoresThreshold = %v, want 8.5", cfg.CloseScoresThreshold)
	}
	if !cfg.DailyWaiver {
		t.Error("DailyWaiver = false, want true")
	}
	if cfg.MonitorReport {
		t.Error("MonitorReport = true, want false")
	}
}
