package apis_test

import (
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/arief-fajri/pgbase/apis"
	"github.com/arief-fajri/pgbase/core"
	"github.com/pocketbase/dbx"
)

// syncBuffer is a mutex-guarded bytes.Buffer usable as a log output target:
// background goroutines may write to it while the test reads its content.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitForInstaller waits until the installer FireAndForget goroutine reaches
// a terminal state (record-create error after losing the terminate race, or
// the successful installer func call), failing the test immediately if any
// background goroutine logged a recovered panic (W-13).
func waitForInstaller(t *testing.T, done <-chan struct{}, stdLog *syncBuffer, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if out := stdLog.String(); strings.Contains(out, "RECOVERED FROM PANIC") {
			t.Fatalf("a background goroutine panicked during serve/terminate (W-13):\n%s", out)
		}

		select {
		case <-done:
			return
		default:
		}

		if time.Now().After(deadline) {
			t.Fatalf("the installer goroutine did not finish within %v\n--- std log ---\n%s",
				timeout, stdLog.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// backendCount returns the number of live PostgreSQL backends attached to the
// given database (excluding this maintenance connection itself).
func backendCount(t *testing.T, maint *dbx.DB, dbName string) int64 {
	t.Helper()

	var count int64
	err := maint.NewQuery(
		"SELECT count(*) FROM pg_stat_activity WHERE datname = '" + dbName + "' AND pid <> pg_backend_pid()",
	).Row(&count)
	if err != nil {
		t.Fatalf("backend count query: %v", err)
	}
	return count
}

// newServeMaintenanceDB opens a maintenance connection to the shared postgres
// database and registers creation/cleanup for a dedicated database.
func newServeMaintenanceDB(t *testing.T) (maint *dbx.DB, dbName string) {
	t.Helper()

	maint, err := core.DefaultDBConnect(core.ResolveDBConfig(core.DBConfig{
		SSLMode:      "disable",
		MaxOpenConns: 2,
		MaxIdleConns: 1,
	}))
	if err != nil {
		t.Fatalf("maintenance connect: %v", err)
	}
	t.Cleanup(func() { maint.Close() })

	dbName = fmt.Sprintf("pb_serve_%d_%d", os.Getpid(), time.Now().UnixNano())

	if _, err := maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute(); err != nil {
		t.Fatalf("failed to drop leftover db: %v", err)
	}
	if _, err := maint.NewQuery("CREATE DATABASE " + dbName).Execute(); err != nil {
		t.Fatalf("failed to create database %s: %v", dbName, err)
	}
	t.Cleanup(func() {
		maint.NewQuery("DROP DATABASE IF EXISTS " + dbName + " WITH (FORCE)").Execute()
	})

	return maint, dbName
}

// waitHTTPReady polls the readiness endpoint until it returns 200, failing the
// test if the server never comes up or (when procCh is non-nil) exits first.
func waitHTTPReady(t *testing.T, url string, procCh <-chan error, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case procErr := <-procCh:
			t.Fatalf("server process exited before becoming ready: %v", procErr)
			return
		default:
		}

		res, err := client.Get(url)
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server did not become ready within %v", timeout)
}

// TestServeOnTerminateReleasesDBResources verifies §1 "Shutdown releases
// database resources" (Phase 0 reliability tests): triggering the terminate
// chain (the same chain a SIGTERM triggers via pgbase.Execute) drains the HTTP
// server, returns from apis.Serve, closes the listener, and closes the
// database pools — no backend connection survives.
//
// It also doubles as the W-13 regression: the first-run installer runs as a
// FireAndForget goroutine and is held inside the record-create hook until
// the test releases it after terminate has reset the pools — the create
// chain then deterministically resumes against the reset handles and must
// degrade with an explicit error instead of nil-dereferencing its db
// builder (recovered panic).
func TestServeOnTerminateReleasesDBResources(t *testing.T) {
	maint, dbName := newServeMaintenanceDB(t)

	app := core.NewBaseApp(core.BaseAppConfig{
		DataDir:       t.TempDir(),
		EncryptionEnv: "pb_serve_test_env",
		DBConnect: func(c core.DBConfig) (*dbx.DB, error) {
			c.DBName = dbName
			return core.DefaultDBConnect(c)
		},
	})
	if err := app.Bootstrap(); err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	defer func() { _ = app.ResetBootstrapState() }()

	// hold the installer's superuser create inside the record-create hook and
	// release it only AFTER terminate has reset the db handles: the create
	// then always resumes against the reset pools — the original W-13
	// interleaving with no timing window. A sleep-based hold raced CI load:
	// a late-scheduled installer goroutine hit the swallowed CountRecords
	// error in needInstallerSuperuser (silent nil return — no warn, no
	// signal) and the wait below timed out (W-15).
	var createEnteredOnce, createReleaseOnce sync.Once
	createHookEntered := make(chan struct{})
	createHookRelease := make(chan struct{})
	t.Cleanup(func() { createReleaseOnce.Do(func() { close(createHookRelease) }) })

	app.OnRecordCreate().BindFunc(func(e *core.RecordEvent) error {
		createEnteredOnce.Do(func() { close(createHookEntered) })
		<-createHookRelease
		return e.Next()
	})

	// both installer terminal states close the done channel: the record-create
	// error path (lost the terminate race) and the successful installer func
	// call (won the race)
	var installerOnce sync.Once
	installerDone := make(chan struct{})
	installerFinished := func() { installerOnce.Do(func() { close(installerDone) }) }
	app.OnRecordAfterCreateError().BindFunc(func(e *core.RecordErrorEvent) error {
		installerFinished()
		return e.Next()
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	app.OnServe().BindFunc(func(e *core.ServeEvent) error {
		e.Listener = ln
		// let the installer run (its create chain is the W-13 race under
		// test), but replace the default installer func with a no-op since
		// the real one launches a browser, which must not happen from tests
		e.InstallerFunc = func(_ core.App, _ *core.Record, _ string) error {
			installerFinished()
			return nil
		}
		return e.Next()
	})
	addr := ln.Addr().String()

	// FireAndForget recovers panics via the std logger - capture it for the
	// whole serve/terminate window to assert that nothing was recovered
	stdLog := new(syncBuffer)
	origLogWriter := log.Writer()
	log.SetOutput(stdLog)
	t.Cleanup(func() { log.SetOutput(origLogWriter) })

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- apis.Serve(app, apis.ServeConfig{
			HttpAddr:        addr,
			ShowStartBanner: false,
		})
	}()

	baseURL := "http://" + addr
	waitHTTPReady(t, baseURL+"/api/ready", serveDone, 20*time.Second)

	// while serving, the pools must hold at least one live backend so the
	// drop-to-zero below is meaningful
	deadline := time.Now().Add(5 * time.Second)
	for backendCount(t, maint, dbName) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("expected at least one backend while serving")
		}
		time.Sleep(100 * time.Millisecond)
	}

	// the installer must reach the create hook while the pools are still
	// alive: loadInstaller exits silently (no warn, no signal) when its
	// CountRecords runs after the reset, so gate terminate on hook entry
	select {
	case <-createHookEntered:
	case <-time.After(10 * time.Second):
		t.Fatal("the installer goroutine never entered the record-create hook (did loadInstaller bail before Save?)")
	}

	// mirror pgbase.Execute: OnTerminate + a one-off ResetBootstrapState
	event := new(core.TerminateEvent)
	event.App = app
	if err := app.OnTerminate().Trigger(event, func(e *core.TerminateEvent) error {
		return e.App.ResetBootstrapState()
	}); err != nil {
		t.Fatalf("terminate chain failed: %v", err)
	}

	select {
	case serveErr := <-serveDone:
		if serveErr != nil {
			t.Fatalf("expected a clean Serve return, got: %v", serveErr)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("apis.Serve did not return after terminate")
	}

	// the listener must be closed with the server
	if res, err := http.Get(baseURL + "/api/ready"); err == nil {
		_ = res.Body.Close()
		t.Fatal("expected the listener to be closed after shutdown")
	}

	// and every database backend must be gone (poll briefly: pool teardown is
	// asynchronous with the TCP FINs)
	deadline = time.Now().Add(5 * time.Second)
	for backendCount(t, maint, dbName) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("expected 0 backends after shutdown, got %d", backendCount(t, maint, dbName))
		}
		time.Sleep(100 * time.Millisecond)
	}

	// pools are reset — release the held create: the installer resumes
	// against the reset pools (the W-13 interleaving) and must degrade with
	// an explicit error instead of a recovered panic
	createReleaseOnce.Do(func() { close(createHookRelease) })

	// the installer goroutine was held inside the record-create hook and
	// resumes against the reset pools - wait for it to degrade cleanly
	// (explicit error, no recovered panic)
	waitForInstaller(t, installerDone, stdLog, 15*time.Second)
}

