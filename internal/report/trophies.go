package report

import (
	"fmt"
	"sort"
	"strings"

	"fantasy_bot/internal/sleeper"
)

type weeklyScore struct {
	rosterID int
	points   float64
}

// Trophies renders the week's scoreboard followed by its awards, mirroring
// gamedaybot's "Trophies of the week" post: high/low score, biggest
// blowout, closest game, lucky/unlucky results, over/underachievers versus
// projection, and the best/worst manager by percentage of optimal lineup
// scored.
func (c *LeagueContext) Trophies(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) string {
	games := pairGames(matchups)
	if len(games) == 0 {
		return NoMatchupData
	}

	scores := sortedScores(matchups)
	if len(scores) == 0 {
		return NoMatchupData
	}
	high := scores[0]
	low := scores[len(scores)-1]

	closest, blowout := blowoutAndClosest(games)

	var out strings.Builder
	for _, g := range games {
		if g.Away.RosterID == -1 {
			out.WriteString(fmt.Sprintf("%s: BYE\n", c.TeamName(g.Home.RosterID)))
			continue
		}
		out.WriteString(formatGameLine(c.TeamAbbrev(g.Home.RosterID), g.Home.Points,
			g.Away.Points, c.TeamAbbrev(g.Away.RosterID)))
	}

	out.WriteString("\nTrophies of the week: \n\n")

	var blocks []string
	blocks = append(blocks, trophyBlock("👑️", "High score",
		fmt.Sprintf("%s with %.2f points", c.TeamName(high.rosterID), high.points)))
	blocks = append(blocks, trophyBlock("💩️", "Low score",
		fmt.Sprintf("%s with %.2f points", c.TeamName(low.rosterID), low.points)))
	if blowout != nil {
		winner, loser := blowout.Winner(), blowout.loserOf()
		blocks = append(blocks, trophyBlock("😱️", "Blow out",
			fmt.Sprintf("%s blew out %s by %.2f points", c.TeamName(winner), c.TeamName(loser), blowout.Margin())))
	}
	if closest != nil {
		winner, loser := closest.Winner(), closest.loserOf()
		blocks = append(blocks, trophyBlock("😅️", "Close win",
			fmt.Sprintf("%s barely beat %s by %.2f points", c.TeamName(winner), c.TeamName(loser), closest.Margin())))
	}

	blocks = append(blocks, c.luckTrophyBlocks(games, scores)...)
	blocks = append(blocks, c.achieverTrophyBlocks(matchups, projections)...)
	blocks = append(blocks, c.managerTrophyBlocks(matchups)...)

	out.WriteString(strings.Join(blocks, "\n"))

	return out.String()
}

// sortedScores builds each roster's weekly score, sorted highest first.
func sortedScores(matchups []sleeper.Matchup) []weeklyScore {
	scores := make([]weeklyScore, 0, len(matchups))
	for _, m := range matchups {
		scores = append(scores, weeklyScore{rosterID: m.RosterID, points: m.Points})
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].points > scores[j].points })
	return scores
}

// blowoutAndClosest finds the week's closest-margin and largest-margin
// decided games, skipping byes.
func blowoutAndClosest(games []Game) (closest, blowout *Game) {
	for i := range games {
		g := &games[i]
		if g.Away.RosterID == -1 {
			continue
		}
		if closest == nil || g.Margin() < closest.Margin() {
			closest = g
		}
		if blowout == nil || g.Margin() > blowout.Margin() {
			blowout = g
		}
	}
	return closest, blowout
}

// trophyBlock renders one trophy as gamedaybot's two-line, emoji-bracketed
// Discord entry: a title line and a content line, each ending in the
// trailing double space Discord's Markdown needs for a line break.
func trophyBlock(emoji, title, content string) string {
	return fmt.Sprintf("%s %s %s \n%s  \n", emoji, title, emoji, content)
}

// loserOf returns the roster ID of the non-winning side of a decided game.
func (g Game) loserOf() int {
	if g.Winner() == g.Home.RosterID {
		return g.Away.RosterID
	}
	return g.Home.RosterID
}

// luckWinners finds the "luckiest" winner (the team that won its game but
// would have lost to the most other teams' scores that week) and the
// "unluckiest" loser (lost its game but would have beaten the most other
// teams' scores). This only needs the week's scores, not projections.
func luckWinners(games []Game, scores []weeklyScore) (luckyID, luckyBeat int, haveLucky bool, unluckyID, unluckyBeat int, haveUnlucky bool) {
	pointsByRoster := make(map[int]float64, len(scores))
	for _, s := range scores {
		pointsByRoster[s.rosterID] = s.points
	}

	beatCount := func(rosterID int) int {
		mine := pointsByRoster[rosterID]
		count := 0
		for _, s := range scores {
			if s.rosterID != rosterID && mine > s.points {
				count++
			}
		}
		return count
	}

	luckyBeat, unluckyBeat = len(scores)+1, -1
	for _, g := range games {
		if g.Away.RosterID == -1 {
			continue
		}
		winner := g.Winner()
		if winner == -1 {
			continue
		}
		loser := g.loserOf()

		// Lucky: the winner who'd have lost to the most other teams' scores
		// that week, i.e. the smallest beat-count among winners.
		if b := beatCount(winner); !haveLucky || b < luckyBeat {
			luckyBeat = b
			luckyID = winner
			haveLucky = true
		}
		// Unlucky: the loser who'd have beaten the most other teams' scores
		// that week, i.e. the largest beat-count among losers.
		if b := beatCount(loser); !haveUnlucky || b > unluckyBeat {
			unluckyBeat = b
			unluckyID = loser
			haveUnlucky = true
		}
	}
	return luckyID, luckyBeat, haveLucky, unluckyID, unluckyBeat, haveUnlucky
}

