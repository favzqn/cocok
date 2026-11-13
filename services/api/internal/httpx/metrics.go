package httpx

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"time"
)

// Metrics is a dependency-free Prometheus-text counter set. Swap for the
// official client when cardinality needs grow.
type Metrics struct {
	mu      sync.Mutex
	status  map[int]uint64
	started time.Time
}

func newMetrics() *Metrics {
	return &Metrics{status: map[int]uint64{}, started: time.Now()}
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Middleware counts responses by status code.
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		m.mu.Lock()
		m.status[rec.status]++
		m.mu.Unlock()
	})
}

// Handler renders Prometheus text exposition format.
func (m *Metrics) Handler(w http.ResponseWriter, _ *http.Request) {
	m.mu.Lock()
	codes := make([]int, 0, len(m.status))
	for c := range m.status {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintln(w, "# HELP cocok_http_responses_total HTTP responses by status code.")
	fmt.Fprintln(w, "# TYPE cocok_http_responses_total counter")
	for _, c := range codes {
		fmt.Fprintf(w, "cocok_http_responses_total{status=\"%d\"} %d\n", c, m.status[c])
	}
	uptime := time.Since(m.started).Seconds()
	fmt.Fprintln(w, "# HELP cocok_process_uptime_seconds Seconds since process start.")
	fmt.Fprintln(w, "# TYPE cocok_process_uptime_seconds gauge")
	fmt.Fprintf(w, "cocok_process_uptime_seconds %.0f\n", uptime)
	m.mu.Unlock()
}

// authRateRPS is configurable for load tests; production default stays strict.
func authRateRPS() float64 {
	if v := os.Getenv("AUTH_RATE_RPS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f > 0 {
			return f
		}
	}
	return 0.5
}
