//go:build windows

package main

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"log"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

const (
	petClassName = "GAAdminDesktopPetWindow"
	petTitle     = "GA Admin Pet"

	petFrameW = 192
	petFrameH = 208

	petRoamStep     int32 = 6
	petRoamMinTicks       = 24
	petRoamMaxTicks       = 44

	petActionIdle         = "idle"
	petActionRunningRight = "running-right"
	petActionRunningLeft  = "running-left"
	petActionWaving       = "waving"
	petActionJumping      = "jumping"
	petActionFailed       = "failed"
	petActionWaiting      = "waiting"
	petActionRunning      = "running"
	petActionReview       = "review"

	petFrameInterval = 130
	petActionMessage = 0x0400 + 90
)

const (
	cwUseDefault = 0x80000000

	wsPopup = 0x80000000

	wsExLayered    = 0x00080000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
	wsExNoActivate = 0x08000000

	swHide       = 0
	swShowNoAct  = 4
	swShowNormal = 1

	wmDestroy     = 0x0002
	wmClose       = 0x0010
	wmShowPet     = 0x0400 + 71
	wmHidePet     = 0x0400 + 72
	wmTogglePet   = 0x0400 + 73
	wmTimer       = 0x0113
	wmLButtonDown = 0x0201
	wmLButtonUp   = 0x0202
	wmMouseMove   = 0x0200
	wmRButtonUp   = 0x0205
	wmNCHitTest   = 0x0084
	htCaption     = 2

	ulwAlpha     = 0x00000002
	acSrcOver    = 0x00
	acSrcAlpha   = 0x01
	biRGB        = 0
	dibRGBColors = 0
)