// luckTrophyBlocks renders the Lucky/Unlucky trophy text blocks (see
// luckWinners for the winner-selection logic).
func (c *LeagueContext) luckTrophyBlocks(games []Game, scores []weeklyScore) []string {
	luckyID, luckyBeat, haveLucky, unluckyID, unluckyBeat, haveUnlucky := luckWinners(games, scores)

	var blocks []string
	others := len(scores) - 1
	if haveLucky {
		blocks = append(blocks, trophyBlock("🍀️", "Lucky",
			fmt.Sprintf("%s was %d-%d against the league, but still got the win",
				c.TeamName(luckyID), luckyBeat, others-luckyBeat)))
	}
	if haveUnlucky {
		blocks = append(blocks, trophyBlock("😡️", "Unlucky",
			fmt.Sprintf("%s was %d-%d against the league, but still took an L",
				c.TeamName(unluckyID), unluckyBeat, others-unluckyBeat)))
	}
	return blocks
}

// achieverWinners finds the team that beat its pre-week projection by the
// widest margin (overachiever) and the team that fell short of its
// projection by the widest margin (underachiever). A team's projection is
// the sum of its starters' projected points for the week.
func (c *LeagueContext) achieverWinners(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) (overID int, overDiff float64, haveOver bool, underID int, underDiff float64, haveUnder bool) {
	if len(projections) == 0 {
		return 0, 0, false, 0, 0, false
	}
	projByPlayer := make(map[string]float64, len(projections))
	for _, p := range projections {
		projByPlayer[p.PlayerID] = p.PointsForSettings(c.League.ScoringSettings)
	}

	for _, m := range matchups {
		var projected float64
		for _, playerID := range m.Starters {
			projected += projByPlayer[playerID]
		}
		diff := m.Points - projected
		if !haveOver || diff > overDiff {
			overDiff = diff
			overID = m.RosterID
			haveOver = true
		}
		if !haveUnder || diff < underDiff {
			underDiff = diff
			underID = m.RosterID
			haveUnder = true
		}
	}
	return overID, overDiff, haveOver, underID, underDiff, haveUnder
}

// achieverTrophyBlocks renders the Overachiever/Underachiever trophy text
// blocks (see achieverWinners for the winner-selection logic).
func (c *LeagueContext) achieverTrophyBlocks(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) []string {
	overID, overDiff, haveOver, underID, underDiff, haveUnder := c.achieverWinners(matchups, projections)

	var blocks []string
	if haveOver {
		blocks = append(blocks, trophyBlock("📈️", "Overachiever",
			fmt.Sprintf("%s was %.2f points over their projection", c.TeamName(overID), overDiff)))
	}
	if haveUnder {
		blocks = append(blocks, trophyBlock("📉️", "Underachiever",
			fmt.Sprintf("%s was %.2f points under their projection", c.TeamName(underID), -underDiff)))
	}
	return blocks
}

// managerWinners finds the best and worst manager of the week by the
// percentage of each team's optimal (highest-scoring legal) lineup they
// actually started, mirroring gamedaybot's "Best/Worst Manager" trophies.
func (c *LeagueContext) managerWinners(matchups []sleeper.Matchup) (bestID int, bestPct float64, haveBest bool, worstID int, worstPct, worstBenchLeft float64, haveWorst bool) {
	for _, m := range matchups {
		optimal := c.optimalLineupPoints(m)
		if optimal <= 0 {
			continue
		}
		pct := m.Points / optimal * 100
		if !haveBest || pct > bestPct {
			bestPct = pct
			bestID = m.RosterID
			haveBest = true
		}
		if !haveWorst || pct < worstPct {
			worstPct = pct
			worstID = m.RosterID
			worstBenchLeft = optimal - m.Points
			haveWorst = true
		}
	}
	return bestID, bestPct, haveBest, worstID, worstPct, worstBenchLeft, haveWorst
}

