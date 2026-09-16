// Package bot ties the Sleeper client, report builders, and Discord sender
// together. It is the Go equivalent of gamedaybot's espn_bot() dispatch
// function: given a report name, fetch what that report needs and post the
// result to Discord.
package bot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/discord"
	"fantasy_bot/internal/llm"
	"fantasy_bot/internal/report"
	"fantasy_bot/internal/settings"
	"fantasy_bot/internal/sleeper"
)

// errSeasonNotStarted signals that Sleeper's NFL state has no active
// scoring week yet (offseason/preseason), so scheduled reports have nothing
// to say. Run() treats it as a no-op rather than an error.
var errSeasonNotStarted = errors.New("bot: season has not started")

type ReportType string

const (
	ReportInit                ReportType = "init"
	ReportScoreboard          ReportType = "scoreboard"
	ReportProjectedScoreboard ReportType = "projected_scoreboard"
	// ReportWeekdayScoreboard posts current scores plus each matchup's
	// approximate projected final score, for the Friday/Monday recap windows.
	ReportWeekdayScoreboard ReportType = "weekday_scoreboard"
	// ReportGameday posts the in-progress scoreboard followed by a close
	// scores callout, for the Sunday-afternoon/evening gameday windows.
	ReportGameday   ReportType = "gameday"
	ReportMatchups  ReportType = "matchups"
	ReportStandings ReportType = "standings"
	// ReportWinMatrix posts the league's all-play standings (see
	// report.LeagueContext.WinMatrix): every team's record if it had played
	// every other team every week, tallied through the most recently
	// completed week.
	ReportWinMatrix ReportType = "win_matrix"
	// ReportPowerRankings posts each team's power score, playoff odds, and
	// week-over-week trend (see report.LeagueContext.PowerRankings), ranked
	// through the most recently completed week.
	ReportPowerRankings ReportType = "power_rankings"
	// ReportFortuneIndex posts each team's season-long schedule-luck ranking
	// (see report.LeagueContext.FortuneIndex), tallied through the most
	// recently completed week.
	ReportFortuneIndex ReportType = "fortune_index"
	ReportTrophies     ReportType = "trophies"
	// ReportTrophyCase posts a season-long crosstab image of every team's
	// trophy counts (see report.LeagueContext.TrophyCaseImage), tallied
	// through the most recently completed week.
	ReportTrophyCase ReportType = "trophy_case"
	// ReportBadManagement posts a bar-chart image of every team's points
	// scored vs. points left on the bench (see
	// report.LeagueContext.BadManagementChart) for the week that just
	// finished, ranked worst manager first.
	ReportBadManagement ReportType = "bad_management"
	ReportCloseScores   ReportType = "close_scores"
	ReportWaiver        ReportType = "waiver"
	ReportMonitor       ReportType = "monitor"
	// ReportFinal posts the previous week's final scores plus trophies,
	// mirroring gamedaybot's Tuesday-morning "get_final" job.
	ReportFinal ReportType = "final"
	// ReportRecap posts the AI-written weekly recap newsletter (see
	// report.LeagueContext.RecapDigest). No-ops if no LLM provider is
	// configured (see config.LLMConfig).
	ReportRecap ReportType = "recap"
)

// Sender posts finished report text and images somewhere — a Discord
// webhook by default, or e.g. stdout for a dry run during testing.
type Sender interface {
	Send(ctx context.Context, text string) error
	// SendRich posts free-form Markdown prose (e.g. the AI weekly recap)
	// without wrapping it in a code block, unlike Send, so Discord renders
	// bold/italic/headers instead of showing them as literal characters.
	SendRich(ctx context.Context, text string) error
	// SendImage posts a PNG image (data), captioned with caption, as an
	// attachment named filename.
	SendImage(ctx context.Context, caption, filename string, data []byte) error
}

type Bot struct {
	cfg     *config.Config
	sleeper *sleeper.Client
	players *sleeper.PlayerCache
	sender  Sender
	// llm is nil unless an AI Weekly Recap provider is configured (see
	// config.LLMConfig); ReportRecap no-ops when it's nil.
	llm llm.Client
	// settingsMgr supplies portal overrides at report-generation time (e.g.
	// the recap prompt), so a portal edit takes effect on the very next run
	// without needing the scheduler to reload anything.
	settingsMgr *settings.Manager
}

func New(cfg *config.Config, mgr *settings.Manager) *Bot {
	return NewWithSender(cfg, mgr, discord.NewClient(cfg.DiscordWebhookURL))
}

