// Package config loads bot settings from environment variables, mirroring
// the env-var-driven configuration style of gamedaybot but trimmed to the
// Sleeper + Discord-webhook-only setup this bot targets.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	// LeagueID is the Sleeper league ID (the numeric ID in a league's URL).
	LeagueID string
	// DiscordWebhookURL is the incoming webhook the bot posts reports to.
	DiscordWebhookURL string

	// Timezone is the IANA timezone used to schedule reports (e.g.
	// "America/New_York").
	Timezone string

	// CloseScoresThreshold is the largest point difference that still
	// counts as a "close" matchup in the close-scores report.
	CloseScoresThreshold float64
	// DailyWaiver posts the waiver report every day instead of just Wednesday.
	DailyWaiver bool
	// MonitorReport enables the Sunday-morning injury/monitor report.
	MonitorReport bool

	// TeamAbbreviations overrides the auto-derived short team codes used in
	// scoreboard-style reports, keyed by Sleeper roster ID. Teams not listed
	// here fall back to a code derived from their team/display name.
	TeamAbbreviations map[int]string

	// InitMessage, if set, is posted once on startup to confirm the bot is
	// wired up correctly.
	InitMessage string

	// LLM holds the AI Weekly Recap's provider settings. LLM.APIKey is empty
	// when the recap isn't configured, in which case the bot skips it
	// entirely (see bot.Bot.Run and scheduler.Run).
	LLM LLMConfig

	// PortalPassword, if set, enables the web config portal (see
	// internal/portal) on PortalPort - the same on/off-by-presence pattern
	// as LLM.APIKey. Leave unset to disable the portal entirely.
	PortalPassword string
	// PortalPort is the port the web config portal listens on.
	PortalPort string
	// SettingsFile is where the web portal persists its overrides (see
	// internal/settings) across restarts.
	SettingsFile string

	// DiscordBotToken, if set, enables the /settings slash command (see
	// internal/discordbot) as a chat-native alternative to the web portal -
	// the same on/off-by-presence pattern as PortalPassword. Leave unset to
	// disable it entirely.
	DiscordBotToken string
	// DiscordGuildID, if set, registers slash commands to this one guild
	// only, which Discord applies within seconds instead of the up-to-an-
	// hour propagation delay for global commands - handy while setting the
	// bot up. The bot still only serves one league/settings file regardless
	// of how many guilds it's actually in.
	DiscordGuildID string
}

// LLMConfig configures the AI provider used to write the weekly recap
// newsletter (see report.LeagueContext.RecapDigest and bot.ReportRecap).
type LLMConfig struct {
	// Provider is one of "anthropic", "openai", or "gemini".
	Provider string
	APIKey   string
	// Model is the provider-specific model name, e.g. "claude-sonnet-5",
	// "gpt-5.1", or "gemini-2.5-pro".
	Model string
}

func Load() (*Config, error) {
	cfg := &Config{
		Timezone:             getEnv("TIMEZONE", "America/New_York"),
		CloseScoresThreshold: getEnvFloat("CLOSE_SCORES_THRESHOLD", 15),
		DailyWaiver:          getEnvBool("DAILY_WAIVER", false),
		MonitorReport:        getEnvBool("MONITOR_REPORT", true),
		InitMessage:          os.Getenv("INIT_MSG"),
		PortalPassword:       os.Getenv("PORTAL_PASSWORD"),
		PortalPort:           getEnv("PORTAL_PORT", "8080"),
		SettingsFile:         getEnv("SETTINGS_FILE", "data/settings.json"),
		DiscordBotToken:      os.Getenv("DISCORD_BOT_TOKEN"),
		DiscordGuildID:       os.Getenv("DISCORD_GUILD_ID"),
	}

	cfg.LeagueID = os.Getenv("LEAGUE_ID")
	if cfg.LeagueID == "" {
		return nil, fmt.Errorf("config: LEAGUE_ID is required (your Sleeper league ID)")
	}

	cfg.DiscordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	if cfg.DiscordWebhookURL == "" {
		return nil, fmt.Errorf("config: DISCORD_WEBHOOK_URL is required")
	}

	abbrevs, err := getEnvIntStringMap("TEAM_ABBREVIATIONS")
	if err != nil {
		return nil, fmt.Errorf("config: TEAM_ABBREVIATIONS: %w", err)
	}
	cfg.TeamAbbreviations = abbrevs

	llm, err := loadLLMConfig()
	if err != nil {
		return nil, err
	}
	cfg.LLM = llm

	return cfg, nil
}

// loadLLMConfig reads the AI Weekly Recap's provider settings. The recap is
// entirely optional: an unset LLM_API_KEY returns a zero-value LLMConfig and
// no error, and the bot skips the recap report. Once an API key is set, the
// provider and model become required so misconfiguration is caught at
// startup rather than the first time the recap tries to run.
func loadLLMConfig() (LLMConfig, error) {
	apiKey := os.Getenv("LLM_API_KEY")
	if apiKey == "" {
		return LLMConfig{}, nil
	}

	provider := strings.ToLower(strings.TrimSpace(os.Getenv("LLM_PROVIDER")))
	switch provider {
	case "anthropic", "openai", "gemini":
	default:
		return LLMConfig{}, fmt.Errorf("config: LLM_PROVIDER must be one of anthropic, openai, gemini (got %q)", provider)
	}

	model := os.Getenv("LLM_MODEL")
	if model == "" {
		return LLMConfig{}, fmt.Errorf("config: LLM_MODEL is required when LLM_API_KEY is set")
	}

	return LLMConfig{Provider: provider, APIKey: apiKey, Model: model}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}

func getEnvFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

// getEnvIntStringMap parses a JSON object of roster-ID-keyed strings, e.g.
// `{"1":"DYNK","2":"ALMO"}`, used for TEAM_ABBREVIATIONS. Returns nil if the
// variable is unset.
func getEnvIntStringMap(key string) (map[int]string, error) {
	v := os.Getenv(key)
	if v == "" {
		return nil, nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(v), &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	out := make(map[int]string, len(raw))
	for k, val := range raw {
		id, err := strconv.Atoi(k)
		if err != nil {
			return nil, fmt.Errorf("key %q is not a roster ID: %w", k, err)
		}
		out[id] = val
	}
	return out, nil
}
