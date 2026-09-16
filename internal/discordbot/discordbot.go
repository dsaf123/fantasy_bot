// Package discordbot lets server admins view and change fantasy_bot's
// portal-configurable settings (see internal/settings) from a Discord slash
// command, as a chat-native alternative to internal/portal's web dashboard.
//
// It connects as a real bot (gateway + bot token, via
// github.com/bwmarrin/discordgo) rather than using Discord's HTTP
// "Interactions Endpoint URL" option, which would require this bot to be
// reachable from the public internet over HTTPS - a much bigger ask than a
// bot token for the home-network/Portainer deployments this project targets
// (see README.md). A gateway connection only needs outbound network access,
// the same as the Sleeper API calls and the existing webhook sender.
package discordbot

import (
	"context"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// Serve connects to Discord, registers the /settings command, and blocks
// until ctx is done, then disconnects. Callers should run it in its own
// goroutine: like internal/portal, this is supplementary to the bot's core
// scheduling function, so a failure here (bad token, network issue) should
// be logged by the caller rather than treated as fatal.
func Serve(ctx context.Context, cfg *config.Config, mgr *settings.Manager) error {
	if cfg.DiscordBotToken == "" {
		return fmt.Errorf("discordbot: DISCORD_BOT_TOKEN is not set, refusing to start")
	}

	session, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		return fmt.Errorf("discordbot: create session: %w", err)
	}
	// Interaction events are delivered over the gateway regardless of
	// intents, and this bot never needs to observe guild or message events.
	session.Identify.Intents = discordgo.IntentsNone
	session.AddHandler(interactionHandler(cfg, mgr))

	if err := session.Open(); err != nil {
		return fmt.Errorf("discordbot: connect: %w", err)
	}
	defer session.Close()

	scope := "globally (may take up to an hour to appear)"
	if cfg.DiscordGuildID != "" {
		scope = "to guild " + cfg.DiscordGuildID
	}
	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, cfg.DiscordGuildID, buildCommands()); err != nil {
		return fmt.Errorf("discordbot: register /%s command: %w", rootCommandName, err)
	}
	log.Printf("discordbot: connected as %s, /%s registered %s", session.State.User.String(), rootCommandName, scope)

	<-ctx.Done()
	return nil
}
