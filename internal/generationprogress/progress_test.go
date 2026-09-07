package generationprogress

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	interval time.Duration
	ticker   *fakeTicker
}
type fakeTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (t *fakeTicker) C() <-chan time.Time { return t.ticks }
func (t *fakeTicker) Stop()               { t.once.Do(func() { close(t.stopped) }) }
func (c *fakeClock) Now() time.Time       { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *fakeClock) NewTicker(d time.Duration) Ticker {
	c.interval = d
	c.ticker = &fakeTicker{ticks: make(chan time.Time, 1), stopped: make(chan struct{})}
	return c.ticker
}
func (c *fakeClock) advance(d time.Duration) { c.mu.Lock(); c.now = c.now.Add(d); c.mu.Unlock() }

type eventWriter chan Event

func (w eventWriter) Write(b []byte) (int, error) {
	var e Event
	if err := json.Unmarshal(b, &e); err != nil {
		return 0, err
	}
	w <- e
	return len(b), nil
}
func nextEvent(t *testing.T, w eventWriter, kind string) Event {
	t.Helper()
	select {
	case e := <-w:
		if e.Type != kind {
			t.Fatalf("event %s, want %s: %#v", e.Type, kind, e)
		}
		return e
	case <-time.After(2 * time.Second):
		t.Fatal("missing event", kind)
		return Event{}
	}
}

func TestFakeClockHeartbeatDeadlineActivityAndStop(t *testing.T) {
	c := &fakeClock{now: time.Now()}
	w := make(eventWriter, 100)
	p := New(w, Options{JSON: true, Verbose: true, Clock: c})
	defer p.Finish(Failed)
	if c.interval != 30*time.Second {
		t.Fatal(c.interval)
	}
	p.Deadline(c.Now().Add(90 * time.Second))
	p.Phase(AgentStarting)
	nextEvent(t, w, "phase")
	c.advance(2 * time.Second)
	p.AgentStarted()
	done := nextEvent(t, w, "phase")
	if done.DurationMS == nil || *done.DurationMS != 2000 {
		t.Fatal("wrong phase duration", done)
	}
	running := nextEvent(t, w, "phase")
	if running.Phase != AgentRunning || running.ActivityAgeMS != nil {
		t.Fatal("start invented activity", running)
	}
	c.advance(28 * time.Second)
	c.ticker.ticks <- c.Now()
	hb := nextEvent(t, w, "heartbeat")
	if hb.ElapsedMS != 30000 || hb.RemainingMS == nil || *hb.RemainingMS != 60000 || hb.ActivityAgeMS != nil {
		t.Fatal(hb)
	}
	p.Activity(Command)
	a := nextEvent(t, w, "activity")
	if a.ActivityAgeMS == nil || *a.ActivityAgeMS != 0 {
		t.Fatal(a)
	}
	c.advance(30 * time.Second)
	c.ticker.ticks <- c.Now()
	hb = nextEvent(t, w, "heartbeat")
	if *hb.ActivityAgeMS != 30000 || *hb.RemainingMS != 30000 {
		t.Fatal("heartbeat reset activity", hb)
	}
	c.advance(40 * time.Second)
	p.Heartbeat()
	hb = nextEvent(t, w, "heartbeat")
	if *hb.ActivityAgeMS != 70000 || *hb.RemainingMS != 0 {
		t.Fatal(hb)
	}
	if !p.Finish(Completed) {
		t.Fatal("finish did not drain")
	}
	nextEvent(t, w, "phase")
	r := nextEvent(t, w, "result")
	if r.Status != Completed {
		t.Fatal(r)
	}
	select {
	case <-c.ticker.stopped:
	default:
		t.Fatal("ticker retained")
	}
	p.Activity(Command)
	p.Phase(Compile)
	p.Heartbeat()
	p.Diagnostic(GenerationError)
	if len(w) != 0 {
		t.Fatal("post-result events")
	}
}

