// Package scheduler wires bot.Run to a weekly cron schedule, mirroring
// gamedaybot's job timings: scores through the weekend, recaps and rankings
// early in the week, matchup previews and waiver activity later on. The
// schedule is rebuilt and hot-swapped whenever settings change (see
// settings.Manager.Subscribe), so portal edits take effect without a
// restart.
package scheduler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"

	"github.com/robfig/cron/v3"

	"fantasy_bot/internal/bot"
	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// gameTimezone is used for jobs tied to actual NFL game windows (Sunday
// scoreboard updates, Thursday matchup previews), independent of the
// league's configured local timezone - mirroring gamedaybot, which always
// schedules those in America/New_York regardless of TIMEZONE. It is never
// overridden by the portal's timezone setting, which only affects "local"
// jobs (see buildJobs).
const gameTimezone = "America/New_York"

// jobSpec is one concrete, ready-to-register cron entry.
type jobSpec struct {
	name string
	spec string
	rt   bot.ReportType
}

// buildJobs computes the concrete cron spec and report type for every
// schedulable message, filtered by settings overrides (see
// settings.JobIsEnabled/DefaultJobEnabled) and the effective timezone/waiver
// days. It's a pure function - no *cron.Cron touched - so tests can assert
// on the resulting specs without waiting on real cron ticks.
//
// "scoreboard" and "close_scores" are registered twice each: close_scores
// fires Sunday 4pm/8pm ET (split out from what used to be one bundled
// "scoreboard_gameday" job) *and* Monday 6:30pm ET, all three firings
// sharing one enabled/disabled toggle, since the README documents them as
// one "Close Scores" message that happens to post three times a week.
func buildJobs(cfg *config.Config, s settings.Settings) []jobSpec {
	tzLocal := s.EffectiveTimezone(cfg.Timezone)

	var jobs []jobSpec
	add := func(name, spec string, rt bot.ReportType) {
		if s.JobIsEnabled(name, settings.DefaultJobEnabled(name, cfg)) {
			jobs = append(jobs, jobSpec{name, spec, rt})
		}
	}

	// Monday 6:30pm ET: who's still sweating a close one.
	add("close_scores", tz(gameTimezone, "30 18 * * 1"), bot.ReportCloseScores)
	// Tuesday 6:30pm local: power rankings for the week just finished.
	add("power_rankings", tz(tzLocal, "30 18 * * 2"), bot.ReportPowerRankings)
	// Tuesday 6:31pm local: fortune index (schedule luck) alongside power rankings.
	add("fortune_index", tz(tzLocal, "31 18 * * 2"), bot.ReportFortuneIndex)
	// Tuesday 7:30am local: final scores + trophies for the week just finished.
	add("final", tz(tzLocal, "30 7 * * 2"), bot.ReportFinal)
	// Tuesday 9:00am local: season-long trophy case, tallied through the
	// week Final just posted.
	add("trophy_case", tz(tzLocal, "0 9 * * 2"), bot.ReportTrophyCase)
	// Wednesday 7:30am local: current standings.
	add("standings", tz(tzLocal, "30 7 * * 3"), bot.ReportStandings)
	// Wednesday 7:30am local: all-play standings (every team vs. every
	// team every week), alongside the real standings.
	add("win_matrix", tz(tzLocal, "30 7 * * 3"), bot.ReportWinMatrix)
	// Thursday 7:30pm ET: next matchups preview.
	add("matchups", tz(gameTimezone, "30 19 * * 4"), bot.ReportMatchups)
	// Friday & Monday 7:30am local: score recap with approximate
	// projected final scores.
	add("scoreboard_weekday", tz(tzLocal, "30 7 * * 1,5"), bot.ReportWeekdayScoreboard)
	// Sunday 4pm & 8pm ET: in-progress scoreboard, and (independently
	// toggleable) which games are currently close enough to be worth
	// watching.
	add("scoreboard", tz(gameTimezone, "0 16,20 * * 0"), bot.ReportScoreboard)
	add("close_scores", tz(gameTimezone, "0 16,20 * * 0"), bot.ReportCloseScores)

	if days := s.EffectiveWaiverDays(settings.DefaultWaiverDays(cfg)); len(days) > 0 {
		add("waiver_report", tz(tzLocal, fmt.Sprintf("31 7 * * %s", joinDays(days))), bot.ReportWaiver)
	}

	// Sunday 7:30am local: injury/monitor report.
	add("monitor", tz(tzLocal, "30 7 * * 0"), bot.ReportMonitor)

	if cfg.LLM.APIKey != "" {
		// Tuesday 6:45pm local: the AI weekly recap, after final scores
		// (7:30am), power rankings (6:30pm), and fortune index (6:31pm)
		// have all posted for the week that just finished. Gated on the
		// API key regardless of the portal's job-enabled override - there's
		// no LLM client to call without one (see bot.Bot.llm).
		add("recap", tz(tzLocal, "45 18 * * 2"), bot.ReportRecap)
	}

	return jobs
}