// TestServeSigtermExitsCleanly verifies the real signal path end-to-end: the
// built binary receives SIGTERM, drains gracefully, exits with code 0, and
// releases all of its database backends.
func TestServeSigtermExitsCleanly(t *testing.T) {
	// build the runnable entrypoint (go build is toolchain-cached)
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve the test file path")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile))
	binPath := filepath.Join(t.TempDir(), "pgbase")
	build := exec.Command("go", "build", "-o", binPath, "./examples/base")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	maint, dbName := newServeMaintenanceDB(t)

	// dedicated free port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	cmd := exec.Command(binPath, "serve", "--http", addr, "--dir", t.TempDir())
	cmd.Env = serveSubprocessEnv(map[string]string{
		"PB_POSTGRES_DBNAME":  dbName,
		"PB_POSTGRES_SSLMODE": "disable",
	})

	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start the serve process: %v", err)
	}
	processDone := make(chan error, 1)
	go func() { processDone <- cmd.Wait() }()
	t.Cleanup(func() {
		// best-effort: never leave the child running (registered before the
		// drop-database cleanup so it runs first, LIFO)
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			<-processDone
		}
	})

	waitHTTPReady(t, "http://"+addr+"/api/ready", processDone, 90*time.Second)

	deadline := time.Now().Add(10 * time.Second)
	for backendCount(t, maint, dbName) < 1 {
		if time.Now().After(deadline) {
			t.Fatal("expected at least one backend while serving")
		}
		time.Sleep(100 * time.Millisecond)
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("failed to send SIGTERM: %v", err)
	}

	select {
	case waitErr := <-processDone:
		if waitErr != nil {
			t.Fatalf("expected a graceful exit code 0 after SIGTERM, got: %v\n--- process output ---\n%s", waitErr, output.String())
		}
	case <-time.After(15 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("process did not exit within 15s after SIGTERM\n--- process output ---\n%s", output.String())
	}

	deadline = time.Now().Add(5 * time.Second)
	for backendCount(t, maint, dbName) > 0 {
		if time.Now().After(deadline) {
			t.Fatalf("expected 0 backends after SIGTERM exit, got %d\n--- process output ---\n%s",
				backendCount(t, maint, dbName), output.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// serveSubprocessEnv builds the child environment from the current one with
// the given keys replaced (duplicate keys are not appended: the child's
// getenv resolves the first occurrence).
func serveSubprocessEnv(overrides map[string]string) []string {
	env := os.Environ()
	for key := range overrides {
		prefix := key + "="
		filtered := env[:0]
		for _, kv := range env {
			if !strings.HasPrefix(kv, prefix) {
				filtered = append(filtered, kv)
			}
		}
		env = filtered
	}
	for key, value := range overrides {
		env = append(env, key+"="+value)
	}
	return env
}
