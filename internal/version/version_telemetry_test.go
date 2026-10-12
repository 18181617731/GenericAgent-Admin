package version

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestUpdateTelemetryPersistentLog(t *testing.T) {
	old := statusPathOverride
	statusPathOverride = filepath.Join(t.TempDir(), "status.json")
	defer func() { statusPathOverride = old }()
	st := UpdateStatus{ID: "telemetry", Stage: "downloading", Message: "Downloading package", Running: true}
	if err := writeStatus(st); err != nil {
		t.Fatal(err)
	}
	st.Download = &DownloadProgress{Name: "package.zip", Attempt: 1, DownloadedBytes: 5, TotalBytes: 10, Percent: 50}
	if err := writeStatus(st); err != nil {
		t.Fatal(err)
	}
	st.Download.Attempt = 2
	for i := 0; i < 2; i++ {
		if err := writeStatus(st); err != nil {
			t.Fatal(err)
		}
	}
	st.Stage, st.Message, st.Download = "ready", "Waiting for restart", nil
	if err := writeStatus(st); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(statusPath())
	first, second := CurrentUpdateStatus(), CurrentUpdateStatus()
	after, _ := os.ReadFile(statusPath())
	if !bytes.Equal(before, after) || first.Log != second.Log {
		t.Fatal("status reads mutated persisted state")
	}
	for _, want := range []string{"Downloading package", "Retry download package.zip (attempt 2)", "Waiting for restart"} {
		if strings.Count(first.Log, want) != 1 {
			t.Fatalf("log missing or duplicated %q: %s", want, first.Log)
		}
	}
	st = UpdateStatus{ID: "next", Stage: "checking", Message: "Next operation"}
	if err := writeStatus(st); err != nil {
		t.Fatal(err)
	}
	next := readStatusLocked()
	if strings.Contains(next.Log, "Downloading") || !strings.Contains(next.Log, "Next operation") {
		t.Fatalf("cross-operation log: %q", next.Log)
	}
	st.Stage, st.Message, st.Error = "failed", "Failed", "disk full"
	if err := writeStatus(st); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readStatusLocked().Log, "disk full") {
		t.Fatal("failure was not persisted")
	}
}

func TestUpdateTelemetryLogBound(t *testing.T) {
	st := UpdateStatus{UpdatedAt: time.Now()}
	for i := 0; i < 3000; i++ {
		appendUpdateLog(&st, fmt.Sprintf("event-%d %s", i, strings.Repeat("x", 40)))
	}
	if len(st.Log) > maxUpdateLogBytes || !strings.Contains(st.Log, "event-2999") {
		t.Fatal("log is not bounded or latest entry lost")
	}
	appendUpdateLog(&st, strings.Repeat("\u4e0b", maxUpdateLogBytes))
	if st.Log == "" || len(st.Log) > maxUpdateLogBytes || !utf8.ValidString(st.Log) {
		t.Fatal("oversized unicode entry broke bounded log")
	}
}

func TestDownloadTelemetryStreaming(t *testing.T) {
	payload := strings.Repeat("x", 128*1024)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(payload)))
		_, _ = w.Write([]byte(payload[:64*1024]))
		w.(http.Flusher).Flush()
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte(payload[64*1024:]))
	}))
	defer srv.Close()
	dest := filepath.Join(t.TempDir(), "package.zip")
	var states []DownloadProgress
	err := downloadWithProgress(context.Background(), srv.URL, dest, int64(len(payload)+1), 1, func(p DownloadProgress) {
		states = append(states, p)
		if p.Complete {
			if _, err := os.Stat(dest); err != nil {
				t.Error("completion emitted before rename")
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	middle := false
	for _, p := range states {
		if !p.Complete && p.DownloadedBytes > 0 && p.Percent > 0 && p.Percent < 100 {
			middle = true
		}
	}
	final := states[len(states)-1]
	if !middle || !final.Complete || final.Percent != 100 || final.TotalBytes != int64(len(payload)) || final.DownloadedBytes != int64(len(payload)) {
		t.Fatalf("wrong streaming states: %+v", states)
	}
}

func TestDownloadTelemetryUnknownAndMetadataSize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("abc"))
	}))
	defer srv.Close()
	for _, expected := range []int64{0, 3} {
		var states []DownloadProgress
		err := downloadWithProgress(context.Background(), srv.URL, filepath.Join(t.TempDir(), "file"), 10, expected, func(p DownloadProgress) { states = append(states, p) })
		if err != nil {
			t.Fatal(err)
		}
		if expected == 0 && states[0].Percent != -1 {
			t.Fatalf("unknown total fabricated percentage: %+v", states)
		}
		final := states[len(states)-1]
		if !final.Complete || final.Percent != 100 || final.DownloadedBytes != 3 || final.TotalBytes != expected {
			t.Fatalf("incorrect final %+v", final)
		}
	}
}

func TestDownloadTelemetryRetryResetsBytes(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Length", "10")
		if attempts == 1 {
			_, _ = w.Write([]byte("abc"))
			return
		}
		_, _ = w.Write([]byte("abcdefghij"))
	}))
	defer srv.Close()
	var states []DownloadProgress
	err := downloadWithProgress(context.Background(), srv.URL, filepath.Join(t.TempDir(), "file"), 20, 10, func(p DownloadProgress) { states = append(states, p) })
	if err != nil {
		t.Fatal(err)
	}
	reset := false
	for _, p := range states {
		if p.Attempt == 2 && p.DownloadedBytes == 0 && !p.Complete {
			reset = true
		}
	}
	final := states[len(states)-1]
	if !reset || final.Attempt != 2 || final.DownloadedBytes != 10 || !final.Complete {
		t.Fatalf("retry did not reset: %+v", states)
	}
}

func TestDownloadTelemetryCancelPreservesDestination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dest := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(dest, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	completed := false
	err := downloadWithProgress(ctx, srv.URL, dest, 1000, 100, func(p DownloadProgress) {
		if p.Complete {
			completed = true
		}
		if p.TotalBytes == 100 && !p.Complete && p.Attempt == 1 {
			cancel()
		}
	})
	got, _ := os.ReadFile(dest)
	if err == nil || completed || string(got) != "old" {
		t.Fatalf("cancel lost old file or completed: %v, %v, %q", err, completed, got)
	}
}

func TestDownloadTelemetryThrottle(t *testing.T) {
	calls := 0
	w := downloadProgressWriter{report: func(p DownloadProgress) { calls++ }, state: DownloadProgress{TotalBytes: 10}}
	w.emit(false)
	for i := 0; i < 10; i++ {
		_, _ = w.Write([]byte("x"))
	}
	if calls != 1 {
		t.Fatalf("unthrottled writes: %d", calls)
	}
	w.last = time.Now().Add(-time.Second)
	_, _ = w.Write([]byte("x"))
	if calls != 2 || w.state.Percent != 99 {
		t.Fatalf("missing throttled report or premature 100: %+v", w.state)
	}
	w.emit(true)
	if calls != 3 || w.state.Percent != 100 {
		t.Fatal("completion report missing")
	}
}
