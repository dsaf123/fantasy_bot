package report

import (
	"fmt"
	"math"
	"sort"

	"fantasy_bot/internal/sleeper"
)

// weeklyAllPlayRate returns rosterID's all-play win rate for one week's
// scores: the fraction of the rest of the league it would have beaten had
// every team played every other team, with ties counting as half a win.
// This is the same "beat count" idea trophies.go uses for the single-week
// lucky/unlucky trophies, just expressed as a rate instead of a count.
func weeklyAllPlayRate(rosterID int, weekPoints map[int]float64, rosterIDs []int) float64 {
	mine := weekPoints[rosterID]
	wins, ties := 0.0, 0.0
	for _, other := range rosterIDs {
		if other == rosterID {
			continue
		}
		switch {
		case mine > weekPoints[other]:
			wins++
		case mine == weekPoints[other]:
			ties++
		}
	}
	return (wins + 0.5*ties) / float64(len(rosterIDs)-1)
}

// FortuneIndex scores each team's season-long schedule luck: for every week
// played, it compares the team's actual result (a win, loss, or tie) against
// its all-play win rate that week (see weeklyAllPlayRate) and sums the gap
// between the two across the season, scaled to a whole number. A team that
// keeps winning close games against the week's low scorers drifts positive;
// a team that keeps running into the week's best score despite scoring well
// itself drifts negative. This turns "I've had a brutal schedule" from an
// opinion into a ranked, signed number.
func (c *LeagueContext) FortuneIndex(history map[int][]sleeper.Matchup) string {
	fortune := make(map[int]float64, len(c.Rosters))
	for _, r := range c.Rosters {
		fortune[r.RosterID] = 0
	}

	for week := 1; week <= c.Week; week++ {
		games := pairGames(history[week])
		if len(games) == 0 {
			continue
		}

		weekPoints := make(map[int]float64, len(games)*2)
		var rosterIDs []int
		for _, g := range games {
			weekPoints[g.Home.RosterID] = g.Home.Points
			rosterIDs = append(rosterIDs, g.Home.RosterID)
			if g.Away.RosterID != -1 {
				weekPoints[g.Away.RosterID] = g.Away.Points
				rosterIDs = append(rosterIDs, g.Away.RosterID)
			}
		}
		if len(rosterIDs) < 2 {
			continue
		}

		for _, g := range games {
			if g.Away.RosterID == -1 {
				continue // bye: no result to compare against
			}
			homeActual := 0.5
			switch g.Winner() {
			case g.Home.RosterID:
				homeActual = 1
			case g.Away.RosterID:
				homeActual = 0
			}
			fortune[g.Home.RosterID] += (homeActual - weeklyAllPlayRate(g.Home.RosterID, weekPoints, rosterIDs)) * 100
			fortune[g.Away.RosterID] += ((1 - homeActual) - weeklyAllPlayRate(g.Away.RosterID, weekPoints, rosterIDs)) * 100
		}
	}

	type ranked struct {
		rosterID int
		points   float64
	}
	standings := make([]ranked, 0, len(fortune))
	for id, pts := range fortune {
		standings = append(standings, ranked{id, pts})
	}
	sort.Slice(standings, func(i, j int) bool { return standings[i].points > standings[j].points })

	out := fmt.Sprintf("Fortune Index - Week %d\n\n", c.Week)
	for i, r := range standings {
		out += fmt.Sprintf("%d. %s%s - %+d\n", i+1, fortuneEmoji(i, len(standings)), c.TeamName(r.rosterID), int(math.Round(r.points)))
	}
	return out
}

// fortuneEmoji marks the luckiest team (crowned), the second-luckiest
// (clovered), the unluckiest (skull), and the second-unluckiest (angry
// face), leaving the middle of the pack unmarked.
func fortuneEmoji(rank, teams int) string {
	switch {
	case rank == 0:
		return "👑 "
	case rank == 1:
		return "🍀 "
	case rank == teams-1:
		return "💀 "
	case rank == teams-2:
		return "😡 "
	default:
		return ""
	}
}
