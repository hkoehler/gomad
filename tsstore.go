package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"errors"
	"path/filepath"
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
	// list of log files in chronological order, i.e. last is current
	Logs []*TimeSeriesLog
}

// open all exisiting time series log files
func NewTimeSeries(path string, rollUp uint32, capacity uint32) (*TimeSeries, error) {
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
							/*if data, err := log.ReadAll(); err == nil {
								count += uint32(len(data))
							} else {
								return nil, err
							}*/
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

	return &TimeSeries{Path: path, RollUp: rollUp, Cap: capacity, Len: count, Logs: logs}, nil
}

// add data point with current time stamp to table
func (ts *TimeSeries) Add(val float64) error {
	var currLog *TimeSeriesLog
	
	if len(ts.Logs) == 0 {
		path := filepath.Join(ts.Path, fmt.Sprintf("%x", time.Now().Unix()))
		if log, err := NewTimeSeriesLog(path); err == nil {
			ts.Logs = append(ts.Logs, log)
			currLog = log
		} else {
			return err
		}
	} else {
		currLog = ts.Logs[len(ts.Logs)-1]
	}
	currLog.Add(val)
	ts.Len++
	return nil
}

// read up to "Cap" data points
func (ts *TimeSeries) ReadAll() ([]DataPoint, error) {
	var data = make([]DataPoint, 0)
	
	for _, log := range ts.Logs {
		// XXX sort by name
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

// time series table
type TimeSeriesTable struct {
	// base path to all time series data files
	Path string
	// name of table
	Name string
	// time series ordered by granularity from higher to lower
	TS []*TimeSeries
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
	tbl := &TimeSeriesTable{Path: path, Name: name, TS: make([]*TimeSeries, 0, len(tsProps))}
	for id, prop := range tsProps {
		tsPath := filepath.Join(path, string(id))
		if ts, err := NewTimeSeries(tsPath, prop.RollUp, prop.Cap); err == nil {
			tbl.TS = append(tbl.TS, ts)
		} else {
			return nil, err
		}
	}
	return tbl, nil
}
