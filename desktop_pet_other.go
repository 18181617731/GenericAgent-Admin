//go:build !windows

package main

const (
	petActionIdle         = "idle"
	petActionRunningRight = "running-right"
	petActionRunningLeft  = "running-left"
	petActionWaving       = "waving"
	petActionJumping      = "jumping"
	petActionFailed       = "failed"
	petActionWaiting      = "waiting"
	petActionRunning      = "running"
	petActionReview       = "review"
)

func startDesktopPet() func()                              { return func() {} }
func showDesktopPet()                                      {}
func hideDesktopPet()                                      {}
func toggleDesktopPet()                                    {}
func setDesktopPetAction(action string)                    {}
func setDesktopPetActionForTicks(action string, ticks int) {}
