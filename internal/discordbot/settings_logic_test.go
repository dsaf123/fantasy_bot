package discordbot

import (
	"reflect"
	"strings"
	"testing"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

func TestParseWaiverDaysNames(t *testing.T) {
	got, err := parseWaiverDays("wed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{3}) {
		t.Errorf("got %v, want [3]", got)
	}
}

func TestParseWaiverDaysMixedListAndDedup(t *testing.T) {
	got, err := parseWaiverDays("Sun, wed fri, 3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(got, []int{0, 3, 5}) {
		t.Errorf("got %v, want [0 3 5] (sorted, deduplicated)", got)
	}
}

func TestParseWaiverDaysAllAndNone(t *testing.T) {
	all, err := parseWaiverDays("all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(all) != 7 {
		t.Errorf("all = %v, want all 7 days", all)
	}

	none, err := parseWaiverDays("none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if none == nil || len(none) != 0 {
		t.Errorf("none = %v, want a non-nil empty slice", none)
	}
}

func TestParseWaiverDaysRejectsUnknownToken(t *testing.T) {
	if _, err := parseWaiverDays("wed,funday"); err == nil {
		t.Fatal("expected error for unrecognized day token")
	}
}

func TestParseWaiverDaysRejectsEmptyInput(t *testing.T) {
	if _, err := parseWaiverDays("   "); err == nil {
		t.Fatal("expected error for blank input")
	}
}

func TestParseWaiverDaysRejectsOutOfRangeNumber(t *testing.T) {
	if _, err := parseWaiverDays("7"); err == nil {
		t.Fatal("expected error for out-of-range day number")
	}
}

func TestFormatWaiverDaysRoundTripsWithParse(t *testing.T) {
	cases := [][]int{{}, {0, 1, 2, 3, 4, 5, 6}, {1, 3}, {0}}
	for _, days := range cases {
		formatted := formatWaiverDays(days)
		reparsed, err := parseWaiverDays(formatted)
		if err != nil {
			t.Fatalf("formatWaiverDays(%v) = %q, which failed to reparse: %v", days, formatted, err)
		}
		if len(reparsed) != len(days) {
			t.Errorf("round trip of %v via %q gave %v", days, formatted, reparsed)
		}
	}
}

func TestApplyJobTogglePreservesOtherOverrides(t *testing.T) {
	cfg := &config.Config{} // recap's default is false (LLM.APIKey == "")
	cur := settings.Settings{JobEnabled: map[string]bool{"monitor": false}}
	next := applyJobToggle(cur, "recap", true, cfg)

	if !next.JobIsEnabled("recap", false) {
		t.Error("recap should be enabled in the result")
	}
	if next.JobIsEnabled("monitor", true) {
		t.Error("monitor override should be preserved as disabled")
	}
	if cur.JobEnabled["recap"] {
		t.Error("applyJobToggle must not mutate the map backing the caller's original Settings")
	}
}

// TestApplyJobToggleMatchingDefaultClearsOverride guards against /settings
// view getting stuck showing "(override)" for a job the admin explicitly
// set back to its .env-derived default - see the doc comment on
// applyJobToggle for why this must clear the stored override, not just
// skip adding a new one.
func TestApplyJobToggleMatchingDefaultClearsOverride(t *testing.T) {
	cfg := &config.Config{MonitorReport: true}
	cur := settings.Settings{JobEnabled: map[string]bool{"monitor": false, "recap": true}}

	next := applyJobToggle(cur, "monitor", true, cfg) // true matches DefaultJobEnabled("monitor", cfg)

	if _, overridden := next.JobEnabled["monitor"]; overridden {
		t.Error("monitor should no longer be a stored override once set back to its default")
	}
	if !next.JobIsEnabled("monitor", true) {
		t.Error("monitor should still resolve to enabled")
	}
	if !next.JobIsEnabled("recap", false) {
		t.Error("recap's unrelated override should be untouched")
	}
}

func TestApplyWaiverDaysExplicitEmptyIsNotNil(t *testing.T) {
	cfg := &config.Config{} // default is {3} (Wednesday), never empty
	next := applyWaiverDays(settings.Settings{}, []int{}, cfg)
	if next.WaiverDays == nil {
		t.Fatal("WaiverDays should be a non-nil pointer even for an empty selection (see settings.Settings doc)")
	}
	if len(*next.WaiverDays) != 0 {
		t.Errorf("WaiverDays = %v, want empty", *next.WaiverDays)
	}
}

func TestApplyWaiverDaysMatchingDefaultClearsOverride(t *testing.T) {
	cfg := &config.Config{} // DailyWaiver=false -> default is {3}
	cur := settings.Settings{WaiverDays: &[]int{1, 3}}

	next := applyWaiverDays(cur, []int{3}, cfg)

	if next.WaiverDays != nil {
		t.Errorf("WaiverDays = %v, want nil once it matches the .env default", *next.WaiverDays)
	}
}

func TestApplyTimezoneMatchingDefaultClearsOverride(t *testing.T) {
	cfg := &config.Config{Timezone: "America/New_York"}
	cur := settings.Settings{Timezone: strPtr("America/Chicago")}

	next := applyTimezone(cur, "America/New_York", cfg)

	if next.Timezone != nil {
		t.Errorf("Timezone = %v, want nil once it matches the .env default", *next.Timezone)
	}
}

func TestApplyRecapPromptMatchingDefaultClearsOverride(t *testing.T) {
	cur := settings.Settings{RecapPrompt: strPtr("custom")}

	next := applyRecapPrompt(cur, settings.DefaultRecapPrompt)

	if next.RecapPrompt != nil {
		t.Error("RecapPrompt should be nil once set back to settings.DefaultRecapPrompt")
	}
}

func TestApplyResetAll(t *testing.T) {
	cur := settings.Settings{RecapPrompt: strPtr("custom"), JobEnabled: map[string]bool{"monitor": false}}
	next, err := applyReset(cur, "all")
	if err != nil {
		t.Fatal(err)
	}
	if next.RecapPrompt != nil || next.JobEnabled != nil || next.Timezone != nil || next.WaiverDays != nil {
		t.Errorf("applyReset(all) = %+v, want zero value", next)
	}
}

func TestApplyResetRecapPromptOnly(t *testing.T) {
	tz := "America/Chicago"
	cur := settings.Settings{RecapPrompt: strPtr("custom"), Timezone: &tz}
	next, err := applyReset(cur, "recap_prompt")
	if err != nil {
		t.Fatal(err)
	}
	if next.RecapPrompt != nil {
		t.Error("RecapPrompt should be cleared")
	}
	if next.EffectiveTimezone("") != tz {
		t.Error("resetting the recap prompt should not touch the timezone override")
	}
}

func TestApplyResetUnknownScope(t *testing.T) {
	if _, err := applyReset(settings.Settings{}, "bogus"); err == nil {
		t.Fatal("expected error for unknown reset scope")
	}
}

func strPtr(s string) *string { return &s }

func TestBuildViewIncludesOverrideMarkers(t *testing.T) {
	cfg := &config.Config{Timezone: "America/New_York", MonitorReport: true}
	tz := "America/Chicago"
	cur := settings.Settings{
		Timezone:   &tz,
		JobEnabled: map[string]bool{"monitor": false},
	}

	view := buildView(cfg, cur)

	if !strings.Contains(view, "America/Chicago") {
		t.Error("view should show the overridden timezone")
	}
	if !strings.Contains(view, "Injury Monitor") || !strings.Contains(view, "🔴") {
		t.Error("view should show the disabled monitor job")
	}
}
