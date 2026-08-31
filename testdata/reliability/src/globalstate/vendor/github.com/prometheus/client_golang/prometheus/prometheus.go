package prometheus

type Counter interface{ Inc() }
type CounterVec struct{ values map[string]float64 }
type Gauge interface{ Set(float64) }
type GaugeVec struct{ values map[string]float64 }
type Histogram interface{ Observe(float64) }
type HistogramVec struct{ values map[string][]float64 }
type Summary interface{ Observe(float64) }
type SummaryVec struct{ values map[string][]float64 }
