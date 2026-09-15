package report

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sort"

	"github.com/golang/freetype/truetype"
	chart "github.com/wcharczuk/go-chart/v2"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"

	"fantasy_bot/internal/sleeper"
)

// trophyCaseColumns is the crosstab's fixed, ordered set of columns,
// matching the emoji/order Trophies uses for its ten weekly categories. The
// rendered grid (see TrophyCaseImage) labels each column with the short
// text label instead of the emoji itself, since a standard font can't
// render color emoji glyphs; TrophyCaseLegend maps them back to the emoji
// and full name for the message text posted alongside the image, where
// Unicode emoji render natively.
var trophyCaseColumns = []struct {
	emoji string
	label string
	name  string
}{
	{"👑️", "HIGH", "High Score"},
	{"💩️", "LOW", "Low Score"},
	{"😱️", "BLOWOUT", "Blowout"},
	{"😅️", "CLOSE", "Close Win"},
	{"🍀️", "LUCKY", "Lucky"},
	{"😡️", "UNLUCKY", "Unlucky"},
	{"📈️", "OVER", "Overachiever"},
	{"📉️", "UNDER", "Underachiever"},
	{"🤖️", "BEST", "Best Manager"},
	{"🤡️", "WORST", "Worst Manager"},
}

// TrophyCaseLegend renders a one-line emoji legend for trophyCaseColumns,
// meant as the Discord message caption posted alongside TrophyCaseImage.
func TrophyCaseLegend() string {
	var out string
	for i, col := range trophyCaseColumns {
		if i > 0 {
			out += "  "
		}
		out += fmt.Sprintf("%s %s", col.emoji, col.name)
	}
	return out
}

// TrophyCaseImage renders a season-long crosstab of every team's trophy
// counts as a PNG: one row per team (most decorated first), one column per
// weekly trophy category (see Trophies), tallied across every week in
// history. historyProjections supplies each week's player projections for
// the over/underachiever columns, keyed the same as history; a week
// missing from it just skips those two columns for that week, same as
// Trophies does for a single week. Returns (nil, nil) if history has no
// weeks to tally yet.
func (c *LeagueContext) TrophyCaseImage(history map[int][]sleeper.Matchup, historyProjections map[int][]sleeper.PlayerProjection) ([]byte, error) {
	if len(history) == 0 {
		return nil, nil
	}

	weeks := make([]int, 0, len(history))
	for week := range history {
		weeks = append(weeks, week)
	}
	sort.Ints(weeks)

	counts := make(map[int]map[string]int, len(c.Rosters))
	for _, week := range weeks {
		for _, w := range c.weekTrophyWinners(history[week], historyProjections[week]) {
			row, ok := counts[w.rosterID]
			if !ok {
				row = make(map[string]int, len(trophyCaseColumns))
				counts[w.rosterID] = row
			}
			row[w.emoji]++
		}
	}

	type teamRow struct {
		rosterID int
		counts   map[string]int
		total    int
	}
	rows := make([]teamRow, 0, len(c.Rosters))
	maxCount := 0
	for _, r := range c.Rosters {
		row := counts[r.RosterID]
		total := 0
		for _, n := range row {
			total += n
			if n > maxCount {
				maxCount = n
			}
		}
		rows = append(rows, teamRow{rosterID: r.RosterID, counts: row, total: total})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].total != rows[j].total {
			return rows[i].total > rows[j].total
		}
		return c.TeamName(rows[i].rosterID) < c.TeamName(rows[j].rosterID)
	})

	names := make([]string, len(rows))
	cellValues := make([][]int, len(rows))
	for i, row := range rows {
		names[i] = c.TeamName(row.rosterID)
		vals := make([]int, len(trophyCaseColumns))
		for j, col := range trophyCaseColumns {
			vals[j] = row.counts[col.emoji]
		}
		cellValues[i] = vals
	}

	subtitle := fmt.Sprintf("Season totals through Week %d", weeks[len(weeks)-1])
	return drawTrophyCaseGrid(names, cellValues, maxCount, subtitle)
}

