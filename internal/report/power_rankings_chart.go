package report

import (
	"bytes"
	"fmt"
	"sort"

	chart "github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"

	"fantasy_bot/internal/sleeper"
)

// seriesPalette cycles a small set of visually distinct line colors so each
// team keeps a consistent, legible color without pulling in a full
// color-scale dependency.
var seriesPalette = []drawing.Color{
	chart.ColorBlue,
	chart.ColorRed,
	chart.ColorGreen,
	chart.ColorOrange,
	{R: 148, G: 0, B: 211, A: 255}, // purple
	chart.ColorCyan,
	{R: 255, G: 105, B: 180, A: 255}, // pink
	chart.ColorYellow,
	{R: 139, G: 69, B: 19, A: 255}, // brown
	chart.ColorBlack,
}

// PowerRankingsChart renders a season-long line chart of every team's
// weekly power-ranking score (see PowerRankings and cumulativeStatsThroughWeek)
// as a PNG, for posting alongside the text report. It returns (nil, nil) if
// there isn't at least two scored weeks yet to plot a trend.
func (c *LeagueContext) PowerRankingsChart(history map[int][]sleeper.Matchup) ([]byte, error) {
	if c.Week < 2 {
		return nil, nil
	}

	rosterIDs := make([]int, 0, len(c.Rosters))
	for _, r := range c.Rosters {
		rosterIDs = append(rosterIDs, r.RosterID)
	}
	sort.Ints(rosterIDs) // stable series/color order across runs

	weeks := make([]float64, 0, c.Week)
	scoresByRoster := make(map[int][]float64, len(rosterIDs))
	for week := 1; week <= c.Week; week++ {
		stats := cumulativeStatsThroughWeek(history, week)
		for _, ts := range scoresFromStats(rosterIDs, stats) {
			scoresByRoster[ts.rosterID] = append(scoresByRoster[ts.rosterID], ts.score)
		}
		weeks = append(weeks, float64(week))
	}

	series := make([]chart.Series, 0, len(rosterIDs)*2)
	for i, id := range rosterIDs {
		abbrev := c.TeamAbbrev(id)
		line := chart.ContinuousSeries{
			Name:    abbrev,
			XValues: weeks,
			YValues: scoresByRoster[id],
			Style: chart.Style{
				StrokeColor: seriesPalette[i%len(seriesPalette)],
				StrokeWidth: 2,
			},
		}
		series = append(series, line, chart.LastValueAnnotationSeries(line, func(interface{}) string { return abbrev }))
	}

	weekTicks := make([]chart.Tick, len(weeks))
	for i, w := range weeks {
		weekTicks[i] = chart.Tick{Value: w, Label: fmt.Sprintf("W%02d", int(w))}
	}

	graph := chart.Chart{
		Title: "Power Rankings — Season Trend",
		Background: chart.Style{
			Padding: chart.Box{Top: 30, Left: 50, Right: 80, Bottom: 20},
		},
		XAxis: chart.XAxis{
			Name:  "Week",
			Ticks: weekTicks,
		},
		YAxis: chart.YAxis{
			Name: "Power Ranking Score (0-100)",
		},
		YAxisSecondary: chart.YAxis{
			Style: chart.Style{Hidden: true},
		},
		Series: series,
	}

	var buf bytes.Buffer
	if err := graph.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("report: render power rankings chart: %w", err)
	}
	return buf.Bytes(), nil
}
