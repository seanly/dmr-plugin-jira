package main

import (
	"testing"
	"time"
)

func TestParseJiraStarted_plus0800(t *testing.T) {
	s := "2026-03-23T09:00:00.000+0800"
	got, err := parseJiraStarted(s)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := time.Parse("2006-01-02 15:04:05 -0700", "2026-03-23 09:00:00 +0800")
	if !got.Equal(want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseJiraStarted_negativeOffset(t *testing.T) {
	s := "2026-01-15T12:00:00.000-0500"
	got, err := parseJiraStarted(s)
	if err != nil {
		t.Fatal(err)
	}
	if got.Year() != 2026 || got.Month() != 1 || got.Day() != 15 {
		t.Fatalf("unexpected date: %v", got)
	}
}
