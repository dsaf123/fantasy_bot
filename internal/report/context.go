// Package report builds Discord-ready text reports (scoreboards, standings,
// power rankings, trophies, waiver activity, injury monitor) from Sleeper
// league data. Each report is a pure function: given a LeagueContext (and,
// where relevant, matchups/transactions for a week) it returns formatted
// text with no side effects, so reports are easy to test and to trigger
// on demand outside the scheduler.
package report

import (
	"fmt"
	"strings"
	"unicode"

	"fantasy_bot/internal/sleeper"
)

// LeagueContext bundles the league-wide data most reports need so callers
// only fetch it once per run.
type LeagueContext struct {
	League  sleeper.League
	Rosters []sleeper.Roster
	Users   []sleeper.User
	Players map[string]sleeper.Player
	Week    int

	rosterByID map[int]sleeper.Roster
	userByID   map[string]sleeper.User
	abbrevByID map[int]string
}

func NewLeagueContext(league sleeper.League, rosters []sleeper.Roster, users []sleeper.User, players map[string]sleeper.Player, week int) *LeagueContext {
	ctx := &LeagueContext{
		League:     league,
		Rosters:    rosters,
		Users:      users,
		Players:    players,
		Week:       week,
		rosterByID: make(map[int]sleeper.Roster, len(rosters)),
		userByID:   make(map[string]sleeper.User, len(users)),
	}
	for _, r := range rosters {
		ctx.rosterByID[r.RosterID] = r
	}
	for _, u := range users {
		ctx.userByID[u.UserID] = u
	}
	return ctx
}

// SetAbbreviations installs manual roster-ID-keyed team abbreviation
// overrides (see config.TeamAbbreviations). Rosters not present in overrides
// fall back to an abbreviation derived from their team name.
func (c *LeagueContext) SetAbbreviations(overrides map[int]string) {
	c.abbrevByID = overrides
}

// TeamName resolves a roster ID to a human-readable team/manager name,
// preferring the owner's custom team name and falling back through their
// display name to a generic "Team <id>" placeholder for orphaned rosters.
func (c *LeagueContext) TeamName(rosterID int) string {
	roster, ok := c.rosterByID[rosterID]
	if !ok {
		return fmt.Sprintf("Team %d", rosterID)
	}
	user, ok := c.userByID[roster.OwnerID]
	if !ok {
		return fmt.Sprintf("Team %d", rosterID)
	}
	if user.Metadata.TeamName != "" {
		return user.Metadata.TeamName
	}
	if user.DisplayName != "" {
		return user.DisplayName
	}
	return fmt.Sprintf("Team %d", rosterID)
}

// TeamAbbrev returns a short (2-4 character) code for a roster, for use in
// compact scoreboard-style reports. It prefers a manual override (see
// SetAbbreviations) and otherwise derives one from the team's name.
func (c *LeagueContext) TeamAbbrev(rosterID int) string {
	if a, ok := c.abbrevByID[rosterID]; ok {
		return a
	}
	return deriveAbbrev(c.TeamName(rosterID))
}

// deriveAbbrev builds a short uppercase code from name. Names split into two
// or more words by whitespace or dashes take one letter/digit per word,
// capped at 4 words so the result still lines up in the scoreboard's
// fixed-width columns (see formatGameLine), e.g. "Andrew's Ass-Kickers" ->
// "AAK", "Kyle's Football Club" -> "KFC". A name with no such split (e.g.
// "andrewtheman123") instead falls back to its first 4 letters/digits, e.g.
// "ANDR". Falls back to "TEAM" if name has no alphanumeric characters at all.
func deriveAbbrev(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || r == '-'
	})

	var initials strings.Builder
	wordsUsed := 0
	for _, w := range words {
		for _, r := range w {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				initials.WriteRune(unicode.ToUpper(r))
				wordsUsed++
				break
			}
		}
		if wordsUsed >= 4 {
			break
		}
	}
	if wordsUsed >= 2 {
		return initials.String()
	}

	var b strings.Builder
	letters := 0
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
			letters++
		}
		if letters >= 4 {
			break
		}
	}
	if b.Len() == 0 {
		return "TEAM"
	}
	return b.String()
}

// PlayerName resolves a Sleeper player ID to a display name.
func (c *LeagueContext) PlayerName(playerID string) string {
	if p, ok := c.Players[playerID]; ok {
		return p.Name()
	}
	return playerID
}
