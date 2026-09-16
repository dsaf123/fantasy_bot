package discordbot

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// rootCommandName is the single slash command this package registers;
// everything lives under /settings <subcommand> rather than as separate
// top-level commands, so there's just one entry for Discord's command list
// and for a server admin's own permission overrides to manage.
const rootCommandName = "settings"

// defaultMemberPermissions restricts /settings to members with "Manage
// Server" by default - the slash-command equivalent of the web portal's
// password gate. Server admins can further customize who can run it from
// Discord's own Integrations settings.
var defaultMemberPermissions = int64(discordgo.PermissionManageGuild)

// commandsUsableInDMs is false because DefaultMemberPermissions is a
// guild-membership concept that Discord does not enforce in DMs - without
// explicitly disabling DMs, anyone able to DM the bot could run /settings
// there and bypass the "Manage Server" gate entirely.
var commandsUsableInDMs = false

// buildCommands returns the /settings command tree, built from
// settings.Jobs so its "job" subcommand's choices can never drift from the
// portal's job table.
func buildCommands() []*discordgo.ApplicationCommand {
	jobNameChoices := make([]*discordgo.ApplicationCommandOptionChoice, len(settings.Jobs))
	for i, j := range settings.Jobs {
		jobNameChoices[i] = &discordgo.ApplicationCommandOptionChoice{Name: j.Label, Value: j.Name}
	}

	return []*discordgo.ApplicationCommand{
		{
			Name:                     rootCommandName,
			Description:              "View or change fantasy_bot's runtime settings (same as the web portal)",
			DefaultMemberPermissions: &defaultMemberPermissions,
			DMPermission:             &commandsUsableInDMs,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "view",
					Description: "Show the current effective settings",
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "job",
					Description: "Turn one scheduled message on or off",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "name",
							Description: "Which scheduled message",
							Required:    true,
							Choices:     jobNameChoices,
						},
						{
							Type:        discordgo.ApplicationCommandOptionBoolean,
							Name:        "enabled",
							Description: "Post this message on its schedule?",
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "waiver-days",
					Description: "Set which day(s) the Waiver Report posts on",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "days",
							Description: `Days, e.g. "wed" or "sun,wed,fri" (also "all"/"none")`,
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "timezone",
					Description: "Set the timezone used for league-time messages",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "value",
							Description: `IANA timezone, e.g. "America/New_York" or "UTC"`,
							Required:    true,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "recap-prompt",
					Description: "View or change the AI Weekly Recap's system prompt",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "prompt",
							Description: "New system prompt, max 1900 characters (omit to view the current one)",
							MaxLength:   1900,
						},
					},
				},
				{
					Type:        discordgo.ApplicationCommandOptionSubCommand,
					Name:        "reset",
					Description: "Reset overrides back to .env defaults",
					Options: []*discordgo.ApplicationCommandOption{
						{
							Type:        discordgo.ApplicationCommandOptionString,
							Name:        "scope",
							Description: "What to reset",
							Required:    true,
							Choices: []*discordgo.ApplicationCommandOptionChoice{
								{Name: "Everything", Value: "all"},
								{Name: "Recap prompt only", Value: "recap_prompt"},
							},
						},
					},
				},
			},
		},
	}
}

// interactionHandler returns the discordgo handler for every interaction
// the session receives, dispatching /settings's subcommands to
// handleSubcommand and always replying ephemerally - these are admin
// controls, not something that should clutter the channel.
func interactionHandler(cfg *config.Config, mgr *settings.Manager) func(*discordgo.Session, *discordgo.InteractionCreate) {
	return func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		if i.Type != discordgo.InteractionApplicationCommand {
			return
		}
		data := i.ApplicationCommandData()
		if data.Name != rootCommandName || len(data.Options) == 0 {
			return
		}

		sub := data.Options[0]
		content, err := handleSubcommand(cfg, mgr, sub)
		if err != nil {
			content = "⚠️ " + err.Error()
		}
		resp := &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: content,
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		}
		if err := s.InteractionRespond(i.Interaction, resp); err != nil {
			log.Printf("discordbot: respond to /%s %s: %v", rootCommandName, sub.Name, err)
		}
	}
}

// handleSubcommand applies one /settings subcommand and returns the message
// to show the invoking admin, or an error describing what was wrong with
// their input (shown to them the same way, prefixed with a warning emoji -
// these are all user-input mistakes, not server errors).
func handleSubcommand(cfg *config.Config, mgr *settings.Manager, sub *discordgo.ApplicationCommandInteractionDataOption) (string, error) {
	opts := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(sub.Options))
	for _, o := range sub.Options {
		opts[o.Name] = o
	}

	switch sub.Name {
	case "view":
		return buildView(cfg, mgr.Get()), nil

	case "job":
		name := opts["name"].StringValue()
		enabled := opts["enabled"].BoolValue()
		if err := mgr.Save(applyJobToggle(mgr.Get(), name, enabled)); err != nil {
			return "", err
		}
		label := name
		for _, j := range settings.Jobs {
			if j.Name == name {
				label = j.Label
			}
		}
		state := "disabled"
		if enabled {
			state = "enabled"
		}
		return fmt.Sprintf("✅ %s is now **%s**.", label, state), nil

	case "waiver-days":
		days, err := parseWaiverDays(opts["days"].StringValue())
		if err != nil {
			return "", err
		}
		if err := mgr.Save(applyWaiverDays(mgr.Get(), days)); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Waiver Report will now post: **%s**.", formatWaiverDays(days)), nil

	case "timezone":
		tz := strings.TrimSpace(opts["value"].StringValue())
		if tz == "" {
			return "", fmt.Errorf("timezone can't be blank")
		}
		if err := mgr.Save(applyTimezone(mgr.Get(), tz)); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Timezone set to **%s**.", tz), nil

	case "recap-prompt":
		promptOpt, given := opts["prompt"]
		if !given {
			return "**Current recap prompt:**\n" + mgr.Get().EffectiveRecapPrompt(settings.DefaultRecapPrompt), nil
		}
		prompt := strings.TrimSpace(promptOpt.StringValue())
		if prompt == "" {
			return "", fmt.Errorf(`recap prompt can't be blank - use "/%s reset" to clear it`, rootCommandName)
		}
		if err := mgr.Save(applyRecapPrompt(mgr.Get(), prompt)); err != nil {
			return "", err
		}
		return "✅ Recap prompt updated.", nil

	case "reset":
		scope := opts["scope"].StringValue()
		next, err := applyReset(mgr.Get(), scope)
		if err != nil {
			return "", err
		}
		if err := mgr.Save(next); err != nil {
			return "", err
		}
		if scope == "all" {
			return "✅ All settings reset to .env defaults.", nil
		}
		return "✅ Recap prompt reset to default.", nil

	default:
		return "", fmt.Errorf("unknown subcommand %q", sub.Name)
	}
}
