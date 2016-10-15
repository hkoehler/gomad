package main

import (
	"io"
	"math"
	"time"

	"github.com/wcharczuk/go-chart"
)

func chartSeries(i int, ts *TimeSeries, prop string, max *float64) chart.Series {
	xvalues := make([]time.Time, 0)
	yvalues := make([]float64, 0)

	if data, err := ts.ReadAll(); err == nil {
		for _, dp := range data {
			xvalues = append(xvalues, dp.Tstamp)
			yvalues = append(yvalues, dp.Val)
			*max = math.Max(*max, dp.Val)
		}
	}
	return chart.TimeSeries{
		Name: prop,
		Style: chart.Style{
			Show:        true,
			StrokeColor: chart.GetDefaultColor(i),
		},
		XValues: xvalues,
		YValues: yvalues,
	}
}

func PlotTimeSeries(w io.Writer, ts []*TimeSeries, prop []string) {
	var max float64 = 1
	series := make([]chart.Series, 0)

	for i := range ts {
		series = append(series, chartSeries(i, ts[i], prop[i], &max))
	}
	graph := chart.Chart{
		XAxis: chart.XAxis{
			Style:          chart.Style{Show: true},
			ValueFormatter: chart.TimeMinuteValueFormatter,
		},
		YAxis: chart.YAxis{
			Style: chart.Style{Show: true},
			Range: &chart.ContinuousRange{Min: 0, Max: max},
		},
		Series: series,
	}
	if len(ts) > 1 {
		graph.Elements = []chart.Renderable{
			chart.Legend(&graph),
		}
	}
	graph.Render(chart.SVG, w)
}
