// Copyright (C) 2016, Heiko Koehler

package main

import (
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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
				// ignore "extra data in buffer" error
				//return nil, err
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

type TimeSeriesLogs []*TimeSeriesLog

// implement Sorter interface for time series log arrays
func (logs TimeSeriesLogs) Len() int           { return len(logs) }
func (logs TimeSeriesLogs) Swap(i, j int)      { logs[i], logs[j] = logs[j], logs[i] }
func (logs TimeSeriesLogs) Less(i, j int) bool { return logs[i].path < logs[j].path }

// time series of data points recorded at same frequency
// data series is partitioned into multiple log to allow for fast deletion
// of expired data points
type TimeSeries struct {
	// base path of all log files
	Path string
	// how many data points to coalesce on roll-up
	RollUp uint32
	// min number of data points preserved
	// this is also the max. number of data points returned by ReadAll
	Cap uint32
	// number of data points
	Len uint32
	// next log ID
	NextID int
	// list of log files in chronological order, i.e. last is current
	Logs []*TimeSeriesLog

	// lower-level time series
	LowerLevel *TimeSeries
	// number of values in current coalescing/roll-up batch
	// can never be greater than RollUp
	BatchLen int
	// cumulative value of all elements in batch
	BatchVal float64
}

// open all exisiting time series log files
func NewTimeSeries(path string, rollUp uint32,
	capacity uint32, lowerLevel *TimeSeries) (*TimeSeries, error) {

	var count uint32
	var logs = make([]*TimeSeriesLog, 0)

	if fi, err := os.Stat(path); err == nil {
		// directory exists already
		if fi.Mode().IsDir() == false {
			return nil, errors.New("Path not a directory")
		}
		if dir, err := os.Open(path); err == nil {
			for {
				if fileInfos, err := dir.Readdir(64); err == nil {
					for _, fi := range fileInfos {
						filePath := filepath.Join(path, fi.Name())
						if log, err := NewTimeSeriesLog(filePath); err == nil {
							if data, err := log.ReadAll(); err == nil {
								count += uint32(len(data))
							} else {
								return nil, err
							}
							logs = append(logs, log)
						} else {
							return nil, err
						}
					}
				} else if err == io.EOF {
					break
				} else {
					return nil, err
				}
			}
		} else {
			return nil, err
		}
	} else if err != os.ErrNotExist {
		// create directory
		os.MkdirAll(path, 0770)
	} else {
		return nil, err
	}

	sort.Sort(TimeSeriesLogs(logs))
	// retrieve ID of next log file for Add()
	nextID := 0
	if len(logs) > 0 {
		currLog := logs[len(logs)-1]
		currLogName := filepath.Base(currLog.path)
		fmt.Sscanf(currLogName, "%d", &nextID)
	}
	return &TimeSeries{Path: path, RollUp: rollUp, Cap: capacity,
		Len: count, Logs: logs, NextID: nextID, LowerLevel: lowerLevel}, nil
}

// calculate max size of a log file
func (ts *TimeSeries) BucketSize() uint32 {
	return ts.Cap / 2
}

// add data point with current time stamp to table
func (ts *TimeSeries) Add(val float64) error {
	var currLog *TimeSeriesLog

	// create new bucket if either bucket is full or no bucket exists yet
	if ts.Len%ts.BucketSize() == 0 {
		// bucket size is ts.Cap divided by 2 hence 2 full buckets
		// are suffient to keep ts.Cap data points
		if len(ts.Logs) > 2 {
			oldLogs := ts.Logs[0 : len(ts.Logs)-2]
			ts.Logs = ts.Logs[len(ts.Logs)-2:]
			for _, oldLog := range oldLogs {
				oldLog.Remove()
			}
		}
		path := filepath.Join(ts.Path, fmt.Sprintf("%d", ts.NextID))
		if log, err := NewTimeSeriesLog(path); err == nil {
			ts.Logs = append(ts.Logs, log)
			currLog = log
			ts.NextID++
		} else {
			return err
		}
	} else {
		currLog = ts.Logs[len(ts.Logs)-1]
	}
	currLog.Add(val)
	ts.Len++

	// coalesce current batch into single value for lower TS level with lower granularity
	if ts.LowerLevel != nil {
		ts.BatchVal += float64(val)
		ts.BatchLen++
		if ts.BatchLen == int(ts.RollUp) {
			err := ts.LowerLevel.Add(ts.BatchVal / float64(ts.BatchLen))
			ts.BatchVal = 0
			ts.BatchLen = 0
			return err
		}
	}
	return nil
}

// read up to "Cap" data points
func (ts *TimeSeries) ReadAll() ([]DataPoint, error) {
	var data = make([]DataPoint, 0)

	for _, log := range ts.Logs {
		if tmp, err := log.ReadAll(); err == nil {
			data = append(data, tmp...)
		} else {
			return nil, err
		}
	}
	return data, nil
}

// close table including all log files
func (ts *TimeSeries) Close() {
	for _, log := range ts.Logs {
		log.Close()
	}
}

// remove table including all log files
func (ts *TimeSeries) Remove() {
	for _, log := range ts.Logs {
		log.Remove()
	}
}

// time series table
type TimeSeriesTable struct {
	// base path to all time series data files
	Path string
	// time series ordered by granularity from lower to higher
	TS []*TimeSeries
}

type TimeSeriesProps struct {
	// number of data points to coalesce into single data point on roll-up
	RollUp uint32
	// total number of data point to be kept in time series
	Cap uint32
}

// create local time series with different levels of granularities as specified in tsProps
func NewTimeSeriesTable(path string, tsProps []TimeSeriesProps) (*TimeSeriesTable, error) {
	var tsList = make([]*TimeSeries, 0)
	var prevTS *TimeSeries

	if len(tsProps) == 0 {
		return nil, errors.New("No time series specified on any level")
	}

	fmt.Println(path)
	for id := len(tsProps) - 1; id >= 0; id-- {
		prop := tsProps[id]
		tsPath := filepath.Join(path, fmt.Sprintf("%d", id))
		if ts, err := NewTimeSeries(tsPath, prop.RollUp, prop.Cap, prevTS); err == nil {
			tsList = append(tsList, ts)
			prevTS = ts
		} else {
			return nil, err
		}
	}
	return &TimeSeriesTable{Path: path, TS: tsList}, nil
}

// return top level time series
func (tbl *TimeSeriesTable) TopLevel() *TimeSeries {
	return tbl.TS[len(tbl.TS)-1]
}

// record new data point at current time
// this might trigger multiple previous data points to be rolled up into single data point
// previous data points are deleted in batches for sake of efficiency
func (tbl *TimeSeriesTable) Add(val float64) error {
	ts := tbl.TopLevel()
	return ts.Add(val)
}

// close all time series logs
func (tbl *TimeSeriesTable) Close() {
	for _, ts := range tbl.TS {
		ts.Close()
	}
}

// remove all time series logs
func (tbl *TimeSeriesTable) Remove() {
	for _, ts := range tbl.TS {
		ts.Remove()
	}
	os.RemoveAll(tbl.Path)
}
