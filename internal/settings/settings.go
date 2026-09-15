// Package settings holds the runtime-configurable overrides exposed by the
// web portal (internal/portal): which scheduled messages are enabled, which
// day(s) the waiver report posts on, the timezone, and the AI weekly
// recap's system prompt. Anything left unset here falls back to the
// .env-derived config.Config value - see the Effective*/JobIsEnabled
// methods and DefaultJobEnabled/DefaultWaiverDays, which are the single
// source of truth for what "unset" resolves to, shared by
// internal/scheduler (to decide what actually runs) and internal/portal
// (to show the user what's a default vs. an override) so neither can drift
// from the other.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"fantasy_bot/internal/config"
)

// Settings holds the portal's overrides. Every field is optional: a nil
// pointer (or absent map key) means "no override, use the .env-derived
// default".
type Settings struct {
	// Timezone overrides config.Config.Timezone when set.
	Timezone *string `json:"timezone,omitempty"`

	// WaiverDays overrides which weekdays the waiver report posts on
	// (0=Sunday..6=Saturday, matching both time.Weekday and cron's
	// day-of-week field). nil means "no override". WaiverDays
	// distinguishes three states through a pointer-to-slice: nil (no
	// override), pointer-to-empty-slice (explicitly zero days, i.e. never
	// post), pointer-to-populated-slice (explicit days). A pointer to a
	// NIL slice marshals to the same JSON ("null") as a nil pointer, so
	// code building this from checkboxes must start from a non-nil slice
	// (e.g. make([]int, 0, 7)) before taking its address, even when
	// nothing gets appended - see portal/handlers.go.
	WaiverDays *[]int `json:"waiver_days"`

	// JobEnabled overrides whether a named scheduled job runs (see Jobs
	// below for the canonical set of names). A missing key means "no
	// override, use DefaultJobEnabled".
	JobEnabled map[string]bool `json:"job_enabled,omitempty"`

	// RecapPrompt overrides the AI weekly recap's system prompt.
	RecapPrompt *string `json:"recap_prompt,omitempty"`
}

// JobInfo describes one schedulable message for display in the portal.
// internal/scheduler's job table holds the actual cron spec and report
// type for each; it must use exactly these names (enforced by
// scheduler's TestJobNamesMatchSettings).
type JobInfo struct {
	Name        string
	Label       string
	Description string
}

// Jobs is the canonical, ordered list of scheduled messages the portal can
// toggle on/off, matching README.md's schedule table.
var Jobs = []JobInfo{
	{"standings", "Standings", "Wednesday 7:30 AM league time — current win-loss-tie standings."},
	{"win_matrix", "Win Matrix", "Wednesday 7:30 AM league time — standings if every team had played every other team every week."},
	{"waiver_report", "Waiver Report", "Completed waiver/free-agent adds and drops with FAAB bids. Which day(s) it posts on is configured separately below."},
	{"matchups", "Matchups", "Thursday 7:30 PM ET — the upcoming week's pairings plus each matchup's projected scoreboard."},
	{"scoreboard_weekday", "Weekday Scoreboard", "Friday & Monday 7:30 AM league time — current scores plus each matchup's approximate projected final score."},
	{"monitor", "Injury Monitor", "Sunday 7:30 AM league time — rostered players who are OUT, flagged Doubtful/IR-eligible, or sitting in an IR slot without a qualifying designation."},
	{"scoreboard", "Scoreboard", "Sunday 4 PM & 8 PM ET — in-progress scores for every matchup."},
	{"close_scores", "Close Scores", "Sunday 4 PM & 8 PM ET, and Monday 6:30 PM ET — matchups still within the close-game threshold."},
	{"final", "Final", "Tuesday 7:30 AM league time — final scores and trophies for the week that just finished."},
	{"trophy_case", "Trophy Case", "Tuesday 9:00 AM league time — season-long crosstab image of every team's trophy counts, tallied through the week Final just posted."},
	{"power_rankings", "Power Rankings", "Tuesday 6:30 PM league time — season-long power ranking score and rank for each team, plus a season-trend chart image."},
	{"fortune_index", "Fortune Index", "Tuesday 6:31 PM league time — schedule-luck ranking: who's over/underperformed based on opponent strength."},
	{"recap", "AI Weekly Recap", "Tuesday 6:45 PM league time — LLM-written newsletter. Requires LLM_API_KEY in .env; only whether it's posted and its prompt are configurable here."},
}

// DefaultRecapPrompt is the AI weekly recap's system prompt when the
// portal hasn't overridden it (see Settings.RecapPrompt and
// bot.ReportRecap) - the persona/instructions given to the LLM; the league
// data itself is the user prompt (see report.LeagueContext.RecapDigest).
// It lives here, rather than in internal/bot where it's used, so
// internal/portal can display and reset to it without importing bot.
const DefaultRecapPrompt = `You are the ghostwriter for a fantasy football league commissioner's weekly newsletter, posted to the league's Discord. You write for people who already know the league - be specific, use team names, and have fun with it.

You'll be given a data digest covering: current standings, active win/loss streaks, season-long head-to-head series between teams that have played more than once, the playoff race, and the week's biggest lineup-decision regret.

Write a short newsletter (250-400 words) that is the ONLY message all season that looks across weeks rather than just at the week that just finished. Hit these beats, in whatever order reads best:
- Call out any notable streaks (hot teams, cold teams).
- If there's a season head-to-head series, mention the rivalry and who's winning it. If there isn't one yet, skip that beat entirely rather than forcing it.
- Give an honest read on the playoff race: who's comfortably in, who's on the bubble, who's in trouble.
- Land on the week's lineup regret as a specific, slightly roasting anecdote about the manager involved.

Use Discord markdown (bold with **double asterisks**, a heading or two, maybe a couple of well-placed emoji) but do NOT wrap anything in a code block or backticks. Sign off as "The Commish". Do not invent facts, scores, or players beyond what's in the digest.`

