package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
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
}

var _ fs.FS = fstest.MapFS{}
