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

// TeamAbbrev returns a short (typically 4-letter) code for a roster, for use
// in compact scoreboard-style reports. It prefers a manual override (see
// SetAbbreviations) and otherwise derives one from the team's name.
func (c *LeagueContext) TeamAbbrev(rosterID int) string {
	if a, ok := c.abbrevByID[rosterID]; ok {
		return a
	}
	return deriveAbbrev(c.TeamName(rosterID))
}

// deriveAbbrev builds a 4-letter uppercase code from the first letters/digits
// of name, e.g. "The Wolfpack" -> "THEW". Falls back to "TEAM" if name has no
// alphanumeric characters at all.
func deriveAbbrev(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToUpper(r))
		}
		if b.Len() >= 4 {
			break
		}
	}
	if b.Len() == 0 {
		return "TEAM"
	}
	return b.String()
}

// ScoringType returns "ppr", "half_ppr", or "std" based on the league's
// reception scoring setting, matching one of the three precomputed point
// totals Sleeper's player projections publish (see
// sleeper.PlayerProjection.Points). Leagues with scoring beyond a plain PPR
// bonus (bonus yardage thresholds, TE premium, etc.) aren't reflected, which
// is why the projected scoreboard is always labeled "Approximate".
func (c *LeagueContext) ScoringType() string {
	switch rec := c.League.ScoringSettings["rec"]; {
	case rec >= 1:
		return "ppr"
	case rec > 0:
		return "half_ppr"
	default:
		return "std"
	}
}

// PlayerName resolves a Sleeper player ID to a display name.
func (c *LeagueContext) PlayerName(playerID string) string {
	if p, ok := c.Players[playerID]; ok {
		return p.Name()
	}
	return playerID
}
