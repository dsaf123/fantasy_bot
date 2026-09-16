package report

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sort"

	"github.com/golang/freetype/truetype"
	chart "github.com/wcharczuk/go-chart/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"fantasy_bot/internal/sleeper"
)

// benchLeftEpsilon treats a bench-points-left value this small as zero, so a
// team that (modulo floating-point noise) started its exact optimal lineup
// renders as one flat-topped bar instead of an invisible sliver of red.
const benchLeftEpsilon = 0.05

// teamBadManagement is one roster's actual-vs-optimal split for
// BadManagementChart.
type teamBadManagement struct {
	name      string
	actual    float64
	benchLeft float64
}

// BadManagementChart renders a bar-chart PNG of every team's points scored
// that week stacked against the points they left on their bench (their
// optimal lineup's score minus what they actually started - see
// optimalLineupPoints), ranked worst manager (most points left on the
// bench) first. Returns (nil, nil) if matchups is empty.
func (c *LeagueContext) BadManagementChart(matchups []sleeper.Matchup) ([]byte, error) {
	if len(matchups) == 0 {
		return nil, nil
	}

	teams := make([]teamBadManagement, 0, len(matchups))
	for _, m := range matchups {
		benchLeft := c.optimalLineupPoints(m) - m.Points
		if benchLeft < benchLeftEpsilon {
			benchLeft = 0
		}
		teams = append(teams, teamBadManagement{
			name:      c.TeamAbbrev(m.RosterID),
			actual:    m.Points,
			benchLeft: benchLeft,
		})
	}

	sort.Slice(teams, func(i, j int) bool {
		if teams[i].benchLeft != teams[j].benchLeft {
			return teams[i].benchLeft > teams[j].benchLeft
		}
		return teams[i].name < teams[j].name
	})

	subtitle := fmt.Sprintf("Week %d - points left on the bench, worst to best", c.Week)
	return drawBadManagementChart(teams, subtitle)
}

// Layout constants for drawBadManagementChart. bmMargin/corner radius match
// TrophyCaseImage's so the two chart images read as one visual family.
const (
	bmMargin     = 24
	bmTitleLineH = 28
	bmSubLineH   = 20
	bmHeaderGap  = 10
	bmLegendRowH = 20
	bmPlotHeight = 300
	bmXLabelH    = 30
	bmYAxisW     = 46
	bmBarWidth   = 84
	bmBarGap     = 34
	bmCornerRad  = 6
	bmTickCount  = 5
)

// Palette for drawBadManagementChart. Background/text colors are shared
// with TrophyCaseImage (see tcBackground et al. in trophy_case.go); bar
// colors match Discord's own blurple/red so "scored" vs. "left on the
// bench" reads as good vs. bad at a glance.
var (
	bmBackground  = tcBackground
	bmTitleColor  = color.RGBA{0xff, 0xff, 0xff, 0xff}
	bmSubColor    = tcSubColor
	bmGridColor   = color.RGBA{0x38, 0x3b, 0x52, 0xff}
	bmScoredColor = color.RGBA{0x58, 0x65, 0xf2, 0xff} // Discord blurple
	bmBenchColor  = color.RGBA{0xed, 0x42, 0x45, 0xff} // Discord red
)

