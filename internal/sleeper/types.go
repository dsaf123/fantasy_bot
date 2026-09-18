package sleeper

// League is the subset of Sleeper's league object the bot needs.
type League struct {
	LeagueID        string             `json:"league_id"`
	Name            string             `json:"name"`
	Season          string             `json:"season"`
	SeasonType      string             `json:"season_type"`
	Status          string             `json:"status"`
	RosterPositions []string           `json:"roster_positions"`
	Settings        LeagueSettings     `json:"settings"`
	ScoringSettings map[string]float64 `json:"scoring_settings"`
}

type LeagueSettings struct {
	WaiverBudget     int `json:"waiver_budget"`
	PlayoffWeekStart int `json:"playoff_week_start"`
	PlayoffTeams     int `json:"playoff_teams"`
}

type Roster struct {
	RosterID int            `json:"roster_id"`
	OwnerID  string         `json:"owner_id"`
	Players  []string       `json:"players"`
	Starters []string       `json:"starters"`
	Reserve  []string       `json:"reserve"`
	Settings RosterSettings `json:"settings"`
}

type RosterSettings struct {
	Wins               int     `json:"wins"`
	Losses             int     `json:"losses"`
	Ties               int     `json:"ties"`
	FPTS               float64 `json:"fpts"`
	FPTSDecimal        float64 `json:"fpts_decimal"`
	FPTSAgainst        float64 `json:"fpts_against"`
	FPTSAgainstDecimal float64 `json:"fpts_against_decimal"`
	WaiverBudgetUsed   int     `json:"waiver_budget_used"`
}

// PointsFor combines Sleeper's whole/decimal fpts fields into one value.
func (s RosterSettings) PointsFor() float64 {
	return s.FPTS + s.FPTSDecimal/100
}

// PointsAgainst combines Sleeper's whole/decimal fpts_against fields into one value.
func (s RosterSettings) PointsAgainst() float64 {
	return s.FPTSAgainst + s.FPTSAgainstDecimal/100
}

type User struct {
	UserID      string       `json:"user_id"`
	DisplayName string       `json:"display_name"`
	Metadata    UserMetadata `json:"metadata"`
}

type UserMetadata struct {
	TeamName string `json:"team_name"`
}

type Matchup struct {
	RosterID       int                `json:"roster_id"`
	MatchupID      int                `json:"matchup_id"`
	Points         float64            `json:"points"`
	Starters       []string           `json:"starters"`
	StartersPoints []float64          `json:"starters_points"`
	Players        []string           `json:"players"`
	PlayersPoints  map[string]float64 `json:"players_points"`
}

type Transaction struct {
	TransactionID string              `json:"transaction_id"`
	Type          string              `json:"type"` // waiver, free_agent, trade
	Status        string              `json:"status"`
	StatusUpdated int64               `json:"status_updated"` // ms epoch
	Adds          map[string]int      `json:"adds"`
	Drops         map[string]int      `json:"drops"`
	RosterIDs     []int               `json:"roster_ids"`
	Settings      TransactionSettings `json:"settings"`
}

type TransactionSettings struct {
	WaiverBid int `json:"waiver_bid"`
}

// Player is the subset of Sleeper's /players/nfl object the bot needs.
type Player struct {
	PlayerID     string `json:"player_id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name"`
	FullName     string `json:"full_name"`
	Position     string `json:"position"`
	Team         string `json:"team"`
	Status       string `json:"status"`
	InjuryStatus string `json:"injury_status"`
}

// Name returns the best available display name for a player.
func (p Player) Name() string {
	if p.FullName != "" {
		return p.FullName
	}
	if p.FirstName != "" || p.LastName != "" {
		return p.FirstName + " " + p.LastName
	}
	return p.PlayerID
}

// PlayerProjection is one player's projected stat line for a week, from
// Sleeper's undocumented /projections/nfl endpoint. Stats is keyed by the
// same per-category names (rec, rec_yd, pass_td, fgm_40_49,
// pts_allow_21_27, ...) as League.ScoringSettings, so PointsForSettings can
// reconstruct exactly the score the league's own scoring rules would
// produce - unlike Sleeper's precomputed pts_std/pts_half_ppr/pts_ppr
// totals, which only cover a plain reception bonus and miss anything else
// custom (TE premium, bonus yardage thresholds, etc.).
type PlayerProjection struct {
	PlayerID string             `json:"player_id"`
	Stats    map[string]float64 `json:"stats"`
}

// PointsForSettings returns the player's projected fantasy points under the
// league's own scoring rules: the dot product of the projected per-category
// stat line with the matching scoring-settings weights, the same way
// Sleeper turns a player's actual box-score stat line into points.
func (p PlayerProjection) PointsForSettings(scoringSettings map[string]float64) float64 {
	var total float64
	for stat, weight := range scoringSettings {
		total += p.Stats[stat] * weight
	}
	return total
}

type NFLState struct {
	Week         int    `json:"week"`
	Season       string `json:"season"`
	SeasonType   string `json:"season_type"`
	LeagueSeason string `json:"league_season"`
}

// ScheduledGame is one NFL game, from Sleeper's undocumented schedule
// endpoint (see Client.GetSchedule). Status is "pre_game", "complete", or
// "canceled" in practice; a live in-progress game presumably reports some
// other value, but that case is never distinguished from "pre_game" by
// callers - both just mean "not confirmed over yet".
type ScheduledGame struct {
	Week   int    `json:"week"`
	Home   string `json:"home"`
	Away   string `json:"away"`
	Status string `json:"status"`
	Date   string `json:"date"`
}

// Final reports whether the game is over and its result won't change, so a
// participating player's stat line for the week - including an actual
// zero - is safe to treat as their final total rather than a stand-in for
// "hasn't played yet".
func (g ScheduledGame) Final() bool {
	return g.Status == "complete" || g.Status == "canceled"
}
