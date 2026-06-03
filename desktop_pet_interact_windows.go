//go:build windows

package main

import (
	"io/fs"
	"sort"
	"unsafe"
)

// abs32 returns the absolute value of a 32-bit integer.
func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// petActionMenuItems lists the actions exposed in the right-click "动作" submenu.
// Only entries whose frames are actually present in the current spritesheet are
// shown (filtered in showContextMenu).
var petActionMenuItems = []struct {
	action string
	label  string
}{
	{petActionWaving, "打招呼 👋"},
	{petActionJumping, "蹦一下 ⬆️"},
	{petActionRunning, "跑两步 🏃"},
	{petActionReview, "认真审阅 📝"},
	{petActionWaiting, "等一下 ⏳"},
	{petActionFailed, "装个死 💥"},
}

// petClickReactions are cycled through on each left-click on the pet. Each pick
// pairs a short speech-bubble line with a matching one-shot animation.
var petClickReactions = []struct {
	action string
	phrase string
}{
	{petActionWaving, "嗨，在忙什么呀？"},
	{petActionJumping, "戳我干嘛~ 嘿嘿"},
	{petActionReview, "要我帮你看看代码吗？"},
	{petActionWaiting, "我在这儿陪着你哦"},
	{petActionRunning, "冲鸭！今天也要加油！"},
	{petActionWaving, "需要开对话就右键我~"},
}

// petWindowRect returns the current screen rectangle of the pet window.
func (p *desktopPet) petWindowRect() rect {
	var wr rect
	if p.hwnd != 0 {
		procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&wr)))
	}
	return wr
}

// sayBubble shows a speech bubble above the pet and (re)starts its hold timer.
func (p *desktopPet) sayBubble(text string) {
	if p.bubble == nil || text == "" {
		return
	}
	p.bubble.showAt(text, p.petWindowRect())
	p.bubbleTicks = petBubbleHoldTicks
}

// onPetClick reacts to a plain left-click on the pet: play a friendly one-shot
// animation and pop a rotating speech-bubble line.
func (p *desktopPet) onPetClick() {
	if len(petClickReactions) == 0 {
		return
	}
	r := petClickReactions[p.clickStep%len(petClickReactions)]
	p.clickStep++
	if _, ok := p.framesByAction[r.action]; ok {
		p.applyAction(r.action, petDefaultActionTicks(r.action))
	}
	p.sayBubble(r.phrase)
}

// listEmbeddedPetIDs enumerates the available pet spritesheet folders so they
// can be offered in the "切换形象" submenu.
func listEmbeddedPetIDs() []string {
	entries, err := fs.ReadDir(gaAdminPetAssetsFS, "assets/ga-admin-pets")
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	sort.Strings(ids)
	return ids
}
