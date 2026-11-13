package bot

import (
	"context"
	"fmt"
	"strings"

	"fantasy_bot/internal/report"
)

// roastSystemPrompt is the persona/instructions for /roast (see Roast).
const roastSystemPrompt = `You are the Discord bot for a fantasy football league on Sleeper, and someone has asked you to roast a league member. Think comedy-roast emcee: sharp, specific, and funny, with the target in on the joke.

You'll be given who asked for the roast, who the target is (a league team name, and/or their Discord names - which may or may not match a Sleeper manager or team name), and a snapshot of league data: standings, streaks, head-to-head series, the playoff race, this week's scores, the week's worst lineup decision, and every team's roster with injury designations.

Guidelines:
- First work out which team is the target's. Use the team name if given; otherwise match their Discord names against the manager and team names in the data. If you can't confidently match them to a team, say so as part of the joke and keep the roast light and general - never pin another team's stats on them.
- Roast their fantasy football: record, points, losing streaks, bench blunders, injured or washed-up players, bad trades or waiver luck, playoff odds. The more specific (real numbers, real player names from the data), the better.
- Keep it to 80-150 words. Punchy, a few jokes that land rather than a list.
- Keep it good-natured: about fantasy football only. Nothing about appearance, race, religion, gender, sexuality, disability, family, or anything else personal - and no slurs or genuinely cruel material.
- Only use facts from the league data; don't invent scores, trades, or players.
- Use Discord markdown sparingly (bold is fine). Don't use headings, code blocks, or @mentions.`

// Roast writes a roast of a league member's fantasy team for the /roast
// Discord command. team is a league team as offered by TeamNames (may be
// empty); targetNames are the Discord names (server nickname, display
// name, username) of the member being roasted (may be empty, but not both
// at once); requester is the Discord display name of whoever asked.
//
// Like Answer, situations it can't help with (no LLM configured,
// offseason) come back as a friendly explanation with a nil error.
func (b *Bot) Roast(ctx context.Context, requester string, targetNames []string, team string) (string, error) {
	if b.llm == nil {
		return noLLMReply, nil
	}
	data, unavailable, err := b.leagueSnapshot(ctx)
	if err != nil || unavailable != "" {
		return unavailable, err
	}

	roast, err := b.llm.Generate(ctx, roastSystemPrompt, buildRoastPrompt(requester, targetNames, team, data))
	if err != nil {
		return "", fmt.Errorf("bot: write roast: %w", err)
	}
	return strings.TrimSpace(roast), nil
}

// buildRoastPrompt assembles Roast's user prompt, target first and the
// league data last, as in buildChatPrompt.
func buildRoastPrompt(requester string, targetNames []string, team, leagueData string) string {
	var p strings.Builder
	fmt.Fprintf(&p, "Requested by: %s\n", requester)
	if team != "" {
		fmt.Fprintf(&p, "Target team: %s\n", team)
	}
	if len(targetNames) > 0 {
		fmt.Fprintf(&p, "Target's Discord names: %s\n", strings.Join(targetNames, ", "))
	}
	p.WriteString("\n=== League data ===\n")
	p.WriteString(leagueData)
	return p.String()
}

// TeamNames lists every team in the league as "Team Name (manager)" - or
// just the team name when it's already the manager's name - for /roast's
// team autocomplete. It only needs rosters and users, so it stays fast
// enough for Discord's 3-second autocomplete deadline.
func (b *Bot) TeamNames(ctx context.Context) ([]string, error) {
	rosters, err := b.sleeper.GetRosters(ctx, b.cfg.LeagueID)
	if err != nil {
		return nil, fmt.Errorf("bot: fetch rosters: %w", err)
	}
	users, err := b.sleeper.GetUsers(ctx, b.cfg.LeagueID)
	if err != nil {
		return nil, fmt.Errorf("bot: fetch users: %w", err)
	}
	return report.TeamLabels(rosters, users), nil
}
