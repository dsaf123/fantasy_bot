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

const (
	// MonitorHeader titles the Sunday-morning monitor report.
	MonitorHeader = "Starting Players to Monitor"
	// MonitorUpdateHeader titles the pre-kickoff follow-up, posted only when
	// something new turned up since the morning report.
	MonitorUpdateHeader = "Starting Players to Monitor - Pre-Kickoff Update"
)

// monitorFooter nudges managers to act on the flagged starters.
const monitorFooter = "\nUpdate your lineup before kickoff - empty slots, byes and anyone ruled out score zero."

// NoMonitorIssues is posted when no starting lineup has anything to fix.
const NoMonitorIssues = "No lineup issues flagged for this week's starters."

// MonitorTeam is one team's flagged starting lineup problems, one display
// line each (e.g. "WR Tee Higgins - Questionable").
type MonitorTeam struct {
	RosterID int
	OwnerID  string
	Lines    []string
}

// Monitor renders MonitorFlags as the Sunday-morning report.
func (c *LeagueContext) Monitor(schedule []sleeper.ScheduledGame) string {
	return c.FormatMonitor(MonitorHeader, c.MonitorFlags(schedule))
}

// MonitorFlags finds problems in each team's starting lineup that a manager
// can still fix before kickoff: empty slots, starters with no NFL team
// (released), starters whose team is on bye this week, and starters
// carrying an injury designation. Bench and IR slots are ignored - nothing
// there costs points - and so are starters whose game has already kicked
// off, since they're locked into the lineup. Teams with nothing flagged
// are left out.
//
// schedule is the season's NFL schedule, used to spot byes (a team with no
// game this week) and games already under way. Pass nil (e.g. if the
// schedule couldn't be fetched) to skip both checks rather than flag every
// starter as on bye.
func (c *LeagueContext) MonitorFlags(schedule []sleeper.ScheduledGame) []MonitorTeam {
	games := weekGames(schedule, c.Week)

	var teams []MonitorTeam
	for _, roster := range c.Rosters {
		var lines []string
		for i, playerID := range roster.Starters {
			if line, ok := c.monitorLine(i, playerID, games); ok {
				lines = append(lines, line)
			}
		}
		if len(lines) > 0 {
			teams = append(teams, MonitorTeam{RosterID: roster.RosterID, OwnerID: roster.OwnerID, Lines: lines})
		}
	}
	return teams
}

// FormatMonitor renders flagged teams as a report titled header, or
// NoMonitorIssues when there are none.
func (c *LeagueContext) FormatMonitor(header string, teams []MonitorTeam) string {
	if len(teams) == 0 {
		return NoMonitorIssues
	}

	var b strings.Builder
	b.WriteString(header + "\n")
	for i, team := range teams {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(c.TeamName(team.RosterID) + ": \n")
		for _, line := range team.Lines {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(monitorFooter)
	return b.String()
}

// NewMonitorFlags returns the teams in current with at least one line not
// already flagged for that team in previous, so a follow-up report can stay
// quiet unless there's something new to act on (a fresh designation, or a
// Questionable starter downgraded to Out). Lines that merely dropped off -
// a player upgraded or swapped out - don't count as new.
func NewMonitorFlags(previous, current []MonitorTeam) []MonitorTeam {
	seen := make(map[int]map[string]bool, len(previous))
	for _, team := range previous {
		lines := make(map[string]bool, len(team.Lines))
		for _, line := range team.Lines {
			lines[line] = true
		}
		seen[team.RosterID] = lines
	}

	var fresh []MonitorTeam
	for _, team := range current {
		for _, line := range team.Lines {
			if !seen[team.RosterID][line] {
				fresh = append(fresh, team)
				break
			}
		}
	}
	return fresh
}

// monitorLine describes the problem, if any, with the starter in lineup slot
// slot. A player with several problems gets the one that matters most: a
// released or bye-week player scores zero whatever their injury status.
func (c *LeagueContext) monitorLine(slot int, playerID string, games weekSchedule) (string, bool) {
	if playerID == "" || playerID == emptySlotID {
		return c.slotLabel(slot) + " - Empty slot", true
	}

	player, ok := c.Players[playerID]
	if !ok || games.started[player.Team] {
		return "", false
	}

	var problem string
	switch {
	case player.Team == "":
		problem = "No NFL team"
	case games.playing != nil && !games.playing[player.Team]:
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

// weekSchedule summarizes one week of the NFL schedule by team.
type weekSchedule struct {
	// playing holds every team with a game this week, or is nil when the
	// schedule has no games for the week at all - most likely a missing or
	// failed schedule fetch, which must not read as a league-wide bye.
	playing map[string]bool
	// started holds teams whose game is under way or over (anything past
	// Sleeper's "pre_game" status), whose players are locked in.
	started map[string]bool
}

func weekGames(schedule []sleeper.ScheduledGame, week int) weekSchedule {
	var ws weekSchedule
	for _, g := range schedule {
		if g.Week != week {
			continue
		}
		if ws.playing == nil {
			ws.playing = make(map[string]bool)
			ws.started = make(map[string]bool)
		}
		ws.playing[g.Home] = true
		ws.playing[g.Away] = true
		if g.Status != "" && g.Status != "pre_game" {
			ws.started[g.Home] = true
			ws.started[g.Away] = true
		}
	}
	return ws
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