func TestPhasesAreOrderedAndVerboseIsOptional(t *testing.T) {
	for _, verbose := range []bool{false, true} {
		var out bytes.Buffer
		c := &fakeClock{now: time.Now()}
		p := New(&out, Options{JSON: true, Verbose: verbose, Clock: c, Interval: time.Second})
		phases := []Phase{Compile, RepositoryValidation, HistorySelection, AgentPreparation, AgentStarting}
		for _, phase := range phases {
			p.Phase(phase)
			c.advance(time.Second)
		}
		// Calling Run is not proof of start; only the backend notification is.
		p.AgentStarted()
		p.Activity(FileChange)
		for _, phase := range []Phase{Cleanup, FinalStatus, HistorySave} {
			p.Phase(phase)
			c.advance(time.Second)
		}
		if !p.Finish(Completed) {
			t.Fatal("in-memory writer did not drain")
		}
		var got []Phase
		activities, durations := 0, 0
		for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
			var e Event
			if err := json.Unmarshal([]byte(line), &e); err != nil {
				t.Fatal(err)
			}
			if e.Type == "phase" {
				if e.DurationMS == nil {
					got = append(got, e.Phase)
				} else {
					durations++
				}
			}
			if e.Type == "activity" {
				activities++
			}
		}
		want := append(phases, AgentRunning, Cleanup, FinalStatus, HistorySave)
		if len(got) != len(want) {
			t.Fatalf("phases %v", got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("phases %v", got)
			}
		}
		if verbose && (activities != 1 || durations != 9) {
			t.Fatalf("verbose details %d %d", activities, durations)
		}
		if !verbose && (activities != 0 || durations != 0) {
			t.Fatal("details without verbose")
		}
	}
}

type stalledWriter struct {
	started, release chan struct{}
	once             sync.Once
}

func (w *stalledWriter) Write(b []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(b), nil
}
func TestSlowDisplayCannotBackpressureOrCrowdOutResult(t *testing.T) {
	w := &stalledWriter{started: make(chan struct{}), release: make(chan struct{})}
	p := New(w, Options{Verbose: true})
	p.Phase(AgentStarting)
	<-w.started
	p.AgentStarted()
	for i := 0; i < 10000; i++ {
		p.Activity(Command)
		p.Activity(FileChange)
		p.Heartbeat()
		p.Diagnostic(InvalidEvent)
	}
	p.mu.Lock()
	n := len(p.queue)
	p.mu.Unlock()
	if n > QueueLimit {
		t.Fatal("unbounded queue", n)
	}
	if p.Finish(Completed) {
		t.Fatal("blocked output unexpectedly drained")
	}
	p.mu.Lock()
	terminal := p.terminal
	p.mu.Unlock()
	if terminal == nil || terminal.Status != Completed {
		t.Fatal("terminal event was dropped")
	}
	close(w.release)
	select {
	case <-p.done:
	case <-time.After(2 * time.Second):
		t.Fatal("writer not released")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("disconnected") }
func TestDisconnectedOutputIsNotAnExecutionFailure(t *testing.T) {
	p := New(brokenWriter{}, Options{})
	p.Phase(Compile)
	if !p.Finish(Completed) {
		t.Fatal("broken writer did not stop")
	}
	if p.Err() == nil {
		t.Fatal("lost display failure")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.terminal == nil || p.terminal.Status != Completed {
		t.Fatal("display changed terminal state")
	}
}
func TestUnknownValuesAndSensitiveTextCannotReachProgress(t *testing.T) {
	var out bytes.Buffer
	p := New(&out, Options{JSON: true, Verbose: true})
	p.Phase(AgentStarting)
	p.AgentStarted()
	p.Phase("SECRET")
	p.Activity("SECRET")
	p.Diagnostic("SECRET")
	if !p.Finish("SECRET") {
		t.Fatal("in-memory writer did not drain")
	}
	if strings.Contains(out.String(), "SECRET") {
		t.Fatal("untrusted value emitted")
	}
}
func TestTextIsNewlineOnlyRegardlessOfTerminal(t *testing.T) {
	// v1 intentionally has no terminal detection or alternate renderer.
	var out bytes.Buffer
	p := New(&out, Options{})
	p.Phase(Compile)
	p.Diagnostic(PartialMutation)
	if !p.Finish(Cancelled) {
		t.Fatal("in-memory writer did not drain")
	}
	if strings.ContainsAny(out.String(), "\r\x1b") || !strings.HasSuffix(out.String(), "\n") || !strings.Contains(out.String(), "interruption is not rollback") {
		t.Fatal(out.String())
	}
}

func TestTextUsesHumanPhaseLabelsWithoutChangingJSONEnums(t *testing.T) {
	for phase, label := range map[Phase]string{Compile: "compiling Forma source", RepositoryValidation: "validating repository", HistorySelection: "selecting generation baseline", AgentPreparation: "preparing agent (authentication and immutable input)", AgentStarting: "starting agent", AgentRunning: "agent running", Cleanup: "cleaning up agent resources", FinalStatus: "capturing final repository status", HistorySave: "saving generation history"} {
		event := Event{Schema: Schema, Type: "phase", Phase: phase}
		if got := text(event); !strings.Contains(got, label) || strings.ContainsAny(got, "\r\x1b") {
			t.Fatalf("phase %s: %q", phase, got)
		}
		encoded, err := json.Marshal(event)
		if err != nil || !strings.Contains(string(encoded), `"phase":"`+string(phase)+`"`) {
			t.Fatal("JSON enum changed")
		}
	}
}