// DefaultJobEnabled reports whether the named job runs when no portal
// override is set, derived from cfg exactly as the scheduler decided
// before per-job overrides existed.
func DefaultJobEnabled(name string, cfg *config.Config) bool {
	switch name {
	case "monitor":
		return cfg.MonitorReport
	case "recap":
		return cfg.LLM.APIKey != ""
	default:
		return true
	}
}

// DefaultWaiverDays is the weekday set the waiver report posts on when no
// portal override is set: every day if cfg.DailyWaiver, Wednesday only
// otherwise.
func DefaultWaiverDays(cfg *config.Config) []int {
	if cfg.DailyWaiver {
		return []int{0, 1, 2, 3, 4, 5, 6}
	}
	return []int{3}
}

// EffectiveTimezone returns the portal override if set, else base.
func (s Settings) EffectiveTimezone(base string) string {
	if s.Timezone != nil && *s.Timezone != "" {
		return *s.Timezone
	}
	return base
}

// EffectiveRecapPrompt returns the portal override if set, else base.
func (s Settings) EffectiveRecapPrompt(base string) string {
	if s.RecapPrompt != nil && *s.RecapPrompt != "" {
		return *s.RecapPrompt
	}
	return base
}

// JobIsEnabled returns the portal override for name if set, else def.
func (s Settings) JobIsEnabled(name string, def bool) bool {
	if v, ok := s.JobEnabled[name]; ok {
		return v
	}
	return def
}

// EffectiveWaiverDays returns the portal override if set (including an
// explicit empty selection, meaning "never"), else def.
func (s Settings) EffectiveWaiverDays(def []int) []int {
	if s.WaiverDays != nil {
		return *s.WaiverDays
	}
	return def
}

// isKnownJob reports whether name is one of Jobs.
func isKnownJob(name string) bool {
	for _, j := range Jobs {
		if j.Name == name {
			return true
		}
	}
	return false
}

// validate rejects Settings that can't have come from the portal's own
// form (an out-of-range waiver day, an unknown job name, an unloadable
// timezone) - guarding against a hand-edited or corrupted settings file on
// disk, since Save and Load both funnel through this.
func validate(s Settings) error {
	if s.Timezone != nil {
		if _, err := time.LoadLocation(*s.Timezone); err != nil {
			return fmt.Errorf("settings: invalid timezone %q: %w", *s.Timezone, err)
		}
	}
	if s.WaiverDays != nil {
		for _, d := range *s.WaiverDays {
			if d < 0 || d > 6 {
				return fmt.Errorf("settings: invalid waiver day %d (want 0-6)", d)
			}
		}
	}
	for name := range s.JobEnabled {
		if !isKnownJob(name) {
			return fmt.Errorf("settings: unknown job %q", name)
		}
	}
	return nil
}

// Manager loads, persists, and serves the current Settings, notifying
// subscribers (see Subscribe) after every successful Save.
type Manager struct {
	mu          sync.RWMutex
	current     Settings
	path        string
	subscribers []func(Settings)
}

// Load reads Settings from path. A missing file is not an error (returns a
// Manager with zero-value Settings); a file that fails to parse, or parses
// but fails validate, logs a warning and also falls back to zero-value,
// rather than failing bot startup over a corrupt settings file.
func Load(path string) (*Manager, error) {
	m := &Manager{path: path}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, fmt.Errorf("settings: read %s: %w", path, err)
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		log.Printf("settings: %s is corrupt, starting with .env defaults: %v", path, err)
		return m, nil
	}
	if err := validate(s); err != nil {
		log.Printf("settings: %s failed validation, starting with .env defaults: %v", path, err)
		return m, nil
	}
	m.current = s
	return m, nil
}

// Get returns the current settings.
func (m *Manager) Get() Settings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// Subscribe registers fn to be called, with the new Settings, after every
// successful Save. Call Subscribe before any initial setup that depends on
// Settings, so a Save landing in between can't be missed.
func (m *Manager) Subscribe(fn func(Settings)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribers = append(m.subscribers, fn)
}

// Save validates, persists (atomically), and applies s, then notifies every
// subscriber in order. Subscribers run while the write lock is held,
// deliberately: this serializes notifications in Save-call order, so two
// near-simultaneous Saves can't notify out of order and let an older one
// silently clobber a newer one.
func (m *Manager) Save(s Settings) error {
	if err := validate(s); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("settings: marshal: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if err := writeAtomic(m.path, data); err != nil {
		return err
	}
	m.current = s
	for _, fn := range m.subscribers {
		fn(s)
	}
	return nil
}

// writeAtomic writes data to path via a temp file + rename, so a crash
// mid-write can't leave a corrupt settings file, creating path's parent
// directory first if needed (e.g. Docker's /data volume mount).
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("settings: create %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("settings: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once renamed into place

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("settings: write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("settings: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("settings: rename into place: %w", err)
	}
	return nil
}
