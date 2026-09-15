package report

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fantasy_bot/internal/sleeper"
)

// allPlayRecord tallies one roster's win-loss-tie record across every
// "all-play" comparison WinMatrix makes: one comparison per week for every
// other roster that played that week.
type allPlayRecord struct {
	rosterID           int
	wins, losses, ties int
	pointsFor          float64
}

// WinMatrix renders the league's standings as if every team had played
// every other team every week, instead of just its actual schedule: for
// each week in history, a roster earns a win against every other roster it
// outscored that week and a loss against every roster that outscored it,
// summed across the whole season. This is the same all-play comparison
// FortuneIndex uses (see weeklyAllPlayRate) but tallied as a counted
// record across the season instead of a single per-week rate, so it
// surfaces how much of a team's actual record comes from a favorable
// schedule versus just scoring well.
func (c *LeagueContext) WinMatrix(history map[int][]sleeper.Matchup) string {
	if len(history) == 0 {
		return NoMatchupData
	}

	records := make(map[int]*allPlayRecord, len(c.Rosters))
	for _, r := range c.Rosters {
		records[r.RosterID] = &allPlayRecord{rosterID: r.RosterID}
	}

	for _, matchups := range history {
		weekPoints := make(map[int]float64, len(matchups))
		rosterIDs := make([]int, 0, len(matchups))
		for _, m := range matchups {
			weekPoints[m.RosterID] = m.Points
			rosterIDs = append(rosterIDs, m.RosterID)
		}

		for _, id := range rosterIDs {
			rec, ok := records[id]
			if !ok {
				continue
			}
			rec.pointsFor += weekPoints[id]
			for _, other := range rosterIDs {
				if other == id {
					continue
				}
				switch {
				case weekPoints[id] > weekPoints[other]:
					rec.wins++
				case weekPoints[id] < weekPoints[other]:
					rec.losses++
				default:
					rec.ties++
				}
			}
		}
	}

	sorted := make([]*allPlayRecord, 0, len(records))
	for _, rec := range records {
		sorted = append(sorted, rec)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].wins != sorted[j].wins {
			return sorted[i].wins > sorted[j].wins
		}
		return sorted[i].pointsFor > sorted[j].pointsFor
	})

	// Column widths are computed from the data instead of hardcoded so the
	// win/loss digits - and the dash between them - line up whether records
	// top out at "9-5" or "112-38", and so a 10+ team league's two-digit
	// ranks don't push the rest of the row out of alignment.
	nameWidth, winWidth, lossWidth := 0, 1, 1
	for _, rec := range sorted {
		if w := len(c.TeamName(rec.rosterID)); w > nameWidth {
			nameWidth = w
		}
		if w := len(strconv.Itoa(rec.wins)); w > winWidth {
			winWidth = w
		}
		if w := len(strconv.Itoa(rec.losses)); w > lossWidth {
			lossWidth = w
		}
	}
	rankWidth := len(strconv.Itoa(len(sorted)))

	var table strings.Builder
	for i, rec := range sorted {
		record := fmt.Sprintf("%*d-%-*d", winWidth, rec.wins, lossWidth, rec.losses)
		if rec.ties > 0 {
			record += fmt.Sprintf("-%d", rec.ties)
		}
		fmt.Fprintf(&table, "%*d. %-*s    (%s)\n", rankWidth, i+1, nameWidth, c.TeamName(rec.rosterID), record)
	}

	return "**Standings if everyone played every team every week**\n```\n" + table.String() + "```"
}
