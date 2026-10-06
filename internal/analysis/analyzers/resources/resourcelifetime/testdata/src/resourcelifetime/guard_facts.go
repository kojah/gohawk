package resourcelifetime

// Repeated guards: an acquisition under one guard and its cleanup under the
// same guard are correlated. The contradicting arm of the later test is
// unknown, never proved infeasible, so a visible store to the guard, another
// guard, or another constant keeps the report.

import (
	"compress/gzip"
	"io"
	"os"
)

type profileOptions struct {
	Profiler string
	Memory   string
	Mode     string
}

func parseFlag(flag *bool) {}

func work() {}

func guardedByLoadedFlag(path string) error {
	var enabled bool
	parseFlag(&enabled)
	var file *os.File
	if enabled {
		created, err := os.Create(path)
		if err != nil {
			return err
		}
		file = created
	}
	work()
	if enabled {
		return file.Close()
	}
	return nil
}

func guardedByBooleanParameter(enabled bool, path string) error {
	var file *os.File
	if enabled {
		created, err := os.Create(path)
		if err != nil {
			return err
		}
		file = created
	}
	work()
	if enabled {
		return file.Close()
	}
	return nil
}

func guardedByOptionField(o *profileOptions) error {
	var cpu *os.File
	if o.Profiler != "" {
		created, err := os.Create(o.Profiler + ".cpu")
		if err != nil {
			return err
		}
		cpu = created
	}
	work()
	if o.Profiler != "" {
		return cpu.Close()
	}
	return nil
}

func guardedByModeComparison(o *profileOptions) error {
	var out *os.File
	if o.Mode != "direct" {
		created, err := os.Create(o.Mode)
		if err != nil {
			return err
		}
		out = created
	}
	work()
	if o.Mode == "direct" {
		return nil
	}
	return out.Close()
}

type compressionCase struct {
	compressed bool
}

func guardedPerIteration(cases []compressionCase, sink io.Writer) {
	for _, test := range cases {
		var compressor *gzip.Writer
		if test.compressed {
			compressor = gzip.NewWriter(sink)
		}
		work()
		if test.compressed {
			_ = compressor.Close()
		}
	}
}

func guardStoredBetween(path string) error {
	var enabled bool
	parseFlag(&enabled)
	var file *os.File
	if enabled {
		created, err := os.Create(path) // want "owned resource from os.Create is not released"
		if err != nil {
			return err
		}
		file = created
	}
	enabled = false
	if enabled {
		return file.Close()
	}
	return nil
}

func guardOppositePolarity(path string) error {
	var skip bool
	parseFlag(&skip)
	var file *os.File
	if !skip {
		created, err := os.Create(path) // want "owned resource from os.Create is not released"
		if err != nil {
			return err
		}
		file = created
	}
	work()
	if skip {
		return file.Close()
	}
	return nil
}

func guardDifferentFields(o *profileOptions) error {
	var cpu *os.File
	if o.Profiler != "" {
		created, err := os.Create(o.Profiler + ".cpu") // want "owned resource from os.Create is not released"
		if err != nil {
			return err
		}
		cpu = created
	}
	work()
	if o.Memory != "" {
		return cpu.Close()
	}
	return nil
}

func guardDifferentConstants(o *profileOptions) error {
	var out *os.File
	if o.Mode != "direct" {
		created, err := os.Create(o.Mode) // want "owned resource from os.Create is not released"
		if err != nil {
			return err
		}
		out = created
	}
	work()
	if o.Mode == "buffered" {
		return out.Close()
	}
	return nil
}

func guardStoredInLoop(path string, next func() bool) error {
	var enabled bool
	parseFlag(&enabled)
	for {
		var file *os.File
		if enabled {
			created, err := os.Create(path) // want "owned resource from os.Create is not released"
			if err != nil {
				return err
			}
			file = created
		}
		enabled = next()
		if enabled {
			_ = file.Close()
		}
		if !enabled {
			return nil
		}
	}
}