type point struct{ X, Y int32 }
type size struct{ CX, CY int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type petAnimation struct {
	Name   string
	Row    int
	Frames int
	Loop   bool
}

var petAnimations = []petAnimation{
	{Name: petActionIdle, Row: 0, Frames: 6, Loop: true},
	{Name: petActionRunningRight, Row: 1, Frames: 8, Loop: true},
	{Name: petActionRunningLeft, Row: 2, Frames: 8, Loop: true},
	{Name: petActionWaving, Row: 3, Frames: 4, Loop: false},
	{Name: petActionJumping, Row: 4, Frames: 5, Loop: false},
	{Name: petActionFailed, Row: 5, Frames: 8, Loop: false},
	{Name: petActionWaiting, Row: 6, Frames: 6, Loop: true},
	{Name: petActionRunning, Row: 7, Frames: 6, Loop: true},
	{Name: petActionReview, Row: 8, Frames: 6, Loop: true},
}

func petActionID(action string) uintptr {
	for i, anim := range petAnimations {
		if anim.Name == action {
			return uintptr(i + 1)
		}
	}
	return 1
}

func petActionByID(id uintptr) string {
	idx := int(id) - 1
	if idx >= 0 && idx < len(petAnimations) {
		return petAnimations[idx].Name
	}
	return petActionIdle
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   syscall.Handle
	Icon       syscall.Handle
	Cursor     syscall.Handle
	Background syscall.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     syscall.Handle
}

type msg struct {
	Hwnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")
	kernel = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procShowWindow          = user32.NewProc("ShowWindow")
	procUpdateWindow        = user32.NewProc("UpdateWindow")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetSystemMetrics    = user32.NewProc("GetSystemMetrics")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procSetCapture          = user32.NewProc("SetCapture")
	procReleaseCapture      = user32.NewProc("ReleaseCapture")
	procGetCursorPos        = user32.NewProc("GetCursorPos")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")

	procGetModuleHandle = kernel.NewProc("GetModuleHandleW")
)

type desktopPet struct {
	frames [][]byte
	width  int32
	height int32
	hwnd   syscall.Handle

	mu              sync.Mutex
	visible         bool
	frameIndex      int
	active          string
	base            string
	oneshot         string
	oneshtTicks     int
	lastDragX       int32
	dragging        bool
	dragOffset      point
	framesByAction  map[string][][]byte
	idleTicks       int
	idleNudge       int
	autoActionStep  int
	roamTicks       int
	roamDX          int32
	roamRestoreBase string
}

var petInstance *desktopPet

func startDesktopPet() func() {
	pet, err := newDesktopPet()
	if err != nil {
		log.Printf("desktop pet disabled: %v", err)
		return func() {}
	}
	petInstance = pet
	ready := make(chan struct{})
	go pet.run(ready)
	<-ready
	return pet.stop
}

func showDesktopPet()   { postPetMessage(wmShowPet) }
func hideDesktopPet()   { postPetMessage(wmHidePet) }
func toggleDesktopPet() { postPetMessage(wmTogglePet) }

func setDesktopPetAction(action string) {
	setDesktopPetActionForTicks(action, 0)
}

func setDesktopPetActionForTicks(action string, ticks int) {
	if petInstance == nil || petInstance.hwnd == 0 {
		return
	}
	procPostMessage.Call(uintptr(petInstance.hwnd), petActionMessage, petActionID(action), uintptr(ticks))
}

func postPetMessage(message uint32) {
	if petInstance == nil || petInstance.hwnd == 0 {
		return
	}
	procPostMessage.Call(uintptr(petInstance.hwnd), uintptr(message), 0, 0)
}

func newDesktopPet() (*desktopPet, error) {
	img, err := png.Decode(bytes.NewReader(gaAdminPetSpritesheetPNG))
	if err != nil {
		return nil, err
	}
	pet := &desktopPet{
		width:          petFrameW,
		height:         petFrameH,
		visible:        true,
		active:         petActionIdle,
		base:           petActionIdle,
		framesByAction: map[string][][]byte{},
	}
	for _, anim := range petAnimations {
		frames := extractPetFrames(img, anim.Row, anim.Frames)
		pet.framesByAction[anim.Name] = frames
		pet.frames = append(pet.frames, frames...)
	}
	return pet, nil
}

func extractPetFrames(src image.Image, row, frames int) [][]byte {
	out := make([][]byte, 0, frames)
	for i := 0; i < frames; i++ {
		r := image.Rect(i*petFrameW, row*petFrameH, (i+1)*petFrameW, (row+1)*petFrameH)
		nrgba := image.NewNRGBA(image.Rect(0, 0, petFrameW, petFrameH))
		draw.Draw(nrgba, nrgba.Bounds(), src, r.Min, draw.Src)
		out = append(out, premultiplyBGRA(nrgba))
	}
	return out
}

func premultiplyBGRA(img *image.NRGBA) []byte {
	b := make([]byte, petFrameW*petFrameH*4)
	j := 0
	for y := 0; y < petFrameH; y++ {
		for x := 0; x < petFrameW; x++ {
			o := img.PixOffset(x, y)
			r := uint32(img.Pix[o])
			g := uint32(img.Pix[o+1])
			bb := uint32(img.Pix[o+2])
			a := uint32(img.Pix[o+3])
			b[j+0] = byte((bb*a + 127) / 255)
			b[j+1] = byte((g*a + 127) / 255)
			b[j+2] = byte((r*a + 127) / 255)
			b[j+3] = byte(a)
			j += 4
		}
	}
	return b
}

func (p *desktopPet) run(ready chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hwnd, err := p.createWindow()
	if err != nil {
		log.Printf("create desktop pet window failed: %v", err)
		close(ready)
		return
	}
	p.hwnd = hwnd
	p.placeInitial()
	p.updateFrame(0)
	procShowWindow.Call(uintptr(hwnd), swShowNoAct)
	procUpdateWindow.Call(uintptr(hwnd))
	procSetTimer.Call(uintptr(hwnd), 1, petFrameInterval, 0)
	close(ready)

	var m msg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func (p *desktopPet) stop() {
	if p.hwnd != 0 {
		procPostMessage.Call(uintptr(p.hwnd), wmClose, 0, 0)
	}
}

func (p *desktopPet) createWindow() (syscall.Handle, error) {
	instance, _, _ := procGetModuleHandle.Call(0)
	className, _ := syscall.UTF16PtrFromString(petClassName)
	cursor, _, _ := procLoadCursor.Call(0, uintptr(unsafe.Pointer(uintptr(32512))))
	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   syscall.NewCallback(p.wndProc),
		Instance:  syscall.Handle(instance),
		Cursor:    syscall.Handle(cursor),
		ClassName: className,
	}
	procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	title, _ := syscall.UTF16PtrFromString(petTitle)
	hwnd, _, err := procCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(title)),
		wsPopup,
		cwUseDefault, cwUseDefault, uintptr(p.width), uintptr(p.height),
		0, 0, instance, 0,
	)
	if hwnd == 0 {
		return 0, err
	}
	return syscall.Handle(hwnd), nil
}