// NewWithSender is like New but posts reports to sender instead of Discord,
// e.g. a stdout printer for dry-run testing/validation.
func NewWithSender(cfg *config.Config, mgr *settings.Manager, sender Sender) *Bot {
	client := sleeper.NewClient()
	b := &Bot{
		cfg:         cfg,
		sleeper:     client,
		players:     sleeper.NewPlayerCache(client, 24*time.Hour),
		sender:      sender,
		settingsMgr: mgr,
	}
	if cfg.LLM.APIKey != "" {
		llmClient, err := llm.New(llm.Config{Provider: cfg.LLM.Provider, APIKey: cfg.LLM.APIKey, Model: cfg.LLM.Model})
		if err != nil {
			// config.Load already validates Provider, so this shouldn't
			// happen in practice; fail soft rather than crash the whole bot
			// over the recap feature.
			log.Printf("bot: AI weekly recap not available: %v", err)
		} else {
			b.llm = llmClient
		}
	}
	return b
}

// Run fetches whatever data the given report needs, builds it, and posts it
// to Discord. It's the single entry point both the scheduler and any manual
// on-demand trigger should call.
func (b *Bot) Run(ctx context.Context, rt ReportType) error {
	if rt == ReportInit {
		if b.cfg.InitMessage == "" {
			return nil
		}
		return b.sender.Send(ctx, b.cfg.InitMessage)
	}

	leagueCtx, week, err := b.buildLeagueContext(ctx)
	if err != nil {
		if errors.Is(err, errSeasonNotStarted) {
			return nil
		}
		return err
	}

	var text string
	switch rt {
	case ReportScoreboard:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.ScoreboardShort(matchups)

	case ReportProjectedScoreboard:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		projections, err := b.projections(ctx, leagueCtx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.ProjectedScoreboard(matchups, projections)

	case ReportWeekdayScoreboard:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		projections, err := b.projections(ctx, leagueCtx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.WeekdayScoreboard(matchups, projections)

	case ReportGameday:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		if err := b.sender.Send(ctx, leagueCtx.ScoreboardShort(matchups)); err != nil {
			return err
		}
		return b.sender.Send(ctx, leagueCtx.CloseScores(matchups, b.cfg.CloseScoresThreshold))

	case ReportMatchups:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		projections, err := b.projections(ctx, leagueCtx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.Matchups(matchups) + "\n" + leagueCtx.ProjectedScoreboard(matchups, projections)

	case ReportStandings:
		text = leagueCtx.Standings()

	case ReportWinMatrix:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to tally before week 1 is complete
		}
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return err
		}
		return b.sender.SendRich(ctx, leagueCtx.WinMatrix(history))

	case ReportPowerRankings:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to rank before week 1 is complete
		}
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return err
		}
		leagueCtx.Week = finalWeek
		text = leagueCtx.PowerRankings(history)
		if err := b.sender.Send(ctx, text); err != nil {
			return err
		}
		chartPNG, err := leagueCtx.PowerRankingsChart(history)
		if err != nil {
			return err
		}
		if chartPNG == nil {
			return nil
		}
		return b.sender.SendImage(ctx, "Power Rankings — Season Trend", "power_rankings.png", chartPNG)

	case ReportFortuneIndex:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to tally before week 1 is complete
		}
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return err
		}
		leagueCtx.Week = finalWeek
		text = leagueCtx.FortuneIndex(history)

	case ReportTrophies:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		projections, err := b.projections(ctx, leagueCtx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.Trophies(matchups, projections)

	case ReportTrophyCase:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to tally before week 1 is complete
		}
		matchupHist, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return err
		}
		projHist, err := b.projectionHistory(ctx, leagueCtx, finalWeek)
		if err != nil {
			return err
		}
		imgPNG, err := leagueCtx.TrophyCaseImage(matchupHist, projHist)
		if err != nil {
			return err
		}
		if imgPNG == nil {
			return nil
		}
		caption := fmt.Sprintf("Trophy Case — season totals through Week %d\n%s", finalWeek, report.TrophyCaseLegend())
		return b.sender.SendImage(ctx, caption, "trophy_case.png", imgPNG)

	case ReportBadManagement:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to chart before week 1 is complete
		}
		matchups, err := b.matchups(ctx, finalWeek)
		if err != nil {
			return err
		}
		leagueCtx.Week = finalWeek
		chartPNG, err := leagueCtx.BadManagementChart(matchups)
		if err != nil {
			return err
		}
		if chartPNG == nil {
			return nil
		}
		return b.sender.SendImage(ctx, fmt.Sprintf("🤡 Bad Management 🤡 — Week %d", finalWeek), "bad_management.png", chartPNG)

	case ReportCloseScores:
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return err
		}
		text = leagueCtx.CloseScores(matchups, b.cfg.CloseScoresThreshold)

	case ReportWaiver:
		txns, err := b.sleeper.GetTransactions(ctx, b.cfg.LeagueID, week)
		if err != nil {
			return fmt.Errorf("bot: fetch transactions: %w", err)
		}
		text = leagueCtx.WaiverReport(txns)

	case ReportMonitor:
		text = leagueCtx.Monitor()

	case ReportFinal:
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to finalize before week 1
		}
		leagueCtx.Week = finalWeek
		matchups, err := b.matchups(ctx, finalWeek)
		if err != nil {
			return err
		}
		projections, err := b.projections(ctx, leagueCtx, finalWeek)
		if err != nil {
			return err
		}
		text = "Final\n\n" + leagueCtx.Trophies(matchups, projections)

	case ReportRecap:
		if b.llm == nil {
			return nil // no LLM provider configured; nothing to do
		}
		finalWeek := week - 1
		if finalWeek < 1 {
			return nil // nothing to recap before week 1 is complete
		}
		history, err := b.matchupHistory(ctx, finalWeek)
		if err != nil {
			return err
		}
		digest := leagueCtx.RecapDigest(history, finalWeek)
		systemPrompt := b.settingsMgr.Get().EffectiveRecapPrompt(settings.DefaultRecapPrompt)
		recap, err := b.llm.Generate(ctx, systemPrompt, digest)
		if err != nil {
			return fmt.Errorf("bot: generate weekly recap: %w", err)
		}
		return b.sender.SendRich(ctx, recap)

	default:
		return fmt.Errorf("bot: unknown report type %q", rt)
	}

	return b.sender.Send(ctx, text)
}