// drawBadManagementChart draws the actual chart image: a title/subtitle top
// left, a scored/bench-left legend top right, horizontal gridlines on a
// "nice" (round-number) scale, and one two-segment stacked bar per team.
func drawBadManagementChart(teams []teamBadManagement, subtitle string) ([]byte, error) {
	baseFont, err := chart.GetDefaultFont()
	if err != nil {
		return nil, fmt.Errorf("report: load font: %w", err)
	}
	titleFace := truetype.NewFace(baseFont, &truetype.Options{Size: 22, DPI: 72})
	subFace := truetype.NewFace(baseFont, &truetype.Options{Size: 13, DPI: 72})
	legendFace := truetype.NewFace(baseFont, &truetype.Options{Size: 13, DPI: 72})
	axisFace := truetype.NewFace(baseFont, &truetype.Options{Size: 13, DPI: 72})

	measure := func(face font.Face, s string) int {
		d := font.Drawer{Face: face}
		return d.MeasureString(s).Round()
	}

	legendItems := []struct {
		col   color.RGBA
		label string
	}{
		{bmScoredColor, "Points Scored"},
		{bmBenchColor, "Points Left on Bench"},
	}
	const legendSwatch, legendGap = 12, 6
	legendBlockW := 0
	for _, item := range legendItems {
		if w := legendSwatch + legendGap + measure(legendFace, item.label); w > legendBlockW {
			legendBlockW = w
		}
	}

	titleBlockH := bmTitleLineH + 4 + bmSubLineH
	legendBlockH := len(legendItems) * bmLegendRowH
	headerH := titleBlockH
	if legendBlockH > headerH {
		headerH = legendBlockH
	}
	headerH += bmHeaderGap

	width := bmMargin*2 + bmYAxisW + len(teams)*bmBarWidth + (len(teams)-1)*bmBarGap
	height := bmMargin*2 + headerH + bmPlotHeight + bmXLabelH

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{bmBackground}, image.Point{}, draw.Src)

	drawLeft(img, titleFace, bmTitleColor, "Bad Management",
		image.Rect(bmMargin, bmMargin, width-bmMargin, bmMargin+bmTitleLineH))
	drawLeft(img, subFace, bmSubColor, subtitle,
		image.Rect(bmMargin, bmMargin+bmTitleLineH+4, width-bmMargin, bmMargin+bmTitleLineH+4+bmSubLineH))

	legendLeft := width - bmMargin - legendBlockW
	for i, item := range legendItems {
		rowTop := bmMargin + i*bmLegendRowH
		swatchBox := image.Rect(legendLeft, rowTop+4, legendLeft+legendSwatch, rowTop+4+legendSwatch)
		fillRoundedRect(img, swatchBox, 2, item.col)
		textBox := image.Rect(legendLeft+legendSwatch+legendGap, rowTop, legendLeft+legendBlockW, rowTop+bmLegendRowH)
		drawLeft(img, legendFace, bmSubColor, item.label, textBox)
	}

	plotTop := bmMargin + headerH
	plotBottom := plotTop + bmPlotHeight
	plotLeft := bmMargin + bmYAxisW
	plotRight := width - bmMargin

	chartMax, step := niceAxisMax(maxStackedTotal(teams))
	for i := 0; i <= bmTickCount; i++ {
		val := step * float64(i)
		if val > chartMax+1e-9 {
			break
		}
		y := plotBottom - int(val/chartMax*float64(bmPlotHeight))
		drawHLine(img, plotLeft, plotRight, y, bmGridColor)
		drawRight(img, axisFace, bmSubColor, fmt.Sprintf("%.0f", val),
			image.Rect(bmMargin, y-8, plotLeft-8, y+8))
	}

	for i, team := range teams {
		x := plotLeft + i*(bmBarWidth+bmBarGap)
		scoredH := int(math.Round(team.actual / chartMax * float64(bmPlotHeight)))
		benchH := int(math.Round(team.benchLeft / chartMax * float64(bmPlotHeight)))

		scoredBox := image.Rect(x, plotBottom-scoredH, x+bmBarWidth, plotBottom)
		if benchH > 0 {
			fillRoundedRect(img, scoredBox, 0, bmScoredColor)
			benchBox := image.Rect(x, plotBottom-scoredH-benchH, x+bmBarWidth, plotBottom-scoredH)
			fillTopRoundedRect(img, benchBox, bmCornerRad, bmBenchColor)
		} else {
			fillTopRoundedRect(img, scoredBox, bmCornerRad, bmScoredColor)
		}

		labelBox := image.Rect(x, plotBottom+8, x+bmBarWidth, plotBottom+bmXLabelH)
		drawCentered(img, axisFace, bmTitleColor, team.name, labelBox)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("report: encode bad management png: %w", err)
	}
	return buf.Bytes(), nil
}

// maxStackedTotal returns the tallest actual+benchLeft total across teams,
// i.e. what each bar's full stacked height represents.
func maxStackedTotal(teams []teamBadManagement) float64 {
	var max float64
	for _, t := range teams {
		if total := t.actual + t.benchLeft; total > max {
			max = total
		}
	}
	return max
}

// niceAxisMax picks a y-axis ceiling and per-gridline step so bmTickCount
// gridlines land on round numbers (5/10/20/25/50/...) instead of an
// awkward fraction of the data's actual max.
func niceAxisMax(dataMax float64) (chartMax, step float64) {
	steps := []float64{5, 10, 20, 25, 50, 100, 200, 250, 500, 1000}
	if dataMax <= 0 {
		return float64(bmTickCount) * steps[0], steps[0]
	}
	rough := dataMax / bmTickCount
	step = steps[len(steps)-1]
	for _, s := range steps {
		if rough <= s {
			step = s
			break
		}
	}
	for chartMax = step; chartMax < dataMax; chartMax += step {
	}
	return chartMax, step
}

// fillTopRoundedRect is like fillRoundedRect but only rounds rect's top two
// corners, so a stacked bar's topmost segment gets a rounded cap while
// staying flush against the segment (or axis) below it.
func fillTopRoundedRect(img *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	if h := rect.Dy(); h < radius {
		radius = h
	}
	if w := rect.Dx() / 2; w < radius {
		radius = w
	}
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if inTopRoundedRect(x, y, rect, radius) {
				img.Set(x, y, col)
			}
		}
	}
}

// inTopRoundedRect reports whether (x, y) falls inside rect once only its
// top-left and top-right corners are rounded to radius.
func inTopRoundedRect(x, y int, rect image.Rectangle, radius int) bool {
	minX, minY, maxX := rect.Min.X, rect.Min.Y, rect.Max.X-1
	if y >= minY+radius {
		return true
	}
	nearestX := x
	inCorner := false
	if x < minX+radius {
		nearestX, inCorner = minX+radius, true
	} else if x > maxX-radius {
		nearestX, inCorner = maxX-radius, true
	}
	if !inCorner {
		return true
	}
	dx, dy := float64(x-nearestX), float64(y-(minY+radius))
	return dx*dx+dy*dy <= float64(radius*radius)
}

// drawHLine draws a horizontal line at y from x0 to x1 inclusive.
func drawHLine(img *image.RGBA, x0, x1, y int, col color.Color) {
	for x := x0; x <= x1; x++ {
		img.Set(x, y, col)
	}
}

// drawRight draws text right-aligned and vertically centered within box.
func drawRight(img *image.RGBA, face font.Face, col color.Color, text string, box image.Rectangle) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face}
	w := d.MeasureString(text).Round()
	m := face.Metrics()
	y := box.Min.Y + (box.Dy()+(m.Ascent-m.Descent).Round())/2
	d.Dot = fixed.Point26_6{X: fixed.I(box.Max.X - w), Y: fixed.I(y)}
	d.DrawString(text)
}
