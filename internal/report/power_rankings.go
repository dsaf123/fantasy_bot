package report

import (
	"fmt"
	"math"
	"sort"

	"fantasy_bot/internal/sleeper"
)

// cumStats is a roster's accumulated win/loss/tie record and points-for
// through some week, used to reconstruct what the power rankings looked
// like at a past point in the season (Sleeper doesn't expose that history
// directly, only the current live totals in RosterSettings).
type cumStats struct {
	wins, losses, ties int
	pf                 float64
}

// teamScore is one roster's power-ranking score (0-100) at some week.
type teamScore struct {
	rosterID int
	score    float64
}

// powerScore scores a win-loss record and points-for on a 0-100 scale,
// weighting win percentage and points-for (normalized against the best PF
// among the group being compared) 50/50. This is a deliberately simple
// stand-in for gamedaybot's ESPN power rankings, which factor in strength
// of schedule and margin of victory via a win-probability matrix across
// every possible pairing of teams and weeks.
func powerScore(wins, losses, ties int, pf, maxPF float64) float64 {
	games := wins + losses + ties
	winPct := 0.0
	if games > 0 {
		winPct = (float64(wins) + 0.5*float64(ties)) / float64(games)
	}
	pfNorm := 0.0
	if maxPF > 0 {
		pfNorm = pf / maxPF
	}
	return 100 * (0.5*winPct + 0.5*pfNorm)
}

// cumulativeStatsThroughWeek sums each roster's win/loss/tie/PF results
// from weekly matchups across weeks 1..upToWeek. weeklyMatchups is keyed by
// week number; a missing week (e.g. not fetched, or not yet played) simply
// contributes nothing. A bye (no opponent) counts its points but no
// win/loss/tie.
func cumulativeStatsThroughWeek(weeklyMatchups map[int][]sleeper.Matchup, upToWeek int) map[int]cumStats {
	cum := make(map[int]cumStats)
	for week := 1; week <= upToWeek; week++ {
		for _, g := range pairGames(weeklyMatchups[week]) {
			cum[g.Home.RosterID] = addGameResult(cum[g.Home.RosterID], g.Home.Points, g.Away.Points, g.Away.RosterID != -1)
			if g.Away.RosterID != -1 {
				cum[g.Away.RosterID] = addGameResult(cum[g.Away.RosterID], g.Away.Points, g.Home.Points, true)
			}
		}
	}
	return cum
}

func addGameResult(s cumStats, pointsFor, pointsAgainst float64, hasOpponent bool) cumStats {
	s.pf += pointsFor
	if !hasOpponent {
		return s
	}
	switch {
	case pointsFor > pointsAgainst:
		s.wins++
	case pointsFor < pointsAgainst:
		s.losses++
	default:
		s.ties++
	}
	return s
}

// scoresFromStats computes each roster's power-ranking score from a set of
// cumulative stats, normalizing points-for against the best PF among
// rosterIDs, then rescales scores so the top team lands at exactly 100 (see
// rescaleToTop).
func scoresFromStats(rosterIDs []int, stats map[int]cumStats) []teamScore {
	var maxPF float64
	for _, id := range rosterIDs {
		if s := stats[id]; s.pf > maxPF {
			maxPF = s.pf
		}
	}
	out := make([]teamScore, 0, len(rosterIDs))
	for _, id := range rosterIDs {
		s := stats[id]
		out = append(out, teamScore{rosterID: id, score: powerScore(s.wins, s.losses, s.ties, s.pf, maxPF)})
	}
	rescaleToTop(out)
	return out
}

// rescaleToTop scales scores in place so the highest-scoring team lands at
// exactly 100, with every other team scaled proportionally against it. This
// is separate from powerScore's own 0-100 scale, since win% and PF-normalized
// scoring rarely both peak for the same team, so the raw top score is usually
// short of 100.
func rescaleToTop(scores []teamScore) {
	var maxScore float64
	for _, s := range scores {
		if s.score > maxScore {
			maxScore = s.score
		}
	}
	if maxScore <= 0 {
		return
	}
	for i := range scores {
		scores[i].score = scores[i].score / maxScore * 100
	}
}

