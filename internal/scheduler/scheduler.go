// Package scheduler wires bot.Run to a weekly cron schedule, mirroring
// gamedaybot's job timings: scores through the weekend, recaps and rankings
// early in the week, matchup previews and waiver activity later on.
package scheduler

import (
	"context"
	"fmt"
	"log"

	"github.com/robfig/cron/v3"

	"fantasy_bot/internal/bot"
	"fantasy_bot/internal/config"
)

// gameTimezone is used for jobs tied to actual NFL game windows (Sunday
// scoreboard updates, Thursday matchup previews), independent of the
// league's configured local timezone - mirroring gamedaybot, which always
// schedules those in America/New_York regardless of TIMEZONE.
const gameTimezone = "America/New_York"

func Run(ctx context.Context, cfg *config.Config, b *bot.Bot) error {
	c := cron.New()

	jobs := []struct {
		name string
		spec string
		rt   bot.ReportType
	}{
		// Monday 6:30pm ET: who's still sweating a close one.
		{"close_scores", tz(gameTimezone, "30 18 * * 1"), bot.ReportCloseScores},
		// Tuesday 6:30pm local: power rankings for the week just finished.
		{"power_rankings", tz(cfg.Timezone, "30 18 * * 2"), bot.ReportPowerRankings},
		// Tuesday 6:31pm local: fortune index (schedule luck) alongside power rankings.
		{"fortune_index", tz(cfg.Timezone, "31 18 * * 2"), bot.ReportFortuneIndex},
		// Tuesday 7:30am local: final scores + trophies for the week just finished.
		{"final", tz(cfg.Timezone, "30 7 * * 2"), bot.ReportFinal},
		// Wednesday 7:30am local: current standings.
		{"standings", tz(cfg.Timezone, "30 7 * * 3"), bot.ReportStandings},
		// Thursday 7:30pm ET: next matchups preview.
		{"matchups", tz(gameTimezone, "30 19 * * 4"), bot.ReportMatchups},
		// Friday & Monday 7:30am local: score recap with approximate
		// projected final scores.
		{"scoreboard_weekday", tz(cfg.Timezone, "30 7 * * 1,5"), bot.ReportWeekdayScoreboard},
		// Sunday 4pm & 8pm ET: in-progress scoreboard, followed by which
		// games are currently close enough to be worth watching.
		{"scoreboard_gameday", tz(gameTimezone, "0 16,20 * * 0"), bot.ReportGameday},
	}

	waiverSpec := tz(cfg.Timezone, "31 7 * * 3") // Wednesday by default
	if cfg.DailyWaiver {
		waiverSpec = tz(cfg.Timezone, "31 7 * * *")
	}
	jobs = append(jobs, struct {
		name string
		spec string
		rt   bot.ReportType
	}{"waiver_report", waiverSpec, bot.ReportWaiver})

	if cfg.MonitorReport {
		jobs = append(jobs, struct {
			name string
			spec string
			rt   bot.ReportType
		}{"monitor", tz(cfg.Timezone, "30 7 * * 0"), bot.ReportMonitor})
	}

	if cfg.LLM.APIKey != "" {
		// Tuesday 6:45pm local: the AI weekly recap, after final scores
		// (7:30am), power rankings (6:30pm), and fortune index (6:31pm)
		// have all posted for the week that just finished.
		jobs = append(jobs, struct {
			name string
			spec string
			rt   bot.ReportType
		}{"recap", tz(cfg.Timezone, "45 18 * * 2"), bot.ReportRecap})
	}

	for _, job := range jobs {
		job := job // capture for closure
		_, err := c.AddFunc(job.spec, func() {
			runJob(ctx, b, job.name, job.rt)
		})
		if err != nil {
			return fmt.Errorf("scheduler: add job %s: %w", job.name, err)
		}
	}

	log.Printf("scheduler: %d jobs scheduled, starting", len(jobs))
	c.Run()
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
