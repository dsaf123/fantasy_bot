package report

import (
	"fmt"
	"sort"
	"strings"
)

// RosterDigest lists every team's manager and current roster (starters,
// bench, and IR/reserve, each with position, NFL team, and any injury
// designation) as plain text. Like RecapDigest it's meant as LLM input -
// here for answering free-form questions when someone @mentions the bot
// (see bot.Bot.Answer) - rather than for posting directly.
func (c *LeagueContext) RosterDigest() string {
	rosters := make([]int, 0, len(c.Rosters))
	for _, r := range c.Rosters {
		rosters = append(rosters, r.RosterID)
	}
	sort.Ints(rosters)

	var out strings.Builder
	out.WriteString("Rosters:\n")
	for _, id := range rosters {
		r := c.rosterByID[id]
		manager := "no manager"
		if u, ok := c.userByID[r.OwnerID]; ok && u.DisplayName != "" {
			manager = "manager " + u.DisplayName
		}
		fmt.Fprintf(&out, "\n%s (%s, %s)\n", c.TeamName(id), c.TeamAbbrev(id), manager)

		starters := make(map[string]bool, len(r.Starters))
		var starterNames []string
		for _, pid := range r.Starters {
			if pid == "" || pid == "0" { // Sleeper's marker for an empty lineup slot
				starterNames = append(starterNames, "(empty slot)")
				continue
			}
			starters[pid] = true
			starterNames = append(starterNames, c.playerLabel(pid))
		}

		reserve := make(map[string]bool, len(r.Reserve))
		var reserveNames []string
		for _, pid := range r.Reserve {
			reserve[pid] = true
			reserveNames = append(reserveNames, c.playerLabel(pid))
		}

		var benchNames []string
		for _, pid := range r.Players {
			if !starters[pid] && !reserve[pid] {
				benchNames = append(benchNames, c.playerLabel(pid))
			}
		}

		fmt.Fprintf(&out, "  Starters: %s\n", joinOrNone(starterNames))
		fmt.Fprintf(&out, "  Bench: %s\n", joinOrNone(benchNames))
		if len(reserveNames) > 0 {
			fmt.Fprintf(&out, "  IR: %s\n", strings.Join(reserveNames, "; "))
		}
	}
	return out.String()
}

// playerLabel renders a player as "Name (POS, TEAM, Injury)", omitting any
// part Sleeper doesn't have.
func (c *LeagueContext) playerLabel(playerID string) string {
	p, ok := c.Players[playerID]
	if !ok {
		return playerID
	}
	var parts []string
	for _, s := range []string{p.Position, p.Team, p.InjuryStatus} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return p.Name()
	}
	return fmt.Sprintf("%s (%s)", p.Name(), strings.Join(parts, ", "))
}

func joinOrNone(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, "; ")
}
