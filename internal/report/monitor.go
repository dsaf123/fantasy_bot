package report

import (
	"fmt"
	"strings"

	"fantasy_bot/internal/sleeper"
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

// emptySlotID is the placeholder Sleeper puts in a roster's starters list for
// a lineup slot with no player in it.
const emptySlotID = "0"

// monitorFooter nudges managers to act on the flagged starters.
const monitorFooter = "\nUpdate your lineup before kickoff - empty slots, byes and anyone ruled out score zero."

// NoMonitorIssues is posted when no starting lineup has anything to fix.
const NoMonitorIssues = "No lineup issues flagged for this week's starters."

// Monitor lists problems in each team's starting lineup so managers can fix
// them before kickoff: empty slots, starters with no NFL team (released),
// starters whose team is on bye this week, and starters carrying an injury
// designation. Bench and IR slots are ignored - nothing there costs points.
//
// schedule is the season's NFL schedule, used to spot byes: a team with no
// game this week is on bye. Pass nil (e.g. if the schedule couldn't be
// fetched) to skip bye detection rather than flag every starter.
func (c *LeagueContext) Monitor(schedule []sleeper.ScheduledGame) string {
	playingTeams := teamsPlayingInWeek(schedule, c.Week)

	var b strings.Builder
	b.WriteString("Starting Players to Monitor\n")

	found := false
	for _, roster := range c.Rosters {
		var lines []string
		for i, playerID := range roster.Starters {
			if line, ok := c.monitorLine(i, playerID, playingTeams); ok {
				lines = append(lines, line)
			}
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
		return NoMonitorIssues
	}
	b.WriteString(monitorFooter)
	return b.String()
}

// monitorLine describes the problem, if any, with the starter in lineup slot
// slot. A player with several problems gets the one that matters most: a
// released or bye-week player scores zero whatever their injury status.
func (c *LeagueContext) monitorLine(slot int, playerID string, playingTeams map[string]bool) (string, bool) {
	if playerID == "" || playerID == emptySlotID {
		return c.slotLabel(slot) + " - Empty slot", true
	}

	player, ok := c.Players[playerID]
	if !ok {
		return "", false
	}

	var problem string
	switch {
	case player.Team == "":
		problem = "No NFL team"
	case playingTeams != nil && !playingTeams[player.Team]:
		problem = "BYE"
	case startersWatchStatuses[player.InjuryStatus]:
		problem = injuryStatusLabel(player.InjuryStatus)
	default:
		return "", false
	}
	return fmt.Sprintf("%s %s - %s", player.Position, player.Name(), problem), true
}

// slotLabel names a starting lineup slot from the league's roster positions
// (whose leading entries line up with Roster.Starters), e.g. "FLEX" or
// "SUPER FLEX".
func (c *LeagueContext) slotLabel(slot int) string {
	if slot < len(c.League.RosterPositions) {
		return strings.ReplaceAll(c.League.RosterPositions[slot], "_", " ")
	}
	return "Lineup"
}

// teamsPlayingInWeek returns the NFL teams with a game scheduled in week, or
// nil when the schedule has no games for that week at all - most likely a
// missing or failed schedule fetch, which must not read as a league-wide bye.
func teamsPlayingInWeek(schedule []sleeper.ScheduledGame, week int) map[string]bool {
	var playing map[string]bool
	for _, g := range schedule {
		if g.Week != week {
			continue
		}
		if playing == nil {
			playing = make(map[string]bool)
		}
		playing[g.Home] = true
		playing[g.Away] = true
	}
	return playing
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
