package discordbot

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// weekdayNames indexes 0=Sunday..6=Saturday, matching both time.Weekday and
// settings.Settings.WaiverDays.
var weekdayNames = [7]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}

// weekdayAliases maps every name/abbreviation parseWaiverDays accepts to its
// 0-6 day number.
var weekdayAliases = map[string]int{
	"sun": 0, "sunday": 0,
	"mon": 1, "monday": 1,
	"tue": 2, "tues": 2, "tuesday": 2,
	"wed": 3, "weds": 3, "wednesday": 3,
	"thu": 4, "thur": 4, "thurs": 4, "thursday": 4,
	"fri": 5, "friday": 5,
	"sat": 6, "saturday": 6,
}

// parseWaiverDays parses a free-text, comma/space-separated list of weekdays
// - names, 3-letter abbreviations, or 0-6 numbers matching time.Weekday
// (Sunday=0) - or the special tokens "all"/"none", into a sorted,
// deduplicated day set. It's the slash-command equivalent of the web
// portal's seven day checkboxes, fitted into a single string option.
func parseWaiverDays(input string) ([]int, error) {
	fields := strings.FieldsFunc(input, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' })
	if len(fields) == 0 {
		return nil, fmt.Errorf(`give at least one day (e.g. "wed" or "sun,wed,fri"), "all", or "none"`)
	}
	if len(fields) == 1 {
		switch strings.ToLower(fields[0]) {
		case "all":
			return []int{0, 1, 2, 3, 4, 5, 6}, nil
		case "none":
			return []int{}, nil
		}
	}

	seen := make(map[int]bool, len(fields))
	for _, f := range fields {
		if d, ok := weekdayAliases[strings.ToLower(f)]; ok {
			seen[d] = true
			continue
		}
		if n, err := strconv.Atoi(f); err == nil && n >= 0 && n <= 6 {
			seen[n] = true
			continue
		}
		return nil, fmt.Errorf("%q isn't a day I recognize - use day names (sun, mon, tue, wed, thu, fri, sat), numbers 0-6 (Sunday=0), \"all\", or \"none\"", f)
	}

	days := make([]int, 0, len(seen))
	for d := range seen {
		days = append(days, d)
	}
	sort.Ints(days)
	return days, nil
}

// formatWaiverDays renders a day set for display using parseWaiverDays's own
// vocabulary, so a value shown by /settings view can be pasted straight
// back into /settings waiver-days.
func formatWaiverDays(days []int) string {
	switch len(days) {
	case 0:
		return "none"
	case 7:
		return "all"
	}
	names := make([]string, len(days))
	for i, d := range days {
		names[i] = weekdayNames[d][:3]
	}
	return strings.Join(names, ", ")
}

// buildView renders the current effective settings as Discord markdown, an
// ephemeral equivalent of the web portal's dashboard.
func buildView(cfg *config.Config, cur settings.Settings) string {
	var b strings.Builder

	fmt.Fprintf(&b, "**Timezone:** %s%s\n", cur.EffectiveTimezone(cfg.Timezone), overrideNote(cur.Timezone != nil))
	fmt.Fprintf(&b, "**Waiver report days:** %s%s\n", formatWaiverDays(cur.EffectiveWaiverDays(settings.DefaultWaiverDays(cfg))), overrideNote(cur.WaiverDays != nil))
	fmt.Fprintf(&b, "**Recap prompt:** %s%s\n", truncate(cur.EffectiveRecapPrompt(settings.DefaultRecapPrompt), 150), overrideNote(cur.RecapPrompt != nil))

	b.WriteString("\n**Scheduled messages:**\n")
	for _, j := range settings.Jobs {
		_, overridden := cur.JobEnabled[j.Name]
		mark := "🟢"
		if !cur.JobIsEnabled(j.Name, settings.DefaultJobEnabled(j.Name, cfg)) {
			mark = "🔴"
		}
		fmt.Fprintf(&b, "%s %s%s\n", mark, j.Label, overrideNote(overridden))
	}

	return b.String()
}

func overrideNote(isOverride bool) string {
	if isOverride {
		return " _(override)_"
	}
	return ""
}

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

// applyJobToggle returns cur with job's enabled override set, leaving every
// other override untouched. It copies cur.JobEnabled rather than mutating it
// in place, since that map is shared with whatever the caller's
// settings.Manager.Get() returned.
func applyJobToggle(cur settings.Settings, job string, enabled bool) settings.Settings {
	next := cur
	jobEnabled := make(map[string]bool, len(cur.JobEnabled)+1)
	for k, v := range cur.JobEnabled {
		jobEnabled[k] = v
	}
	jobEnabled[job] = enabled
	next.JobEnabled = jobEnabled
	return next
}

func applyWaiverDays(cur settings.Settings, days []int) settings.Settings {
	next := cur
	next.WaiverDays = &days
	return next
}

func applyTimezone(cur settings.Settings, tz string) settings.Settings {
	next := cur
	next.Timezone = &tz
	return next
}

func applyRecapPrompt(cur settings.Settings, prompt string) settings.Settings {
	next := cur
	next.RecapPrompt = &prompt
	return next
}

// applyReset mirrors the web portal's "reset_all"/"reset_prompt" actions
// (see internal/portal/handlers.go's handleSaveSettings).
func applyReset(cur settings.Settings, scope string) (settings.Settings, error) {
	switch scope {
	case "all":
		return settings.Settings{}, nil
	case "recap_prompt":
		next := cur
		next.RecapPrompt = nil
		return next, nil
	default:
		return settings.Settings{}, fmt.Errorf("unknown reset scope %q", scope)
	}
}
