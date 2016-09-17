package main

import (
	_ "io"
	_ "os"
	"time"
)

// single data point
type DataPoint struct {
	Tstamp time.Time
	Val    float64
}

// time series of data points recorded at same frequency
type TimeSeries struct {
	// name of time series
	Name string
	// how many data points to coalesce on roll-over
	RollUp uint32
	// min number of data points preserved
	Cap uint32
}

func NewTimeSeries(rollUp uint32, capacity uint32) *TimeSeries {
	return &TimeSeries{RollUp: rollUp, Cap: capacity}
}

// record new data point at current time
// this might trigger multiple previous data points to be rolled up into single data point
// previous data points are deleted in batches for sake of efficiency
func (ts *TimeSeries) Add(val float64) {

}

// return cursor at beginning of time series
func (ts *TimeSeries) Begin() int {
	return 0
}

// advance cursor by one data point
func (ts *TimeSeries) Next(cursor int) int {
	return cursor + 1
}

// return cursor at end of time series
func (ts *TimeSeries) End() int {
	return -1
}

// time series table
type TimeSeriesTable struct {
	// name of table
	Name string
	// time series ordered by granularity from higher to lower
	TS []TimeSeries
}

func NewTimeSeriesTable(rollUps []uint32, caps []uint32) *TimeSeriesTable {
	return &TimeSeriesTable{}
}
	