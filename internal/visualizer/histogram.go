// Package visualizer renders charts as PNG bytes for the bot to send
// as Telegram photos. Pure-Go implementation (go-chart/v2 +
// freetype) — no CGo, scratch-Docker friendly.
package visualizer

import (
	"bytes"
	"fmt"

	"github.com/wcharczuk/go-chart/v2"
	"github.com/wcharczuk/go-chart/v2/drawing"

	"github.com/OlexiyOdarchuk/abit-assistant/pkg/abit"
)

// Histogram renders a bar chart of competitive-score distribution.
// Bars are colored by their relation to userScore: red for buckets at
// or above the user (competitors), green for buckets below.
//
// userScore=0 disables color coding (everything in neutral blue) — the
// chart is still useful as a "shape of the field" view.
//
// bucketSize controls the bin width; 5 (5-point bins) works well for
// 100..200-score programs.
func Histogram(abits []abit.Abiturient, userScore float64, bucketSize float64) ([]byte, error) {
	if len(abits) == 0 {
		return nil, fmt.Errorf("visualizer: empty applicant list")
	}
	if bucketSize <= 0 {
		bucketSize = 5
	}

	buckets := abit.Distribution(abits, bucketSize)
	if len(buckets) == 0 {
		return nil, fmt.Errorf("visualizer: distribution returned no buckets")
	}

	bars := make([]chart.Value, 0, len(buckets))
	for _, bkt := range buckets {
		bars = append(bars, chart.Value{
			Value: float64(bkt.Count),
			Label: fmt.Sprintf("%.0f", bkt.Lo),
			Style: bucketStyle(bkt, userScore),
		})
	}

	title := "Розподіл конкурсних балів"
	if userScore > 0 {
		title += fmt.Sprintf("   ·   твій бал: %.1f", userScore)
	}

	g := chart.BarChart{
		Title: title,
		TitleStyle: chart.Style{
			Padding:   chart.Box{Top: 20},
			FontSize:  16,
			FontColor: drawing.ColorBlack,
		},
		Width:      1280,
		Height:     640,
		BarWidth:   42,
		BarSpacing: 6,
		Background: chart.Style{
			Padding: chart.Box{Top: 50, Left: 30, Right: 30, Bottom: 30},
		},
		XAxis: chart.Style{FontSize: 10},
		YAxis: chart.YAxis{
			Name: "Кількість заяв",
			NameStyle: chart.Style{
				FontSize: 12,
				Padding:  chart.Box{Right: 10},
			},
			Style: chart.Style{FontSize: 10},
		},
		Bars: bars,
	}

	var buf bytes.Buffer
	if err := g.Render(chart.PNG, &buf); err != nil {
		return nil, fmt.Errorf("visualizer: render: %w", err)
	}
	return buf.Bytes(), nil
}

// bucketStyle picks a fill color for a bucket based on its position
// relative to userScore. Red = "at or above me", green = "below me".
// Buckets straddling the line (Lo < userScore < Hi) get a softer amber.
func bucketStyle(bkt abit.Bucket, userScore float64) chart.Style {
	var fill drawing.Color
	switch {
	case userScore <= 0:
		fill = drawing.ColorFromHex("4F8EF7") // neutral blue
	case bkt.Hi <= userScore:
		fill = drawing.ColorFromHex("4CAF50") // green — entirely below
	case bkt.Lo >= userScore:
		fill = drawing.ColorFromHex("E53935") // red — entirely above
	default:
		fill = drawing.ColorFromHex("FFB300") // amber — split bucket
	}
	return chart.Style{
		FillColor:   fill,
		StrokeColor: fill,
		StrokeWidth: 1,
	}
}
