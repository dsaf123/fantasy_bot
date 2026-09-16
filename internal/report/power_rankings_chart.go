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
// color-scale dependency. Colors are Discord's own UI palette (blurple, its
// status/role colors, plus a few extras) so the chart reads as native to
// Discord's dark theme instead of a generic charting library; none of them
// are dark enough to disappear against powerRankingsPalette's background.
var seriesPalette = []drawing.Color{
	{R: 0x58, G: 0x65, B: 0xf2, A: 255}, // blurple
	{R: 0xed, G: 0x42, B: 0x45, A: 255}, // red
	{R: 0x57, G: 0xf2, B: 0x87, A: 255}, // green
	{R: 0xfe, G: 0xe7, B: 0x5c, A: 255}, // yellow
	{R: 0xeb, G: 0x45, B: 0x9e, A: 255}, // fuchsia
	{R: 0x4f, G: 0xd6, B: 0xe8, A: 255}, // cyan
	{R: 0xf5, G: 0xa6, B: 0x23, A: 255}, // orange
	{R: 0x9b, G: 0x84, B: 0xec, A: 255}, // periwinkle
	{R: 0xff, G: 0x8f, B: 0xb3, A: 255}, // pink
	{R: 0xb9, G: 0xbb, B: 0xbe, A: 255}, // greyple
}

// powerRankingsPalette themes the chart's background, canvas, axes, and text
// to match tcBackground/tcSubColor/bmGridColor (see trophy_case.go and
// bad_management.go) so this line chart reads as the same visual family as
// the other dark-themed report images instead of go-chart's default
// light theme.
type powerRankingsPalette struct{}

func (powerRankingsPalette) BackgroundColor() drawing.Color       { return drawing.Color(tcBackground) }
func (powerRankingsPalette) BackgroundStrokeColor() drawing.Color { return drawing.Color(tcBackground) }
func (powerRankingsPalette) CanvasColor() drawing.Color           { return drawing.Color(tcBackground) }
func (powerRankingsPalette) CanvasStrokeColor() drawing.Color     { return drawing.Color(tcBackground) }
func (powerRankingsPalette) AxisStrokeColor() drawing.Color       { return drawing.Color(bmGridColor) }
func (powerRankingsPalette) TextColor() drawing.Color             { return drawing.Color(tcSubColor) }
func (powerRankingsPalette) GetSeriesColor(index int) drawing.Color {
	return seriesPalette[index%len(seriesPalette)]
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
		seriesColor := seriesPalette[i%len(seriesPalette)]
		line := chart.ContinuousSeries{
			Name:    abbrev,
			XValues: weeks,
			YValues: scoresByRoster[id],
			Style: chart.Style{
				StrokeColor: seriesColor,
				StrokeWidth: 2,
			},
		}

		// Built by hand rather than via chart.LastValueAnnotationSeries: that
		// helper forwards only the line's StrokeColor/StrokeWidth, so its
		// label pill would fall back to go-chart's hardcoded black-on-white
		// (see AnnotationSeries.annotationStyleDefaults) instead of this
		// chart's dark theme. FillColor is set only here, not on the line's
		// own Style, since a line's FillColor tells go-chart to shade the
		// area under it.
		lastWeek := weeks[len(weeks)-1]
		lastScore := scoresByRoster[id][len(scoresByRoster[id])-1]
		label := chart.AnnotationSeries{
			Name: abbrev + " label",
			Style: chart.Style{
				FontColor:   drawing.Color(tcNameText),
				FillColor:   drawing.Color(tcHeaderBG),
				StrokeColor: seriesColor,
				StrokeWidth: 1.5,
				FontSize:    11,
			},
			Annotations: []chart.Value2{{XValue: lastWeek, YValue: lastScore, Label: abbrev}},
		}
		series = append(series, line, label)
	}

	weekTicks := make([]chart.Tick, len(weeks))
	for i, w := range weeks {
		weekTicks[i] = chart.Tick{Value: w, Label: fmt.Sprintf("W%02d", int(w))}
	}

	graph := chart.Chart{
		Title: "Power Rankings — Season Trend",
		TitleStyle: chart.Style{
			FontColor: drawing.Color(bmTitleColor),
		},
		ColorPalette: powerRankingsPalette{},
		Background: chart.Style{
			Padding: chart.Box{Top: 30, Left: 50, Right: 80, Bottom: 20},
		},
		XAxis: chart.XAxis{
			Name:  "Week",
			Ticks: weekTicks,
		},
		YAxis: chart.YAxis{
			Name: "Power Ranking Score (0-100)",
			GridMajorStyle: chart.Style{
				StrokeColor: drawing.Color(bmGridColor),
				StrokeWidth: 1,
			},
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
