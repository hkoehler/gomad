// Copyright (C) 2016, Heiko Koehler

package main

import (
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMarshalling(t *testing.T) {
	var dp2 DataPoint
	var dp = DataPoint{time.Now(), 0xdeadbeef}
	var path = filepath.Join(os.TempDir(), "TestMarshalling.gob")

	defer os.Remove(path)

	if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_SYNC, 0666); err != nil {
		t.Fatal(err)
	} else {
		defer file.Close()
		enc := gob.NewEncoder(file)
		enc.Encode(dp)
	}

	if file, err := os.Open(path); err != nil {
		t.Fatal(err)
	} else {
		defer file.Close()
		dec := gob.NewDecoder(file)
		dec.Decode(&dp2)
		if dp != dp2 {
			t.Fatal("encoded and decoded data points don't match: %v vs %v", dp, dp2)
		}
	}
}

func TestTimeSeriesLog(t *testing.T) {
	dir := os.TempDir()
	path := filepath.Join(dir, "timeSeriesTest.log")

	t.Logf("Created new time series log at: %s\n", path)
	log, err := NewTimeSeriesLog(path)
	if err != nil {
		t.Fatal(err)
	}
	// clean up file
	defer func() {
		log.Close()
		log.Remove()
	}()

	// log test data
	for i := 0; i < 1000; i++ {
		if err := log.Add(float64(i)); err != nil {
			t.Fatal(err)
		}
	}
	// read test data
	if data, err := log.ReadAll(); err != nil {
		t.Fatal(err)
	} else if len(data) != 1000 {
		t.Fatalf("Only read %d entries\n", len(data))
	} else {
		// check test data
		for i := 0; i < 1000; i++ {
			if data[i].Val != float64(i) {
				t.Fatalf("Expected val = %d got %d\n", i, data[i].Val)
			}
		}
	}
}

func validateTimeSeries(t *testing.T, ts *TimeSeries, expLen int, expIncr float64) {
	if data, err := ts.ReadAll(); err != nil {
		t.Fatal(err)
	} else {
		if len(data) < expLen {
			t.Fatalf("only %d data points read", len(data))
		}
		for i := 1; i < expLen; i++ {
			if data[i].Val != data[i-1].Val+expIncr {
				t.Fatalf("read %d at %d (expected %d) in ts %s",
					int(data[i].Val), i, int(data[i-1].Val+expIncr), ts.Path)
			}
		}
	}
}

func TestTimeSeries(t *testing.T) {
	path := filepath.Join(os.TempDir(), "TestTimeSeries")

	if ts, err := NewTimeSeries(path, 10, 100, nil); err == nil {
		defer ts.Close()

		for i := 0; i < 200; i++ {
			if err := ts.Add(float64(i)); err != nil {
				t.Fatal(err)
			}
		}
		validateTimeSeries(t, ts, 100, 1)
	} else {
		t.Fatal(err)
	}
}

func TestCoalescing(t *testing.T) {
	var ts1, ts2 *TimeSeries
	var err error

	defer func() {
		if ts1 != nil {
			ts1.Close()
		}
		if ts2 != nil {
			ts2.Close()
		}
	}()

	tsPath1 := filepath.Join(os.TempDir(), "TestCoalescing", "ts1")
	tsPath2 := filepath.Join(os.TempDir(), "TestCoalescing", "ts2")

	if ts2, err = NewTimeSeries(tsPath2, 10, 100, nil); err != nil {
		t.Fatal(err)
	}
	// use ts2 as lower level for ts1
	if ts1, err = NewTimeSeries(tsPath1, 10, 100, ts2); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 200; i++ {
		if err := ts1.Add(float64(i)); err != nil {
			t.Fatal(err)
		}
	}

	validateTimeSeries(t, ts1, 100, 1)
	validateTimeSeries(t, ts2, 10, 10)
}

func TestTimeSeriesTable(t *testing.T) {
	path := filepath.Join(os.TempDir(), "TestTimeSeriesTable")
	// keep 100 data points on each level, roll up every 10 data points
	if tbl, err := NewTimeSeriesTable(path, []TimeSeriesProps{{10, 100}, {10, 100}, {10, 100}}); err == nil {
		defer tbl.Remove()

		for i := 0; i < 2000; i++ {
			if err := tbl.Add(float64(i)); err != nil {
				t.Fatal(err)
			}
		}
		validateTimeSeries(t, tbl.TS[2], 100, 1)
		validateTimeSeries(t, tbl.TS[1], 100, 10)
		validateTimeSeries(t, tbl.TS[0], 20, 100)
	} else {
		t.Fatal(err)
	}
}
