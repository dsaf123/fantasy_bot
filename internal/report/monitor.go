package report

import (
	"fmt"
	"strings"
)

// startersWatchStatuses are injury designations on an active starter worth
// flagging before kickoff.
var startersWatchStatuses = map[string]bool{
	"Questionable": true,
	"Doubtful":     true,
	"Out":          true,
}

// Monitor lists starters carrying a notable injury designation, plus any
// player parked in an IR roster slot, so managers can catch lineup problems
// before kickoff. A reserve player with no current injury designation is
// flagged as "Not IR eligible" - Sleeper (like most platforms) requires an
// active qualifying designation to legally occupy an IR slot, so a cleared
// player left there needs to be moved before the roster locks.
func (c *LeagueContext) Monitor() string {
	var b strings.Builder
	b.WriteString("Starting Players to Monitor\n")

	found := false
	first := true
	for _, roster := range c.Rosters {
		var lines []string

		for _, playerID := range roster.Starters {
			player, ok := c.Players[playerID]
			if !ok || !startersWatchStatuses[player.InjuryStatus] {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s %s - %s", player.Position, player.Name(), player.InjuryStatus))
		}

		for _, playerID := range roster.Reserve {
			player, ok := c.Players[playerID]
			if !ok {
				continue
			}
			status := reserveStatusLabel(player.InjuryStatus)
			lines = append(lines, fmt.Sprintf("%s %s - %s", player.Position, player.Name(), status))
		}

		if len(lines) == 0 {
			continue
		}
		found = true
		if !first {
			b.WriteString("\n")
		}
		first = false
		b.WriteString(c.TeamName(roster.RosterID) + ": \n")
		for _, line := range lines {
			b.WriteString(line + "\n")
		}
	}

	if !found {
		return "No starters flagged with an injury designation."
	}
	return b.String()
}

// reserveStatusLabel describes why a player is sitting in an IR roster slot.
// A player with no current injury designation no longer qualifies for IR and
// needs to be moved.
func reserveStatusLabel(injuryStatus string) string {
	if injuryStatus == "" {
		return "Not IR eligible"
	}
	if injuryStatus == "IR" {
		return "Injury Reserve"
	}
	return injuryStatus
}