// Layout and palette constants for drawTrophyCaseGrid. Colors aim for a
// dark-theme heatmap that reads well against Discord's dark mode: cells
// darken toward the background at zero and brighten toward a periwinkle
// blue at the grid's highest count.
const (
	tcMargin      = 24
	tcTitleHeight = 40
	tcSubHeight   = 26
	tcHeaderH     = 40
	tcRowHeight   = 40
	tcCellPad     = 14
	tcNamePad     = 20
	tcCornerRad   = 8
)

var (
	tcBackground = color.RGBA{0x1b, 0x1d, 0x2a, 0xff}
	tcTitleColor = color.RGBA{0xf2, 0xc9, 0x4c, 0xff}
	tcSubColor   = color.RGBA{0xa9, 0xae, 0xc7, 0xff}
	tcHeaderBG   = color.RGBA{0x2a, 0x2d, 0x42, 0xff}
	tcHeaderText = color.RGBA{0xd3, 0xd6, 0xf0, 0xff}
	tcNameText   = color.RGBA{0xf5, 0xf6, 0xfa, 0xff}
	tcCellLow    = color.RGBA{0x26, 0x28, 0x3d, 0xff}
	tcCellHigh   = color.RGBA{0x8f, 0x9c, 0xf7, 0xff}
	tcCellText   = color.RGBA{0xff, 0xff, 0xff, 0xff}
)

// drawTrophyCaseGrid draws the actual crosstab image: a title/subtitle,
// one header cell per trophyCaseColumns entry, and one row per name with a
// heatmap-shaded cell per column value.
func drawTrophyCaseGrid(names []string, values [][]int, maxCount int, subtitle string) ([]byte, error) {
	baseFont, err := chart.GetDefaultFont()
	if err != nil {
		return nil, fmt.Errorf("report: load font: %w", err)
	}
	titleFace := truetype.NewFace(baseFont, &truetype.Options{Size: 22, DPI: 72})
	subFace := truetype.NewFace(baseFont, &truetype.Options{Size: 13, DPI: 72})
	headerFace := truetype.NewFace(baseFont, &truetype.Options{Size: 12, DPI: 72})
	nameFace := truetype.NewFace(baseFont, &truetype.Options{Size: 14, DPI: 72})
	cellFace := truetype.NewFace(baseFont, &truetype.Options{Size: 14, DPI: 72})

	measure := func(face font.Face, s string) int {
		d := font.Drawer{Face: face}
		return d.MeasureString(s).Round()
	}

	nameColWidth := 0
	for _, n := range names {
		if w := measure(nameFace, n); w > nameColWidth {
			nameColWidth = w
		}
	}
	nameColWidth += tcNamePad * 2

	cellWidth := measure(cellFace, "00")
	for _, col := range trophyCaseColumns {
		if w := measure(headerFace, col.label); w > cellWidth {
			cellWidth = w
		}
	}
	cellWidth += tcCellPad * 2

	gridWidth := nameColWidth + cellWidth*len(trophyCaseColumns)
	width := gridWidth + tcMargin*2
	height := tcMargin + tcTitleHeight + tcSubHeight + tcHeaderH + tcRowHeight*len(names) + tcMargin

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(img, img.Bounds(), &image.Uniform{tcBackground}, image.Point{}, draw.Src)

	top := tcMargin
	drawCentered(img, titleFace, tcTitleColor, "Trophy Case", image.Rect(0, top, width, top+tcTitleHeight))
	top += tcTitleHeight
	drawCentered(img, subFace, tcSubColor, subtitle, image.Rect(0, top, width, top+tcSubHeight))
	top += tcSubHeight

	for j, col := range trophyCaseColumns {
		x := tcMargin + nameColWidth + j*cellWidth
		box := image.Rect(x+2, top+2, x+cellWidth-2, top+tcHeaderH-2)
		fillRoundedRect(img, box, tcCornerRad, tcHeaderBG)
		drawCentered(img, headerFace, tcHeaderText, col.label, box)
	}
	top += tcHeaderH

	for i, name := range names {
		rowTop := top + i*tcRowHeight
		nameBox := image.Rect(tcMargin, rowTop, tcMargin+nameColWidth-tcNamePad, rowTop+tcRowHeight)
		drawLeft(img, nameFace, tcNameText, name, nameBox)

		for j, v := range values[i] {
			x := tcMargin + nameColWidth + j*cellWidth
			box := image.Rect(x+2, rowTop+2, x+cellWidth-2, rowTop+tcRowHeight-2)
			fillRoundedRect(img, box, tcCornerRad, heatColor(v, maxCount))
			drawCentered(img, cellFace, tcCellText, fmt.Sprintf("%d", v), box)
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("report: encode trophy case png: %w", err)
	}
	return buf.Bytes(), nil
}

// drawCentered draws text horizontally and vertically centered within box.
func drawCentered(img *image.RGBA, face font.Face, col color.Color, text string, box image.Rectangle) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face}
	w := d.MeasureString(text).Round()
	m := face.Metrics()
	x := box.Min.X + (box.Dx()-w)/2
	y := box.Min.Y + (box.Dy()+(m.Ascent-m.Descent).Round())/2
	d.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d.DrawString(text)
}

