package main

import "testing"

func TestPeakRSSKiB(t *testing.T) {
	tests := []struct {
		name  string
		usage any
		goos  string
		want  int64
	}{
		{name: "linux value is already KiB", usage: &testRusage{Maxrss: 4096}, goos: "linux", want: 4096},
		{name: "darwin value is bytes", usage: &testRusage{Maxrss: 4096}, goos: "darwin", want: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := peakRSSKiB(test.usage, test.goos)
			if err != nil {
				t.Fatalf("peakRSSKiB() error = %v", err)
			}
			if got != test.want {
				t.Errorf("peakRSSKiB() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestPeakRSSKiBRejectsUnavailableUsage(t *testing.T) {
	if _, err := peakRSSKiB(nil, "linux"); err == nil {
		t.Fatal("peakRSSKiB() error = nil, want unavailable usage error")
	}
}

type testRusage struct {
	Maxrss int64
}
