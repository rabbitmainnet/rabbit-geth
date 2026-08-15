package lqc

type FallbackWindow struct {
	SlotIndex uint16
	StartSec  uint64
	EndSec    uint64
}

func BuildFallbackWindows(fallbackSlots uint64, windowSec uint64) []FallbackWindow {
	if fallbackSlots == 0 || windowSec == 0 {
		return nil
	}
	out := make([]FallbackWindow, 0, fallbackSlots)
	for i := uint64(0); i < fallbackSlots; i++ {
		start := i * windowSec
		end := start + windowSec
		out = append(out, FallbackWindow{
			SlotIndex: uint16(i),
			StartSec:  start,
			EndSec:    end,
		})
	}
	return out
}
