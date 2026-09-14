package report

import (
	"fmt"
	"sort"
	"strings"

	"fantasy_bot/internal/sleeper"
)

// Game is one head-to-head matchup: two rosters sharing a MatchupID. Sleeper
// pairs opponents this way rather than nesting them, so every report that
// needs "who's playing whom" goes through pairGames.
type Game struct {
	MatchupID int
	Home      GameSide
	Away      GameSide
}

type GameSide struct {
	RosterID int
	Points   float64
}

// Margin returns the absolute point difference between the two sides.
func (g Game) Margin() float64 {
	d := g.Home.Points - g.Away.Points
	if d < 0 {
		return -d
	}
	return d
}

// Winner returns the roster ID of the leading/winning side, or -1 for a tie.
func (g Game) Winner() int {
	switch {
	case g.Home.Points > g.Away.Points:
		return g.Home.RosterID
	case g.Away.Points > g.Home.Points:
		return g.Away.RosterID
	default:
		return -1
	}
}

// pairGames groups Sleeper's flat per-roster matchup rows into head-to-head
// games by MatchupID. A roster with a bye (no opponent that week) is
// returned with an empty Away side.
func pairGames(matchups []sleeper.Matchup) []Game {
	byMatchupID := make(map[int][]sleeper.Matchup)
	var order []int
	for _, m := range matchups {
		if _, seen := byMatchupID[m.MatchupID]; !seen {
			order = append(order, m.MatchupID)
		}
		byMatchupID[m.MatchupID] = append(byMatchupID[m.MatchupID], m)
	}

	games := make([]Game, 0, len(order))
	for _, id := range order {
		sides := byMatchupID[id]
		g := Game{MatchupID: id}
		g.Home = GameSide{RosterID: sides[0].RosterID, Points: sides[0].Points}
		if len(sides) > 1 {
			g.Away = GameSide{RosterID: sides[1].RosterID, Points: sides[1].Points}
		} else {
			g.Away = GameSide{RosterID: -1}
		}
		games = append(games, g)
	}

	sort.Slice(games, func(i, j int) bool { return games[i].MatchupID < games[j].MatchupID })
	return games
}

// formatGameLine renders one game as an aligned " ABBR 121.06 -  88.42 ABBR"
// line, matching gamedaybot's fixed-width scoreboard style (abbreviations
// right-aligned to 4 characters, scores right-aligned to 6, so both line up
// in a Discord code block).
func formatGameLine(homeAbbrev string, homePoints, awayPoints float64, awayAbbrev string) string {
	return fmt.Sprintf("%4s %6.2f - %6.2f %s\n", homeAbbrev, homePoints, awayPoints, awayAbbrev)
}

// formatRecordLine renders one game as " ABBR (8-6) vs (7-7) ABBR", the
// abbreviated/record companion to the full-name matchup line, matching
// gamedaybot's Thursday preview style.
func formatRecordLine(homeAbbrev string, homeWins, homeLosses, awayWins, awayLosses int, awayAbbrev string) string {
	return fmt.Sprintf("%4s (%d-%d) vs (%d-%d) %s\n", homeAbbrev, homeWins, homeLosses, awayWins, awayLosses, awayAbbrev)
}

// Matchups renders the upcoming week's pairings twice: first as full team
// names, then in a compact abbreviation-and-record format, mirroring
// gamedaybot's Thursday matchup preview.
func (c *LeagueContext) Matchups(matchups []sleeper.Matchup) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}

	var names, records strings.Builder
	for _, g := range games {
		if g.Away.RosterID == -1 {
			line := fmt.Sprintf("%s: BYE\n", c.TeamName(g.Home.RosterID))
			names.WriteString(line)
			records.WriteString(line)
			continue
		}
		names.WriteString(fmt.Sprintf("%s vs %s\n", c.TeamName(g.Home.RosterID), c.TeamName(g.Away.RosterID)))

		homeWins, homeLosses := c.recordFor(g.Home.RosterID)
		awayWins, awayLosses := c.recordFor(g.Away.RosterID)
		records.WriteString(formatRecordLine(c.TeamAbbrev(g.Home.RosterID), homeWins, homeLosses,
			awayWins, awayLosses, c.TeamAbbrev(g.Away.RosterID)))
	}
	return names.String() + "\n" + records.String()
}

// recordFor returns a roster's win-loss record, or 0-0 for an unknown roster.
func (c *LeagueContext) recordFor(rosterID int) (wins, losses int) {
	r, ok := c.rosterByID[rosterID]
	if !ok {
		return 0, 0
	}
	return r.Settings.Wins, r.Settings.Losses
}
