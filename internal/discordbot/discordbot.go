// Package discordbot lets server admins view and change fantasy_bot's
// portal-configurable settings (see internal/settings) from a Discord slash
// command, as a chat-native alternative to internal/portal's web dashboard,
// answers league questions when someone @mentions the bot (see
// mentions.go), and roasts league members on request (/roast, see
// roast.go).
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

// Serve connects to Discord, registers the /settings and /roast commands,
// starts answering @mentions via ai, and blocks until ctx is done, then
// disconnects. Callers should run it in its own
// goroutine: like internal/portal, this is supplementary to the bot's core
// scheduling function, so a failure here (bad token, network issue) should
// be logged by the caller rather than treated as fatal.
func Serve(ctx context.Context, cfg *config.Config, mgr *settings.Manager, ai LeagueAI) error {
	if cfg.DiscordBotToken == "" {
		return fmt.Errorf("discordbot: DISCORD_BOT_TOKEN is not set, refusing to start")
	}

	session, err := discordgo.New("Bot " + cfg.DiscordBotToken)
	if err != nil {
		return fmt.Errorf("discordbot: create session: %w", err)
	}
	// Interaction events are delivered over the gateway regardless of
	// intents; GuildMessages is only for noticing @mentions. That doesn't
	// need the privileged Message Content intent: Discord always includes a
	// message's content for bots it mentions by user (including a reply
	// that pings the bot) - see mentions.go for the role-mention caveat.
	session.Identify.Intents = discordgo.IntentsGuildMessages
	cool := newCooldowns(aiCooldown)
	session.AddHandler(interactionHandler(cfg, mgr, newRoastHandler(ai, cool)))
	session.AddHandler(newMentionHandler(cfg, mgr, ai, cool).handle)

	if err := session.Open(); err != nil {
		return fmt.Errorf("discordbot: connect: %w", err)
	}
	defer session.Close()

	scope := "globally (may take up to an hour to appear)"
	if cfg.DiscordGuildID != "" {
		scope = "to guild " + cfg.DiscordGuildID
	}
	if _, err := session.ApplicationCommandBulkOverwrite(session.State.User.ID, cfg.DiscordGuildID, buildCommands()); err != nil {
		return fmt.Errorf("discordbot: register /%s and /%s commands: %w", rootCommandName, roastCommandName, err)
	}
	log.Printf("discordbot: connected as %s, /%s and /%s registered %s", session.State.User.String(), rootCommandName, roastCommandName, scope)

	<-ctx.Done()
	return nil
}
