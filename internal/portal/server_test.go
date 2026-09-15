package portal

import (
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"fantasy_bot/internal/config"
	"fantasy_bot/internal/settings"
)

// newTestServer spins up a real httptest.Server backed by a fresh
// settings.Manager (persisted to a temp file), and an http.Client with a
// cookie jar so login sessions carry across requests like a real browser.
func newTestServer(t *testing.T) (*httptest.Server, *http.Client, *settings.Manager, *config.Config) {
	t.Helper()

	mgr, err := settings.Load(filepath.Join(t.TempDir(), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Timezone:       "America/New_York",
		MonitorReport:  true,
		PortalPassword: "s3cret",
	}

	srv := httptest.NewServer(newServer(cfg, mgr).mux())
	t.Cleanup(srv.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // inspect redirects instead of following them
		},
	}
	return srv, client, mgr, cfg
}

func login(t *testing.T, srv *httptest.Server, client *http.Client, password string) *http.Response {
	t.Helper()
	resp, err := client.PostForm(srv.URL+"/login", url.Values{"password": {password}})
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUnauthenticatedDashboardRedirectsToLogin(t *testing.T) {
	srv, client, _, _ := newTestServer(t)

	resp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/login" {
		t.Errorf("status=%d location=%q, want 303 to /login", resp.StatusCode, resp.Header.Get("Location"))
	}
}

func TestLoginWrongPasswordRejected(t *testing.T) {
	srv, client, _, _ := newTestServer(t)

	resp := login(t, srv, client, "wrong")
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "Incorrect password") {
		t.Errorf("body = %q, want an incorrect-password message", body)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookieName {
			t.Error("a session cookie should not be set on a failed login")
		}
	}
}

func TestLoginCorrectPasswordThenDashboardLoads(t *testing.T) {
	srv, client, _, _ := newTestServer(t)

	resp := login(t, srv, client, "s3cret")
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("login status=%d location=%q, want 303 to /", resp.StatusCode, resp.Header.Get("Location"))
	}

	dash, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer dash.Body.Close()
	if dash.StatusCode != http.StatusOK {
		t.Fatalf("dashboard status = %d, want 200 after login", dash.StatusCode)
	}
	body, _ := io.ReadAll(dash.Body)
	if !strings.Contains(string(body), "Scheduled messages") {
		t.Error("dashboard body missing expected content")
	}
}

// csrfToken logs in and scrapes the CSRF token out of the rendered
// dashboard's hidden field, mirroring what a real browser's form submit
// would send.
func csrfToken(t *testing.T, srv *httptest.Server, client *http.Client) string {
	t.Helper()
	login(t, srv, client, "s3cret").Body.Close()

	resp, err := client.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	const marker = `name="csrf_token" value="`
	i := strings.Index(string(body), marker)
	if i == -1 {
		t.Fatal("csrf_token field not found in dashboard HTML")
	}
	rest := string(body)[i+len(marker):]
	return rest[:strings.Index(rest, `"`)]
}

func TestSaveSettingsPersistsAndReflectsInManager(t *testing.T) {
	srv, client, mgr, _ := newTestServer(t)
	token := csrfToken(t, srv, client)

	form := url.Values{
		"csrf_token":    {token},
		"action":        {"save"},
		"timezone":      {"America/Chicago"},
		"waiver_day_1":  {"on"},
		"waiver_day_3":  {"on"},
		"job_standings": {"on"},
		// monitor deliberately left unchecked to verify it's saved as disabled
		"recap_prompt": {"Be brief."},
	}
	resp, err := client.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save status = %d, want 303", resp.StatusCode)
	}

	got := mgr.Get()
	if got.EffectiveTimezone("") != "America/Chicago" {
		t.Errorf("Timezone = %v, want America/Chicago", got.Timezone)
	}
	if got.EffectiveRecapPrompt("") != "Be brief." {
		t.Errorf("RecapPrompt = %v, want %q", got.RecapPrompt, "Be brief.")
	}
	if !got.JobIsEnabled("standings", false) {
		t.Error("standings should be enabled")
	}
	if got.JobIsEnabled("monitor", true) {
		t.Error("monitor should be disabled (checkbox was omitted)")
	}
	days := got.EffectiveWaiverDays(nil)
	if len(days) != 2 || days[0] != 1 || days[1] != 3 {
		t.Errorf("WaiverDays = %v, want [1 3]", days)
	}
}

func TestSaveSettingsInvalidTimezoneRejectedNotPersisted(t *testing.T) {
	srv, client, mgr, _ := newTestServer(t)
	token := csrfToken(t, srv, client)

	form := url.Values{"csrf_token": {token}, "action": {"save"}, "timezone": {"Not/AZone"}}
	resp, err := client.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (re-rendered form with error)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "invalid timezone") {
		t.Errorf("body missing validation error: %s", body)
	}
	if mgr.Get().Timezone != nil {
		t.Error("an invalid save must not be persisted")
	}
}

func TestSaveSettingsMissingCSRFRejected(t *testing.T) {
	srv, client, mgr, _ := newTestServer(t)
	login(t, srv, client, "s3cret").Body.Close()

	form := url.Values{"action": {"save"}, "timezone": {"America/Chicago"}}
	resp, err := client.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("status = %d, want 403 without a CSRF token", resp.StatusCode)
	}
	if mgr.Get().Timezone != nil {
		t.Error("a save without a valid CSRF token must not be persisted")
	}
}

func TestResetAllRestoresEnvDefaults(t *testing.T) {
	srv, client, mgr, _ := newTestServer(t)
	token := csrfToken(t, srv, client)

	tzOverride := "America/Chicago"
	if err := mgr.Save(settings.Settings{Timezone: &tzOverride}); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"csrf_token": {token}, "action": {"reset_all"}}
	resp, err := client.PostForm(srv.URL+"/settings", form)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := mgr.Get(); got.Timezone != nil {
		t.Errorf("Timezone = %v after reset_all, want nil", got.Timezone)
	}
}
