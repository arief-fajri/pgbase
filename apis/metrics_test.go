package apis

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/router"
	"github.com/arief-fajri/pgbase/tools/subscriptions"
	"github.com/pocketbase/dbx"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestRouteLabel(t *testing.T) {
	scenarios := []struct {
		pattern string
		want    string
	}{
		{"", "unmatched"},
		{"/", "/"},
		{"GET /api/health", "/api/health"},
		{"POST /api/collections/{collection}/records", "/api/collections/{collection}/records"},
		{"GET /_/{path...}", "/_/{path...}"},
	}
	for _, s := range scenarios {
		if got := routeLabel(s.pattern); got != s.want {
			t.Errorf("routeLabel(%q) = %q, want %q", s.pattern, got, s.want)
		}
	}
}

func TestMetricsBindAddr(t *testing.T) {
	t.Setenv(MetricsAddrEnv, "")
	if got := metricsBindAddr(); got != "" {
		t.Fatalf("expected empty (disabled), got %q", got)
	}

	t.Setenv(MetricsAddrEnv, "  127.0.0.1:9090  ")
	if got := metricsBindAddr(); got != "127.0.0.1:9090" {
		t.Fatalf("expected trimmed addr, got %q", got)
	}
}

// TestMetricsMiddlewareObservesRoutePattern verifies the HTTP histogram is
// labeled by the low-cardinality matched route pattern (e.g. "/api/test/{id}")
// rather than the raw request path, and captures method + status.
func TestMetricsMiddlewareObservesRoutePattern(t *testing.T) {
	reg := prometheus.NewRegistry()
	hist := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "test",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})
	reg.MustRegister(hist)
	m := &appMetrics{registry: reg, httpDuration: hist}

	r := router.NewRouter(func(w http.ResponseWriter, req *http.Request) (*core.RequestEvent, router.EventCleanupFunc) {
		e := new(core.RequestEvent)
		e.Response = w
		e.Request = req
		return e, nil
	})
	r.Bind(metricsMiddleware(m))
	r.GET("/api/test/{id}", func(e *core.RequestEvent) error {
		return e.String(http.StatusOK, "ok")
	})

	mux, err := r.BuildMux()
	if err != nil {
		t.Fatalf("BuildMux: %v", err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/test/123", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	// two distinct ids must collapse onto the SAME templated series
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/api/test/456", nil))

	count := histogramSampleCount(t, reg, "pgbase_http_request_duration_seconds", map[string]string{
		"method": "GET",
		"route":  "/api/test/{id}",
		"status": "200",
	})
	if count != 2 {
		t.Fatalf("expected 2 observations collapsed onto the templated route, got %d", count)
	}
}

// TestServeMetricsListenerServesEndpoint verifies the serving path actually
// exposes GET /metrics over HTTP and renders a registered pgbase metric.
func TestServeMetricsListenerServesEndpoint(t *testing.T) {
	reg := prometheus.NewRegistry()
	probe := prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricsNamespace,
		Name:      "e2e_probe",
		Help:      "test",
	})
	probe.Set(1)
	reg.MustRegister(probe)
	m := &appMetrics{registry: reg}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	app := core.NewBaseApp(core.BaseAppConfig{})
	shutdown := serveMetricsListener(app, m, ln)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = shutdown(ctx)
	}()

	resp, err := http.Get("http://" + ln.Addr().String() + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !strings.Contains(string(body), "pgbase_e2e_probe") {
		t.Fatalf("expected pgbase_e2e_probe in scrape output, got:\n%s", body)
	}
}

func TestSQLDBFromBuilder(t *testing.T) {
	db, err := sql.Open(fakeDriverName, "")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	if got := sqlDBFromBuilder(dbx.NewFromDB(db, "postgres")); got == nil {
		t.Fatal("expected non-nil *sql.DB from a *dbx.DB builder")
	}

	if got := sqlDBFromBuilder(nil); got != nil {
		t.Fatalf("expected nil for a nil builder, got %v", got)
	}
}

