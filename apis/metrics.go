package apis

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/arief-fajri/pgbase/core"
	"github.com/arief-fajri/pgbase/tools/hook"
	"github.com/arief-fajri/pgbase/tools/router"
	"github.com/arief-fajri/pgbase/tools/subscriptions"
	"github.com/pocketbase/dbx"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// MetricsAddrEnv is the env var that enables and configures the Prometheus
// metrics endpoint. When set to a non-empty bind address (e.g.
// "127.0.0.1:9090") a dedicated HTTP server exposing GET /metrics is started
// on that address; when unset/empty the endpoint is disabled and no extra
// listener is opened.
//
// The listener is deliberately SEPARATE from the public API server so metrics
// can be bound to loopback (or an internal interface) and never exposed on the
// same port that serves user traffic.
const MetricsAddrEnv = "PB_METRICS_ADDR"

// metricsNamespace prefixes every custom pgbase metric.
const metricsNamespace = "pgbase"

// metricsMiddlewareId is the id of the HTTP instrumentation middleware so it
// can be referenced/unbound if needed.
const metricsMiddlewareId = "pbMetrics"

// metricsBindAddr returns the trimmed PB_METRICS_ADDR value ("" = disabled).
func metricsBindAddr() string {
	return strings.TrimSpace(os.Getenv(MetricsAddrEnv))
}

// appMetrics bundles the Prometheus registry and the request histogram for a
// single app/server instance. A fresh registry (not the global default) is
// used so metrics are isolated per instance, which keeps tests hermetic and
// avoids accidental duplicate-registration panics.
type appMetrics struct {
	registry     *prometheus.Registry
	httpDuration *prometheus.HistogramVec
}

// newAppMetrics builds the registry with the free Go runtime + process
// collectors, the DB pool and realtime scrape-time collectors, and the HTTP
// request histogram.
func newAppMetrics(app core.App) *appMetrics {
	reg := prometheus.NewRegistry()

	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	httpDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricsNamespace,
		Subsystem: "http",
		Name:      "request_duration_seconds",
		Help:      "Duration of handled HTTP requests, labeled by route pattern, method and status.",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route", "status"})
	reg.MustRegister(httpDuration)

	reg.MustRegister(newDBStatsCollector(app))
	reg.MustRegister(newRealtimeCollector(app))

	return &appMetrics{registry: reg, httpDuration: httpDuration}
}

// metricsMiddleware instruments every request with a duration observation
// labeled by the matched route pattern (low cardinality, from the Go 1.22+
// [http.Request.Pattern]) rather than the raw URL path (unbounded cardinality).
func metricsMiddleware(m *appMetrics) *hook.Handler[*core.RequestEvent] {
	return &hook.Handler[*core.RequestEvent]{
		Id: metricsMiddlewareId,
		Func: func(e *core.RequestEvent) error {
			// capture the status tracker BEFORE Next(): downstream middlewares
			// (e.g. gzip) may replace e.Response with their own writer, but the
			// underlying router.ResponseWriter keeps tracking the real status.
			tracker, _ := e.Response.(router.StatusTracker)

			start := time.Now()
			err := e.Next()
			elapsed := time.Since(start).Seconds()

			status := http.StatusOK
			if tracker != nil {
				if s := tracker.Status(); s != 0 {
					status = s
				}
			}

			m.httpDuration.
				WithLabelValues(e.Request.Method, routeLabel(e.Request.Pattern), strconv.Itoa(status)).
				Observe(elapsed)

			return err
		},
	}
}

// routeLabel normalizes a matched mux pattern into a low-cardinality route
// label by stripping the leading method token (kept as its own label) and
// mapping the empty (unmatched) pattern to a stable placeholder.
func routeLabel(pattern string) string {
	if pattern == "" {
		return "unmatched"
	}
	if sp := strings.IndexByte(pattern, ' '); sp >= 0 {
		return pattern[sp+1:]
	}
	return pattern
}

// serveMetrics starts a dedicated HTTP server exposing GET /metrics on addr and
// returns a shutdown func. It is intentionally isolated from the main API
// server; bind addr to loopback to keep the endpoint private.
func serveMetrics(app core.App, m *appMetrics, addr string) (func(context.Context) error, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return serveMetricsListener(app, m, ln), nil
}

// serveMetricsListener serves GET /metrics on an already-open listener and
// returns a shutdown func. It is split from serveMetrics so the serving path
// can be exercised over a real ephemeral-port listener in tests without
// guessing a free port.
func serveMetricsListener(app core.App, m *appMetrics, ln net.Listener) func(context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// never fail a scrape because a single collector hiccuped
		ErrorHandling: promhttp.ContinueOnError,
		ErrorLog:      metricsErrorLogger{app: app},
	}))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			app.Logger().Error("Metrics server stopped unexpectedly", "error", serveErr.Error())
		}
	}()

	app.Logger().Info("Metrics endpoint enabled", "addr", ln.Addr().String(), "path", "/metrics")

	return server.Shutdown
}

// metricsErrorLogger adapts the app logger to the promhttp.Logger interface.
type metricsErrorLogger struct {
	app core.App
}

