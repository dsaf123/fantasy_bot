package report

import (
	"fmt"

	"fantasy_bot/internal/sleeper"
)

// CloseScores lists games whose current point margin is within threshold.
// Sleeper's matchups endpoint doesn't expose per-game "in progress" state
// the way ESPN's box scores do, so this reports margin only, not whether
// the game is still live - fine for a Monday-evening "these are still
// worth sweating" post, less precise for an early-Sunday "still in play"
// alert.
func (c *LeagueContext) CloseScores(matchups []sleeper.Matchup, threshold float64) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}

	out := fmt.Sprintf("Close Scores (within %.1f points)\n", threshold)
	found := false
	for _, g := range games {
		if g.Away.RosterID == -1 {
			continue
		}
		if g.Margin() <= threshold {
			found = true
			out += formatGameLine(c.TeamAbbrev(g.Home.RosterID), g.Home.Points,
				g.Away.Points, c.TeamAbbrev(g.Away.RosterID))
		}
	}
	if !found {
		return "No close scores this week."
	}
	return out
}
