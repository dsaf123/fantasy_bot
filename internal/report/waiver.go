package report

import (
	"fmt"
	"sort"
	"strings"

	"fantasy_bot/internal/sleeper"
)

// WaiverReport lists completed waiver/free-agent adds and drops for the
// week, grouped by team, with FAAB bid amounts and the next-highest
// competing bid where the league uses a waiver budget.
func (c *LeagueContext) WaiverReport(transactions []sleeper.Transaction) string {
	var completed []sleeper.Transaction
	var failedWaivers []sleeper.Transaction
	for _, t := range transactions {
		switch {
		case t.Status == "complete" && (t.Type == "waiver" || t.Type == "free_agent"):
			completed = append(completed, t)
		case t.Status == "failed" && t.Type == "waiver":
			failedWaivers = append(failedWaivers, t)
		}
	}
	if len(completed) == 0 {
		return "No waiver or free agent moves to report."
	}

	sort.Slice(completed, func(i, j int) bool { return completed[i].StatusUpdated < completed[j].StatusUpdated })

	var teamOrder []int
	seen := make(map[int]bool)
	byTeam := make(map[int][]string)
	for _, t := range completed {
		rosterID := 0
		if len(t.RosterIDs) > 0 {
			rosterID = t.RosterIDs[0]
		}
		if !seen[rosterID] {
			seen[rosterID] = true
			teamOrder = append(teamOrder, rosterID)
		}

		for _, playerID := range sortedKeys(t.Adds) {
			line := fmt.Sprintf("ADDED %s - %s", c.playerPosition(playerID), c.PlayerName(playerID))
			if t.Type == "waiver" && t.Settings.WaiverBid > 0 {
				line += fmt.Sprintf(" ($%d%s)", t.Settings.WaiverBid, c.competingBidNote(playerID, rosterID, t.Settings.WaiverBid, failedWaivers))
			}
			byTeam[rosterID] = append(byTeam[rosterID], line)
		}
		for _, playerID := range sortedKeys(t.Drops) {
			line := fmt.Sprintf("DROPPED %s - %s", c.playerPosition(playerID), c.PlayerName(playerID))
			byTeam[rosterID] = append(byTeam[rosterID], line)
		}
	}

	var out strings.Builder
	for i, rosterID := range teamOrder {
		if i > 0 {
			out.WriteString("\n")
		}
		out.WriteString(c.TeamName(rosterID) + "\n")
		for _, line := range byTeam[rosterID] {
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

// playerPosition resolves a Sleeper player ID to its position abbreviation.
func (c *LeagueContext) playerPosition(playerID string) string {
	if p, ok := c.Players[playerID]; ok && p.Position != "" {
		return p.Position
	}
	return "?"
}

// competingBidNote describes the next-highest losing bid for a player, e.g.
// ", Bench Mob outbid by $1" or ", TIED with Almost There" (when the runner-up
// bid the same amount but lost on waiver priority). Returns "" if no other
// roster bid on the player.
func (c *LeagueContext) competingBidNote(playerID string, winningRosterID, winningBid int, failedWaivers []sleeper.Transaction) string {
	bestBid := -1
	bestRosterID := 0
	for _, t := range failedWaivers {
		if _, ok := t.Adds[playerID]; !ok {
			continue
		}
		rosterID := 0
		if len(t.RosterIDs) > 0 {
			rosterID = t.RosterIDs[0]
		}
		if rosterID == winningRosterID {
			continue
		}
		if t.Settings.WaiverBid > bestBid || (t.Settings.WaiverBid == bestBid && rosterID < bestRosterID) {
			bestBid = t.Settings.WaiverBid
			bestRosterID = rosterID
		}
	}
	if bestBid < 0 {
		return ""
	}
	if bestBid == winningBid {
		return fmt.Sprintf(", TIED with %s", c.TeamName(bestRosterID))
	}
	return fmt.Sprintf(", %s outbid by $%d", c.TeamName(bestRosterID), winningBid-bestBid)
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
