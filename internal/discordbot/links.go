package discordbot

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/settings"
)

// linkSubcommands are the /settings subcommands that manage team links
// (see settings.TeamLink). They're handled by handleLinkSubcommand rather
// than handleSubcommand because they need the Sleeper team list and the
// interaction's resolved member data.
var linkSubcommands = map[string]bool{"link": true, "unlink": true}

func linkCommandOptions() []*discordgo.ApplicationCommandOption {
	return []*discordgo.ApplicationCommandOption{
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "link",
			Description: "Record which league team a Discord member manages (for /roast and @mentions)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "who",
					Description: "The Discord member",
					Required:    true,
				},
				{
					Type:         discordgo.ApplicationCommandOptionString,
					Name:         "team",
					Description:  "Their league team",
					Required:     true,
					Autocomplete: true,
				},
			},
		},
		{
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Name:        "unlink",
			Description: "Remove a Discord member's team link",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionUser,
					Name:        "who",
					Description: "The Discord member",
					Required:    true,
				},
			},
		},
	}
}

// handleLinkSubcommand applies /settings link or unlink, returning the
// message to show the invoking admin or an error describing what was wrong
// with their input, as handleSubcommand does.
func handleLinkSubcommand(ctx context.Context, mgr *settings.Manager, teams *teamDirectory, sub *discordgo.ApplicationCommandInteractionDataOption, resolved *discordgo.ApplicationCommandInteractionDataResolved) (string, error) {
	opts := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(sub.Options))
	for _, o := range sub.Options {
		opts[o.Name] = o
	}
	userID, _ := opts["who"].Value.(string)
	if userID == "" {
		return "", fmt.Errorf("pick a Discord member")
	}
	name := memberName(resolved, userID)

	switch sub.Name {
	case "link":
		teamInput := opts["team"].StringValue()
		team, ok := teams.resolve(ctx, teamInput)
		if !ok {
			return "", fmt.Errorf("couldn't find a team matching %q - pick one from the list as you type", teamInput)
		}
		next := applyTeamLink(mgr.Get(), userID, settings.TeamLink{
			SleeperUserID: team.OwnerID,
			DiscordName:   name,
			TeamLabel:     team.Label,
		})
		if err := mgr.Save(next); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Linked **%s** to **%s**.", name, team.Label), nil

	case "unlink":
		cur := mgr.Get()
		if _, ok := cur.TeamLinks[userID]; !ok {
			return "", fmt.Errorf("%s isn't linked to a team", name)
		}
		if err := mgr.Save(applyTeamUnlink(cur, userID)); err != nil {
			return "", err
		}
		return fmt.Sprintf("✅ Unlinked **%s**.", name), nil

	default:
		return "", fmt.Errorf("unknown subcommand %q", sub.Name)
	}
}

// applyTeamLink returns cur with discordUserID linked as given, copying
// TeamLinks rather than mutating the map shared with Manager.Get().
func applyTeamLink(cur settings.Settings, discordUserID string, link settings.TeamLink) settings.Settings {
	next := cur
	next.TeamLinks = make(map[string]settings.TeamLink, len(cur.TeamLinks)+1)
	for k, v := range cur.TeamLinks {
		next.TeamLinks[k] = v
	}
	next.TeamLinks[discordUserID] = link
	return next
}

// applyTeamUnlink returns cur without discordUserID's link (see
// applyTeamLink).
func applyTeamUnlink(cur settings.Settings, discordUserID string) settings.Settings {
	next := cur
	next.TeamLinks = make(map[string]settings.TeamLink, len(cur.TeamLinks))
	for k, v := range cur.TeamLinks {
		if k != discordUserID {
			next.TeamLinks[k] = v
		}
	}
	if len(next.TeamLinks) == 0 {
		next.TeamLinks = nil
	}
	return next
}

// formatTeamLinks renders the links for /settings view, sorted by
// Discord name, using the labels saved at linking (view has no Sleeper
// access - the same as the portal).
func formatTeamLinks(links map[string]settings.TeamLink) string {
	if len(links) == 0 {
		return fmt.Sprintf("none yet - use `/%s link`\n", rootCommandName)
	}
	lines := make([]string, 0, len(links))
	for _, l := range links {
		lines = append(lines, fmt.Sprintf("• %s → %s", l.DiscordName, l.TeamLabel))
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

// memberName is the best display name for userID in an interaction's
// resolved data: server nickname, else display name, else their ID.
func memberName(resolved *discordgo.ApplicationCommandInteractionDataResolved, userID string) string {
	if names := targetNames(resolved, userID); len(names) > 0 {
		return names[0]
	}
	return userID
}
