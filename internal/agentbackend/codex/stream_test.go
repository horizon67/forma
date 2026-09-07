package codex

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/horizon67/forma/internal/agentrunner"
	"github.com/horizon67/forma/internal/generationprogress"
)

func TestStreamDrainsMalformedOversizedUnknownAndUnlimitedTotalOutput(t *testing.T) {
	var notifications []agentrunner.Notification
	s := newEventStream(func(n agentrunner.Notification) { notifications = append(notifications, n) })
	inputs := []string{"SECRET invalid\n", `{"type":"future","text":"SECRET"}` + "\n", strings.Repeat("SECRET", eventLimit/6+1) + "\n", `{"type":"item.completed","item":{"type":"command_execution","command":"SECRET","aggregated_output":"SECRET"}}` + "\n"}
	for _, input := range inputs {
		for len(input) > 0 {
			size := 17
			if len(input) < size {
				size = len(input)
			}
			n, err := s.Write([]byte(input[:size]))
			if n != size || err != nil {
				t.Fatal("stream stopped draining")
			}
			input = input[size:]
		}
	}
	if len(notifications) != 3 || notifications[0].Diagnostic != generationprogress.InvalidEvent || notifications[1].Diagnostic != generationprogress.OversizedEvent || notifications[2].Activity != generationprogress.Command {
		t.Fatalf("notifications %#v", notifications)
	}
	// Total bytes far exceed the process runner's old 1 MiB capture ceiling.
	line := []byte(`{"type":"future","text":"` + strings.Repeat("SECRET", 1000) + `"}` + "\n")
	for i := 0; i < 2000; i++ {
		_, _ = s.Write(line)
	}
	_, _ = s.Write([]byte(`{"type":"turn.completed"}`))
	s.finish()
	if len(notifications) != 4 || notifications[3].Activity != generationprogress.Turn {
		t.Fatal("large total stream lost final event")
	}
	if strings.Contains(fmt.Sprint(notifications), "SECRET") {
		t.Fatal("payload escaped allowlist")
	}
}

func TestStreamEventLimitIsPerLineAndResynchronizes(t *testing.T) {
	var codes []generationprogress.Code
	s := newEventStream(func(n agentrunner.Notification) {
		if n.Diagnostic != "" {
			codes = append(codes, n.Diagnostic)
		}
	})
	_, _ = s.Write(bytes.Repeat([]byte("x"), eventLimit))
	if s.discard {
		t.Fatal("limit rejected early")
	}
	_, _ = s.Write([]byte("x"))
	if !s.discard || len(s.line) != 0 {
		t.Fatal("oversized line retained")
	}
	_, _ = s.Write([]byte("\n{}\nnull\n[]\n{\n"))
	s.finish()
	if len(codes) != 2 || codes[0] != generationprogress.OversizedEvent || codes[1] != generationprogress.InvalidEvent {
		t.Fatalf("diagnostics not bounded %#v", codes)
	}
}
