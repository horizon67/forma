package mail

import (
	"errors"
	"testing"
)

func TestOutboxRecordsEmissionEvenWhenDeliveryFails(t *testing.T) {
	outbox := NewOutbox()
	outbox.FailNextDelivery()
	err := outbox.Send(Message{To: "member@example.com", Subject: "Verify", Link: "/verify?token=abc"})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("send error = %v, want a delivery failure", err)
	}
	if outbox.Count() != 1 {
		t.Fatalf("emissions = %d, want the failed delivery to stay recorded", outbox.Count())
	}
	if err := outbox.Send(Message{To: "member@example.com"}); err != nil {
		t.Fatalf("only the next delivery must fail: %v", err)
	}
	if outbox.Count() != 2 {
		t.Fatalf("emissions = %d, want 2", outbox.Count())
	}
}

func TestOutboxExposesTheLatestLinkPerAddress(t *testing.T) {
	outbox := NewOutbox()
	for _, link := range []string{"/verify?token=first", "/verify?token=second"} {
		if err := outbox.Send(Message{To: "member@example.com", Link: link}); err != nil {
			t.Fatal(err)
		}
	}
	if err := outbox.Send(Message{To: "other@example.com", Link: "/verify?token=other"}); err != nil {
		t.Fatal(err)
	}
	latest, ok := outbox.Latest()
	if !ok || latest.To != "other@example.com" {
		t.Fatalf("latest = %#v", latest)
	}
	forMember := outbox.For("member@example.com")
	if len(forMember) != 2 || forMember[1].Link != "/verify?token=second" {
		t.Fatalf("messages for member = %#v", forMember)
	}
}
