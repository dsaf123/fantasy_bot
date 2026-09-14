package report

import (
	"fmt"
	"sort"
	"strings"

	"fantasy_bot/internal/sleeper"
)

// RecapDigest assembles the cross-week facts an AI weekly recap needs into
// plain text: current standings, win/loss streaks, season-long head-to-head
// series, the playoff race, and the week's worst lineup decision. It's fed
// to an LLM as the user prompt (see bot.ReportRecap) rather than posted
// directly - the digest is deliberately terse and data-only so the model
// does the storytelling.
//
// history must contain matchups for every week from 1 through throughWeek
// (see bot.Bot.matchupHistory); throughWeek is the most recently completed
// week, not the current in-progress one.
func (c *LeagueContext) RecapDigest(history map[int][]sleeper.Matchup, throughWeek int) string {
	rosterIDs := make([]int, 0, len(c.Rosters))
	for _, r := range c.Rosters {
		rosterIDs = append(rosterIDs, r.RosterID)
	}

	var out strings.Builder
	fmt.Fprintf(&out, "League: %s - Week %d complete\n\n", c.League.Name, throughWeek)

	out.WriteString(c.recapStandings())
	out.WriteString("\n")
	out.WriteString(recapStreaks(history, rosterIDs, throughWeek, c))
	out.WriteString("\n")
	out.WriteString(recapHeadToHead(history, throughWeek, c))
	out.WriteString("\n")
	out.WriteString(c.recapPlayoffRace())
	out.WriteString("\n")
	out.WriteString(c.recapLineupRegret(history[throughWeek]))

	return out.String()
}

// recapStandings renders current win-loss-tie records and points for, the
// same ordering as Standings, but including PF/PA since the recap benefits
// from more raw numbers than the Discord-facing standings report needs.
func (c *LeagueContext) recapStandings() string {
	rows := c.sortedStandings()

	var out strings.Builder
	out.WriteString("Standings:\n")
	for i, r := range rows {
		fmt.Fprintf(&out, "%d. %s (%s), %.1f PF, %.1f PA\n",
			i+1, c.TeamName(r.RosterID), recordString(r.Wins, r.Losses, r.Ties), r.PF, r.PA)
	}
	return out.String()
}

