package portal

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"fantasy_bot/internal/settings"
)

// commonTimezones seeds the timezone field's autocomplete list; the input
// itself stays free-text so any valid IANA zone works.
var commonTimezones = []string{
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Phoenix",
	"America/Los_Angeles",
	"America/Anchorage",
	"Pacific/Honolulu",
	"UTC",
}

var weekdayLabels = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

type loginData struct {
	Error string
}

type jobRow struct {
	Name        string
	Label       string
	Description string
	Checked     bool
	IsOverride  bool
}

type dayRow struct {
	Value   int
	Label   string
	Checked bool
}

type dashboardData struct {
	CSRFToken string
	Error     string
	Saved     bool

	Jobs []jobRow

	WaiverDays           []dayRow
	WaiverDaysIsOverride bool

	Timezone           string
	TimezoneIsOverride bool
	EnvTimezone        string
	CommonTimezones    []string

	RecapPrompt           string
	RecapPromptIsOverride bool
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	s.render(w, "login", loginData{})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	if !passwordMatches(r.FormValue("password"), s.cfg.PortalPassword) {
		time.Sleep(500 * time.Millisecond) // slow down brute-force guessing
		s.render(w, "login", loginData{Error: "Incorrect password."})
		return
	}

	token, _, err := s.sessions.create()
	if err != nil {
		log.Printf("portal: create session: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(sessionDuration),
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request, sess sessionInfo) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		s.sessions.destroy(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request, sess sessionInfo) {
	saved := r.URL.Query().Get("saved") == "1"
	s.render(w, "dashboard", s.buildDashboardData(s.mgr.Get(), sess.csrfToken, "", saved))
}

func (s *Server) handleSaveSettings(w http.ResponseWriter, r *http.Request, sess sessionInfo) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	if !checkCSRF(r.FormValue("csrf_token"), sess.csrfToken) {
		http.Error(w, "invalid or missing CSRF token - reload the page and try again", http.StatusForbidden)
		return
	}

	var next settings.Settings
	switch action := r.FormValue("action"); action {
	case "reset_all":
		next = settings.Settings{}
	case "reset_prompt":
		next = s.mgr.Get()
		next.RecapPrompt = nil
	case "save":
		next = settingsFromForm(r)
	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
		return
	}

	if err := s.mgr.Save(next); err != nil {
		s.render(w, "dashboard", s.buildDashboardData(next, sess.csrfToken, err.Error(), false))
		return
	}
	http.Redirect(w, r, "/?saved=1", http.StatusSeeOther)
}

// settingsFromForm builds a full Settings snapshot from the dashboard
// form's POST body. The form always renders every job/day checkbox and
// pre-fills the timezone/prompt fields with their effective values, so a
// save is always a complete replacement, never a partial patch.
func settingsFromForm(r *http.Request) settings.Settings {
	var next settings.Settings

	if tzInput := strings.TrimSpace(r.FormValue("timezone")); tzInput != "" {
		next.Timezone = &tzInput
	}

	// Must start non-nil even when nothing is checked, or this collapses to
	// "no override" once it round-trips through JSON - see the WaiverDays
	// doc comment in internal/settings.
	days := make([]int, 0, 7)
	for d := 0; d <= 6; d++ {
		if r.FormValue(fmt.Sprintf("waiver_day_%d", d)) != "" {
			days = append(days, d)
		}
	}
	next.WaiverDays = &days

	jobEnabled := make(map[string]bool, len(settings.Jobs))
	for _, j := range settings.Jobs {
		jobEnabled[j.Name] = r.FormValue("job_"+j.Name) != ""
	}
	next.JobEnabled = jobEnabled

	if prompt := strings.TrimSpace(r.FormValue("recap_prompt")); prompt != "" {
		next.RecapPrompt = &prompt
	}

	return next
}

// buildDashboardData computes what the dashboard template needs from cur
// (either the persisted settings, for a normal page load, or a
// just-submitted-but-rejected snapshot, so a validation error preserves
// what the user typed instead of reverting to the last saved state).
func (s *Server) buildDashboardData(cur settings.Settings, csrfToken, errMsg string, saved bool) dashboardData {
	cfg := s.cfg

	jobs := make([]jobRow, 0, len(settings.Jobs))
	for _, j := range settings.Jobs {
		_, overridden := cur.JobEnabled[j.Name]
		jobs = append(jobs, jobRow{
			Name:        j.Name,
			Label:       j.Label,
			Description: j.Description,
			Checked:     cur.JobIsEnabled(j.Name, settings.DefaultJobEnabled(j.Name, cfg)),
			IsOverride:  overridden,
		})
	}

	effectiveDays := cur.EffectiveWaiverDays(settings.DefaultWaiverDays(cfg))
	effectiveSet := make(map[int]bool, len(effectiveDays))
	for _, d := range effectiveDays {
		effectiveSet[d] = true
	}
	days := make([]dayRow, 7)
	for d := 0; d < 7; d++ {
		days[d] = dayRow{Value: d, Label: weekdayLabels[d], Checked: effectiveSet[d]}
	}

	return dashboardData{
		CSRFToken: csrfToken,
		Error:     errMsg,
		Saved:     saved,

		Jobs: jobs,

		WaiverDays:           days,
		WaiverDaysIsOverride: cur.WaiverDays != nil,

		Timezone:           cur.EffectiveTimezone(cfg.Timezone),
		TimezoneIsOverride: cur.Timezone != nil,
		EnvTimezone:        cfg.Timezone,
		CommonTimezones:    commonTimezones,

		RecapPrompt:           cur.EffectiveRecapPrompt(settings.DefaultRecapPrompt),
		RecapPromptIsOverride: cur.RecapPrompt != nil,
	}
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("portal: render %s: %v", name, err)
	}
}
