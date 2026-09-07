// Package generationprogress reports safe, provider-independent execution
// observations. It never accepts AI text, paths, commands, or raw errors.
package generationprogress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const Schema = "forma/generation-progress/v0alpha1"
const DefaultInterval = 30 * time.Second
const QueueLimit = 64

type Phase string

const (
	Compile              Phase = "compile"
	RepositoryValidation Phase = "repository_validation"
	HistorySelection     Phase = "history_selection"
	AgentPreparation     Phase = "agent_preparation"
	AgentStarting        Phase = "agent_starting"
	AgentRunning         Phase = "agent_running"
	Cleanup              Phase = "cleanup"
	FinalStatus          Phase = "final_status"
	HistorySave          Phase = "history_save"
)

type Activity string

const (
	Turn       Activity = "turn"
	Message    Activity = "message"
	Reasoning  Activity = "reasoning"
	Command    Activity = "command"
	FileChange Activity = "file_change"
	Tool       Activity = "tool"
	Search     Activity = "search"
	Plan       Activity = "plan"
)

type Code string

const (
	InvalidEvent    Code = "invalid_agent_event"
	OversizedEvent  Code = "oversized_agent_event"
	AgentError      Code = "agent_reported_error"
	GenerationError Code = "generation_failed"
	CompilerError   Code = "compilation_failed"
	InvalidOptions  Code = "invalid_options"
	PartialMutation Code = "review_partial_changes"
)

type Status string

const (
	Completed Status = "completed"
	NoOp      Status = "no_op"
	Failed    Status = "failed"
	Cancelled Status = "cancelled"
	TimedOut  Status = "timed_out"
)

func Outcome(err error) Status {
	if errors.Is(err, context.DeadlineExceeded) {
		return TimedOut
	}
	if errors.Is(err, context.Canceled) {
		return Cancelled
	}
	if err != nil {
		return Failed
	}
	return Completed
}

// Event is deliberately closed: there is no arbitrary message or payload.
type Event struct {
	Schema        string   `json:"schema"`
	Type          string   `json:"type"`
	ElapsedMS     int64    `json:"elapsedMs"`
	RemainingMS   *int64   `json:"remainingMs,omitempty"`
	ActivityAgeMS *int64   `json:"activityAgeMs,omitempty"`
	Phase         Phase    `json:"phase,omitempty"`
	DurationMS    *int64   `json:"durationMs,omitempty"`
	Activity      Activity `json:"activity,omitempty"`
	Code          Code     `json:"code,omitempty"`
	Status        Status   `json:"status,omitempty"`
}

type Ticker interface {
	C() <-chan time.Time
	Stop()
}
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
}
type realClock struct{}
type realTicker struct{ *time.Ticker }

func (realClock) Now() time.Time                   { return time.Now() }
func (realClock) NewTicker(d time.Duration) Ticker { return realTicker{time.NewTicker(d)} }
func (t realTicker) C() <-chan time.Time           { return t.Ticker.C }

type Options struct {
	JSON, Verbose bool
	Interval      time.Duration
	Clock         Clock
}

// Reporter owns a bounded queue and at most one writer goroutine. A stalled
// writer can lose observations, but cannot backpressure execution or cleanup.
// Finish waits only briefly; it never requires a blocked Write to return.
type Reporter struct {
	mu                                        sync.Mutex
	clock                                     Clock
	start, phaseStart, deadline, lastActivity time.Time
	phase                                     Phase
	options                                   Options
	queue                                     []Event
	terminal                                  *Event
	finished                                  bool
	wake                                      chan struct{}
	stop                                      chan struct{}
	done                                      chan struct{}
	ticker                                    Ticker
	writeErr                                  error
}

