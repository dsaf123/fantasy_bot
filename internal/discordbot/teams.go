package discordbot

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"fantasy_bot/internal/report"
	"fantasy_bot/internal/settings"
)

const (
	// teamCacheTTL is how long the team list is reused between Sleeper
	// lookups - teams rarely change, and autocomplete fires on every
	// keystroke.
	teamCacheTTL = 5 * time.Minute
	// teamLookupTimeout keeps autocomplete inside Discord's 3-second
	// deadline even when Sleeper is slow.
	teamLookupTimeout = 2500 * time.Millisecond
	// maxAutocompleteChoices is Discord's cap on autocomplete suggestions.
	maxAutocompleteChoices = 25
	// maxChoiceLen is Discord's cap on an option choice's name and value.
	maxChoiceLen = 100
)

// teamDirectory is the league's team list, shared by /roast, /settings
// link, and @mention replies, plus lookups between teams and the Discord
// members linked to them (see settings.TeamLink).
type teamDirectory struct {
	ai  LeagueAI
	mgr *settings.Manager

	mu      sync.Mutex
	teams   []report.TeamRef
	fetched time.Time
}

func newTeamDirectory(ai LeagueAI, mgr *settings.Manager) *teamDirectory {
	return &teamDirectory{ai: ai, mgr: mgr}
}

// list returns the league's teams, cached for teamCacheTTL. On a failed
// refresh it falls back to the last good list, if any.
func (d *teamDirectory) list(ctx context.Context) ([]report.TeamRef, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.teams != nil && time.Since(d.fetched) < teamCacheTTL {
		return d.teams, nil
	}
	teams, err := d.ai.Teams(ctx)
	if err != nil {
		return d.teams, err
	}
	d.teams, d.fetched = teams, time.Now()
	return teams, nil
}

// resolve finds the team a /roast or /settings link "team" option names.
// Picking from autocomplete sends the owner's Sleeper ID; typing without
// picking sends raw text, matched against labels (exactly, else as the
// one label containing it).
func (d *teamDirectory) resolve(ctx context.Context, input string) (report.TeamRef, bool) {
	input = strings.TrimSpace(input)
	if input == "" {
		return report.TeamRef{}, false
	}
	teams, err := d.list(ctx)
	if err != nil {
		log.Printf("discordbot: look up teams: %v", err)
	}
	lower := strings.ToLower(input)
	var partial []report.TeamRef
	for _, t := range teams {
		if t.OwnerID == input || strings.ToLower(t.Label) == lower {
			return t, true
		}
		if strings.Contains(strings.ToLower(t.Label), lower) {
			partial = append(partial, t)
		}
	}
	if len(partial) == 1 {
		return partial[0], true
	}
	return report.TeamRef{}, false
}

// linkedTeam returns the label of the team discordUserID is linked to: the
// current name from Sleeper when available, else the one saved at linking.
func (d *teamDirectory) linkedTeam(ctx context.Context, discordUserID string) (string, bool) {
	link, ok := d.mgr.Get().TeamLinks[discordUserID]
	if !ok {
		return "", false
	}
	return d.label(ctx, link.SleeperUserID, link.TeamLabel), true
}

// label returns ownerID's current team label, or fallback if Sleeper can't
// be reached or no longer lists that owner.
func (d *teamDirectory) label(ctx context.Context, ownerID, fallback string) string {
	teams, err := d.list(ctx)
	if err != nil {
		log.Printf("discordbot: look up teams: %v", err)
	}
	for _, t := range teams {
		if t.OwnerID == ownerID {
			return t.Label
		}
	}
	return fallback
}

// autocomplete answers an autocomplete interaction for a "team" option
// with the teams matching what's been typed so far.
func (d *teamDirectory) autocomplete(s *discordgo.Session, i *discordgo.InteractionCreate) {
	typed := focusedValue(i.ApplicationCommandData().Options)

	ctx, cancel := context.WithTimeout(context.Background(), teamLookupTimeout)
	defer cancel()
	teams, err := d.list(ctx)
	if err != nil {
		log.Printf("discordbot: team autocomplete: %v", err)
	}

	matches := filterTeams(teams, typed)
	choices := make([]*discordgo.ApplicationCommandOptionChoice, len(matches))
	for n, t := range matches {
		choices[n] = &discordgo.ApplicationCommandOptionChoice{Name: t.Label, Value: t.OwnerID}
	}
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionApplicationCommandAutocompleteResult,
		Data: &discordgo.InteractionResponseData{Choices: choices},
	}); err != nil {
		log.Printf("discordbot: respond to team autocomplete: %v", err)
	}
}

// focusedValue returns the text typed into whichever option (at any
// subcommand depth) the user is currently filling in.
func focusedValue(opts []*discordgo.ApplicationCommandInteractionDataOption) string {
	for _, o := range opts {
		if o.Focused {
			if v, ok := o.Value.(string); ok {
				return v
			}
			return ""
		}
		if v := focusedValue(o.Options); v != "" {
			return v
		}
	}
	return ""
}

// filterTeams returns the teams whose label contains typed
// (case-insensitively), capped and trimmed to Discord's autocomplete
// limits.
func filterTeams(teams []report.TeamRef, typed string) []report.TeamRef {
	typed = strings.ToLower(strings.TrimSpace(typed))
	var out []report.TeamRef
	for _, t := range teams {
		if len(out) == maxAutocompleteChoices {
			break
		}
		if typed != "" && !strings.Contains(strings.ToLower(t.Label), typed) {
			continue
		}
		if r := []rune(t.Label); len(r) > maxChoiceLen {
			t.Label = string(r[:maxChoiceLen])
		}
		out = append(out, t)
	}
	return out
}
