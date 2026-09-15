package scheduler

import (
	"sort"
	"testing"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

func baseConfig() *config.Config {
	return &config.Config{
		Timezone:      "America/New_York",
		DailyWaiver:   false,
		MonitorReport: true,
	}
}

func jobByName(jobs []jobSpec, name string) []jobSpec {
	var out []jobSpec
	for _, j := range jobs {
		if j.name == name {
			out = append(out, j)
		}
	}
	return out
}

func TestBuildJobsDefaultWaiverIsWednesdayOnly(t *testing.T) {
	jobs := buildJobs(baseConfig(), settings.Settings{})
	matches := jobByName(jobs, "waiver_report")
	if len(matches) != 1 {
		t.Fatalf("waiver_report jobs = %d, want 1", len(matches))
	}
	if want := tz("America/New_York", "31 7 * * 3"); matches[0].spec != want {
		t.Errorf("waiver_report spec = %q, want %q", matches[0].spec, want)
	}
}

func TestBuildJobsDailyWaiverFromConfig(t *testing.T) {
	cfg := baseConfig()
	cfg.DailyWaiver = true
	jobs := buildJobs(cfg, settings.Settings{})
	matches := jobByName(jobs, "waiver_report")
	if len(matches) != 1 {
		t.Fatalf("waiver_report jobs = %d, want 1", len(matches))
	}
	if want := tz("America/New_York", "31 7 * * 0,1,2,3,4,5,6"); matches[0].spec != want {
		t.Errorf("waiver_report spec = %q, want %q", matches[0].spec, want)
	}
}

func TestBuildJobsWaiverDaysOverride(t *testing.T) {
	days := []int{1, 3, 5}
	jobs := buildJobs(baseConfig(), settings.Settings{WaiverDays: &days})
	matches := jobByName(jobs, "waiver_report")
	if len(matches) != 1 {
		t.Fatalf("waiver_report jobs = %d, want 1", len(matches))
	}
	if want := tz("America/New_York", "31 7 * * 1,3,5"); matches[0].spec != want {
		t.Errorf("waiver_report spec = %q, want %q", matches[0].spec, want)
	}
}

func TestBuildJobsWaiverDaysExplicitlyEmptySkipsJob(t *testing.T) {
	empty := []int{}
	jobs := buildJobs(baseConfig(), settings.Settings{WaiverDays: &empty})
	if matches := jobByName(jobs, "waiver_report"); len(matches) != 0 {
		t.Errorf("waiver_report jobs = %d, want 0 when explicitly set to no days", len(matches))
	}
}

func TestBuildJobsTimezoneOverrideAffectsLocalJobsOnly(t *testing.T) {
	tzOverride := "America/Los_Angeles"
	jobs := buildJobs(baseConfig(), settings.Settings{Timezone: &tzOverride})

	standings := jobByName(jobs, "standings")
	if len(standings) != 1 || standings[0].spec != tz("America/Los_Angeles", "30 7 * * 3") {
		t.Errorf("standings spec = %+v, want override timezone applied", standings)
	}

	// Game-window jobs (matchups, Thursday ET) must stay pinned to
	// gameTimezone regardless of the local timezone override.
	matchups := jobByName(jobs, "matchups")
	if len(matchups) != 1 || matchups[0].spec != tz(gameTimezone, "30 19 * * 4") {
		t.Errorf("matchups spec = %+v, want gameTimezone unaffected by override", matchups)
	}
}

func TestBuildJobsJobEnabledOverrideCanDisable(t *testing.T) {
	jobs := buildJobs(baseConfig(), settings.Settings{JobEnabled: map[string]bool{"standings": false}})
	if matches := jobByName(jobs, "standings"); len(matches) != 0 {
		t.Errorf("standings jobs = %d, want 0 when explicitly disabled", len(matches))
	}
}

func TestBuildJobsJobEnabledOverrideCanEnableBeyondConfigDefault(t *testing.T) {
	cfg := baseConfig()
	cfg.MonitorReport = false // env default: monitor off
	jobs := buildJobs(cfg, settings.Settings{JobEnabled: map[string]bool{"monitor": true}})
	if matches := jobByName(jobs, "monitor"); len(matches) != 1 {
		t.Errorf("monitor jobs = %d, want 1 when portal explicitly enables it over the .env default", len(matches))
	}
}

func TestBuildJobsMonitorDefaultsFromConfig(t *testing.T) {
	cfg := baseConfig()
	cfg.MonitorReport = false
	jobs := buildJobs(cfg, settings.Settings{})
	if matches := jobByName(jobs, "monitor"); len(matches) != 0 {
		t.Errorf("monitor jobs = %d, want 0 when cfg.MonitorReport is false and no override", len(matches))
	}
}

func TestBuildJobsRecapRequiresAPIKeyRegardlessOfOverride(t *testing.T) {
	cfg := baseConfig() // no LLM API key
	jobs := buildJobs(cfg, settings.Settings{JobEnabled: map[string]bool{"recap": true}})
	if matches := jobByName(jobs, "recap"); len(matches) != 0 {
		t.Errorf("recap jobs = %d, want 0 with no LLM API key even if the portal enables it", len(matches))
	}

	cfg.LLM.APIKey = "key"
	jobs = buildJobs(cfg, settings.Settings{})
	if matches := jobByName(jobs, "recap"); len(matches) != 1 {
		t.Errorf("recap jobs = %d, want 1 once an API key is configured", len(matches))
	}

	jobs = buildJobs(cfg, settings.Settings{JobEnabled: map[string]bool{"recap": false}})
	if matches := jobByName(jobs, "recap"); len(matches) != 0 {
		t.Errorf("recap jobs = %d, want 0 when the portal disables it even with an API key set", len(matches))
	}
}

func TestBuildJobsScoreboardAndCloseScoresSplitFromGameday(t *testing.T) {
	jobs := buildJobs(baseConfig(), settings.Settings{})

	scoreboard := jobByName(jobs, "scoreboard")
	if len(scoreboard) != 1 || scoreboard[0].spec != tz(gameTimezone, "0 16,20 * * 0") {
		t.Errorf("scoreboard = %+v, want one Sunday gameday entry", scoreboard)
	}

	closeScores := jobByName(jobs, "close_scores")
	if len(closeScores) != 2 {
		t.Fatalf("close_scores jobs = %d, want 2 (Sunday gameday + Monday)", len(closeScores))
	}
	specs := map[string]bool{closeScores[0].spec: true, closeScores[1].spec: true}
	if !specs[tz(gameTimezone, "0 16,20 * * 0")] || !specs[tz(gameTimezone, "30 18 * * 1")] {
		t.Errorf("close_scores specs = %v, want Sunday gameday + Monday 6:30pm ET", specs)
	}
}

func TestBuildJobsDisablingCloseScoresRemovesBothFirings(t *testing.T) {
	jobs := buildJobs(baseConfig(), settings.Settings{JobEnabled: map[string]bool{"close_scores": false}})
	if matches := jobByName(jobs, "close_scores"); len(matches) != 0 {
		t.Errorf("close_scores jobs = %d, want 0 (both firings) when disabled", len(matches))
	}
}

// TestJobNamesMatchSettingsJobs guards against scheduler.go and
// settings.Jobs drifting apart: every name buildJobs can ever produce
// (with everything enabled) must be described in settings.Jobs, and vice
// versa, or the portal's toggle list would silently omit or mislabel a
// real scheduled message.
func TestJobNamesMatchSettingsJobs(t *testing.T) {
	cfg := baseConfig()
	cfg.MonitorReport = true
	cfg.LLM.APIKey = "key" // so recap is included too

	seen := map[string]bool{}
	for _, j := range buildJobs(cfg, settings.Settings{}) {
		seen[j.name] = true
	}

	want := map[string]bool{}
	for _, j := range settings.Jobs {
		want[j.Name] = true
	}

	if len(seen) != len(want) {
		t.Fatalf("buildJobs produced %d distinct names, settings.Jobs describes %d", len(seen), len(want))
	}
	var missing []string
	for name := range want {
		if !seen[name] {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("settings.Jobs describes names buildJobs never produces: %v", missing)
	}
}