func New(writer io.Writer, options Options) *Reporter {
	if options.Clock == nil {
		options.Clock = realClock{}
	}
	if options.Interval < time.Second || options.Interval > 5*time.Minute {
		options.Interval = DefaultInterval
	}
	p := &Reporter{clock: options.Clock, options: options, wake: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{}), queue: make([]Event, 0, QueueLimit)}
	p.start = p.clock.Now()
	p.ticker = p.clock.NewTicker(options.Interval)
	go p.render(writer)
	go func() {
		for {
			select {
			case <-p.stop:
				return
			case <-p.ticker.C():
				p.Heartbeat()
			}
		}
	}()
	return p
}

func (p *Reporter) Deadline(deadline time.Time) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.deadline = deadline
}
func milliseconds(d time.Duration) int64 {
	if d < 0 {
		return 0
	}
	return d.Milliseconds()
}
func (p *Reporter) event(kind string) Event {
	now := p.clock.Now()
	e := Event{Schema: Schema, Type: kind, Phase: p.phase, ElapsedMS: milliseconds(now.Sub(p.start))}
	if !p.deadline.IsZero() {
		n := milliseconds(p.deadline.Sub(now))
		e.RemainingMS = &n
	}
	if !p.lastActivity.IsZero() {
		n := milliseconds(now.Sub(p.lastActivity))
		e.ActivityAgeMS = &n
	}
	return e
}
func (p *Reporter) enqueue(e Event) {
	if p.finished || p.writeErr != nil {
		return
	}
	// Coalesce repetitive observations; retain lifecycle events in preference
	// to activity/heartbeats. A dedicated terminal slot cannot be crowded out.
	if e.Type == "heartbeat" || e.Type == "activity" || e.Type == "diagnostic" {
		for i := len(p.queue) - 1; i >= 0; i-- {
			if p.queue[i].Type == e.Type && p.queue[i].Activity == e.Activity && p.queue[i].Code == e.Code {
				p.queue = append(p.queue[:i], p.queue[i+1:]...)
				p.queue = append(p.queue, e)
				p.signal()
				return
			}
		}
	}
	if len(p.queue) == QueueLimit {
		index := -1
		for i, old := range p.queue {
			if old.Type == "activity" || old.Type == "heartbeat" {
				index = i
				break
			}
		}
		if index < 0 {
			if e.Type != "phase" {
				return
			}
			index = 0
		}
		p.queue = append(p.queue[:index], p.queue[index+1:]...)
	}
	p.queue = append(p.queue, e)
	p.signal()
}
func (p *Reporter) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}
func validPhase(phase Phase) bool {
	switch phase {
	case Compile, RepositoryValidation, HistorySelection, AgentPreparation, AgentStarting, AgentRunning, Cleanup, FinalStatus, HistorySave:
		return true
	}
	return false
}
func (p *Reporter) phaseLocked(phase Phase) {
	if p.finished || p.phase == phase {
		return
	}
	if p.options.Verbose && p.phase != "" {
		e := p.event("phase")
		n := milliseconds(p.clock.Now().Sub(p.phaseStart))
		e.DurationMS = &n
		p.enqueue(e)
	}
	p.phase, p.phaseStart = phase, p.clock.Now()
	p.enqueue(p.event("phase"))
}
func (p *Reporter) Phase(phase Phase) {
	if p == nil || !validPhase(phase) {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.phaseLocked(phase)
}
func (p *Reporter) AgentStarted() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.phase == AgentStarting {
		p.phaseLocked(AgentRunning)
	}
}
func (p *Reporter) Activity(activity Activity) {
	if p == nil {
		return
	}
	switch activity {
	case Turn, Message, Reasoning, Command, FileChange, Tool, Search, Plan:
	default:
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished || (p.phase != AgentRunning && p.phase != AgentStarting) {
		return
	}
	p.lastActivity = p.clock.Now()
	if p.options.Verbose {
		e := p.event("activity")
		e.Activity = activity
		p.enqueue(e)
	}
}
func (p *Reporter) Diagnostic(code Code) {
	if p == nil {
		return
	}
	switch code {
	case InvalidEvent, OversizedEvent, AgentError, GenerationError, CompilerError, InvalidOptions, PartialMutation:
	default:
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.event("diagnostic")
	e.Code = code
	p.enqueue(e)
}
func (p *Reporter) Heartbeat() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enqueue(p.event("heartbeat"))
}

// Finish must be called after private cleanup, history save, and lock release.
// A display error does not change execution success and never causes a retry.
func (p *Reporter) Finish(status Status) bool {
	if p == nil {
		return true
	}
	switch status {
	case Completed, NoOp, Failed, Cancelled, TimedOut:
	default:
		status = Failed
	}
	p.mu.Lock()
	if !p.finished {
		if p.options.Verbose && p.phase != "" {
			e := p.event("phase")
			duration := milliseconds(p.clock.Now().Sub(p.phaseStart))
			e.DurationMS = &duration
			p.enqueue(e)
		}
		e := p.event("result")
		e.Status = status
		p.terminal = &e
		p.finished = true
		p.ticker.Stop()
		close(p.stop)
		p.signal()
	}
	p.mu.Unlock()
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-p.done:
		return true
	case <-timer.C:
		return false
	}
}
func (p *Reporter) Err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.writeErr
}
func (p *Reporter) render(writer io.Writer) {
	defer close(p.done)
	for {
		<-p.wake
		for {
			p.mu.Lock()
			var e Event
			if len(p.queue) > 0 {
				e = p.queue[0]
				p.queue = p.queue[1:]
			} else if p.terminal != nil {
				e = *p.terminal
				p.terminal = nil
			} else {
				finished := p.finished
				p.mu.Unlock()
				if finished {
					return
				}
				break
			}
			p.mu.Unlock()
			var err error
			if p.options.JSON {
				err = json.NewEncoder(writer).Encode(e)
			} else {
				_, err = io.WriteString(writer, text(e))
			}
			if err != nil {
				p.mu.Lock()
				p.writeErr = err
				p.queue = nil
				p.mu.Unlock()
				return
			}
		}
	}
}
func phaseLabel(phase Phase) string {
	switch phase {
	case Compile:
		return "compiling Forma source"
	case RepositoryValidation:
		return "validating repository"
	case HistorySelection:
		return "selecting generation baseline"
	case AgentPreparation:
		return "preparing agent (authentication and immutable input)"
	case AgentStarting:
		return "starting agent"
	case AgentRunning:
		return "agent running"
	case Cleanup:
		return "cleaning up agent resources"
	case FinalStatus:
		return "capturing final repository status"
	case HistorySave:
		return "saving generation history"
	default:
		return ""
	}
}
func text(e Event) string {
	detail := phaseLabel(e.Phase)
	switch e.Type {
	case "activity":
		detail = "agent activity: " + string(e.Activity)
	case "diagnostic":
		detail = string(e.Code)
		if e.Code == PartialMutation {
			detail = "interruption is not rollback; review partial repository changes before retrying"
		}
	case "result":
		detail = string(e.Status)
	case "heartbeat":
		detail += " (waiting; heartbeat is not evidence of agent activity)"
	}
	line := fmt.Sprintf("forma: [%s] %s", (time.Duration(e.ElapsedMS) * time.Millisecond).Truncate(time.Second), detail)
	if e.DurationMS != nil {
		line += fmt.Sprintf(" (phase duration %s)", time.Duration(*e.DurationMS)*time.Millisecond)
	}
	if e.RemainingMS != nil {
		line += fmt.Sprintf("; remaining %s", (time.Duration(*e.RemainingMS) * time.Millisecond).Truncate(time.Second))
	}
	if e.ActivityAgeMS != nil {
		line += fmt.Sprintf("; last agent activity %s ago", (time.Duration(*e.ActivityAgeMS) * time.Millisecond).Truncate(time.Second))
	}
	return line + "\n"
}
