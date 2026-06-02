//go:build windows

package main

import "testing"

func TestDesktopPetDragUsesDirectionalRunningActions(t *testing.T) {
	p := &desktopPet{
		base:            petActionWaiting,
		active:          petActionWaiting,
		oneshot:         petActionWaving,
		oneshtTicks:     3,
		roamTicks:       4,
		roamDX:          petRoamStep,
		roamRestoreBase: petActionIdle,
		framesByAction: map[string][][]byte{
			petActionIdle:         {{1}},
			petActionWaiting:      {{2}},
			petActionRunningRight: {{3}},
			petActionRunningLeft:  {{4}},
		},
	}

	p.beginDrag(100, point{X: 10, Y: 20})
	if !p.dragging {
		t.Fatalf("beginDrag did not mark dragging")
	}
	if p.active != petActionRunningRight {
		t.Fatalf("beginDrag active=%q, want %q", p.active, petActionRunningRight)
	}
	if p.base != petActionWaiting {
		t.Fatalf("beginDrag changed base to %q", p.base)
	}
	if p.oneshot != "" || p.oneshtTicks != 0 {
		t.Fatalf("beginDrag should clear oneshot, got %q/%d", p.oneshot, p.oneshtTicks)
	}
	if p.roamTicks != 0 || p.roamDX != 0 || p.roamRestoreBase != "" {
		t.Fatalf("beginDrag should stop roam, got ticks=%d dx=%d restore=%q", p.roamTicks, p.roamDX, p.roamRestoreBase)
	}

	p.updateDrag(140)
	if p.active != petActionRunningRight {
		t.Fatalf("right drag active=%q, want %q", p.active, petActionRunningRight)
	}
	p.updateDrag(80)
	if p.active != petActionRunningLeft {
		t.Fatalf("left drag active=%q, want %q", p.active, petActionRunningLeft)
	}

	p.finishDrag()
	if p.dragging {
		t.Fatalf("finishDrag left dragging=true")
	}
	if p.active != petActionWaiting || p.base != petActionWaiting {
		t.Fatalf("finishDrag restored active/base=%q/%q, want %q", p.active, p.base, petActionWaiting)
	}
}

func TestDesktopPetDragRestoresIdleFromRunningBase(t *testing.T) {
	p := &desktopPet{
		base:   petActionRunningLeft,
		active: petActionRunningLeft,
		framesByAction: map[string][][]byte{
			petActionIdle:         {{1}},
			petActionRunningRight: {{2}},
			petActionRunningLeft:  {{3}},
		},
	}
	p.beginDrag(50, point{})
	p.updateDrag(20)
	p.finishDrag()
	if p.active != petActionIdle || p.base != petActionIdle {
		t.Fatalf("running base should restore to idle, got active/base=%q/%q", p.active, p.base)
	}
}