// playoffOdds is a simple, non-simulated heuristic for a team's chance of
// making the playoffs: it measures how many spots a team's current rank is
// inside or outside the playoff cutoff, then squashes that through a
// logistic curve whose steepness grows as fewer weeks remain in the regular
// season (a 2-spot gap means little in week 3, a lot in the final week).
// It intentionally does not simulate the remaining schedule game-by-game.
func playoffOdds(rank, playoffSpots, teams, weeksRemaining int) float64 {
	if playoffSpots <= 0 || teams <= 0 {
		return 0
	}
	if playoffSpots >= teams {
		return 100
	}
	spotsFromCutoff := float64(playoffSpots-rank) + 0.5
	steepness := 2.5 / float64(weeksRemaining+1)
	return 100 / (1 + math.Exp(-steepness*spotsFromCutoff))
}

// PowerRankings scores and ranks every team for the current week (see
// powerScore), alongside each team's estimated playoff odds (see
// playoffOdds) and its week-over-week trend against its reconstructed score
// from the previous week (see cumulativeStatsThroughWeek). history supplies
// the weekly matchup data needed to reconstruct that previous-week score;
// pass the same data to PowerRankingsChart to also render the season trend.
func (c *LeagueContext) PowerRankings(history map[int][]sleeper.Matchup) string {
	type ranked struct {
		rosterID int
		score    float64
	}

	rosterIDs := make([]int, 0, len(c.Rosters))
	var maxPF float64
	for _, r := range c.Rosters {
		rosterIDs = append(rosterIDs, r.RosterID)
		if pf := r.Settings.PointsFor(); pf > maxPF {
			maxPF = pf
		}
	}

	current := make([]ranked, 0, len(c.Rosters))
	for _, r := range c.Rosters {
		current = append(current, ranked{
			rosterID: r.RosterID,
			score:    powerScore(r.Settings.Wins, r.Settings.Losses, r.Settings.Ties, r.Settings.PointsFor(), maxPF),
		})
	}
	var maxScore float64
	for _, r := range current {
		if r.score > maxScore {
			maxScore = r.score
		}
	}
	if maxScore > 0 {
		for i := range current {
			current[i].score = current[i].score / maxScore * 100
		}
	}
	sort.Slice(current, func(i, j int) bool { return current[i].score > current[j].score })

	var prevScores map[int]float64
	if c.Week > 1 {
		prevStats := cumulativeStatsThroughWeek(history, c.Week-1)
		prevScores = make(map[int]float64, len(rosterIDs))
		for _, ts := range scoresFromStats(rosterIDs, prevStats) {
			prevScores[ts.rosterID] = ts.score
		}
	}

	playoffSpots := c.League.Settings.PlayoffTeams
	if playoffSpots <= 0 {
		playoffSpots = len(c.Rosters) / 2
	}
	weeksRemaining := 0
	if c.League.Settings.PlayoffWeekStart > 0 {
		weeksRemaining = c.League.Settings.PlayoffWeekStart - 1 - c.Week
		if weeksRemaining < 0 {
			weeksRemaining = 0
		}
	}

	out := "Power Rankings (Playoff %)\n\n"
	for i, r := range current {
		rank := i + 1
		playoffPct := playoffOdds(rank, playoffSpots, len(current), weeksRemaining)

		trend := ""
		if prev, ok := prevScores[r.rosterID]; ok {
			delta := r.score - prev
			icon := "🟢"
			if delta < 0 {
				icon = "🔻"
			}
			pctChange := 0.0
			if prev != 0 {
				pctChange = math.Abs(delta) / math.Abs(prev) * 100
			}
			trend = fmt.Sprintf("[%s %4.1f%%] ", icon, pctChange)
		}

		out += fmt.Sprintf("%5.2f %s(%5.1f%%) - %s\n", r.score, trend, playoffPct, c.TeamAbbrev(r.rosterID))
	}
	return out
}
