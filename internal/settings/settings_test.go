package settings

import (
	"os"
	"path/filepath"
	"testing"

	"fantasy_bot/internal/config"
)

func strPtr(s string) *string { return &s }

func TestLoadMissingFileReturnsZeroValue(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mgr.Get(); got.Timezone != nil || got.WaiverDays != nil || got.RecapPrompt != nil || len(got.JobEnabled) != 0 {
		t.Errorf("Get() = %+v, want zero value", got)
	}
}

func TestLoadCorruptFileFallsBackToDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := mgr.Get(); got.Timezone != nil {
		t.Errorf("Get() = %+v, want zero value after corrupt file", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mgr, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	days := []int{1, 3, 5}
	want := Settings{
		Timezone:    strPtr("America/Chicago"),
		WaiverDays:  &days,
		JobEnabled:  map[string]bool{"monitor": false, "recap": true},
		RecapPrompt: strPtr("Be extremely sarcastic."),
	}
	if err := mgr.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Get()

	if got.EffectiveTimezone("x") != "America/Chicago" {
		t.Errorf("Timezone = %v, want America/Chicago", got.Timezone)
	}
	if got.EffectiveRecapPrompt("x") != "Be extremely sarcastic." {
		t.Errorf("RecapPrompt = %v", got.RecapPrompt)
	}
	if !got.JobIsEnabled("recap", false) {
		t.Error("JobIsEnabled(recap) = false, want true")
	}
	if got.JobIsEnabled("monitor", true) {
		t.Error("JobIsEnabled(monitor) = true, want false")
	}
	gotDays := got.EffectiveWaiverDays(nil)
	if len(gotDays) != 3 || gotDays[0] != 1 || gotDays[1] != 3 || gotDays[2] != 5 {
		t.Errorf("EffectiveWaiverDays = %v, want [1 3 5]", gotDays)
	}
}

// TestWaiverDaysEmptyVsNilOverride locks down the sharp edge documented on
// Settings.WaiverDays: an explicit "post on no days" (non-nil pointer to an
// empty slice) must survive a save/load round trip distinctly from "no
// override at all" (nil pointer), which would otherwise silently revert to
// the .env-derived default.
func TestWaiverDaysEmptyVsNilOverride(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	mgr, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	empty := make([]int, 0, 7) // non-nil, deliberately - see WaiverDays doc
	if err := mgr.Save(Settings{WaiverDays: &empty}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reloaded.Get()
	if got.WaiverDays == nil {
		t.Fatal("WaiverDays = nil after saving an explicit empty selection, want non-nil")
	}
	days := got.EffectiveWaiverDays([]int{3})
	if len(days) != 0 {
		t.Errorf("EffectiveWaiverDays = %v, want empty (not falling back to default)", days)
	}
}

func TestEffectiveFallbacksWhenUnset(t *testing.T) {
	var s Settings
	if got := s.EffectiveTimezone("America/New_York"); got != "America/New_York" {
		t.Errorf("EffectiveTimezone = %q, want fallback", got)
	}
	if got := s.EffectiveRecapPrompt("default prompt"); got != "default prompt" {
		t.Errorf("EffectiveRecapPrompt = %q, want fallback", got)
	}
	if !s.JobIsEnabled("standings", true) {
		t.Error("JobIsEnabled should fall back to def when unset")
	}
	if got := s.EffectiveWaiverDays([]int{3}); len(got) != 1 || got[0] != 3 {
		t.Errorf("EffectiveWaiverDays = %v, want fallback [3]", got)
	}
}

func TestSaveRejectsInvalidTimezone(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.Save(Settings{Timezone: strPtr("Not/AZone")})
	if err == nil {
		t.Fatal("expected error for invalid timezone")
	}
	if got := mgr.Get(); got.Timezone != nil {
		t.Error("Save should not apply settings that fail validation")
	}
}

func TestSaveRejectsUnknownJob(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	err = mgr.Save(Settings{JobEnabled: map[string]bool{"not_a_real_job": true}})
	if err == nil {
		t.Fatal("expected error for unknown job name")
	}
}

func TestSaveRejectsOutOfRangeWaiverDay(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	days := []int{7}
	err = mgr.Save(Settings{WaiverDays: &days})
	if err == nil {
		t.Fatal("expected error for out-of-range waiver day")
	}
}

func TestSubscribeNotifiedInSaveOrder(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var seen []string
	mgr.Subscribe(func(s Settings) {
		if s.RecapPrompt != nil {
			seen = append(seen, *s.RecapPrompt)
		}
	})

	if err := mgr.Save(Settings{RecapPrompt: strPtr("first")}); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Save(Settings{RecapPrompt: strPtr("second")}); err != nil {
		t.Fatal(err)
	}

	if len(seen) != 2 || seen[0] != "first" || seen[1] != "second" {
		t.Errorf("subscriber saw %v, want [first second]", seen)
	}
}

func TestSubscribeCalledBeforeSaveNotMissed(t *testing.T) {
	mgr, err := Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	called := false
	mgr.Subscribe(func(Settings) { called = true })
	if err := mgr.Save(Settings{Timezone: strPtr("UTC")}); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Error("subscriber registered before Save was not notified")
	}
}

func TestDefaultJobEnabled(t *testing.T) {
	cfg := &config.Config{MonitorReport: false, LLM: config.LLMConfig{APIKey: ""}}
	if DefaultJobEnabled("monitor", cfg) {
		t.Error("monitor default should follow cfg.MonitorReport = false")
	}
	if DefaultJobEnabled("recap", cfg) {
		t.Error("recap default should be false with no LLM API key")
	}
	if !DefaultJobEnabled("standings", cfg) {
		t.Error("standings should default to enabled")
	}

	cfg2 := &config.Config{MonitorReport: true, LLM: config.LLMConfig{APIKey: "key"}}
	if !DefaultJobEnabled("monitor", cfg2) {
		t.Error("monitor default should follow cfg.MonitorReport = true")
	}
	if !DefaultJobEnabled("recap", cfg2) {
		t.Error("recap default should be true when an LLM API key is set")
	}
}

func TestDefaultWaiverDays(t *testing.T) {
	if got := DefaultWaiverDays(&config.Config{DailyWaiver: false}); len(got) != 1 || got[0] != 3 {
		t.Errorf("DefaultWaiverDays(false) = %v, want [3]", got)
	}
	if got := DefaultWaiverDays(&config.Config{DailyWaiver: true}); len(got) != 7 {
		t.Errorf("DefaultWaiverDays(true) = %v, want all 7 days", got)
	}
}
