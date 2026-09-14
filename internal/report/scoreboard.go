package report

import (
	"fmt"

	"fantasy_bot/internal/sleeper"
)

// ScoreboardShort renders each game as a one-line "Team A 101.2 - 98.4 Team B".
func (c *LeagueContext) ScoreboardShort(matchups []sleeper.Matchup) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}

	out := fmt.Sprintf("Week %d Scores\n", c.Week)
	for _, g := range games {
		if g.Away.RosterID == -1 {
			out += fmt.Sprintf("%s: BYE\n", c.TeamName(g.Home.RosterID))
			continue
		}
		out += formatGameLine(c.TeamAbbrev(g.Home.RosterID), g.Home.Points,
			g.Away.Points, c.TeamAbbrev(g.Away.RosterID))
	}
	return out
}

// WeekdayScoreboard renders the Friday/Monday recap: current scores followed
// by each matchup's approximate projected final score, mirroring
// gamedaybot's weekday "Score Update" post.
func (c *LeagueContext) WeekdayScoreboard(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}

	out := "Score Update\n"
	for _, g := range games {
		if g.Away.RosterID == -1 {
			out += fmt.Sprintf("%s: BYE\n", c.TeamName(g.Home.RosterID))
			continue
		}
		out += formatGameLine(c.TeamAbbrev(g.Home.RosterID), g.Home.Points,
			g.Away.Points, c.TeamAbbrev(g.Away.RosterID))
	}

	out += "\n" + c.projectedScoreboardBody(games, matchups, projections)
	return out
}

// ProjectedScoreboard renders just the approximate projected final score for
// each matchup (see projectedTeamPoints for how "approximate" is computed).
func (c *LeagueContext) ProjectedScoreboard(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}
	return c.projectedScoreboardBody(games, matchups, projections)
}

func (c *LeagueContext) projectedScoreboardBody(games []Game, matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) string {
	projByPlayer := make(map[string]float64, len(projections))
	scoringType := c.ScoringType()
	for _, p := range projections {
		projByPlayer[p.PlayerID] = p.Points(scoringType)
	}

	byRoster := make(map[int]sleeper.Matchup, len(matchups))
	for _, m := range matchups {
		byRoster[m.RosterID] = m
	}

	out := "Approximate Projected Scores\n"
	for _, g := range games {
		if g.Away.RosterID == -1 {
			continue
		}
		homeProj := projectedTeamPoints(byRoster[g.Home.RosterID], projByPlayer)
		awayProj := projectedTeamPoints(byRoster[g.Away.RosterID], projByPlayer)
		out += formatGameLine(c.TeamAbbrev(g.Home.RosterID), homeProj,
			awayProj, c.TeamAbbrev(g.Away.RosterID))
	}
	return out
}

// projectedTeamPoints approximates a team's final score as the sum, over its
// starting lineup, of each starter's actual points so far or - for starters
// who haven't put up any points yet, whether because their game hasn't
// started or they're still mid-game with a zero stat line - their projected
// points for the week. Sleeper's public matchup data doesn't say whether a
// given player's game has actually finished, so a starter who plays and
// scores exactly zero is indistinguishable from one who hasn't played yet;
// this is the same tradeoff gamedaybot makes, and the reason the report is
// labeled "Approximate".
func projectedTeamPoints(m sleeper.Matchup, projByPlayer map[string]float64) float64 {
	var total float64
	for i, playerID := range m.Starters {
		if i < len(m.StartersPoints) && m.StartersPoints[i] > 0 {
			total += m.StartersPoints[i]
			continue
		}
		total += projByPlayer[playerID]
	}
	return total
}
