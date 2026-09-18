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
func (c *LeagueContext) WeekdayScoreboard(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection, schedule []sleeper.ScheduledGame) string {
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

	out += "\n" + c.projectedScoreboardBody(games, matchups, projections, schedule)
	return out
}

// ProjectedScoreboard renders just the approximate projected final score for
// each matchup (see projectedTeamPoints for how "approximate" is computed).
func (c *LeagueContext) ProjectedScoreboard(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection, schedule []sleeper.ScheduledGame) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}
	return c.projectedScoreboardBody(games, matchups, projections, schedule)
}

func (c *LeagueContext) projectedScoreboardBody(games []Game, matchups []sleeper.Matchup, projections []sleeper.PlayerProjection, schedule []sleeper.ScheduledGame) string {
	projByPlayer := make(map[string]float64, len(projections))
	for _, p := range projections {
		projByPlayer[p.PlayerID] = p.PointsForSettings(c.League.ScoringSettings)
	}
	completedTeams := completedTeamsForWeek(schedule, c.Week)

	byRoster := make(map[int]sleeper.Matchup, len(matchups))
	for _, m := range matchups {
		byRoster[m.RosterID] = m
	}

	out := "Approximate Projected Scores\n"
	for _, g := range games {
		if g.Away.RosterID == -1 {
			continue
		}
		homeProj := c.projectedTeamPoints(byRoster[g.Home.RosterID], projByPlayer, completedTeams)
		awayProj := c.projectedTeamPoints(byRoster[g.Away.RosterID], projByPlayer, completedTeams)
		out += formatGameLine(c.TeamAbbrev(g.Home.RosterID), homeProj,
			awayProj, c.TeamAbbrev(g.Away.RosterID))
	}
	return out
}

// completedTeamsForWeek returns the set of NFL team abbreviations whose game
// for the given week is confirmed over, so projectedTeamPoints can tell a
// starter who truly scored zero from one who simply hasn't played yet.
func completedTeamsForWeek(schedule []sleeper.ScheduledGame, week int) map[string]bool {
	complete := make(map[string]bool)
	for _, g := range schedule {
		if g.Week == week && g.Final() {
			complete[g.Home] = true
			complete[g.Away] = true
		}
	}
	return complete
}

// projectedTeamPoints approximates a team's final score as the sum, over its
// starting lineup, of each starter's actual points or - for a starter whose
// NFL game isn't confirmed over - their projected points for the week.
// "Confirmed over" comes from completedTeams (see completedTeamsForWeek),
// built from the real NFL schedule, so a starter who plays and scores
// exactly zero is correctly read as final rather than mistaken for one who
// hasn't played yet. A starter on a bye or otherwise missing from the
// schedule instead falls back to gamedaybot's original heuristic - trust a
// nonzero actual, otherwise use the projection - which is why the report is
// still labeled "Approximate".
func (c *LeagueContext) projectedTeamPoints(m sleeper.Matchup, projByPlayer map[string]float64, completedTeams map[string]bool) float64 {
	var total float64
	for i, playerID := range m.Starters {
		var actual float64
		if i < len(m.StartersPoints) {
			actual = m.StartersPoints[i]
		}
		if actual != 0 || completedTeams[c.Players[playerID].Team] {
			total += actual
			continue
		}
		total += projByPlayer[playerID]
	}
	return total
}
