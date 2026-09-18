package main

import (
	"strings"
	"testing"
	"time"
)

func TestRenderProgressLine(t *testing.T) {
	got := renderProgressLine(6*1024*1024, 12*1024*1024, 3*time.Second)
	if !strings.Contains(got, "50.0%") || !strings.Contains(got, "6.0/12.0") || !strings.Contains(got, "MB/s") {
		t.Fatalf("known-total line=%q", got)
	}
	got = renderProgressLine(3*1024*1024, 0, 2*time.Second)
	if strings.Contains(got, "%") || !strings.Contains(got, "3.0 MB") {
		t.Fatalf("unknown-total line=%q", got)
	}
	got = renderProgressLine(0, 10*1024*1024, 0)
	if !strings.Contains(got, "0.0%") || !strings.Contains(got, "0.0 MB/s") {
		t.Fatalf("zero line=%q", got)
	}
	got = renderProgressLine(1024, 0, 0)
	if strings.Contains(got, "%") || strings.Contains(got, "NaN") || strings.Contains(got, "Inf") {
		t.Fatalf("tiny unknown line=%q", got)
	}
}

func TestProgressWriterNonTerminal(t *testing.T) {
	pw := newProgressWriter(100, false, "DL")
	pw.start = pw.start.Add(-3 * time.Second)
	pw.update(5)
	pw.update(50)
	if pw.lastPct < 0 {
		t.Fatal("non-terminal writer never emitted")
	}
}
func TestParseBatchOutcome(t *testing.T) {
	if o := parseBatchOutcome("success: swapped"); o.code != updateExitOK {
		t.Fatalf("success %+v", o)
	}
	if o := parseBatchOutcome("rolled back: old binary restored"); o.code != updateExitRollback {
		t.Fatalf("rollback %+v", o)
	}
	if o := parseBatchOutcome("FAILED move after 10 attempts"); o.code != updateExitFail {
		t.Fatalf("fail %+v", o)
	}
	if o := parseBatchOutcome("success line\nrolled back line"); o.code != updateExitRollback {
		t.Fatalf("rollback must win %+v", o)
	}
}
