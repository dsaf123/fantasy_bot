package report

import (
	"fmt"
	"strings"
)

// startersWatchStatuses are injury designations on a player in a starting
// lineup slot worth flagging before kickoff - either a risk to sit
// (Questionable/Doubtful) or a guaranteed zero (Out, IR, PUP, suspended).
var startersWatchStatuses = map[string]bool{
	"Questionable": true,
	"Doubtful":     true,
	"Out":          true,
	"IR":           true,
	"PUP":          true,
	"Sus":          true,
}

// monitorFooter nudges managers to act on the flagged starters.
const monitorFooter = "\nUpdate your lineup before kickoff if any of these players are ruled out."

// Monitor lists players in each team's starting lineup who carry an injury
// designation, so managers can swap them out before kickoff. Bench and IR
// slots are ignored - an injured player there doesn't cost any points.
func (c *LeagueContext) Monitor() string {
	var b strings.Builder
	b.WriteString("Starting Players to Monitor\n")

	found := false
	for _, roster := range c.Rosters {
		var lines []string
		for _, playerID := range roster.Starters {
			player, ok := c.Players[playerID]
			if !ok || !startersWatchStatuses[player.InjuryStatus] {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s - %s", player.Position, player.Name(), injuryStatusLabel(player.InjuryStatus)))
		}

		if len(lines) == 0 {
			continue
		}
		if found {
			b.WriteString("\n")
		}
		found = true
		b.WriteString(c.TeamName(roster.RosterID) + ": \n")
		for _, line := range lines {
			b.WriteString(line + "\n")
		}
	}

	if !found {
		return "No starters flagged with an injury designation."
	}
	b.WriteString(monitorFooter)
	return b.String()
}

// injuryStatusLabel expands Sleeper's abbreviated designations for display.
func injuryStatusLabel(injuryStatus string) string {
	switch injuryStatus {
	case "IR":
		return "Injury Reserve"
	case "Sus":
		return "Suspended"
	}
	return injuryStatus
}
