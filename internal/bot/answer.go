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

You'll be given the question, who asked it (their Discord display name, which may or may not match a Sleeper manager or team name - if it clearly matches one, treat that team as theirs), and a snapshot of league data: standings, streaks, head-to-head series, the playoff race, this week's scores, and every team's roster with injury designations.

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
// Discord display name of whoever asked; replyingTo, if non-empty, is the
// text of the message they replied to (usually an earlier bot answer),
// giving the model just enough context for a follow-up question.
//
// Situations the bot can't help with (no LLM configured, offseason) come
// back as a friendly explanation with a nil error, since they should be
// shown to the asker as-is; a non-nil error means something actually broke.
func (b *Bot) Answer(ctx context.Context, asker, question, replyingTo string) (string, error) {
	if b.llm == nil {
		return "I can't answer questions yet - no AI provider is configured (LLM_API_KEY in .env).", nil
	}

	leagueCtx, week, err := b.buildLeagueContext(ctx)
	if errors.Is(err, errSeasonNotStarted) {
		return "The season hasn't started yet, so I don't have any league data to talk about. Ask me again once Week 1 kicks off!", nil
	}
	if err != nil {
		return "", err
	}

	var data strings.Builder
	if finalWeek := week - 1; finalWeek >= 1 {
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return "", err
		}
		data.WriteString(leagueCtx.RecapDigest(history, finalWeek))
	} else {
		fmt.Fprintf(&data, "League: %s - Week 1 in progress, no completed weeks yet\n\n", leagueCtx.League.Name)
		data.WriteString(leagueCtx.Standings())
	}
	data.WriteString("\n")

	current, err := b.matchups(ctx, week)
	if err != nil {
		return "", err
	}
	data.WriteString("This week (in progress):\n")
	data.WriteString(leagueCtx.ScoreboardShort(current))
	data.WriteString("\n")
	data.WriteString(leagueCtx.RosterDigest())

	userPrompt := buildChatPrompt(asker, question, replyingTo, data.String())
	answer, err := b.llm.Generate(ctx, chatSystemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("bot: answer question: %w", err)
	}
	return strings.TrimSpace(answer), nil
}

// buildChatPrompt assembles Answer's user prompt. The question goes first
// and the (much longer) league data last, clearly fenced, so the model
// can't confuse data with instructions or lose the question in the noise.
func buildChatPrompt(asker, question, replyingTo, leagueData string) string {
	var p strings.Builder
	fmt.Fprintf(&p, "Asked by: %s\n", asker)
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
