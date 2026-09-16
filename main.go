// Command fantasy_bot posts scheduled fantasy football reports (scores,
// standings, power rankings, trophies, waiver activity, injury monitor) for
// a Sleeper league to a Discord channel via webhook.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"fantasy_bot/internal/bot"
	"fantasy_bot/internal/config"
	"fantasy_bot/internal/portal"
	"fantasy_bot/internal/scheduler"
	"fantasy_bot/internal/settings"
)

func main() {
	report := flag.String("report", "", "run a single report and exit instead of starting the scheduler "+
		"(one of: init, scoreboard, projected_scoreboard, gameday, matchups, standings, win_matrix, power_rankings, fortune_index, trophies, trophy_case, bad_management, close_scores, waiver, monitor, final, recap)")
	dryRun := flag.Bool("dry-run", false, "print report output to stdout instead of posting to Discord")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	mgr, err := settings.Load(cfg.SettingsFile)
	if err != nil {
		log.Fatalf("settings: %v", err)
	}

	var b *bot.Bot
	if *dryRun {
		b = bot.NewWithSender(cfg, mgr, stdoutSender{})
	} else {
		b = bot.New(cfg, mgr)
	}

	ctx := context.Background()

	if *report != "" {
		if err := b.Run(ctx, bot.ReportType(*report)); err != nil {
			log.Fatalf("report %s failed: %v", *report, err)
		}
		return
	}

	if err := b.Run(ctx, bot.ReportInit); err != nil {
		log.Printf("init message failed: %v", err)
	}

	// The web config portal is supplementary to the bot's core scheduling
	// function, so a failure to start it (e.g. the port is already in use)
	// is logged rather than fatal.
	if cfg.PortalPassword != "" {
		go func() {
			if err := portal.Serve(ctx, cfg, mgr); err != nil {
				log.Printf("portal: %v", err)
			}
		}()
	} else {
		log.Printf("portal: PORTAL_PASSWORD not set, portal disabled")
	}

	if err := scheduler.Run(ctx, cfg, mgr, b); err != nil {
		log.Fatalf("scheduler: %v", err)
	}
}

// stdoutSender prints report text to stdout instead of posting it to
// Discord, for -dry-run testing/validation.
type stdoutSender struct{}

func (stdoutSender) Send(_ context.Context, text string) error {
	fmt.Println(text)
	fmt.Println(strings.Repeat("-", 60))
	return nil
}

// SendRich is Send's counterpart for free-form Markdown (e.g. the AI weekly
// recap); stdout has no rendering to worry about, so it's printed the same.
func (stdoutSender) SendRich(_ context.Context, text string) error {
	fmt.Println(text)
	fmt.Println(strings.Repeat("-", 60))
	return nil
}

// SendImage saves the image to the working directory instead of posting it,
// so -dry-run still lets you inspect chart output.
func (stdoutSender) SendImage(_ context.Context, caption, filename string, data []byte) error {
	if caption != "" {
		fmt.Println(caption)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("[image written to %s]\n", filename)
	fmt.Println(strings.Repeat("-", 60))
	return nil
}