func (b *Bot) matchups(ctx context.Context, week int) ([]sleeper.Matchup, error) {
	matchups, err := b.sleeper.GetMatchups(ctx, b.cfg.LeagueID, week)
	if err != nil {
		return nil, fmt.Errorf("bot: fetch matchups for week %d: %w", week, err)
	}
	return matchups, nil
}

// matchupHistory fetches every week's matchups from week 1 through
// throughWeek, for reconstructing past power-ranking scores (Sleeper has no
// endpoint for that history directly — see report.PowerRankings).
func (b *Bot) matchupHistory(ctx context.Context, throughWeek int) (map[int][]sleeper.Matchup, error) {
	history := make(map[int][]sleeper.Matchup, throughWeek)
	for week := 1; week <= throughWeek; week++ {
		matchups, err := b.matchups(ctx, week)
		if err != nil {
			return nil, err
		}
		history[week] = matchups
	}
	return history, nil
}

// projectionHistory fetches every week's player projections from week 1
// through throughWeek, for TrophyCase's season-long achiever tally -
// mirrors matchupHistory's per-week fetch loop.
func (b *Bot) projectionHistory(ctx context.Context, leagueCtx *report.LeagueContext, throughWeek int) (map[int][]sleeper.PlayerProjection, error) {
	history := make(map[int][]sleeper.PlayerProjection, throughWeek)
	for week := 1; week <= throughWeek; week++ {
		projections, err := b.projections(ctx, leagueCtx, week)
		if err != nil {
			return nil, err
		}
		history[week] = projections
	}
	return history, nil
}

func (b *Bot) projections(ctx context.Context, leagueCtx *report.LeagueContext, week int) ([]sleeper.PlayerProjection, error) {
	seasonType := leagueCtx.League.SeasonType
	if seasonType == "" {
		seasonType = "regular"
	}
	projections, err := b.sleeper.GetProjections(ctx, leagueCtx.League.Season, week, seasonType)
	if err != nil {
		return nil, fmt.Errorf("bot: fetch projections for week %d: %w", week, err)
	}
	return projections, nil
}

func (b *Bot) buildLeagueContext(ctx context.Context) (*report.LeagueContext, int, error) {
	league, err := b.sleeper.GetLeague(ctx, b.cfg.LeagueID)
	if err != nil {
		return nil, 0, fmt.Errorf("bot: fetch league: %w", err)
	}

	rosters, err := b.sleeper.GetRosters(ctx, b.cfg.LeagueID)
	if err != nil {
		return nil, 0, fmt.Errorf("bot: fetch rosters: %w", err)
	}

	users, err := b.sleeper.GetUsers(ctx, b.cfg.LeagueID)
	if err != nil {
		return nil, 0, fmt.Errorf("bot: fetch users: %w", err)
	}

	state, err := b.sleeper.GetNFLState(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("bot: fetch NFL state: %w", err)
	}
	if state.Week == 0 {
		return nil, 0, errSeasonNotStarted
	}

	players, err := b.players.Get(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("bot: fetch players: %w", err)
	}

	leagueCtx := report.NewLeagueContext(*league, rosters, users, players, state.Week)
	leagueCtx.SetAbbreviations(b.cfg.TeamAbbreviations)
	return leagueCtx, state.Week, nil
}
