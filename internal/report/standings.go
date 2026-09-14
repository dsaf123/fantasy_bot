package report

import (
	"fmt"
	"sort"
)

// Standings renders current win-loss-tie records sorted by wins then points
// for, matching how Sleeper's own standings tab breaks ties.
func (c *LeagueContext) Standings() string {
	var rosters []sleeperRosterView
	for _, r := range c.Rosters {
		rosters = append(rosters, sleeperRosterView{
			RosterID: r.RosterID,
			Wins:     r.Settings.Wins,
			Losses:   r.Settings.Losses,
			Ties:     r.Settings.Ties,
			PF:       r.Settings.PointsFor(),
			PA:       r.Settings.PointsAgainst(),
		})
	}

	sort.Slice(rosters, func(i, j int) bool {
		if rosters[i].Wins != rosters[j].Wins {
			return rosters[i].Wins > rosters[j].Wins
		}
		return rosters[i].PF > rosters[j].PF
	})

	out := "Current Standings\n"
	for i, r := range rosters {
		out += fmt.Sprintf("%2d: (%s) %s\n", i+1, recordString(r.Wins, r.Losses, r.Ties), c.TeamName(r.RosterID))
	}
	return out
}

// recordString renders a win-loss(-tie) record, omitting the ties segment
// entirely for the common case of a league with no tied games.
func recordString(wins, losses, ties int) string {
	if ties > 0 {
		return fmt.Sprintf("%d-%d-%d", wins, losses, ties)
	}
	return fmt.Sprintf("%d-%d", wins, losses)
}

type sleeperRosterView struct {
	RosterID int
	Wins     int
	Losses   int
	Ties     int
	PF       float64
	PA       float64
}