// TestDBStatsCollector verifies the collector emits pool metrics for BOTH the
// data and aux pools (labeled db="data"/"aux").
func TestDBStatsCollector(t *testing.T) {
	data, err := sql.Open(fakeDriverName, "")
	if err != nil {
		t.Fatalf("open data: %v", err)
	}
	defer data.Close()

	aux, err := sql.Open(fakeDriverName, "")
	if err != nil {
		t.Fatalf("open aux: %v", err)
	}
	defer aux.Close()

	c := newDBStatsCollector(stubDBProvider{
		data: dbx.NewFromDB(data, "postgres"),
		aux:  dbx.NewFromDB(aux, "postgres"),
	})

	reg := prometheus.NewRegistry()
	reg.MustRegister(c)

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	seen := map[string]bool{}
	for _, mf := range mfs {
		if mf.GetName() != "pgbase_db_open_connections" {
			continue
		}
		for _, metric := range mf.GetMetric() {
			for _, l := range metric.GetLabel() {
				if l.GetName() == "db" {
					seen[l.GetValue()] = true
				}
			}
		}
	}
	if !seen["data"] || !seen["aux"] {
		t.Fatalf("expected open_connections for both data and aux pools, got %v", seen)
	}
}

// TestRealtimeCollector verifies the connected-clients gauge and the
// dropped-messages sum reflect live broker state.
func TestRealtimeCollector(t *testing.T) {
	broker := subscriptions.NewBroker()
	c := newRealtimeCollector(stubRealtimeProvider{broker: broker})

	if got := gaugeValue(t, c, "pgbase_realtime_connected_clients"); got != 0 {
		t.Fatalf("expected 0 connected clients on an empty broker, got %v", got)
	}

	client := subscriptions.NewDefaultClient()
	broker.Register(client)
	broker.Register(subscriptions.NewDefaultClient())

	// overflow the per-client buffer (cap 32) without a reader to force drops
	for i := 0; i < 64; i++ {
		client.Send(subscriptions.Message{Name: "x"})
	}

	if got := gaugeValue(t, c, "pgbase_realtime_connected_clients"); got != 2 {
		t.Fatalf("expected 2 connected clients, got %v", got)
	}
	if got := gaugeValue(t, c, "pgbase_realtime_dropped_messages"); got < 1 {
		t.Fatalf("expected at least 1 dropped message, got %v", got)
	}
}

// --- test helpers -----------------------------------------------------------

const fakeDriverName = "apismetricsfakedriver"

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) {
	// never actually invoked: the tests only read pool Stats(), which does not
	// open a physical connection.
	return nil, errors.New("metrics test: no real connection")
}

func init() {
	sql.Register(fakeDriverName, fakeDriver{})
}

type stubDBProvider struct {
	data dbx.Builder
	aux  dbx.Builder
}

func (s stubDBProvider) DB() dbx.Builder    { return s.data }
func (s stubDBProvider) AuxDB() dbx.Builder { return s.aux }

type stubRealtimeProvider struct {
	broker *subscriptions.Broker
}

func (s stubRealtimeProvider) SubscriptionsBroker() *subscriptions.Broker { return s.broker }

func histogramSampleCount(t *testing.T, g prometheus.Gatherer, name string, labels map[string]string) uint64 {
	t.Helper()
	mfs, err := g.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, metric := range mf.GetMetric() {
			if labelsMatch(metric.GetLabel(), labels) {
				return metric.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func labelsMatch(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(pairs) != len(want) {
		return false
	}
	for _, p := range pairs {
		if want[p.GetName()] != p.GetValue() {
			return false
		}
	}
	return true
}

func gaugeValue(t *testing.T, c prometheus.Collector, name string) float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(c)
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		if len(mf.GetMetric()) == 0 {
			return 0
		}
		return mf.GetMetric()[0].GetGauge().GetValue()
	}
	return 0
}
