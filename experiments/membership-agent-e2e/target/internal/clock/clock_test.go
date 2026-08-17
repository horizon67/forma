package clock

import (
	"testing"
	"time"
)

func TestFixedAdvancesOnlyWhenAsked(t *testing.T) {
	start := time.Date(2026, 8, 17, 10, 0, 0, 0, time.UTC)
	fixed := NewFixed(start)
	if !fixed.Now().Equal(start) {
		t.Fatalf("now = %v, want %v", fixed.Now(), start)
	}
	if !fixed.Now().Equal(start) {
		t.Fatal("reading the clock must not advance it")
	}
	fixed.Advance(30 * time.Minute)
	if want := start.Add(30 * time.Minute); !fixed.Now().Equal(want) {
		t.Fatalf("now = %v, want %v", fixed.Now(), want)
	}
}

func TestSystemClockIsUTC(t *testing.T) {
	if System().Now().Location() != time.UTC {
		t.Fatal("the server clock must report UTC")
	}
}