// managerTrophyBlocks renders the Best/Worst Manager trophy text blocks
// (see managerWinners for the winner-selection logic).
func (c *LeagueContext) managerTrophyBlocks(matchups []sleeper.Matchup) []string {
	bestID, bestPct, haveBest, worstID, worstPct, worstBenchLeft, haveWorst := c.managerWinners(matchups)

	var blocks []string
	if haveBest {
		blocks = append(blocks, trophyBlock("🤖️", "Best Manager",
			fmt.Sprintf("%s scored %.2f%% of their optimal score!", c.TeamName(bestID), bestPct)))
	}
	if haveWorst {
		blocks = append(blocks, trophyBlock("🤡️", "Worst Manager",
			fmt.Sprintf("%s left %.2f points on their bench. Only scoring %.2f%% of their optimal score.",
				c.TeamName(worstID), worstBenchLeft, worstPct)))
	}
	return blocks
}

// trophyWinner pairs a trophy category's emoji (matching Trophies' output)
// with the roster ID that won it for one week.
type trophyWinner struct {
	emoji    string
	rosterID int
}

// weekTrophyWinners computes the winning roster ID for each of the week's
// trophy categories, mirroring Trophies' own winner-selection exactly but
// without building any display text. A category is omitted if it has no
// winner that week (e.g. no projections, or a bye leaving no decided
// games). Used to tally season totals - see TrophyCase.
func (c *LeagueContext) weekTrophyWinners(matchups []sleeper.Matchup, projections []sleeper.PlayerProjection) []trophyWinner {
	games := pairGames(matchups)
	if len(games) == 0 {
		return nil
	}
	scores := sortedScores(matchups)
	if len(scores) == 0 {
		return nil
	}

	winners := []trophyWinner{
		{"👑️", scores[0].rosterID},
		{"💩️", scores[len(scores)-1].rosterID},
	}

	closest, blowout := blowoutAndClosest(games)
	if blowout != nil {
		winners = append(winners, trophyWinner{"😱️", blowout.Winner()})
	}
	if closest != nil {
		winners = append(winners, trophyWinner{"😅️", closest.Winner()})
	}

	luckyID, _, haveLucky, unluckyID, _, haveUnlucky := luckWinners(games, scores)
	if haveLucky {
		winners = append(winners, trophyWinner{"🍀️", luckyID})
	}
	if haveUnlucky {
		winners = append(winners, trophyWinner{"😡️", unluckyID})
	}

	overID, _, haveOver, underID, _, haveUnder := c.achieverWinners(matchups, projections)
	if haveOver {
		winners = append(winners, trophyWinner{"📈️", overID})
	}
	if haveUnder {
		winners = append(winners, trophyWinner{"📉️", underID})
	}

	bestID, _, haveBest, worstID, _, _, haveWorst := c.managerWinners(matchups)
	if haveBest {
		winners = append(winners, trophyWinner{"🤖️", bestID})
	}
	if haveWorst {
		winners = append(winners, trophyWinner{"🤡️", worstID})
	}

	return winners
}

// flexEligible returns the player positions that may fill a roster slot.
// Single-position slots (QB, RB, WR, TE, K, DEF, ...) are eligible only for
// themselves.
func flexEligible(slot string) []string {
	switch slot {
	case "FLEX":
		return []string{"RB", "WR", "TE"}
	case "WRRB_FLEX":
		return []string{"RB", "WR"}
	case "REC_FLEX":
		return []string{"WR", "TE"}
	case "SUPER_FLEX":
		return []string{"QB", "RB", "WR", "TE"}
	default:
		return []string{slot}
	}
}

// startingSlots returns a league's non-bench roster slots (drops BN, IR,
// and taxi-squad slots, none of which count toward the optimal lineup).
func startingSlots(positions []string) []string {
	slots := make([]string, 0, len(positions))
	for _, pos := range positions {
		if pos == "BN" || pos == "IR" || pos == "TAXI" {
			continue
		}
		slots = append(slots, pos)
	}
	return slots
}

// optimalLineupPoints estimates the highest score a roster could have
// posted that week: it greedily fills the league's starting slots, most
// restrictive positions first, so a wide flex slot only takes a player once
// every single-position slot that could have used them is already filled.
// This greedy fill isn't a provably optimal assignment in every possible
// roster, but matches the approach gamedaybot uses and is exact for the
// standard FLEX/SUPER_FLEX shapes.
func (c *LeagueContext) optimalLineupPoints(m sleeper.Matchup) float64 {
	type candidate struct {
		playerID string
		position string
		points   float64
	}
	pool := make([]candidate, 0, len(m.Players))
	for _, pid := range m.Players {
		p, ok := c.Players[pid]
		if !ok {
			continue
		}
		pool = append(pool, candidate{playerID: pid, position: p.Position, points: m.PlayersPoints[pid]})
	}

	slots := startingSlots(c.League.RosterPositions)
	sort.Slice(slots, func(i, j int) bool {
		return len(flexEligible(slots[i])) < len(flexEligible(slots[j]))
	})

	used := make(map[string]bool, len(pool))
	var total float64
	for _, slot := range slots {
		eligible := flexEligible(slot)
		best := -1
		for i, cand := range pool {
			if used[cand.playerID] || !containsStr(eligible, cand.position) {
				continue
			}
			if best == -1 || cand.points > pool[best].points {
				best = i
			}
		}
		if best == -1 {
			continue
		}
		used[pool[best].playerID] = true
		total += pool[best].points
	}
	return total
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
