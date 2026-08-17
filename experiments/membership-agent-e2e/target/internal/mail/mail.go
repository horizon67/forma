// Package mail records outgoing messages. The experiment has no external
// provider: the verification link is observable only through this outbox, which
// is the same surface the tests read.
package mail

import (
	"errors"
	"sync"
)

// Message carries only what the recipient sees. The verification token lives in
// Link and is never written anywhere else in plaintext.
type Message struct {
	To      string
	Subject string
	Link    string
}

var ErrDeliveryFailed = errors.New("mail delivery failed")

// Outbox is a durable record of emissions. A delivery failure still records the
// emission so the caller can retry or resend, and never rolls back the account.
type Outbox struct {
	mu       sync.Mutex
	messages []Message
	failNext bool
}

func NewOutbox() *Outbox { return &Outbox{} }

// FailNextDelivery makes the next Send report a delivery failure after the
// emission has been recorded.
func (o *Outbox) FailNextDelivery() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failNext = true
}

func (o *Outbox) Send(message Message) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.messages = append(o.messages, message)
	if o.failNext {
		o.failNext = false
		return ErrDeliveryFailed
	}
	return nil
}

func (o *Outbox) Count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.messages)
}

// Latest returns the most recent emission. Tests read the verification link
// from here rather than from the store, which never holds the plaintext token.
func (o *Outbox) Latest() (Message, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if len(o.messages) == 0 {
		return Message{}, false
	}
	return o.messages[len(o.messages)-1], true
}

func (o *Outbox) For(address string) []Message {
	o.mu.Lock()
	defer o.mu.Unlock()
	var result []Message
	for _, message := range o.messages {
		if message.To == address {
			result = append(result, message)
		}
	}
	return result
}
