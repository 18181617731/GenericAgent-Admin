//go:build windows

package main

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	petClassName = "GAAdminDesktopPetWindow"
	petTitle     = "GA Admin Pet"

	petFrameW = 192
	petFrameH = 208

	petRoamStep       int32 = 6
	petRoamMinTicks         = 24
	petRoamMaxTicks         = 44
	petIdleDelayTicks       = 32

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
	petJumpHeight    = int32(52)

	petMenuOpenChat = 1091
	petMenuHide     = 1101
	petMenuSayHello = 1111

	// Dynamic submenu command IDs are allocated by adding the item index to
	// these bases, so keep the ranges far enough apart to never overlap.
	petMenuActionBase = 1200
	petMenuPetBase    = 1300

	mfPopup = 0x00000010

	// petBubbleHoldTicks controls how long a speech bubble stays on screen
	// (timer fires every petFrameInterval ms).
	petBubbleHoldTicks = 36
	// petClickDragThreshold is the pixel distance below which a press+release
	// counts as a click rather than a drag.
	petClickDragThreshold = 4
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

	wmDestroy      = 0x0002
	wmClose        = 0x0010
	wmShowPet      = 0x0400 + 71
	wmHidePet      = 0x0400 + 72
	wmTogglePet    = 0x0400 + 73
	wmReloadPet    = 0x0400 + 74
	wmTimer        = 0x0113
	wmLButtonDown  = 0x0201
	wmLButtonUp    = 0x0202
	wmMouseMove    = 0x0200
	wmRButtonUp    = 0x0205
	wmNCHitTest    = 0x0084
	wmCommand      = 0x0111
	mfString       = 0x00000000
	mfSeparator    = 0x00000800
	tpmRightButton = 0x0002
	tpmReturnCmd   = 0x0100
	htClient       = 1
	htCaption      = 2

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

func petAnimationByName(action string) (petAnimation, bool) {
	for _, anim := range petAnimations {
		if anim.Name == action {
			return anim, true
		}
	}
	return petAnimation{}, false
}

func petFrameHoldTicks(action string) int {
	switch action {
	case petActionWaving, petActionJumping:
		return 3
	case petActionFailed:
		return 2
	default:
		return 1
	}
}

func petDefaultActionTicks(action string) int {
	anim, ok := petAnimationByName(action)
	if !ok {
		return 0
	}
	ticks := anim.Frames * petFrameHoldTicks(action)
	if !anim.Loop {
		ticks += 2 // Hold the last pose briefly so short one-shot actions do not flicker away.
	}
	return ticks
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
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procAppendMenu          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu      = user32.NewProc("TrackPopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")

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

	openChat func()

	mu              sync.Mutex
	requestedPNG    []byte
	visible         bool
	frameIndex      int
	active          string
	base            string
	oneshot         string
	oneshtTicks     int
	lastDragX       int32
	dragging        bool
	dragOffset      point
	dragRestoreBase string
	dragAction      string
	framesByAction  map[string][][]byte
	idleTicks       int
	idleNudge       int
	autoActionStep  int
	roamTicks       int
	roamDX          int32
	roamRestoreBase string
	jumpTicks       int
	jumpTotalTicks  int
	jumpBaseY       int32
	jumpOffsetY     int32

	bubble      *petBubble
	bubbleTicks int
	dragStart   point
	dragMoved   bool
	clickStep   int
	menuActions []string
	menuPetIDs  []string
}

var petInstance *desktopPet

func switchDesktopPet(petID string) error {
	petID = strings.TrimSpace(petID)
	if petID == "" {
		return nil
	}
	if strings.Contains(petID, "..") || strings.ContainsAny(petID, `/\\`) {
		return errors.New("invalid pet id")
	}
	data, err := loadDesktopPetSpritesheetPNG(petID)
	if err != nil {
		return err
	}
	if petInstance == nil {
		gaAdminPetSpritesheetPNG = data
		return nil
	}
	return petInstance.reloadSpritesheet(data)
}

func loadDesktopPetSpritesheetPNG(petID string) ([]byte, error) {
	rel := filepath.Join("assets", "ga-admin-pets", petID, "spritesheet.png")
	if data, err := os.ReadFile(rel); err == nil {
		return data, nil
	}
	return fs.ReadFile(gaAdminPetAssetsFS, filepath.ToSlash(rel))
}