// joinDays renders weekday numbers (0=Sunday..6=Saturday) as a cron
// day-of-week field, e.g. []int{1,3,5} -> "1,3,5".
func joinDays(days []int) string {
	strs := make([]string, len(days))
	for i, d := range days {
		strs[i] = strconv.Itoa(d)
	}
	return strings.Join(strs, ",")
}

// registerCron builds a fresh, started *cron.Cron from cfg+s, or an error
// if any job's spec fails to register (e.g. an invalid IANA timezone).
func registerCron(ctx context.Context, cfg *config.Config, b *bot.Bot, s settings.Settings) (*cron.Cron, int, error) {
	jobs := buildJobs(cfg, s)

	c := cron.New()
	for _, job := range jobs {
		job := job // capture for closure
		if _, err := c.AddFunc(job.spec, func() { runJob(ctx, b, job.name, job.rt) }); err != nil {
			return nil, 0, fmt.Errorf("add job %s: %w", job.name, err)
		}
	}
	c.Start()
	return c, len(jobs), nil
}

// liveSchedule holds the currently-running *cron.Cron so it can be rebuilt
// and swapped in place when settings change.
type liveSchedule struct {
	mu   sync.Mutex
	cron *cron.Cron
}

// reload rebuilds the schedule from cfg+s and swaps it in only if every job
// registers successfully; on failure it logs and leaves the previous
// schedule running untouched (this path is reached from a settings.Manager
// subscriber, so a bad save must never take down an already-running bot -
// contrast Run's initial build, which is allowed to be fatal).
//
// It starts the new schedule before stopping the old one, not the reverse:
// stop-then-start risks silently missing a fire that lands in the gap,
// whereas start-then-stop risks - only if a save lands in the exact same
// second as a job's fire time - a duplicate post. For a schedule that fires
// at most a few times a day and settings that change rarely, missing a
// post is worse than an occasional duplicate. Note also that Stop() cannot
// cancel a job already dispatched on the outgoing cron (e.g. a long recap
// generation); it only reports, via the context it returns, once any
// in-flight jobs finish - which this deliberately doesn't wait for.
func (ls *liveSchedule) reload(ctx context.Context, cfg *config.Config, b *bot.Bot, s settings.Settings) {
	newCron, n, err := registerCron(ctx, cfg, b, s)
	if err != nil {
		log.Printf("scheduler: reload failed, keeping previous schedule: %v", err)
		return
	}

	ls.mu.Lock()
	old := ls.cron
	ls.cron = newCron
	ls.mu.Unlock()

	if old != nil {
		old.Stop()
	}
	log.Printf("scheduler: %d jobs scheduled (reload)", n)
}

// Run builds the initial schedule from cfg and mgr's current settings, then
// keeps it live-reloaded for as long as ctx stays open: every subsequent
// settings.Manager.Save rebuilds and hot-swaps the schedule (see
// liveSchedule.reload) without needing a process restart. Subscribing
// before the initial build means a Save landing during startup can't be
// missed; an initial build failure (e.g. a bad TIMEZONE in .env) is
// returned as an error, matching the previous behavior of failing fast at
// startup, while a failure on a later, portal-triggered reload only logs
// and keeps the bot running on its last-known-good schedule.
func Run(ctx context.Context, cfg *config.Config, mgr *settings.Manager, b *bot.Bot) error {
	ls := &liveSchedule{}
	mgr.Subscribe(func(s settings.Settings) { ls.reload(ctx, cfg, b, s) })

	initial, n, err := registerCron(ctx, cfg, b, mgr.Get())
	if err != nil {
		return fmt.Errorf("scheduler: %w", err)
	}
	ls.mu.Lock()
	ls.cron = initial
	ls.mu.Unlock()

	log.Printf("scheduler: %d jobs scheduled, starting", n)
	<-ctx.Done()
	return nil
}

func runJob(ctx context.Context, b *bot.Bot, name string, rt bot.ReportType) {
	log.Printf("scheduler: running %s", name)
	if err := b.Run(ctx, rt); err != nil {
		log.Printf("scheduler: %s failed: %v", name, err)
	}
}

// tz prefixes a 5-field cron spec with a CRON_TZ directive so each job can
// run in its own timezone (robfig/cron evaluates CRON_TZ=<zone> per-entry).
func tz(zone, spec string) string {
	return fmt.Sprintf("CRON_TZ=%s %s", zone, spec)
}
