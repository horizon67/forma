package codex

import (
	"bytes"
	"encoding/json"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

const eventLimit = 256 << 10

// eventStream drains an arbitrarily long stream using at most one bounded
// event buffer. Oversized lines are discarded until newline, then parsing
// resumes. No payload field is ever copied into a notification.
type eventStream struct {
	notify          agentrunner.Notify
	line            []byte
	discard, failed bool
	diagnosed       map[generationprogress.Code]bool
}

func newEventStream(notify agentrunner.Notify) *eventStream {
	return &eventStream{notify: notify, line: make([]byte, 0, eventLimit), diagnosed: make(map[generationprogress.Code]bool)}
}
func (s *eventStream) diagnostic(code generationprogress.Code) {
	if !s.diagnosed[code] {
		s.diagnosed[code] = true
		s.notify(agentrunner.Notification{Diagnostic: code})
	}
}
func (s *eventStream) Write(data []byte) (int, error) {
	n := len(data)
	for len(data) > 0 {
		end := bytes.IndexByte(data, '\n')
		part := data
		if end >= 0 {
			part = data[:end]
		}
		if !s.discard {
			if len(s.line)+len(part) > eventLimit {
				s.line = s.line[:0]
				s.discard = true
				s.diagnostic(generationprogress.OversizedEvent)
			} else {
				s.line = append(s.line, part...)
			}
		}
		if end < 0 {
			break
		}
		if !s.discard {
			s.parse()
		}
		s.line = s.line[:0]
		s.discard = false
		data = data[end+1:]
	}
	return n, nil
}
func (s *eventStream) finish() {
	if !s.discard && len(s.line) > 0 {
		s.parse()
	}
	s.line = nil
}
func (s *eventStream) parse() {
	var event struct {
		Type string `json:"type"`
		Item struct {
			Type string `json:"type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(s.line, &event); err != nil || event.Type == "" {
		s.diagnostic(generationprogress.InvalidEvent)
		return
	}
	var activity generationprogress.Activity
	switch event.Type {
	case "thread.started", "turn.started", "turn.completed":
		activity = generationprogress.Turn
	case "turn.failed":
		s.failed = true
		s.diagnostic(generationprogress.AgentError)
		return
	case "error":
		s.diagnostic(generationprogress.AgentError)
		return
	case "item.started", "item.updated", "item.completed":
		switch event.Item.Type {
		case "agent_message":
			activity = generationprogress.Message
		case "reasoning":
			activity = generationprogress.Reasoning
		case "command_execution":
			activity = generationprogress.Command
		case "file_change":
			activity = generationprogress.FileChange
		case "mcp_tool_call":
			activity = generationprogress.Tool
		case "web_search":
			activity = generationprogress.Search
		case "plan":
			activity = generationprogress.Plan
		}
	}
	if activity != "" {
		s.notify(agentrunner.Notification{Activity: activity})
	}
}
