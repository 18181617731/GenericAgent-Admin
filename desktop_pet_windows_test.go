//go:build windows

package main

import "testing"

func TestDesktopPetDragUsesDirectionalRunningActions(t *testing.T) {
	p := &desktopPet{
		visible:         true,
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
	if p.active != petActionRunningRight || p.base != petActionIdle || p.dragAction != petActionRunningRight {
		t.Fatalf("beginDrag active/base/dragAction=%q/%q/%q, want running-right/idle/running-right", p.active, p.base, p.dragAction)
	}
	if p.oneshot != "" || p.oneshtTicks != 0 {
		t.Fatalf("beginDrag should clear oneshot, got %q/%d", p.oneshot, p.oneshtTicks)
	}
	if p.roamTicks != 0 || p.roamDX != 0 || p.roamRestoreBase != "" {
		t.Fatalf("beginDrag should stop roam, got ticks=%d dx=%d restore=%q", p.roamTicks, p.roamDX, p.roamRestoreBase)
	}

	p.updateDrag(140)
	if p.active != petActionRunningRight || p.base != petActionIdle || p.oneshot != "" || p.oneshtTicks != 0 {
		t.Fatalf("right drag state active/base/oneshot/ticks=%q/%q/%q/%d, want running-right/idle/empty/0", p.active, p.base, p.oneshot, p.oneshtTicks)
	}
	p.onTimer()
	p.onTimer()
	p.onTimer()
	if p.active != petActionRunningRight || p.base != petActionIdle || p.dragAction != petActionRunningRight {
		t.Fatalf("right drag should keep running while held, got active/base/dragAction=%q/%q/%q", p.active, p.base, p.dragAction)
	}
	p.updateDrag(80)
	if p.active != petActionRunningLeft || p.base != petActionIdle || p.dragAction != petActionRunningLeft {
		t.Fatalf("left drag state active/base/dragAction=%q/%q/%q, want running-left/idle/running-left", p.active, p.base, p.dragAction)
	}

	p.finishDrag()
	if p.dragging {
		t.Fatalf("finishDrag left dragging=true")
	}
	if p.active != petActionIdle || p.base != petActionIdle {
		t.Fatalf("finishDrag restored active/base=%q/%q, want idle after temporary waiting", p.active, p.base)
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

func TestDesktopPetDragPreservesLongRunningBase(t *testing.T) {
	p := &desktopPet{
		visible: true,
		base:    petActionRunning,
		active:  petActionRunning,
		framesByAction: map[string][][]byte{
			petActionIdle:         {{0}},
			petActionRunning:      {{1}},
			petActionRunningLeft:  {{2}},
			petActionRunningRight: {{3}},
		},
	}

	p.beginDrag(50, point{})
	p.updateDrag(20)
	p.finishDrag()
	if p.active != petActionRunning || p.base != petActionRunning {
		t.Fatalf("long running base should survive drag, got active/base=%q/%q", p.active, p.base)
	}
}

func TestDesktopPetOneShotActionDefaultsToReadablePace(t *testing.T) {
	p := &desktopPet{
		visible: true,
		base:    petActionIdle,
		active:  petActionIdle,
		framesByAction: map[string][][]byte{
			petActionIdle:   {{1}},
			petActionWaving: {{2}, {3}, {4}, {5}},
		},
	}

	p.applyAction(petActionWaving, 0)
	wantTicks := petDefaultActionTicks(petActionWaving)
	if wantTicks != 14 {
		t.Fatalf("waving default ticks=%d, want 14", wantTicks)
	}
	if p.oneshot != petActionWaving || p.oneshtTicks != wantTicks {
		t.Fatalf("waving oneshot/ticks=%q/%d, want %q/%d", p.oneshot, p.oneshtTicks, petActionWaving, wantTicks)
	}
	for i := 0; i < wantTicks-1; i++ {
		p.onTimer()
	}
	if p.oneshot == "" || p.oneshtTicks != 1 || p.active != petActionIdle {
		t.Fatalf("waving should remain visible until final tick, got oneshot/ticks/active=%q/%d/%q", p.oneshot, p.oneshtTicks, p.active)
	}
	p.onTimer()
	if p.oneshot != "" || p.oneshtTicks != 0 || p.active != petActionIdle {
		t.Fatalf("waving should finish after readable one-shot duration, got oneshot/ticks/active=%q/%d/%q", p.oneshot, p.oneshtTicks, p.active)
	}
}

func TestDesktopPetIdleAutoActionResumesAfterDrag(t *testing.T) {
	p := &desktopPet{
		visible: true,
		base:    petActionIdle,
		active:  petActionIdle,
		framesByAction: map[string][][]byte{
			petActionIdle:         {{1}},
			petActionRunningRight: {{2}},
			petActionRunningLeft:  {{3}},
			petActionWaving:       {{4}, {5}, {6}, {7}},
		},
	}

	p.beginDrag(100, point{})
	p.updateDrag(60)
	p.finishDrag()
	if p.active != petActionIdle || p.base != petActionIdle {
		t.Fatalf("finishDrag restored active/base=%q/%q, want idle/idle", p.active, p.base)
	}

	for i := 0; i < petIdleDelayTicks; i++ {
		p.onTimer()
	}
	if p.active == petActionIdle && p.base == petActionIdle && p.oneshot == "" && p.roamTicks == 0 {
		t.Fatalf("idle auto action did not resume after %d ticks", petIdleDelayTicks)
	}
}