// drawLeft draws text left-aligned and vertically centered within box.
func drawLeft(img *image.RGBA, face font.Face, col color.Color, text string, box image.Rectangle) {
	d := &font.Drawer{Dst: img, Src: image.NewUniform(col), Face: face}
	m := face.Metrics()
	y := box.Min.Y + (box.Dy()+(m.Ascent-m.Descent).Round())/2
	d.Dot = fixed.Point26_6{X: fixed.I(box.Min.X), Y: fixed.I(y)}
	d.DrawString(text)
}

// heatColor interpolates a heatmap cell's fill between tcCellLow (value 0)
// and tcCellHigh (value == max).
func heatColor(value, max int) color.RGBA {
	if max <= 0 {
		return tcCellLow
	}
	t := float64(value) / float64(max)
	lerp := func(a, b uint8) uint8 {
		return uint8(float64(a) + t*(float64(b)-float64(a)))
	}
	return color.RGBA{
		R: lerp(tcCellLow.R, tcCellHigh.R),
		G: lerp(tcCellLow.G, tcCellHigh.G),
		B: lerp(tcCellLow.B, tcCellHigh.B),
		A: 255,
	}
}

// fillRoundedRect fills rect with col, rounding its four corners to radius.
func fillRoundedRect(img *image.RGBA, rect image.Rectangle, radius int, col color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if inRoundedRect(x, y, rect, radius) {
				img.Set(x, y, col)
			}
		}
	}
}

// inRoundedRect reports whether (x, y) falls inside rect once its four
// corners are rounded to radius: true everywhere except the four corner
// squares, where it further requires falling within radius of that
// corner's center.
func inRoundedRect(x, y int, rect image.Rectangle, radius int) bool {
	minX, minY, maxX, maxY := rect.Min.X, rect.Min.Y, rect.Max.X-1, rect.Max.Y-1
	nearestX, nearestY := x, y
	inCornerX, inCornerY := false, false
	if x < minX+radius {
		nearestX, inCornerX = minX+radius, true
	} else if x > maxX-radius {
		nearestX, inCornerX = maxX-radius, true
	}
	if y < minY+radius {
		nearestY, inCornerY = minY+radius, true
	} else if y > maxY-radius {
		nearestY, inCornerY = maxY-radius, true
	}
	if inCornerX && inCornerY {
		dx, dy := float64(x-nearestX), float64(y-nearestY)
		return dx*dx+dy*dy <= float64(radius*radius)
	}
	return true
}
