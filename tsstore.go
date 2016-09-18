package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"time"
)

// single data point
type DataPoint struct {
	Tstamp time.Time
	Val    float64
}

// time series log file
// representing a single partition of a time series
type TimeSeriesLog struct {
	// path to underlying file
	path string
	// underlying file
	file *os.File
	// encoder transmitting on file
	enc *gob.Encoder
}

// Open or create new time series log file
func NewTimeSeriesLog(path string) (*TimeSeriesLog, error) {
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666); err != nil {
		return nil, err
	} else {
		enc := gob.NewEncoder(f)
		return &TimeSeriesLog{path, f, enc}, nil
	}
}

// Stringer interface
func (log *TimeSeriesLog) String() string {
	return fmt.Sprintf("path=%s, file=%d, enc=%s", log.path, log.file.Fd(), log.enc)
}

// append new record to log file
func (log *TimeSeriesLog) Add(val float64) error {
	return log.enc.Encode(DataPoint{time.Now(), val})
}

// read and decode whole log file
func (log *TimeSeriesLog) ReadAll() ([]DataPoint, error) {
	data := make([]DataPoint, 0)

	if f, err := os.Open(log.path); err != nil {
		return nil, err
	} else {
		defer f.Close()
		dec := gob.NewDecoder(f)
		for {
			var dp DataPoint

			if err := dec.Decode(&dp); err == nil {
				data = append(data, dp)
			} else if err == io.EOF {
				break
			} else {
				return nil, err
			}
		}
	}
	return data, nil
}

// close log file
func (log *TimeSeriesLog) Close() {
	if log.file != nil {
		log.file.Close()
		log.file = nil
	}
}

// remove log file
func (log *TimeSeriesLog) Remove() {
	os.Remove(log.path)
}

// time series of data points recorded at same frequency
type TimeSeries struct {
	// identifier within time series table
	ID uint32
	// how many data points to coalesce on roll-up
	RollUp uint32
	// min number of data points preserved
	Cap uint32
	// number of data points
	Len uint32
}

func NewTimeSeries(id uint32, rollUp uint32, capacity uint32) TimeSeries {
	return TimeSeries{ID: id, RollUp: rollUp, Cap: capacity}
}

// add data point with current time stamp to table
func (ts *TimeSeries) Add(val float64) {
	ts.Len++
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
	// base path to all time series data files
	Path string
	// name of table
	Name string
	// time series ordered by granularity from higher to lower
	TS []TimeSeries
}

// record new data point at current time
// this might trigger multiple previous data points to be rolled up into single data point
// previous data points are deleted in batches for sake of efficiency
func (tbl *TimeSeriesTable) Add(val float64) {

}

type TimeSeriesProps struct {
	// number of data points to coalesce into single data point on roll-up
	RollUp uint32
	// total number of data point to be kept in time series
	Cap uint32
}

func NewTimeSeriesTable(path string, name string, tsProps []TimeSeriesProps) (*TimeSeriesTable, error) {
	tbl := &TimeSeriesTable{Path: path, Name: name, TS: make([]TimeSeries, 0, len(tsProps))}
	for id, prop := range tsProps {
		ts := NewTimeSeries(uint32(id), prop.RollUp, prop.Cap)
		tbl.TS = append(tbl.TS, ts)
	}
	return tbl, nil
}
