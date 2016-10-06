package main 

import (
	"time"
	"io"
	"math"
	
	"github.com/wcharczuk/go-chart"
)

func PlotTimeSeries(w io.Writer, ts *TimeSeries, prop string) {
	xvalues := make([]time.Time, 0)
	yvalues := make([]float64, 0)
	var max float64 = 1

	if data, err := ts.ReadAll(); err == nil {
		for _, dp := range data {
			xvalues = append(xvalues, dp.Tstamp)
			yvalues = append(yvalues, dp.Val)
			max = math.Max(max, dp.Val)
		}
	}

	graph := chart.Chart{
		XAxis: chart.XAxis{
			Style: chart.Style{Show: true},
			ValueFormatter: chart.TimeMinuteValueFormatter,
		},
		YAxis: chart.YAxis{
			Style: chart.Style{Show: true},
			Range: &chart.ContinuousRange{Min: 0, Max: max},
		},
		Series: []chart.Series{
			chart.TimeSeries{
				Name: prop,
				Style: chart.Style{
					Show:        true,
					StrokeColor: chart.GetDefaultColor(0),
				},
				XValues: xvalues,
				YValues: yvalues,
			},
		},
	}
	graph.Render(chart.SVG, w)
}