func (l metricsErrorLogger) Println(v ...any) {
	l.app.Logger().Warn(fmt.Sprint(v...))
}

// dbStatsProvider is the narrow slice of core.App the DB pool collector needs
// (segregated so the collector can be unit-tested without a full app).
type dbStatsProvider interface {
	DB() dbx.Builder
	AuxDB() dbx.Builder
}

// realtimeStatsProvider is the narrow slice of core.App the realtime collector
// needs.
type realtimeStatsProvider interface {
	SubscriptionsBroker() *subscriptions.Broker
}

// dbStatsCollector reports the [sql.DB.Stats] of the data and aux connection
// pools at scrape time (directly addresses the pool-ceiling concern from the
// performance audit — operators can watch open/in-use/wait grow).
type dbStatsCollector struct {
	provider dbStatsProvider

	open      *prometheus.Desc
	inUse     *prometheus.Desc
	idle      *prometheus.Desc
	maxOpen   *prometheus.Desc
	waitCount *prometheus.Desc
	waitDur   *prometheus.Desc
}

func newDBStatsCollector(provider dbStatsProvider) *dbStatsCollector {
	labels := []string{"db"}
	desc := func(name, help string) *prometheus.Desc {
		return prometheus.NewDesc(prometheus.BuildFQName(metricsNamespace, "db", name), help, labels, nil)
	}
	return &dbStatsCollector{
		provider:  provider,
		open:      desc("open_connections", "The number of established connections both in use and idle."),
		inUse:     desc("in_use_connections", "The number of connections currently in use."),
		idle:      desc("idle_connections", "The number of idle connections."),
		maxOpen:   desc("max_open_connections", "Maximum number of open connections allowed (0 = unlimited)."),
		waitCount: desc("wait_count_total", "The total number of connections waited for."),
		waitDur:   desc("wait_duration_seconds_total", "The total time blocked waiting for a new connection."),
	}
}

func (c *dbStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.open
	ch <- c.inUse
	ch <- c.idle
	ch <- c.maxOpen
	ch <- c.waitCount
	ch <- c.waitDur
}

func (c *dbStatsCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectPool(ch, "data", c.provider.DB())
	c.collectPool(ch, "aux", c.provider.AuxDB())
}

func (c *dbStatsCollector) collectPool(ch chan<- prometheus.Metric, name string, b dbx.Builder) {
	sqlDB := sqlDBFromBuilder(b)
	if sqlDB == nil {
		return
	}
	s := sqlDB.Stats()
	ch <- prometheus.MustNewConstMetric(c.open, prometheus.GaugeValue, float64(s.OpenConnections), name)
	ch <- prometheus.MustNewConstMetric(c.inUse, prometheus.GaugeValue, float64(s.InUse), name)
	ch <- prometheus.MustNewConstMetric(c.idle, prometheus.GaugeValue, float64(s.Idle), name)
	ch <- prometheus.MustNewConstMetric(c.maxOpen, prometheus.GaugeValue, float64(s.MaxOpenConnections), name)
	ch <- prometheus.MustNewConstMetric(c.waitCount, prometheus.CounterValue, float64(s.WaitCount), name)
	ch <- prometheus.MustNewConstMetric(c.waitDur, prometheus.CounterValue, s.WaitDuration.Seconds(), name)
}

// sqlDBFromBuilder unwraps the underlying *sql.DB from a dbx builder so its
// pool stats can be read; returns nil if the builder is not a *dbx.DB.
func sqlDBFromBuilder(b dbx.Builder) *sql.DB {
	if d, ok := b.(*dbx.DB); ok {
		return d.DB()
	}
	return nil
}

// realtimeCollector reports live realtime (SSE) subscription metrics at scrape
// time: the number of connected clients and the messages dropped due to full
// client buffers (slow consumers).
type realtimeCollector struct {
	provider realtimeStatsProvider

	clients *prometheus.Desc
	dropped *prometheus.Desc
}

func newRealtimeCollector(provider realtimeStatsProvider) *realtimeCollector {
	return &realtimeCollector{
		provider: provider,
		clients: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, "realtime", "connected_clients"),
			"The number of currently connected realtime (SSE) clients.", nil, nil,
		),
		dropped: prometheus.NewDesc(
			prometheus.BuildFQName(metricsNamespace, "realtime", "dropped_messages"),
			"Messages dropped due to full client buffers, summed over currently connected clients.", nil, nil,
		),
	}
}

func (c *realtimeCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.clients
	ch <- c.dropped
}

func (c *realtimeCollector) Collect(ch chan<- prometheus.Metric) {
	broker := c.provider.SubscriptionsBroker()

	ch <- prometheus.MustNewConstMetric(c.clients, prometheus.GaugeValue, float64(broker.TotalClients()))

	var dropped uint64
	for _, client := range broker.Clients() {
		// DroppedCount is intentionally not part of the Client interface, so
		// assert the concrete capability rather than the interface.
		if dc, ok := client.(interface{ DroppedCount() uint64 }); ok {
			dropped += dc.DroppedCount()
		}
	}
	ch <- prometheus.MustNewConstMetric(c.dropped, prometheus.GaugeValue, float64(dropped))
}
