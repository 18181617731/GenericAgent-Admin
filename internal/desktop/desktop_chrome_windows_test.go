//go:build windows

package desktop

import (
	"encoding/binary"
	"testing"

	"genericagent-admin-go/internal/appicon"
)

// icoWith builds a directory for frames of the given sides, with payloads long
// enough to tell the frames apart by length.
func icoWith(sides ...int) []byte {
	const header, entry = 6, 16
	out := make([]byte, header+entry*len(sides))
	binary.LittleEndian.PutUint16(out[2:], 1)
	binary.LittleEndian.PutUint16(out[4:], uint16(len(sides)))
	for i, side := range sides {
		at := header + i*entry
		out[at] = byte(side)
		out[at+1] = byte(side)
		binary.LittleEndian.PutUint32(out[at+8:], uint32(side))
		binary.LittleEndian.PutUint32(out[at+12:], uint32(len(out)))
		out = append(out, make([]byte, side)...)
	}
	return out
}

func TestBestICOFrameTakesTheExactSizeWhenThereIsOne(t *testing.T) {
	ico := icoWith(16, 20, 24, 32, 40, 48)
	for _, want := range []int{16, 20, 24, 32, 40, 48} {
		if _, length := bestICOFrame(ico, want, want); length != want {
			t.Errorf("asked for %d, got the %d frame", want, length)
		}
	}
}

func TestBestICOFrameShrinksRatherThanStretches(t *testing.T) {
	ico := icoWith(16, 32, 64)
	for _, tc := range []struct{ want, frame int }{
		{20, 32}, {40, 64}, {64, 64},
		{96, 64}, // Nothing larger left to shrink, so the largest frame it is.
	} {
		if _, length := bestICOFrame(ico, tc.want, tc.want); length != tc.frame {
			t.Errorf("asked for %d, got the %d frame, want the %d one", tc.want, length, tc.frame)
		}
	}
}

func TestBestICOFrameReportsNothingForUnusableData(t *testing.T) {
	full := icoWith(16, 32)
	cases := map[string][]byte{
		"empty":     nil,
		"truncated": full[:10],
		"no frames": icoWith(),
	}
	for name, ico := range cases {
		if _, length := bestICOFrame(ico, 32, 32); length != 0 {
			t.Errorf("%s: got a frame of %d bytes", name, length)
		}
	}
	if _, length := bestICOFrame(full, 0, 0); length != 0 {
		t.Error("a zero size asked for a frame")
	}
}

func TestBestICOFrameSkipsFramesThatRunPastTheFile(t *testing.T) {
	ico := icoWith(16, 32)
	// Claim the 32px frame is longer than the file so only the 16px one is left.
	binary.LittleEndian.PutUint32(ico[6+16+8:], uint32(len(ico)))
	offset, length := bestICOFrame(ico, 32, 32)
	if length != 16 || offset+length > len(ico) {
		t.Errorf("got frame at %d of %d bytes, want the 16px one", offset, length)
	}
}

// Windows asks the caption, the taskbar and Alt-Tab for 16 and 32 scaled by
// the display, so every scaling from 100% to 300% needs its own frame; any it
// has to reach by stretching is the soft icon this table exists to prevent.
func TestShippedIconHasAFrameForEveryScaledWindowSize(t *testing.T) {
	sides := map[int]int{}
	for i := 0; i < int(binary.LittleEndian.Uint16(appicon.ICO[4:])); i++ {
		at := 6 + i*16
		sides[int(binary.LittleEndian.Uint32(appicon.ICO[at+12:]))] = int(appicon.ICO[at])
	}
	for _, want := range []int{16, 20, 24, 28, 32, 36, 40, 48, 56, 64, 72, 80, 96} {
		offset, length := bestICOFrame(appicon.ICO, want, want)
		if length == 0 {
			t.Fatalf("%dpx: no usable frame in internal/appicon/assets/tray_windows.ico", want)
		}
		if side := sides[offset]; side != want {
			t.Errorf("%dpx: would be drawn from the %dpx frame", want, side)
		}
	}
}

// The 256px frame every icon file ends with is recorded as a zero side.
func TestBestICOFrameReadsZeroAsTheLargestSide(t *testing.T) {
	ico := icoWith(32, 200)
	ico[6+16], ico[6+16+1] = 0, 0
	if _, length := bestICOFrame(ico, 128, 128); length != 200 {
		t.Errorf("got the %d frame, want the 256px one", length)
	}
}
