package api

import (
	"compress/gzip"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStaticRejectsTraversalSegments(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"index.html":      &fstest.MapFile{Data: []byte("index")},
		"assets/app.js":   &fstest.MapFile{Data: []byte("app")},
		"../secret.txt":   &fstest.MapFile{Data: []byte("secret")},
		`assets\\evil.js`: &fstest.MapFile{Data: []byte("evil")},
	}}
	for _, target := range []string{
		"/../secret.txt",
		"/assets/../secret.txt",
		`/assets\\evil.js`,
	} {
		t.Run(target, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			s.static(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
			}
		})
	}
}

func TestStaticRejectsNonReadMethods(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("index")},
	}}
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/", nil)
			s.static(rr, req)
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusMethodNotAllowed, rr.Body.String())
			}
		})
	}
}

func TestStaticServesCleanAssetPath(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"assets/app.js": &fstest.MapFile{Data: []byte("console.log('ok')")},
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	s.static(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if rr.Body.String() != "console.log('ok')" {
		t.Fatalf("body=%q", rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "no-cache, no-store, must-revalidate" {
		t.Fatalf("cache-control=%q", got)
	}
}

func TestStaticMissingAssetDoesNotFallbackToIndex(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("index")},
	}}
	rr := httptest.NewRecorder()
	s.static(rr, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
	if rr.Body.String() == "index" {
		t.Fatal("missing module must not receive index.html")
	}
}

func TestStaticCachesAndCompressesHashedAssets(t *testing.T) {
	body := strings.Repeat("console.log('compress me');", 40)
	s := &Server{Static: fstest.MapFS{
		"assets/app-abc123.js": &fstest.MapFile{Data: []byte(body)},
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app-abc123.js", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	s.static(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control=%q", got)
	}
	if got := rr.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding=%q", got)
	}
	if got := rr.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary=%q", got)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/javascript" {
		t.Fatalf("Content-Type=%q", got)
	}
	zr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if err := zr.Close(); err != nil {
		t.Fatal(err)
	}
	if string(decoded) != body {
		t.Fatalf("decoded body mismatch: got %d bytes want %d", len(decoded), len(body))
	}
}

func TestStaticDoesNotGzipWhenExplicitlyRejected(t *testing.T) {
	body := strings.Repeat("body{}", 40)
	s := &Server{Static: fstest.MapFS{
		"assets/app.css": &fstest.MapFile{Data: []byte(body)},
	}}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	req.Header.Set("Accept-Encoding", "br, gzip;q=0")
	s.static(rr, req)

	if got := rr.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding=%q", got)
	}
	if rr.Body.String() != body {
		t.Fatalf("body mismatch: got %d bytes want %d", rr.Body.Len(), len(body))
	}
}

func TestStaticDoesNotCacheIndexOrSPAFallback(t *testing.T) {
	s := &Server{Static: fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<main>app</main>")},
	}}
	for _, target := range []string{"/", "/settings/models"} {
		t.Run(target, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, target, nil)
			s.static(rr, req)
			if got := rr.Header().Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("Cache-Control=%q", got)
			}
			if got := rr.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Fatalf("Content-Type=%q", got)
			}
			if rr.Body.String() != "<main>app</main>" {
				t.Fatalf("body=%q", rr.Body.String())
			}
		})
	}
}

func TestStaticHEADMatchesSelectedGzipRepresentationWithoutBody(t *testing.T) {
	body := strings.Repeat("export const answer = 42;", 40)
	s := &Server{Static: fstest.MapFS{
		"assets/app-hash.js": &fstest.MapFile{Data: []byte(body)},
	}}
	get := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/assets/app-hash.js", nil)
	getReq.Header.Set("Accept-Encoding", "gzip")
	s.static(get, getReq)

	head := httptest.NewRecorder()
	headReq := httptest.NewRequest(http.MethodHead, "/assets/app-hash.js", nil)
	headReq.Header.Set("Accept-Encoding", "gzip")
	s.static(head, headReq)

	if head.Body.Len() != 0 {
		t.Fatalf("HEAD body has %d bytes", head.Body.Len())
	}
	for _, name := range []string{"Cache-Control", "Content-Encoding", "Content-Length", "Content-Type", "Vary"} {
		if got, want := head.Header().Get(name), get.Header().Get(name); got != want {
			t.Fatalf("%s=%q want %q", name, got, want)
		}
	}
}

var _ fs.FS = fstest.MapFS{}
