package desktop

import (
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeDesktopWindow struct {
	mu      sync.Mutex
	focused int
	visited []string
	closed  chan struct{}
	once    sync.Once
}

func newFakeDesktopWindow() *fakeDesktopWindow {
	return &fakeDesktopWindow{closed: make(chan struct{})}
}

func (f *fakeDesktopWindow) Focus() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.focused++
}

func (f *fakeDesktopWindow) Navigate(url string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.visited = append(f.visited, url)
}

func (f *fakeDesktopWindow) Close() {
	f.once.Do(func() { close(f.closed) })
}

func (f *fakeDesktopWindow) focusCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focused
}

func (f *fakeDesktopWindow) navigations() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.visited...)
}

// fakeRunner mimics runDesktopWindow: it publishes a window and stays alive
// until that window is closed.
func fakeRunner(created chan<- *fakeDesktopWindow) func(desktopWindowSpec, func(desktopWindow)) error {
	return func(_ desktopWindowSpec, ready func(desktopWindow)) error {
		win := newFakeDesktopWindow()
		ready(win)
		created <- win
		<-win.closed
		return nil
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestOpenNewChatCreatesIndependentWindows(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 2)
	specs := make(chan desktopWindowSpec, 2)
	manager := newDesktopWindows("", nil)
	manager.run = func(spec desktopWindowSpec, ready func(desktopWindow)) error {
		specs <- spec
		return fakeRunner(created)(spec, ready)
	}
	ui := &UI{windows: manager}

	ui.OpenNewChat("http://0.0.0.0:8787")
	ui.OpenNewChat("http://0.0.0.0:8787")

	firstSpec, secondSpec := <-specs, <-specs
	firstWindow, secondWindow := <-created, <-created
	defer firstWindow.Close()
	defer secondWindow.Close()

	if firstSpec.Name == secondSpec.Name {
		t.Fatalf("new chat windows reused name %q", firstSpec.Name)
	}
	for _, spec := range []desktopWindowSpec{firstSpec, secondSpec} {
		if spec.URL != "http://127.0.0.1:8787" {
			t.Errorf("new chat URL = %q", spec.URL)
		}
		if !spec.Transient {
			t.Errorf("new chat window %q did not mark its slot transient", spec.Name)
		}
	}
	if firstWindow == secondWindow {
		t.Fatal("multi-open reused the same window")
	}
}

func TestDesktopWindowsReusesWindowPerName(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 8)
	manager := newDesktopWindows("", nil)
	manager.run = fakeRunner(created)

	spec := desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"}
	manager.Open(spec)
	win := <-created

	manager.Open(spec)
	manager.Open(spec)

	if got := win.focusCount(); got != 2 {
		t.Fatalf("focus count = %d, want 2", got)
	}
	if extra := len(created); extra != 0 {
		t.Fatalf("created %d extra windows, want 0", extra)
	}
}

func TestDesktopWindowsRoutesReusedWindowOnlyWhenAsked(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 8)
	manager := newDesktopWindows("", nil)
	manager.run = fakeRunner(created)

	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"})
	win := <-created

	// Reopening the admin window leaves the user on whatever page they were
	// reading; only a destination entry such as settings moves them.
	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"})
	if got := win.navigations(); len(got) != 0 {
		t.Fatalf("plain reopen navigated to %v, want no navigation", got)
	}

	settings := "http://127.0.0.1:8787" + settingsRoute
	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: settings, Reroute: true})
	if got := win.navigations(); len(got) != 1 || got[0] != settings {
		t.Fatalf("settings navigation = %v, want [%s]", got, settings)
	}
	if got := win.focusCount(); got != 2 {
		t.Fatalf("focus count = %d, want 2", got)
	}
}

func TestDesktopWindowsSeparateWindowPerName(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 8)
	manager := newDesktopWindows("", nil)
	manager.run = fakeRunner(created)

	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"})
	manager.Open(desktopWindowSpec{Name: chatWindowName, URL: "http://127.0.0.1:8787/chat"})

	if got := len(created); got != 2 {
		t.Fatalf("created %d windows, want 2", got)
	}
}

func TestDesktopWindowsReopensAfterClose(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 8)
	manager := newDesktopWindows("", nil)
	manager.run = fakeRunner(created)

	spec := desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"}
	manager.Open(spec)
	first := <-created
	first.Close()
	waitFor(t, "window deregistration", func() bool { return manager.window(adminWindowName) == nil })

	manager.Open(spec)
	second := <-created
	if second == first {
		t.Fatal("expected a new window after the previous one closed")
	}
	if got := first.focusCount(); got != 0 {
		t.Fatalf("closed window was focused %d times, want 0", got)
	}
}

func TestDesktopWindowsFallsBackToBrowser(t *testing.T) {
	var mu sync.Mutex
	var opened []string
	manager := newDesktopWindows("", func(url string) {
		mu.Lock()
		defer mu.Unlock()
		opened = append(opened, url)
	})
	manager.run = func(desktopWindowSpec, func(desktopWindow)) error {
		return errors.New("no webview runtime")
	}

	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"})

	mu.Lock()
	defer mu.Unlock()
	if len(opened) != 1 || opened[0] != "http://127.0.0.1:8787" {
		t.Fatalf("browser fallback got %v, want [http://127.0.0.1:8787]", opened)
	}
}

func TestDesktopWindowsCloseAll(t *testing.T) {
	created := make(chan *fakeDesktopWindow, 8)
	manager := newDesktopWindows("", nil)
	manager.run = fakeRunner(created)

	manager.Open(desktopWindowSpec{Name: adminWindowName, URL: "http://127.0.0.1:8787"})
	manager.Open(desktopWindowSpec{Name: chatWindowName, URL: "http://127.0.0.1:8787/chat"})
	manager.CloseAll()

	waitFor(t, "all windows to deregister", func() bool {
		return manager.window(adminWindowName) == nil && manager.window(chatWindowName) == nil
	})
}

func TestAppUIBrowserOnlyDoesNotCreateWindows(t *testing.T) {
	ui := NewUI(t.TempDir(), true)
	if ui.windows != nil {
		t.Fatal("browser-only mode must not build a window manager")
	}
	ui.CloseAll()
}

func TestLoopbackURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "loopback is unchanged", in: "http://127.0.0.1:8787", want: "http://127.0.0.1:8787"},
		{name: "ipv4 wildcard is rewritten", in: "http://0.0.0.0:8787", want: "http://127.0.0.1:8787"},
		{name: "ipv6 wildcard is rewritten", in: "http://[::]:8787", want: "http://127.0.0.1:8787"},
		{name: "lan address is unchanged", in: "http://192.168.1.10:8787", want: "http://192.168.1.10:8787"},
		{name: "hostname is unchanged", in: "http://localhost:8787", want: "http://localhost:8787"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := loopbackURL(tc.in); got != tc.want {
				t.Fatalf("loopbackURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
