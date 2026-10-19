package app

const (
	voiceNone = iota
	voiceStart
	voiceStop
	voiceCancel
)

func voiceKey(name string) uint32 {
	switch name {
	case "RAlt":
		return 0xA5
	case "RCtrl":
		return 0xA3
	case "RShift":
		return 0xA1
	}
	return 0
}

// 只由钩子线程驱动。单击触发键开始录音，再单击结束；按着触发键时按了别的键算组合键：
// 第一下的组合键取消这次录音，结束那下的组合键忽略（继续录）。
type voiceKeys struct {
	held      uint32 // 我们接管的这次触发键按下，松开前非 0
	recording bool   // 第一下已经松开，正在录音，等结束那下
	combo     bool
	escape    bool
	pressed   [256]bool
}

func (s *voiceKeys) othersDown(trigger uint32) bool {
	for key, pressed := range s.pressed {
		if pressed && uint32(key) != trigger {
			return true
		}
	}
	return false
}

func (s *voiceKeys) step(vk uint32, down bool, trigger uint32, busy bool) (action int, swallow bool) {
	repeat := vk < 256 && s.pressed[vk]
	if vk < 256 {
		s.pressed[vk] = down
	}
	if vk == 0x1B {
		if down && (busy || s.recording || (s.held != 0 && !s.combo)) {
			s.escape, s.recording, s.combo = true, false, true
			return voiceCancel, true
		}
		if s.escape {
			if !down {
				s.escape = false
			}
			return voiceNone, true
		}
	}
	if s.held != 0 {
		if vk == s.held {
			if down {
				return voiceNone, false
			}
			s.held = 0
			switch {
			case s.recording && !s.combo:
				s.recording = false
				return voiceStop, false
			case s.recording:
				return voiceNone, false
			case !s.combo:
				s.recording = true
				return voiceNone, false
			}
			return voiceNone, false
		}
		if down && !s.combo {
			s.combo = true
			if !s.recording {
				return voiceCancel, false
			}
		}
		return voiceNone, false
	}
	if !down || trigger == 0 || vk != trigger || repeat {
		return voiceNone, false
	}
	if s.recording {
		s.held, s.combo = trigger, s.othersDown(trigger)
		return voiceNone, false
	}
	if busy || s.othersDown(trigger) {
		return voiceNone, false
	}
	s.held, s.combo = trigger, false
	return voiceStart, false
}