func (p *desktopPet) wndProc(hwnd syscall.Handle, message uint32, wparam, lparam uintptr) uintptr {
	switch message {
	case wmTimer:
		p.onTimer()
		return 0
	case wmShowPet:
		p.visible = true
		procShowWindow.Call(uintptr(hwnd), swShowNoAct)
		p.updateActionFrame(p.active, p.frameIndex)
		return 0
	case wmHidePet:
		p.visible = false
		procShowWindow.Call(uintptr(hwnd), swHide)
		return 0
	case wmTogglePet:
		if p.visible {
			p.visible = false
			procShowWindow.Call(uintptr(hwnd), swHide)
		} else {
			p.visible = true
			procShowWindow.Call(uintptr(hwnd), swShowNoAct)
			p.updateActionFrame(p.active, p.frameIndex)
		}
		return 0
	case petActionMessage:
		p.applyAction(petActionByID(wparam), int(lparam))
		return 0
	case wmLButtonDown:
		var cur point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
		var wr rect
		procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		p.dragging = true
		p.dragOffset = point{X: cur.X - wr.Left, Y: cur.Y - wr.Top}
		p.lastDragX = cur.X
		p.applyAction(petActionWaving, 24)
		procSetCapture.Call(uintptr(hwnd))
		return 0
	case wmMouseMove:
		if p.dragging {
			var cur point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
			if cur.X > p.lastDragX+1 {
				p.applyAction(petActionRunningRight, 0)
			} else if cur.X < p.lastDragX-1 {
				p.applyAction(petActionRunningLeft, 0)
			}
			p.lastDragX = cur.X
			procSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), uintptr(cur.X-p.dragOffset.X), uintptr(cur.Y-p.dragOffset.Y), 0, 0, 0x0001|0x0010|0x0040)
		}
		return 0
	case wmLButtonUp:
		p.dragging = false
		p.applyAction(petActionIdle, 0)
		procReleaseCapture.Call()
		return 0
	case wmRButtonUp:
		p.visible = false
		procShowWindow.Call(uintptr(hwnd), swHide)
		return 0
	case wmNCHitTest:
		return htCaption
	case wmClose:
		procDestroyWindow.Call(uintptr(hwnd))
		return 0
	case wmDestroy:
		procKillTimer.Call(uintptr(hwnd), 1)
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wparam, lparam)
	return r
}

func (p *desktopPet) onTimer() {
	if !p.visible {
		return
	}
	if !p.dragging && p.roamTicks > 0 {
		p.stepRoam()
	} else if p.oneshot == "" {
		p.idleTicks++
		if p.idleTicks >= 32 { // about 4 seconds at petFrameInterval=130ms
			p.idleTicks = 0
			p.playNextIdleAction()
			return
		}
	} else {
		p.idleTicks = 0
	}
	action := p.currentAction()
	p.updateActionFrame(action, p.frameIndex)
	p.frameIndex++
	if p.oneshtTicks > 0 {
		p.oneshtTicks--
		if p.oneshtTicks == 0 {
			p.oneshot = ""
			p.active = p.base
			p.frameIndex = 0
		}
	}
}

func (p *desktopPet) playNextIdleAction() {
	if p.autoActionStep%2 == 0 {
		dx := -petRoamStep
		if (p.autoActionStep/2)%2 == 1 {
			dx = petRoamStep
		}
		p.autoActionStep++
		p.startRoam(dx, petRoamMinTicks+(p.autoActionStep%(petRoamMaxTicks-petRoamMinTicks+1)))
		return
	}
	sequence := []struct {
		action string
		ticks  int
	}{
		{petActionWaving, 14},
		{petActionJumping, 16},
		{petActionWaiting, 18},
		{petActionRunning, 18},
		{petActionReview, 18},
	}
	for tries := 0; tries < len(sequence); tries++ {
		item := sequence[(p.autoActionStep/2)%len(sequence)]
		p.autoActionStep++
		if _, ok := p.framesByAction[item.action]; ok {
			p.applyAction(item.action, item.ticks)
			return
		}
	}
	p.applyAction(petActionWaving, 14)
}

func (p *desktopPet) startRoam(dx int32, ticks int) {
	if dx == 0 {
		return
	}
	if ticks <= 0 {
		ticks = petRoamMinTicks
	}
	p.roamDX = dx
	p.roamTicks = ticks
	p.roamRestoreBase = p.base
	if p.roamRestoreBase == "" || p.roamRestoreBase == petActionRunningRight || p.roamRestoreBase == petActionRunningLeft {
		p.roamRestoreBase = petActionIdle
	}
	p.oneshot = ""
	p.oneshtTicks = 0
	p.base = petActionRunningRight
	if dx < 0 {
		p.base = petActionRunningLeft
	}
	p.active = p.base
	p.frameIndex = 0
	p.stepRoam()
}

