package cron

import (
	"encoding/json"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestCronNew(t *testing.T) {
	t.Parallel()

	c := New()

	expectedInterval := 1 * time.Minute
	if c.interval != expectedInterval {
		t.Fatalf("Expected default interval %v, got %v", expectedInterval, c.interval)
	}

	expectedTimezone := time.UTC
	if c.timezone.String() != expectedTimezone.String() {
		t.Fatalf("Expected default timezone %v, got %v", expectedTimezone, c.timezone)
	}

	if len(c.jobs) != 0 {
		t.Fatalf("Expected no jobs by default, got \n%v", c.jobs)
	}

	if c.ticker != nil {
		t.Fatal("Expected the ticker NOT to be initialized")
	}
}

func TestCronSetInterval(t *testing.T) {
	t.Parallel()

	c := New()

	interval := 2 * time.Minute

	c.SetInterval(interval)

	if c.interval != interval {
		t.Fatalf("Expected interval %v, got %v", interval, c.interval)
	}
}

func TestCronSetTimezone(t *testing.T) {
	t.Parallel()

	c := New()

	timezone, _ := time.LoadLocation("Asia/Tokyo")

	c.SetTimezone(timezone)

	if c.timezone.String() != timezone.String() {
		t.Fatalf("Expected timezone %v, got %v", timezone, c.timezone)
	}
}

func TestCronAddAndRemove(t *testing.T) {
	t.Parallel()

	c := New()

	if err := c.Add("test0", "* * * * *", nil); err == nil {
		t.Fatal("Expected nil function error")
	}

	if err := c.Add("test1", "invalid", func() {}); err == nil {
		t.Fatal("Expected invalid cron expression error")
	}

	if err := c.Add("test2", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test3", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test4", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	// overwrite test2
	if err := c.Add("test2", "1 2 3 4 5", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test5", "1 2 3 4 5", func() {}); err != nil {
		t.Fatal(err)
	}

	// mock job deletion
	c.Remove("test4")

	// try to remove non-existing (should be no-op)
	c.Remove("missing")

	indexedJobs := make(map[string]*Job, len(c.jobs))
	for _, j := range c.jobs {
		indexedJobs[j.Id()] = j
	}

	// check job keys
	{
		expectedKeys := []string{"test3", "test2", "test5"}

		if v := len(c.jobs); v != len(expectedKeys) {
			t.Fatalf("Expected %d jobs, got %d", len(expectedKeys), v)
		}

		for _, k := range expectedKeys {
			if indexedJobs[k] == nil {
				t.Fatalf("Expected job with key %s, got nil", k)
			}
		}
	}

	// check the jobs schedule
	{
		expectedSchedules := map[string]string{
			"test2": `{"minutes":{"1":{}},"hours":{"2":{}},"days":{"3":{}},"months":{"4":{}},"daysOfWeek":{"5":{}}}`,
			"test3": `{"minutes":{"0":{},"1":{},"10":{},"11":{},"12":{},"13":{},"14":{},"15":{},"16":{},"17":{},"18":{},"19":{},"2":{},"20":{},"21":{},"22":{},"23":{},"24":{},"25":{},"26":{},"27":{},"28":{},"29":{},"3":{},"30":{},"31":{},"32":{},"33":{},"34":{},"35":{},"36":{},"37":{},"38":{},"39":{},"4":{},"40":{},"41":{},"42":{},"43":{},"44":{},"45":{},"46":{},"47":{},"48":{},"49":{},"5":{},"50":{},"51":{},"52":{},"53":{},"54":{},"55":{},"56":{},"57":{},"58":{},"59":{},"6":{},"7":{},"8":{},"9":{}},"hours":{"0":{},"1":{},"10":{},"11":{},"12":{},"13":{},"14":{},"15":{},"16":{},"17":{},"18":{},"19":{},"2":{},"20":{},"21":{},"22":{},"23":{},"3":{},"4":{},"5":{},"6":{},"7":{},"8":{},"9":{}},"days":{"1":{},"10":{},"11":{},"12":{},"13":{},"14":{},"15":{},"16":{},"17":{},"18":{},"19":{},"2":{},"20":{},"21":{},"22":{},"23":{},"24":{},"25":{},"26":{},"27":{},"28":{},"29":{},"3":{},"30":{},"31":{},"4":{},"5":{},"6":{},"7":{},"8":{},"9":{}},"months":{"1":{},"10":{},"11":{},"12":{},"2":{},"3":{},"4":{},"5":{},"6":{},"7":{},"8":{},"9":{}},"daysOfWeek":{"0":{},"1":{},"2":{},"3":{},"4":{},"5":{},"6":{}}}`,
			"test5": `{"minutes":{"1":{}},"hours":{"2":{}},"days":{"3":{}},"months":{"4":{}},"daysOfWeek":{"5":{}}}`,
		}
		for k, v := range expectedSchedules {
			raw, err := json.Marshal(indexedJobs[k].schedule)
			if err != nil {
				t.Fatal(err)
			}

			if string(raw) != v {
				t.Fatalf("Expected %q schedule \n%s, \ngot \n%s", k, v, raw)
			}
		}
	}
}

func TestCronMustAdd(t *testing.T) {
	t.Parallel()

	c := New()

	defer func() {
		if r := recover(); r == nil {
			t.Errorf("test1 didn't panic")
		}
	}()

	c.MustAdd("test1", "* * * * *", nil)

	c.MustAdd("test2", "* * * * *", func() {})

	if !slices.ContainsFunc(c.jobs, func(j *Job) bool { return j.Id() == "test2" }) {
		t.Fatal("Couldn't find job test2")
	}
}

func TestCronRemoveAll(t *testing.T) {
	t.Parallel()

	c := New()

	if err := c.Add("test1", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test2", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test3", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if v := len(c.jobs); v != 3 {
		t.Fatalf("Expected %d jobs, got %d", 3, v)
	}

	c.RemoveAll()

	if v := len(c.jobs); v != 0 {
		t.Fatalf("Expected %d jobs, got %d", 0, v)
	}
}

func TestCronTotal(t *testing.T) {
	t.Parallel()

	c := New()

	if v := c.Total(); v != 0 {
		t.Fatalf("Expected 0 jobs, got %v", v)
	}

	if err := c.Add("test1", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("test2", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	// overwrite
	if err := c.Add("test1", "* * * * *", func() {}); err != nil {
		t.Fatal(err)
	}

	if v := c.Total(); v != 2 {
		t.Fatalf("Expected 2 jobs, got %v", v)
	}
}

func TestCronJobs(t *testing.T) {
	t.Parallel()

	c := New()

	calls := ""

	if err := c.Add("a", "1 * * * *", func() { calls += "a" }); err != nil {
		t.Fatal(err)
	}

	if err := c.Add("b", "2 * * * *", func() { calls += "b" }); err != nil {
		t.Fatal(err)
	}

	// overwrite
	if err := c.Add("b", "3 * * * *", func() { calls += "b" }); err != nil {
		t.Fatal(err)
	}

	jobs := c.Jobs()

	if len(jobs) != 2 {
		t.Fatalf("Expected 2 jobs, got %v", len(jobs))
	}

	for _, j := range jobs {
		j.Run()
	}

	expectedCalls := "ab"
	if calls != expectedCalls {
		t.Fatalf("Expected %q calls, got %q", expectedCalls, calls)
	}
}

func TestCronStartStop(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	test1 := 0
	test2 := 0

	c := New()

	c.SetInterval(250 * time.Millisecond)

	c.Add("test1", "* * * * *", func() {
		mu.Lock()
		defer mu.Unlock()
		test1++
	})

	c.Add("test2", "* * * * *", func() {
		mu.Lock()
		defer mu.Unlock()
		test2++
	})

	readCounts := func() (int, int) {
		mu.Lock()
		defer mu.Unlock()
		return test1, test2
	}

	// call twice Start to check if the previous ticker will be reseted
	c.Start()
	c.Start()

	time.Sleep(505 * time.Millisecond)

	// call twice Stop to ensure that the second stop is no-op
	c.Stop()
	c.Stop()

	// let any in-flight job goroutine settle before reading the counters
	time.Sleep(50 * time.Millisecond)

	// The exact number of ticks in the window depends on the wall-clock
	// alignment performed in Start() (the initial delay is in the
	// (0, interval] range), so tolerate a small range instead of asserting
	// an exact count to avoid timing flakiness. A ~2x count (or more) would
	// still be caught here and would indicate that the double Start() failed
	// to reset the previous ticker.
	run1a, run1b := readCounts()
	if run1a < 1 || run1a > 3 {
		t.Fatalf("Expected 1-3 test1 calls after the first run, got %d", run1a)
	}
	if run1a != run1b {
		t.Fatalf("Expected test1 and test2 to fire together, got %d vs %d", run1a, run1b)
	}

	// ensure Stop() actually halted the ticker (no additional calls)
	time.Sleep(300 * time.Millisecond)
	stopped1, stopped2 := readCounts()
	if stopped1 != run1a || stopped2 != run1b {
		t.Fatalf("Expected no additional calls after Stop, got %d/%d (was %d/%d)", stopped1, stopped2, run1a, run1b)
	}

	// resume for ~1 second
	c.Start()

	time.Sleep(1005 * time.Millisecond)

	c.Stop()

	// let any in-flight job goroutine settle before reading the counters
	time.Sleep(50 * time.Millisecond)

	run2a, run2b := readCounts()
	if run2a != run2b {
		t.Fatalf("Expected test1 and test2 to fire together after resume, got %d vs %d", run2a, run2b)
	}

	// ~4 more ticks are expected in a 1s window with a 250ms interval
	// (3-5 depending on the Start() alignment described above)
	delta := run2a - stopped1
	if delta < 3 || delta > 5 {
		t.Fatalf("Expected 3-5 additional calls after resume, got %d", delta)
	}
}