func startDesktopPet(openChat func()) func() {
	pet, err := newDesktopPet(openChat)
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

func (p *desktopPet) reloadSpritesheet(data []byte) error {
	if _, err := decodePetSpritesheet(data); err != nil {
		return err
	}
	p.mu.Lock()
	p.requestedPNG = append(p.requestedPNG[:0], data...)
	p.mu.Unlock()
	postPetMessage(wmReloadPet)
	return nil
}

func decodePetSpritesheet(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

func (p *desktopPet) applyRequestedSpritesheet() {
	p.mu.Lock()
	if len(p.requestedPNG) == 0 {
		p.mu.Unlock()
		return
	}
	data := append([]byte(nil), p.requestedPNG...)
	p.requestedPNG = nil
	p.mu.Unlock()
	img, err := decodePetSpritesheet(data)
	if err != nil {
		log.Printf("desktop pet reload failed: %v", err)
		return
	}
	framesByAction := map[string][][]byte{}
	framesAll := [][]byte{}
	for _, anim := range petAnimations {
		frames := extractPetFrames(img, anim.Row, anim.Frames)
		framesByAction[anim.Name] = frames
		framesAll = append(framesAll, frames...)
	}
	p.framesByAction = framesByAction
	p.frames = framesAll
	p.frameIndex = 0
	p.active = petActionIdle
	p.base = petActionIdle
	p.oneshot = ""
	p.oneshtTicks = 0
	p.updateActionFrame(p.active, 0)
}

func newDesktopPet(openChat func()) (*desktopPet, error) {
	img, err := decodePetSpritesheet(gaAdminPetSpritesheetPNG)
	if err != nil {
		return nil, err
	}
	pet := &desktopPet{
		width:          petFrameW,
		height:         petFrameH,
		visible:        true,
		active:         petActionIdle,
		base:           petActionIdle,
		openChat:       openChat,
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
	if b := newPetBubble(); b != nil {
		if err := b.createWindow(); err != nil {
			log.Printf("create desktop pet bubble window failed: %v", err)
		} else {
			p.bubble = b
		}
	}
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
	cursor, _, _ := procLoadCursor.Call(0, 32512) // IDC_ARROW
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
	case wmReloadPet:
		p.applyRequestedSpritesheet()
		return 0
	case wmLButtonDown:
		var cur point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
		var wr rect
		procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		p.dragStart = cur
		p.dragMoved = false
		p.beginDrag(cur.X, point{X: cur.X - wr.Left, Y: cur.Y - wr.Top})
		procSetCapture.Call(uintptr(hwnd))
		return 0
	case wmMouseMove:
		if p.dragging {
			var cur point
			procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
			if abs32(cur.X-p.dragStart.X) > petClickDragThreshold || abs32(cur.Y-p.dragStart.Y) > petClickDragThreshold {
				p.dragMoved = true
			}
			p.updateDrag(cur.X)
			procSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), uintptr(cur.X-p.dragOffset.X), uintptr(cur.Y-p.dragOffset.Y), 0, 0, 0x0001|0x0010|0x0040)
			if p.bubble != nil && p.bubbleTicks > 0 {
				p.bubble.reposition(p.petWindowRect())
			}
		}
		return 0
	case wmLButtonUp:
		moved := p.dragMoved
		p.finishDrag()
		procReleaseCapture.Call()
		if !moved {
			p.onPetClick()
		}
		return 0
	case wmRButtonUp:
		p.showContextMenu(hwnd)
		return 0
	case wmCommand:
		p.handleMenuCommand(uint16(wparam & 0xffff))
		return 0
	case wmNCHitTest:
		return htClient
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

func (p *desktopPet) showContextMenu(hwnd syscall.Handle) {
	menu, _, err := procCreatePopupMenu.Call()
	if menu == 0 {
		log.Printf("create desktop pet menu failed: %v", err)
		return
	}
	defer procDestroyMenu.Call(menu)

	p.appendMenuItem(menu, petMenuOpenChat, "打开对话 (/chat)")
	p.appendMenuItem(menu, petMenuSayHello, "说句话 💬")

	// Action submenu: trigger fun one-shot animations.
	if actionMenu, _, _ := procCreatePopupMenu.Call(); actionMenu != 0 {
		p.menuActions = p.menuActions[:0]
		for _, it := range petActionMenuItems {
			if _, ok := p.framesByAction[it.action]; !ok {
				continue
			}
			id := uint16(petMenuActionBase + len(p.menuActions))
			p.appendMenuItem(actionMenu, id, it.label)
			p.menuActions = append(p.menuActions, it.action)
		}
		if len(p.menuActions) > 0 {
			p.appendSubMenu(menu, actionMenu, "动作 🎭")
		} else {
			procDestroyMenu.Call(actionMenu)
		}
	}

	// Pet switching submenu: enumerate embedded spritesheets.
	if ids := listEmbeddedPetIDs(); len(ids) > 1 {
		if petsMenu, _, _ := procCreatePopupMenu.Call(); petsMenu != 0 {
			p.menuPetIDs = ids
			for i, id := range ids {
				p.appendMenuItem(petsMenu, uint16(petMenuPetBase+i), id)
			}
			p.appendSubMenu(menu, petsMenu, "切换形象 🐾")
		}
	}

	p.appendSeparator(menu)
	p.appendMenuItem(menu, petMenuHide, "隐藏桌宠")

	var cur point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&cur)))
	procSetForegroundWindow.Call(uintptr(hwnd))
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton|tpmReturnCmd, uintptr(cur.X), uintptr(cur.Y), 0, uintptr(hwnd), 0)
	if cmd != 0 {
		p.handleMenuCommand(uint16(cmd))
	}
}

