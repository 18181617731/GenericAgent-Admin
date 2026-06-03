//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// petBubble is a small speech-balloon window rendered above the desktop pet.
// It is a borderless, click-through, top-most layered window. We rely on the
// classic LWA_COLORKEY trick (a fixed magenta key colour becomes transparent)
// so we can paint the balloon and its text with plain GDI calls and still get
// a non-rectangular, transparent result without per-pixel alpha bookkeeping.
const (
	bubbleClassName = "GAAdminDesktopPetBubble"
	bubbleTitle     = "GA Admin Pet Bubble"

	// COLORREF values are 0x00BBGGRR.
	bubbleKeyColor    = 0x00FF00FF // magenta -> transparent
	bubbleFillColor   = 0x00EEF6FA // warm cream
	bubbleBorderColor = 0x0078AAC8 // soft brown
	bubbleTextColor   = 0x00413732 // dark slate

	bubblePadX    = 16
	bubblePadY    = 11
	bubbleRadius  = 18
	bubbleTailH   = 11
	bubbleTailHW  = 10 // tail half width
	bubbleGapY    = 6  // gap between balloon bottom and pet top
	bubbleMaxTxtW = 260
	bubbleFontH   = 19

	lwaColorKey     = 0x00000001
	wsExTransparent = 0x00000020
	wmPaint         = 0x0000000F

	dtCenter    = 0x00000001
	dtWordBreak = 0x00000010
	dtCalcRect  = 0x00000400
	dtNoPrefix  = 0x00000800

	transparentBkMode = 1
	psSolid           = 0
	defaultCharSet    = 1
)

var (
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procBeginPaint                 = user32.NewProc("BeginPaint")
	procEndPaint                   = user32.NewProc("EndPaint")
	procFillRect                   = user32.NewProc("FillRect")
	procDrawText                   = user32.NewProc("DrawTextW")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procInvalidateRect             = user32.NewProc("InvalidateRect")

	procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	procCreatePen        = gdi32.NewProc("CreatePen")
	procRoundRect        = gdi32.NewProc("RoundRect")
	procPolygon          = gdi32.NewProc("Polygon")
	procSetBkMode        = gdi32.NewProc("SetBkMode")
	procSetTextColor     = gdi32.NewProc("SetTextColor")
	procCreateFont       = gdi32.NewProc("CreateFontW")
)

type paintStruct struct {
	Hdc         syscall.Handle
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type petBubble struct {
	hwnd   syscall.Handle
	font   syscall.Handle
	width  int32
	height int32
	text   []uint16 // UTF-16, null terminated
}

func newPetBubble() *petBubble {
	return &petBubble{}
}

func (b *petBubble) createWindow() error {
	instance, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString(bubbleClassName)
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(b.wndProc),
		Instance:  syscall.Handle(instance),
		ClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	face, _ := syscall.UTF16PtrFromString("Microsoft YaHei")
	font, _, _ := procCreateFont.Call(
		uintptr(int32(bubbleFontH)), 0, 0, 0,
		400, 0, 0, 0,
		defaultCharSet, 0, 0, 4, 0,
		uintptr(unsafe.Pointer(face)),
	)
	b.font = syscall.Handle(font)

	title, _ := syscall.UTF16PtrFromString(bubbleTitle)
	hwnd, _, err := procCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate|wsExTransparent,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		cwUseDefault, cwUseDefault, 10, 10,
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return err
	}
	b.hwnd = syscall.Handle(hwnd)
	procSetLayeredWindowAttributes.Call(uintptr(b.hwnd), uintptr(bubbleKeyColor), 0, lwaColorKey)
	return nil
}

func (b *petBubble) wndProc(hwnd syscall.Handle, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmPaint:
		b.paint(hwnd)
		return 0
	case wmDestroy:
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wparam, lparam)
	return r
}

// showAt sizes, positions and shows the balloon above the given pet rectangle.
func (b *petBubble) showAt(text string, petRect rect) {
	if b == nil || b.hwnd == 0 {
		return
	}
	utf16, _ := syscall.UTF16FromString(text)
	b.text = utf16

	tw, th := b.measure(utf16)
	w := tw + 2*bubblePadX
	h := th + 2*bubblePadY + bubbleTailH
	if w < 60 {
		w = 60
	}
	b.width = w
	b.height = h

	petCX := (petRect.Left + petRect.Right) / 2
	x := petCX - w/2
	y := petRect.Top - h - bubbleGapY
	if y < 0 {
		y = 0
	}
	// SWP_NOACTIVATE(0x0010) | SWP_SHOWWINDOW(0x0040)
	procSetWindowPos.Call(uintptr(b.hwnd), ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0x0010|0x0040)
	procInvalidateRect.Call(uintptr(b.hwnd), 0, 1)
}