func (p *desktopPet) stepRoam() {
	if p.hwnd == 0 || p.roamTicks <= 0 {
		return
	}
	var wr rect
	procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	maxX := int32(sw) - p.width
	maxY := int32(sh) - p.height
	if maxX < 0 {
		maxX = 0
	}
	if maxY < 0 {
		maxY = 0
	}
	x := wr.Left + p.roamDX
	y := wr.Top
	if y < 0 {
		y = 0
	} else if y > maxY {
		y = maxY
	}
	if x < 0 {
		x = 0
		p.roamDX = petRoamStep
		p.active = petActionRunningRight
		p.base = petActionRunningRight
	} else if x > maxX {
		x = maxX
		p.roamDX = -petRoamStep
		p.active = petActionRunningLeft
		p.base = petActionRunningLeft
	}
	procSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(x), uintptr(y), 0, 0, 0x0001|0x0010|0x0040)
	p.roamTicks--
	if p.roamTicks <= 0 {
		p.roamDX = 0
		restore := p.roamRestoreBase
		if restore == "" {
			restore = petActionIdle
		}
		p.roamRestoreBase = ""
		p.active = restore
		p.base = restore
		p.frameIndex = 0
	}
}

func (p *desktopPet) currentAction() string {
	if p.oneshot != "" && p.oneshtTicks > 0 {
		return p.oneshot
	}
	if p.active != "" {
		return p.active
	}
	return petActionIdle
}

func (p *desktopPet) applyAction(action string, ticks int) {
	requested := action
	if _, ok := p.framesByAction[action]; !ok {
		action = petActionIdle
	}
	loop := true
	for _, anim := range petAnimations {
		if anim.Name == action {
			loop = anim.Loop
			if ticks == 0 && !anim.Loop {
				ticks = anim.Frames * 3
			}
			break
		}
	}
	if !loop || ticks > 0 {
		p.oneshot = action
		p.oneshtTicks = ticks
	} else {
		p.base = action
		p.active = action
		p.oneshot = ""
		p.oneshtTicks = 0
	}
	p.idleTicks = 0
	p.frameIndex = 0
	if requested != action {
		log.Printf("desktop pet action %q not found; fallback to %q", requested, action)
	}
	log.Printf("desktop pet action=%s ticks=%d loop=%v", action, ticks, loop)
	if p.hwnd != 0 {
		p.updateActionFrame(p.currentAction(), 0)
	}
}

func (p *desktopPet) updateActionFrame(action string, frameIndex int) {
	frames := p.framesByAction[action]
	if len(frames) == 0 {
		frames = p.framesByAction[petActionIdle]
	}
	if len(frames) == 0 {
		return
	}
	p.updateFrameBytes(frames[frameIndex%len(frames)])
}

func (p *desktopPet) placeInitial() {
	sw, _, _ := procGetSystemMetrics.Call(0)
	sh, _, _ := procGetSystemMetrics.Call(1)
	x := int32(sw) - p.width - 72
	y := int32(sh) - p.height - 96
	if x < 0 {
		x = 40
	}
	if y < 0 {
		y = 40
	}
	procSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(x), uintptr(y), uintptr(p.width), uintptr(p.height), 0x0040)
}

func (p *desktopPet) updateFrame(index int) {
	if len(p.frames) == 0 {
		return
	}
	p.updateFrameBytes(p.frames[index%len(p.frames)])
}

func (p *desktopPet) updateFrameBytes(frame []byte) {
	if len(frame) == 0 || p.hwnd == 0 {
		return
	}
	memDC, _, _ := procCreateCompatibleDC.Call(0)
	if memDC == 0 {
		return
	}
	defer procDeleteDC.Call(memDC)

	bi := bitmapInfo{}
	bi.Header.Size = uint32(unsafe.Sizeof(bitmapInfoHeader{}))
	bi.Header.Width = p.width
	bi.Header.Height = -p.height
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = biRGB
	var bits uintptr
	hbmp, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == 0 {
		return
	}
	defer procDeleteObject.Call(hbmp)
	copy(unsafe.Slice((*byte)(unsafe.Pointer(bits)), len(frame)), frame)
	old, _, _ := procSelectObject.Call(memDC, hbmp)
	defer procSelectObject.Call(memDC, old)

	screenDC, _, _ := procGetDC.Call(0)
	if screenDC == 0 {
		return
	}
	defer procReleaseDC.Call(0, screenDC)
	var wr rect
	procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
	pos := point{X: wr.Left, Y: wr.Top}
	sz := size{CX: p.width, CY: p.height}
	src := point{}
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	updateOK, _, updateErr := procUpdateLayeredWindow.Call(
		uintptr(p.hwnd), screenDC,
		uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&sz)),
		memDC, uintptr(unsafe.Pointer(&src)), 0,
		uintptr(unsafe.Pointer(&blend)), ulwAlpha,
	)
	if updateOK == 0 {
		log.Printf("update desktop pet frame failed: %v", updateErr)
	}
}
