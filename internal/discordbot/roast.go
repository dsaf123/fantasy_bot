package discordbot

import (
	"context"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

// roastCommandName is the /roast slash command: an AI-written roast of a
// league member's fantasy team, posted publicly. Unlike /settings it's open
// to everyone by default; server admins can restrict or disable it from
// Discord's own Integrations settings.
const roastCommandName = "roast"

func roastCommand() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:         roastCommandName,
		Description:  "Have the bot roast a league member's fantasy team",
		DMPermission: &commandsUsableInDMs,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionUser,
				Name:        "who",
				Description: "The league member to roast (they'll be pinged; uses their linked team if set)",
			},
			{
				Type:         discordgo.ApplicationCommandOptionString,
				Name:         "team",
				Description:  "The team to roast (pings whoever's linked to it)",
				Autocomplete: true,
			},
		},
	}
}

// roastHandler serves /roast. Its team autocomplete is answered by the
// shared teamDirectory.
type roastHandler struct {
	ai    LeagueAI
	cool  *cooldowns
	teams *teamDirectory
}

func newRoastHandler(ai LeagueAI, cool *cooldowns, teams *teamDirectory) *roastHandler {
	return &roastHandler{ai: ai, cool: cool, teams: teams}
}

// handle runs /roast. Writing the roast takes longer than Discord's
// 3-second reply deadline, so it acknowledges first ("bot is thinking…")
// and edits in the roast once it's ready.
func (h *roastHandler) handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()
	var targetID, teamInput string
	for _, o := range data.Options {
		switch o.Name {
		case "who":
			if id, ok := o.Value.(string); ok {
				targetID = id
			}
		case "team":
			teamInput = strings.TrimSpace(o.StringValue())
		}
	}
	if targetID == "" && teamInput == "" {
		respondEphemeral(s, i, "Pick someone to roast with `who`, a team with `team`, or both.")
		return
	}

	invoker := interactionUser(i)
	if invoker == nil {
		return
	}
	if !h.cool.allow(invoker.ID, time.Now()) {
		respondEphemeral(s, i, "⏳ Easy there - give it a few seconds before the next one.")
		return
	}

	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		log.Printf("discordbot: acknowledge /%s: %v", roastCommandName, err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), answerTimeout)
	defer cancel()
	team, pings := h.target(ctx, targetID, teamInput)
	roast, err := h.ai.Roast(ctx, interactionUserName(i), targetNames(data.Resolved, targetID), team)
	if err != nil {
		log.Printf("discordbot: /%s from %s: %v", roastCommandName, invoker.ID, err)
		roast = "My roast writer walked off the job. Try again in a bit."
	}
	if strings.TrimSpace(roast) == "" {
		roast = "I tried, but honestly, their team roasts itself."
	}

	// Ping the target (that's half the fun) but nobody else, whatever the
	// LLM wrote.
	allowed := &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: pings}
	if len(pings) > 0 {
		var prefix strings.Builder
		for _, id := range pings {
			prefix.WriteString("<@" + id + "> ")
		}
		roast = prefix.String() + roast
	}

	for n, chunk := range splitMessage(roast, messageLimit) {
		if n == 0 {
			_, err = s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{Content: &chunk, AllowedMentions: allowed})
		} else {
			_, err = s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{Content: chunk, AllowedMentions: allowed})
		}
		if err != nil {
			log.Printf("discordbot: post /%s result: %v", roastCommandName, err)
			return
		}
	}
}

// target works out which team /roast is about and who to ping, using the
// team links (see settings.TeamLink): a picked team pings whoever's linked
// to it, and a picked member with no team given is roasted for their
// linked team. team is "" when neither says, leaving the model to match
// the member's Discord names itself; an unrecognized typed team is passed
// through as-is for the same reason.
func (h *roastHandler) target(ctx context.Context, targetID, teamInput string) (team string, pings []string) {
	if teamInput != "" {
		if ref, ok := h.teams.resolve(ctx, teamInput); ok {
			team = ref.Label
			pings = h.teams.mgr.Get().DiscordUsersForSleeperUser(ref.OwnerID)
		} else {
			team = teamInput
		}
	}
	if targetID != "" {
		if team == "" {
			team, _ = h.teams.linkedTeam(ctx, targetID)
		}
		if !slices.Contains(pings, targetID) {
			pings = append([]string{targetID}, pings...)
		}
	}
	return team, pings
}

// targetNames collects the distinct names the /roast target goes by in
// this server - nickname, display name, username - for matching them to a
// Sleeper manager or team.
func targetNames(resolved *discordgo.ApplicationCommandInteractionDataResolved, userID string) []string {
	if resolved == nil || userID == "" {
		return nil
	}
	var candidates []string
	if m, ok := resolved.Members[userID]; ok && m != nil {
		candidates = append(candidates, m.Nick)
	}
	if u, ok := resolved.Users[userID]; ok && u != nil {
		candidates = append(candidates, u.GlobalName, u.Username)
	}
	seen := make(map[string]bool, len(candidates))
	var names []string
	for _, n := range candidates {
		if n = strings.TrimSpace(n); n != "" && !seen[strings.ToLower(n)] {
			seen[strings.ToLower(n)] = true
			names = append(names, n)
		}
	}
	return names
}

// interactionUser is whoever ran the command: Member.User in a server,
// User in a DM.
func interactionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Member != nil && i.Member.User != nil {
		return i.Member.User
	}
	return i.User
}

// interactionUserName is the invoker's server nickname, else their display
// name.
func interactionUserName(i *discordgo.InteractionCreate) string {
	if i.Member != nil && i.Member.Nick != "" {
		return i.Member.Nick
	}
	if u := interactionUser(i); u != nil {
		return u.DisplayName()
	}
	return "Someone"
}

func respondEphemeral(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	}); err != nil {
		log.Printf("discordbot: respond to interaction: %v", err)
	}
}