// reposition keeps the visible balloon centered above the pet (used while the
// pet roams or is dragged).
func (b *petBubble) reposition(petRect rect) {
	if b == nil || b.hwnd == 0 || b.width == 0 {
		return
	}
	petCX := (petRect.Left + petRect.Right) / 2
	x := petCX - b.width/2
	y := petRect.Top - b.height - bubbleGapY
	if y < 0 {
		y = 0
	}
	// SWP_NOSIZE(0x0001) | SWP_NOACTIVATE(0x0010) | SWP_NOZORDER? keep topmost via NOZORDER off.
	procSetWindowPos.Call(uintptr(b.hwnd), ^uintptr(0), uintptr(x), uintptr(y), 0, 0, 0x0001|0x0010)
}

func (b *petBubble) hide() {
	if b == nil || b.hwnd == 0 {
		return
	}
	b.width = 0
	b.height = 0
	procShowWindow.Call(uintptr(b.hwnd), swHide)
}

func (b *petBubble) measure(text []uint16) (int32, int32) {
	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return bubbleMaxTxtW, bubbleFontH
	}
	defer procReleaseDC.Call(0, screenDC)
	memDC, _, _ := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return bubbleMaxTxtW, bubbleFontH
	}
	defer procDeleteDC.Call(memDC)
	old, _, _ := procSelectObject.Call(memDC, uintptr(b.font))
	defer procSelectObject.Call(memDC, old)

	rc := rect{Left: 0, Top: 0, Right: bubbleMaxTxtW, Bottom: 0}
	procDrawText.Call(memDC, uintptr(unsafe.Pointer(&text[0])), ^uintptr(0),
		uintptr(unsafe.Pointer(&rc)), dtCalcRect|dtWordBreak|dtCenter|dtNoPrefix)
	w := rc.Right - rc.Left
	h := rc.Bottom - rc.Top
	if h < bubbleFontH {
		h = bubbleFontH
	}
	return w, h
}

func (b *petBubble) paint(hwnd syscall.Handle) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&ps)))

	var rc rect
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&rc)))

	// Flood the client area with the transparent key colour.
	keyBrush, _, _ := procCreateSolidBrush.Call(uintptr(bubbleKeyColor))
	procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), keyBrush)
	procDeleteObject.Call(keyBrush)

	fillBrush, _, _ := procCreateSolidBrush.Call(uintptr(bubbleFillColor))
	pen, _, _ := procCreatePen.Call(psSolid, 2, uintptr(bubbleBorderColor))
	oldBrush, _, _ := procSelectObject.Call(hdc, fillBrush)
	oldPen, _, _ := procSelectObject.Call(hdc, pen)

	bodyBottom := rc.Bottom - bubbleTailH
	procRoundRect.Call(hdc, 1, 1, uintptr(rc.Right-1), uintptr(bodyBottom), bubbleRadius, bubbleRadius)

	// Tail pointing down toward the pet.
	cx := rc.Right / 2
	pts := [3]point{
		{X: cx - bubbleTailHW, Y: bodyBottom - 2},
		{X: cx + bubbleTailHW, Y: bodyBottom - 2},
		{X: cx, Y: rc.Bottom - 1},
	}
	procPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), 3)

	procSelectObject.Call(hdc, oldBrush)
	procSelectObject.Call(hdc, oldPen)
	procDeleteObject.Call(fillBrush)
	procDeleteObject.Call(pen)

	if len(b.text) > 0 {
		oldFont, _, _ := procSelectObject.Call(hdc, uintptr(b.font))
		procSetBkMode.Call(hdc, transparentBkMode)
		procSetTextColor.Call(hdc, uintptr(bubbleTextColor))
		textRc := rect{Left: bubblePadX, Top: bubblePadY, Right: rc.Right - bubblePadX, Bottom: bodyBottom - bubblePadY}
		procDrawText.Call(hdc, uintptr(unsafe.Pointer(&b.text[0])), ^uintptr(0),
			uintptr(unsafe.Pointer(&textRc)), dtCenter|dtWordBreak|dtNoPrefix)
		procSelectObject.Call(hdc, oldFont)
	}
}
