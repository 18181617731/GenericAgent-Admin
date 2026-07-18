package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

func TestPackageRuntimeEmbedsWorldlineAndLeavesGuardMarker(t *testing.T) {
	worker := []byte(`payload = "__GA_ADMIN_BUNDLED_WORLDLINE_B64__"
if payload == "__GA_ADMIN_BUNDLED_WORLDLINE_B64__": pass
`)
	want := []byte("worldline runtime")
	packaged, err := packageRuntime(worker, want)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Count(packaged, []byte(marker)) != 1 {
		t.Fatalf("packaged marker count=%d want 1", bytes.Count(packaged, []byte(marker)))
	}
	line := strings.Split(string(packaged), "\n")[0]
	payload := strings.TrimSuffix(strings.TrimPrefix(line, `payload = "`), `"`)
	compressed, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	_ = zr.Close()
	if !bytes.Equal(got, want) {
		t.Fatalf("decoded runtime=%q want=%q", got, want)
	}
}

func TestPackageRuntimeRejectsUnexpectedMarkerCount(t *testing.T) {
	if _, err := packageRuntime([]byte("worker without marker"), []byte("runtime")); err == nil {
		t.Fatal("expected missing marker error")
	}
}
