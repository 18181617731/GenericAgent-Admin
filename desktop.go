package main

import (
	"errors"
	"log"
	"net"
	"net/url"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

// errDesktopWindowUnsupported reports that this platform has no native window
// backend, so callers fall back to the system browser.
var errDesktopWindowUnsupported = errors.New("native desktop window is not supported on this platform")

// Creating a native webview window downloads nothing but does spin up a
// browser process, so allow a generous startup budget before giving up on
// the wait.
const desktopWindowStartTimeout = 30 * time.Second

// nativeThemeBinding is the function the page calls to tell its host window
// which palette is on screen. It is declared here so the web bundle and the
// native window backends cannot drift apart silently.
const nativeThemeBinding = "gaNativeTheme"

const (
	adminWindowName = "admin"
	chatWindowName  = "chat"
	settingsRoute   = "/admin/settings"
	// Chat and the admin console are both full working surfaces, so they open
	// at the same size rather than ranking one as a side panel.
	windowWidth     = 1280
	windowHeight    = 860
	minWindowWidth  = 900
	minWindowHeight = 600
)

type desktopWindowSpec struct {
	Name      string
	Title     string
	URL       string
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
	DataPath  string
	// Reroute sends an already-open window to URL instead of only focusing it.
	// Plain "open" entries leave the window where the user left it; entries
	// that name a destination, such as the settings page, set this.
	Reroute bool
}

// desktopWindow is a live native window. Every method is safe to call from any
// goroutine.
type desktopWindow interface {
	Focus()
	Navigate(url string)
	Close()
}

// appUI opens the admin and chat UIs. A nil windows field means this launch
// uses the system browser instead of native windows.
type appUI struct {
	windows *desktopWindows
}

func newAppUI(appRoot string, browserOnly bool) *appUI {
	if browserOnly {
		return &appUI{}
	}
	dataPath := filepath.Join(appRoot, "data", "webview")
	return &appUI{windows: newDesktopWindows(dataPath, openBrowser)}
}

// OpenChat opens the primary interface, which the web bundle serves at the
// root path.
func (u *appUI) OpenChat(baseURL string) {
	u.open(desktopWindowSpec{
		Name:   chatWindowName,
		Title:  "GenericAgent Chat",
		URL:    loopbackURL(baseURL),
		Width:  windowWidth,
		Height: windowHeight,
	})
}

// OpenSettings is the only way in to the admin console, so it names the page it
// wants: a window left on another tab is sent back to settings.
func (u *appUI) OpenSettings(baseURL string) {
	u.open(desktopWindowSpec{
		Name:    adminWindowName,
		Title:   "GenericAgent Admin",
		URL:     loopbackURL(baseURL) + settingsRoute,
		Width:   windowWidth,
		Height:  windowHeight,
		Reroute: true,
	})
}

func (u *appUI) CloseAll() {
	if u == nil {
		return
	}
	u.windows.CloseAll()
}

func (u *appUI) open(spec desktopWindowSpec) {
	if u == nil {
		return
	}
	if u.windows == nil {
		openBrowser(spec.URL)
		return
	}
	spec.MinWidth = minWindowWidth
	spec.MinHeight = minWindowHeight
	u.windows.Open(spec)
}

// desktopWindows keeps at most one native window per name and falls back to
// the system browser whenever a window cannot be created.
type desktopWindows struct {
	dataPath string
	fallback func(string)
	run      func(desktopWindowSpec, func(desktopWindow)) error

	mu    sync.Mutex
	slots map[string]*desktopWindowSlot
}

type desktopWindowSlot struct {
	// open serializes Open for a single window name so that repeated clicks
	// cannot race two windows into existence.
	open sync.Mutex
	// win is guarded by desktopWindows.mu, not by open, so that a window
	// closing never has to wait on an in-flight Open.
	win desktopWindow
}

func newDesktopWindows(dataPath string, fallback func(string)) *desktopWindows {
	return &desktopWindows{
		dataPath: dataPath,
		fallback: fallback,
		run:      runDesktopWindow,
		slots:    map[string]*desktopWindowSlot{},
	}
}

func (d *desktopWindows) Open(spec desktopWindowSpec) {
	if d == nil {
		return
	}
	slot := d.slot(spec.Name)
	slot.open.Lock()
	defer slot.open.Unlock()

	if win := d.window(spec.Name); win != nil {
		if spec.Reroute {
			win.Navigate(spec.URL)
		}
		win.Focus()
		return
	}

	spec.DataPath = d.dataPath
	started := make(chan error, 1)
	var report sync.Once
	go func() {
		// The window owns this OS thread for its whole lifetime. WebView2
		// delivers messages to the thread that created the window; on macOS
		// this goroutine only waits, because AppKit already owns the main
		// thread via the tray.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		err := d.run(spec, func(win desktopWindow) {
			d.setWindow(spec.Name, win)
			report.Do(func() { started <- nil })
		})
		d.setWindow(spec.Name, nil)
		report.Do(func() { started <- err })
	}()

	select {
	case err := <-started:
		if err == nil {
			return
		}
		log.Printf("desktop window %q unavailable (%v); opening the system browser instead", spec.Name, err)
		if d.fallback != nil {
			d.fallback(spec.URL)
		}
	case <-time.After(desktopWindowStartTimeout):
		// The window may still be on its way; opening a browser now would
		// leave the user with two copies of the UI.
		log.Printf("desktop window %q has not appeared after %s; still waiting", spec.Name, desktopWindowStartTimeout)
	}
}

func (d *desktopWindows) CloseAll() {
	if d == nil {
		return
	}
	d.mu.Lock()
	open := make([]desktopWindow, 0, len(d.slots))
	for _, slot := range d.slots {
		if slot.win != nil {
			open = append(open, slot.win)
		}
	}
	d.mu.Unlock()
	for _, win := range open {
		win.Close()
	}
}

func (d *desktopWindows) slot(name string) *desktopWindowSlot {
	d.mu.Lock()
	defer d.mu.Unlock()
	slot, ok := d.slots[name]
	if !ok {
		slot = &desktopWindowSlot{}
		d.slots[name] = slot
	}
	return slot
}

func (d *desktopWindows) window(name string) desktopWindow {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot, ok := d.slots[name]; ok {
		return slot.win
	}
	return nil
}

func (d *desktopWindows) setWindow(name string, win desktopWindow) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if slot, ok := d.slots[name]; ok {
		slot.win = win
	}
}

// loopbackURL rewrites a wildcard listen address into an explicit loopback
// host. Requests from loopback skip admin authentication, so a window pointed
// at 0.0.0.0 would otherwise face a native credential prompt.
func loopbackURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return raw
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsUnspecified() {
		return raw
	}
	parsed.Host = net.JoinHostPort("127.0.0.1", port)
	return parsed.String()
}
