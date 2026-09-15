package portal

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
	"time"
)

const (
	sessionCookieName = "fantasy_bot_session"
	// sessionDuration is deliberately long - this is a single-admin tool
	// with no persisted sessions (a restart logs everyone out), so there's
	// little upside to forcing frequent re-logins.
	sessionDuration = 30 * 24 * time.Hour
)

// sessionInfo is what a logged-in session remembers: when it expires, and
// the CSRF token every form it renders must echo back.
type sessionInfo struct {
	expiresAt time.Time
	csrfToken string
}

// sessionStore holds logged-in sessions in memory only. That means a
// process restart logs everyone out, which is an acceptable tradeoff for a
// single-admin tool that otherwise has no reason to persist session state.
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]sessionInfo
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]sessionInfo)}
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// create starts a new session and returns its cookie token and CSRF token.
func (st *sessionStore) create() (token, csrfToken string, err error) {
	token, err = randomToken()
	if err != nil {
		return "", "", err
	}
	csrfToken, err = randomToken()
	if err != nil {
		return "", "", err
	}

	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[token] = sessionInfo{expiresAt: time.Now().Add(sessionDuration), csrfToken: csrfToken}
	return token, csrfToken, nil
}

// lookup returns the session for token, if it exists and hasn't expired.
func (st *sessionStore) lookup(token string) (sessionInfo, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()

	info, ok := st.sessions[token]
	if !ok {
		return sessionInfo{}, false
	}
	if time.Now().After(info.expiresAt) {
		delete(st.sessions, token)
		return sessionInfo{}, false
	}
	return info, true
}

func (st *sessionStore) destroy(token string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	delete(st.sessions, token)
}

// passwordMatches compares got to want in constant time, so a login
// attempt can't be used to time-probe the configured password. Both sides
// are hashed to a fixed length first: subtle.ConstantTimeCompare itself
// returns early (non-constant time) when its inputs' lengths differ, so
// hashing first keeps even the password's length from leaking.
func passwordMatches(got, want string) bool {
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

// checkCSRF reports whether r's csrf_token form value matches want.
func checkCSRF(csrfTokenFromForm, want string) bool {
	return csrfTokenFromForm != "" && subtle.ConstantTimeCompare([]byte(csrfTokenFromForm), []byte(want)) == 1
}