func (p *desktopPet) appendSubMenu(menu, sub uintptr, text string) {
	label, _ := syscall.UTF16PtrFromString(text)
	procAppendMenu.Call(menu, mfPopup, sub, uintptr(unsafe.Pointer(label)))
}

func (p *desktopPet) appendSeparator(menu uintptr) {
	procAppendMenu.Call(menu, mfSeparator, 0, 0)
}

func (p *desktopPet) appendMenuItem(menu uintptr, id uint16, text string) {
	label, _ := syscall.UTF16PtrFromString(text)
	procAppendMenu.Call(menu, mfString, uintptr(id), uintptr(unsafe.Pointer(label)))
}

func (p *desktopPet) handleMenuCommand(id uint16) {
	switch {
	case id == petMenuOpenChat:
		if p.openChat != nil {
			go p.openChat()
		}
	case id == petMenuSayHello:
		p.onPetClick()
	case id == petMenuHide:
		p.visible = false
		if p.bubble != nil {
			p.bubbleTicks = 0
			p.bubble.hide()
		}
		procShowWindow.Call(uintptr(p.hwnd), swHide)
	case id >= petMenuActionBase && int(id)-petMenuActionBase < len(p.menuActions):
		action := p.menuActions[id-petMenuActionBase]
		p.applyAction(action, petDefaultActionTicks(action))
	case id >= petMenuPetBase && int(id)-petMenuPetBase < len(p.menuPetIDs):
		target := p.menuPetIDs[id-petMenuPetBase]
		go func() {
			if err := switchDesktopPet(target); err != nil {
				log.Printf("switch desktop pet to %q failed: %v", target, err)
			}
		}()
	}
}

func (p *desktopPet) onTimer() {
	if !p.visible {
		if p.bubble != nil && p.bubbleTicks > 0 {
			p.bubbleTicks = 0
			p.bubble.hide()
		}
		return
	}
	if p.bubble != nil && p.bubbleTicks > 0 {
		p.bubbleTicks--
		if p.bubbleTicks <= 0 {
			p.bubble.hide()
		} else {
			p.bubble.reposition(p.petWindowRect())
		}
	}
	if p.jumpTicks > 0 && !p.dragging {
		p.stepJump()
	}
	if !p.dragging && p.roamTicks > 0 {
		p.stepRoam()
	} else if p.dragging {
		p.idleTicks = 0
	} else if p.oneshot == "" && p.base == petActionIdle {
		p.idleTicks++
		if p.idleTicks >= petIdleDelayTicks {
			p.idleTicks = 0
			p.playNextIdleAction()
		}
	} else {
		p.idleTicks = 0
	}
	action := p.currentAction()
	holdTicks := petFrameHoldTicks(action)
	p.updateActionFrame(action, p.frameIndex/holdTicks)
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
	p.stopJump()
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

func (p *desktopPet) stopJump() {
	if p.hwnd != 0 && p.jumpTicks > 0 && p.jumpOffsetY != 0 {
		var wr rect
		procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
		procSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(wr.Left), uintptr(p.jumpBaseY), 0, 0, 0x0001|0x0010|0x0040)
	}
	p.jumpTicks = 0
	p.jumpTotalTicks = 0
	p.jumpBaseY = 0
	p.jumpOffsetY = 0
}

func (p *desktopPet) startJump(ticks int) {
	if p.hwnd == 0 || p.dragging {
		return
	}
	if ticks <= 0 {
		ticks = 15
	}
	if p.jumpTicks <= 0 || p.jumpOffsetY == 0 {
		var wr rect
		procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
		p.jumpBaseY = wr.Top
	} else {
		p.stopJump()
		var wr rect
		procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
		p.jumpBaseY = wr.Top
	}
	p.jumpTotalTicks = ticks
	p.jumpTicks = ticks
	p.jumpOffsetY = 0
}

