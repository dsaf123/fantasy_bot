package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// chatSystemPrompt is the persona/instructions for answering @mentions (see
// Answer). Unlike settings.DefaultRecapPrompt it isn't portal-configurable,
// so it lives here next to its only user.
const chatSystemPrompt = `You are the Discord bot for a fantasy football league on Sleeper. League members @mention you in the league's Discord to ask questions, talk trash, or ask for your take, and you reply in the channel.

You'll be given the question, who asked it (their Discord display name, plus their team if they've linked one - otherwise, if their name clearly matches a Sleeper manager or team name, treat that team as theirs), any Discord members they mention (with those members' teams when linked), and a snapshot of league data: standings, streaks, head-to-head series, the playoff race, this week's scores, and every team's roster with injury designations.

Guidelines:
- Keep it short: a few sentences, under ~150 words unless the question genuinely needs more (e.g. a list).
- Be specific and use team names. Light, good-natured trash talk is welcome; be fun, not mean.
- Only state facts that are in the league data. If the answer isn't there (e.g. player news, future NFL schedules, other leagues), say you don't know rather than guessing. You can still give opinions, clearly framed as opinions.
- Use Discord markdown sparingly (bold is fine). Don't use code blocks, headings, or @mentions.`

// maxQuestionLen and maxReplyContextLen cap how much user-supplied text goes
// into the prompt, so one enormous message can't run up the LLM bill.
const (
	maxQuestionLen     = 1000
	maxReplyContextLen = 1500
)

// Answer replies to a free-form question about the league, for when someone
// @mentions the Discord bot (see internal/discordbot). asker is the
// Discord display name of whoever asked, and askerTeam the label of the
// league team they're linked to (see settings.TeamLink), if any; replyingTo, if non-empty, is the
// text of the message they replied to (usually an earlier bot answer),
// giving the model just enough context for a follow-up question.
//
// Situations the bot can't help with (no LLM configured, offseason) come
// back as a friendly explanation with a nil error, since they should be
// shown to the asker as-is; a non-nil error means something actually broke.
func (b *Bot) Answer(ctx context.Context, asker, askerTeam, question, replyingTo string) (string, error) {
	if b.llm == nil {
		return noLLMReply, nil
	}
	data, unavailable, err := b.leagueSnapshot(ctx)
	if err != nil || unavailable != "" {
		return unavailable, err
	}

	userPrompt := buildChatPrompt(asker, askerTeam, question, replyingTo, data)
	answer, err := b.llm.Generate(ctx, chatSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("bot: answer question: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

const (
	noLLMReply       = "I can't do that yet - no AI provider is configured (LLM_API_KEY in .env)."
	seasonNotStarted = "The season hasn't started yet, so I don't have any league data to work with. Try again once Week 1 kicks off!"
)

// leagueSnapshot gathers the league data the AI chat features (Answer,
// Roast) work from: the recap digest through the last completed week
// (standings, streaks, head-to-head, playoff race, lineup regret), this
// week's in-progress scores, and every roster. If there's nothing to
// snapshot yet (offseason), it returns a friendly explanation in
// unavailable instead.
func (b *Bot) leagueSnapshot(ctx context.Context) (data, unavailable string, err error) {
	leagueCtx, week, err := b.buildLeagueContext(ctx)
	if errors.Is(err, errSeasonNotStarted) {
		return "", seasonNotStarted, nil
	}
	if err != nil {
		return "", "", err
	}

	var out strings.Builder
	if finalWeek := week - 1; finalWeek >= 1 {
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return "", "", err
		}
		out.WriteString(leagueCtx.RecapDigest(history, finalWeek))
	} else {
		fmt.Fprintf(&out, "League: %s - Week 1 in progress, no completed weeks yet\n\n", leagueCtx.League.Name)
		out.WriteString(leagueCtx.Standings())
	}
	out.WriteString("\n")

	current, err := b.matchups(ctx, week)
	if err != nil {
		return "", "", err
	}
	out.WriteString("This week (in progress):\n")
	out.WriteString(leagueCtx.ScoreboardShort(current))
	out.WriteString("\n")
	out.WriteString(leagueCtx.RosterDigest())
	return out.String(), "", nil
}

// buildChatPrompt assembles Answer's user prompt. The question goes first
// and the (much longer) league data last, clearly fenced, so the model
// can't confuse data with instructions or lose the question in the noise.
func buildChatPrompt(asker, askerTeam, question, replyingTo, leagueData string) string {
	var p strings.Builder
	fmt.Fprintf(&p, "Asked by: %s\n", asker)
	if askerTeam != "" {
		fmt.Fprintf(&p, "Their team: %s\n", askerTeam)
	}
	if replyingTo = strings.TrimSpace(replyingTo); replyingTo != "" {
		fmt.Fprintf(&p, "They're replying to this earlier message:\n%s\n\n", truncateRunes(replyingTo, maxReplyContextLen))
	}
	fmt.Fprintf(&p, "Question: %s\n\n", truncateRunes(strings.TrimSpace(question), maxQuestionLen))
	p.WriteString("=== League data ===\n")
	p.WriteString(leagueData)
	return p.String()
}

func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
