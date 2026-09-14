// Package sleeper is a minimal client for Sleeper's public, unauthenticated
// fantasy football API (https://docs.sleeper.com/). Unlike ESPN, Sleeper
// requires no cookies or API keys for public league data.
package sleeper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultBaseURL = "https://api.sleeper.app/v1"
	// projectionsBaseURL is separate from defaultBaseURL because the
	// undocumented projections endpoint lives outside Sleeper's /v1 API.
	projectionsBaseURL = "https://api.sleeper.app/projections/nfl"
)

type Client struct {
	httpClient *http.Client
	baseURL    string
}

func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

func (c *Client) get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("sleeper: request %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sleeper: %s returned status %d", url, resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("sleeper: decode %s: %w", url, err)
	}
	return nil
}

func (c *Client) GetLeague(ctx context.Context, leagueID string) (*League, error) {
	var league League
	if err := c.get(ctx, c.baseURL+"/league/"+leagueID, &league); err != nil {
		return nil, err
	}
	return &league, nil
}

func (c *Client) GetRosters(ctx context.Context, leagueID string) ([]Roster, error) {
	var rosters []Roster
	if err := c.get(ctx, c.baseURL+"/league/"+leagueID+"/rosters", &rosters); err != nil {
		return nil, err
	}
	return rosters, nil
}

func (c *Client) GetUsers(ctx context.Context, leagueID string) ([]User, error) {
	var users []User
	if err := c.get(ctx, c.baseURL+"/league/"+leagueID+"/users", &users); err != nil {
		return nil, err
	}
	return users, nil
}

func (c *Client) GetMatchups(ctx context.Context, leagueID string, week int) ([]Matchup, error) {
	var matchups []Matchup
	url := c.baseURL + "/league/" + leagueID + "/matchups/" + strconv.Itoa(week)
	if err := c.get(ctx, url, &matchups); err != nil {
		return nil, err
	}
	return matchups, nil
}

// GetTransactions fetches waiver/free-agent/trade transactions for a given
// week ("round" in Sleeper's API).
func (c *Client) GetTransactions(ctx context.Context, leagueID string, week int) ([]Transaction, error) {
	var txns []Transaction
	url := c.baseURL + "/league/" + leagueID + "/transactions/" + strconv.Itoa(week)
	if err := c.get(ctx, url, &txns); err != nil {
		return nil, err
	}
	return txns, nil
}

func (c *Client) GetNFLState(ctx context.Context) (*NFLState, error) {
	var state NFLState
	if err := c.get(ctx, c.baseURL+"/state/nfl", &state); err != nil {
		return nil, err
	}
	return &state, nil
}

// GetAllPlayers fetches Sleeper's full NFL player dictionary (several MB).
// Sleeper asks that this endpoint be called at most once per day; use
// PlayerCache rather than calling this directly from report code.
func (c *Client) GetAllPlayers(ctx context.Context) (map[string]Player, error) {
	var players map[string]Player
	if err := c.get(ctx, c.baseURL+"/players/nfl", &players); err != nil {
		return nil, err
	}
	return players, nil
}

// GetProjections fetches Sleeper's per-player projected stat lines for every
// NFL player for a given week, via an undocumented endpoint (not part of
// Sleeper's published /v1 API, and not tied to any particular league - point
// totals must be picked out per league scoring format, see
// PlayerProjection.Points). seasonType is typically "regular".
func (c *Client) GetProjections(ctx context.Context, season string, week int, seasonType string) ([]PlayerProjection, error) {
	var projections []PlayerProjection
	url := projectionsBaseURL + "/" + season + "/" + strconv.Itoa(week) + "?season_type=" + seasonType
	if err := c.get(ctx, url, &projections); err != nil {
		return nil, err
	}
	return projections, nil
}