func (p *desktopPet) stepJump() {
	if p.hwnd == 0 || p.jumpTicks <= 0 {
		return
	}
	if p.jumpTotalTicks <= 0 {
		p.jumpTotalTicks = p.jumpTicks
	}
	elapsed := p.jumpTotalTicks - p.jumpTicks + 1
	if elapsed < 0 {
		elapsed = 0
	} else if elapsed > p.jumpTotalTicks {
		elapsed = p.jumpTotalTicks
	}
	t := float64(elapsed) / float64(p.jumpTotalTicks)
	offset := -int32(float64(petJumpHeight) * 4 * t * (1 - t))
	var wr rect
	procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
	x := wr.Left
	y := p.jumpBaseY + offset
	if y < 0 {
		y = 0
	}
	procSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(x), uintptr(y), 0, 0, 0x0001|0x0010|0x0040)
	p.jumpOffsetY = offset
	p.jumpTicks--
	if p.jumpTicks <= 0 {
		procSetWindowPos.Call(uintptr(p.hwnd), ^uintptr(0), uintptr(x), uintptr(p.jumpBaseY), 0, 0, 0x0001|0x0010|0x0040)
		p.jumpTotalTicks = 0
		p.jumpBaseY = 0
		p.jumpOffsetY = 0
	}
}

func (p *desktopPet) beginDrag(cursorX int32, offset point) {
	p.stopJump()
	p.dragging = true
	p.dragOffset = offset
	p.lastDragX = cursorX
	p.dragRestoreBase = restoredDragBase(p.base)
	p.roamTicks = 0
	p.roamDX = 0
	p.roamRestoreBase = ""
	p.oneshot = ""
	p.oneshtTicks = 0
	p.base = p.dragRestoreBase
	p.dragAction = petActionRunningRight
	p.setDragAction(p.dragAction)
}

func (p *desktopPet) updateDrag(cursorX int32) {
	action := p.dragAction
	if action == "" {
		action = petActionRunningRight
	}
	if cursorX > p.lastDragX+1 {
		action = petActionRunningRight
	} else if cursorX < p.lastDragX-1 {
		action = petActionRunningLeft
	}
	p.lastDragX = cursorX
	p.dragAction = action
	p.setDragAction(action)
}

func restoredDragBase(action string) string {
	switch action {
	case petActionRunning:
		return petActionRunning
	default:
		return petActionIdle
	}
}

func (p *desktopPet) finishDrag() {
	p.dragging = false
	restore := p.dragRestoreBase
	if restore == "" {
		restore = petActionIdle
	}
	p.dragRestoreBase = ""
	p.dragAction = ""
	p.active = restore
	p.base = restore
	p.oneshot = ""
	p.oneshtTicks = 0
	p.frameIndex = 0
	p.updateActionFrame(p.active, 0)
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

func (p *desktopPet) setDragAction(action string) {
	if _, ok := p.framesByAction[action]; !ok {
		action = petActionIdle
	}
	if p.active == action && p.oneshot == "" {
		return
	}
	p.active = action
	p.oneshot = ""
	p.oneshtTicks = 0
	p.frameIndex = 0
	p.updateActionFrame(action, 0)
}

func (p *desktopPet) setDragActionForTicks(action string, ticks int) {
	if _, ok := p.framesByAction[action]; !ok {
		action = petActionIdle
	}
	if ticks <= 0 {
		ticks = 3
	}
	p.base = p.dragRestoreBase
	if p.base == "" {
		p.base = petActionIdle
	}
	p.active = p.base
	if p.oneshot == action {
		if p.oneshtTicks < ticks {
			p.oneshtTicks = ticks
		}
		return
	}
	p.oneshot = action
	p.oneshtTicks = ticks
	p.frameIndex = 0
	p.updateActionFrame(action, 0)
}

func (p *desktopPet) applyAction(action string, ticks int) {
	requested := action
	if _, ok := p.framesByAction[action]; !ok {
		action = petActionIdle
	}
	anim, ok := petAnimationByName(action)
	loop := true
	if ok {
		loop = anim.Loop
		if ticks == 0 && !anim.Loop {
			ticks = petDefaultActionTicks(action)
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
	if action == petActionJumping && ticks > 0 {
		p.startJump(ticks)
	} else if action != petActionJumping {
		p.stopJump()
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
	var bits unsafe.Pointer
	hbmp, _, _ := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		return
	}
	defer procDeleteObject.Call(hbmp)
	copy(unsafe.Slice((*byte)(bits), len(frame)), frame)
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