// sortedStandings ranks rosters by wins then points-for, matching Standings.
func (c *LeagueContext) sortedStandings() []sleeperRosterView {
	rows := make([]sleeperRosterView, 0, len(c.Rosters))
	for _, r := range c.Rosters {
		rows = append(rows, sleeperRosterView{
			RosterID: r.RosterID,
			Wins:     r.Settings.Wins,
			Losses:   r.Settings.Losses,
			Ties:     r.Settings.Ties,
			PF:       r.Settings.PointsFor(),
			PA:       r.Settings.PointsAgainst(),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Wins != rows[j].Wins {
			return rows[i].Wins > rows[j].Wins
		}
		return rows[i].PF > rows[j].PF
	})
	return rows
}

// weekResults maps rosterID -> "W"/"L"/"T" for one week, omitting rosters
// that had a bye (no game, so no effect on a streak).
func weekResults(matchups []sleeper.Matchup) map[int]string {
	results := make(map[int]string)
	for _, g := range pairGames(matchups) {
		if g.Away.RosterID == -1 {
			continue
		}
		switch g.Winner() {
		case g.Home.RosterID:
			results[g.Home.RosterID] = "W"
			results[g.Away.RosterID] = "L"
		case g.Away.RosterID:
			results[g.Away.RosterID] = "W"
			results[g.Home.RosterID] = "L"
		default:
			results[g.Home.RosterID] = "T"
			results[g.Away.RosterID] = "T"
		}
	}
	return results
}

// recapStreaks reports each team's current active win/loss streak, longest
// first, skipping teams with no streak (a tie in their most recent game, or
// no games played).
func recapStreaks(history map[int][]sleeper.Matchup, rosterIDs []int, throughWeek int, c *LeagueContext) string {
	byWeek := make(map[int]map[int]string, throughWeek)
	for week := 1; week <= throughWeek; week++ {
		byWeek[week] = weekResults(history[week])
	}

	type streak struct {
		rosterID int
		kind     string
		length   int
	}
	var streaks []streak
	for _, id := range rosterIDs {
		kind := ""
		length := 0
		for week := throughWeek; week >= 1; week-- {
			res, played := byWeek[week][id]
			if !played {
				continue // bye week: doesn't break or extend a streak
			}
			if kind == "" {
				kind = res
				length = 1
				continue
			}
			if res != kind {
				break
			}
			length++
		}
		if kind != "" && kind != "T" && length >= 2 {
			streaks = append(streaks, streak{id, kind, length})
		}
	}
	sort.Slice(streaks, func(i, j int) bool { return streaks[i].length > streaks[j].length })

	var out strings.Builder
	out.WriteString("Active streaks (2+ games, ties/byes don't count):\n")
	if len(streaks) == 0 {
		out.WriteString("None of note.\n")
		return out.String()
	}
	for _, s := range streaks {
		word := "win"
		if s.kind == "L" {
			word = "loss"
		}
		fmt.Fprintf(&out, "%s: %d-game %s streak\n", c.TeamName(s.rosterID), s.length, word)
	}
	return out.String()
}

// h2hKey is an unordered roster pair, always stored with the smaller roster
// ID first so the two sides of a matchup hash to the same key regardless of
// who was home/away.
type h2hKey struct {
	lo, hi int
}

type h2hRecord struct {
	loWins, hiWins, ties int
	games                []string // "Team A 120.4 - 110.2 Team B (Week 3)" lines, in order
}

// recapHeadToHead surfaces season series between teams that have played each
// other more than once (the common case, one meeting per pair, isn't a
// "series" worth narrating). This only fires in leagues with rematches -
// divisional schedules, or a season long enough to loop the round robin.
func recapHeadToHead(history map[int][]sleeper.Matchup, throughWeek int, c *LeagueContext) string {
	records := make(map[h2hKey]*h2hRecord)
	for week := 1; week <= throughWeek; week++ {
		for _, g := range pairGames(history[week]) {
			if g.Away.RosterID == -1 {
				continue
			}
			key := h2hKey{g.Home.RosterID, g.Away.RosterID}
			flipped := false
			if key.lo > key.hi {
				key.lo, key.hi = key.hi, key.lo
				flipped = true
			}
			rec, ok := records[key]
			if !ok {
				rec = &h2hRecord{}
				records[key] = rec
			}
			switch g.Winner() {
			case g.Home.RosterID:
				if flipped {
					rec.hiWins++
				} else {
					rec.loWins++
				}
			case g.Away.RosterID:
				if flipped {
					rec.loWins++
				} else {
					rec.hiWins++
				}
			default:
				rec.ties++
			}
			rec.games = append(rec.games, fmt.Sprintf("Week %d: %s %.1f - %.1f %s",
				week, c.TeamName(g.Home.RosterID), g.Home.Points, g.Away.Points, c.TeamName(g.Away.RosterID)))
		}
	}

	type series struct {
		key h2hKey
		rec *h2hRecord
	}
	var multi []series
	for k, r := range records {
		if len(r.games) >= 2 {
			multi = append(multi, series{k, r})
		}
	}
	sort.Slice(multi, func(i, j int) bool { return len(multi[i].rec.games) > len(multi[j].rec.games) })

	var out strings.Builder
	out.WriteString("Season head-to-head series (teams that have played twice or more):\n")
	if len(multi) == 0 {
		out.WriteString("None - every pairing has met at most once so far.\n")
		return out.String()
	}
	for _, s := range multi {
		fmt.Fprintf(&out, "%s vs %s: %d-%d-%d (%s)\n",
			c.TeamName(s.key.lo), c.TeamName(s.key.hi), s.rec.loWins, s.rec.hiWins, s.rec.ties,
			strings.Join(s.rec.games, "; "))
	}
	return out.String()
}

// recapPlayoffRace lists standings against the playoff cutoff line, with a
// simple "games back" figure for teams on the wrong side of it.
func (c *LeagueContext) recapPlayoffRace() string {
	rows := c.sortedStandings()

	playoffSpots := c.League.Settings.PlayoffTeams
	if playoffSpots <= 0 {
		playoffSpots = len(rows) / 2
	}
	if playoffSpots <= 0 || playoffSpots >= len(rows) {
		return "Playoff race: not enough teams/settings to compute a cutoff.\n"
	}

	cutoff := rows[playoffSpots-1]

	var out strings.Builder
	fmt.Fprintf(&out, "Playoff race (top %d make it):\n", playoffSpots)
	for i, r := range rows {
		status := "IN"
		if i >= playoffSpots {
			status = "OUT"
		}
		gb := (float64(cutoff.Wins-cutoff.Losses) - float64(r.Wins-r.Losses)) / 2
		gbNote := ""
		switch {
		case i == playoffSpots-1:
			gbNote = " (last team in)"
		case gb > 0:
			gbNote = fmt.Sprintf(" (%.1f games back)", gb)
		case gb < 0:
			gbNote = fmt.Sprintf(" (%.1f games clear)", -gb)
		}
		fmt.Fprintf(&out, "%d. %s (%s) - %s%s\n", i+1, c.TeamName(r.RosterID), recordString(r.Wins, r.Losses, r.Ties), status, gbNote)
	}
	return out.String()
}

// recapLineupRegret finds the week's clearest "this lineup call cost you the
// week" story: a team that lost, but whose best possible (optimal) lineup
// would have scored more than the winning opponent did. Falls back to
// whichever loser left the most points on their bench, if no lineup call
// actually would have flipped a result.
func (c *LeagueContext) recapLineupRegret(weekMatchups []sleeper.Matchup) string {
	matchupByRoster := make(map[int]sleeper.Matchup, len(weekMatchups))
	for _, m := range weekMatchups {
		matchupByRoster[m.RosterID] = m
	}

	type regret struct {
		rosterID     int
		opponentID   int
		actual       float64
		optimal      float64
		opponentPts  float64
		wouldHaveWon bool
	}
	var best *regret

	for _, g := range pairGames(weekMatchups) {
		if g.Away.RosterID == -1 || g.Winner() == -1 {
			continue // bye or tie: no "loss" to second-guess
		}
		loserID, opponentID, opponentPts := g.Away.RosterID, g.Home.RosterID, g.Home.Points
		if g.Winner() == g.Away.RosterID {
			loserID, opponentID, opponentPts = g.Home.RosterID, g.Away.RosterID, g.Away.Points
		}
		m, ok := matchupByRoster[loserID]
		if !ok {
			continue
		}
		optimal := c.optimalLineupPoints(m)
		wouldHaveWon := optimal > opponentPts
		cand := regret{rosterID: loserID, opponentID: opponentID, actual: m.Points, optimal: optimal, opponentPts: opponentPts, wouldHaveWon: wouldHaveWon}

		// Prefer a candidate that would have flipped the result over one
		// that wouldn't; within the same tier, prefer whoever left the most
		// points on the bench.
		switch {
		case best == nil:
			best = &cand
		case wouldHaveWon != best.wouldHaveWon:
			if wouldHaveWon {
				best = &cand
			}
		case (cand.optimal - cand.actual) > (best.optimal - best.actual):
			best = &cand
		}
	}

	if best == nil {
		return "This week's lineup regret: no completed matchups to check.\n"
	}

	benchPoints := best.optimal - best.actual
	if best.wouldHaveWon {
		return fmt.Sprintf(
			"This week's lineup regret: %s lost to %s (%.1f vs %.1f), but %s's optimal lineup would have scored %.1f - enough to win. That's %.1f points left on the bench in a loss.\n",
			c.TeamName(best.rosterID), c.TeamName(best.opponentID), best.actual, best.opponentPts,
			c.TeamName(best.rosterID), best.optimal, benchPoints)
	}
	return fmt.Sprintf(
		"This week's lineup regret: %s left the most on the bench in a loss - %.1f points (scored %.1f, optimal lineup was %.1f), losing to %s (%.1f). Wouldn't have flipped the result, but stung all the same.\n",
		c.TeamName(best.rosterID), benchPoints, best.actual, best.optimal, c.TeamName(best.opponentID), best.opponentPts)
}
