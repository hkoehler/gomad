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

	if file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_SYNC, 0x666); err != nil {
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
