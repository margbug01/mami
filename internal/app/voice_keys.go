package app

import "fmt"

const (
	voiceNone = iota
	voiceStart
	voiceStop
	voiceCancel
)

// voiceKeyNames 能单按当语音触发键的键：左右修饰键、几个锁定键、F1–F24，
// 还有媒体键和浏览器键（蓝牙遥控器、翻页笔、带媒体键的小键盘按下去多半是这些）。
// 组合键（带 +）不在这里，走 GlobalShortcut 注册。
var voiceKeyNames = map[string]uint32{
	"LShift": 0xA0, "RShift": 0xA1, "LCtrl": 0xA2, "RCtrl": 0xA3, "LAlt": 0xA4, "RAlt": 0xA5,
	"VolumeMute": 0xAD, "VolumeDown": 0xAE, "VolumeUp": 0xAF,
	"MediaNext": 0xB0, "MediaPrev": 0xB1, "MediaStop": 0xB2, "MediaPlayPause": 0xB3,
	"BrowserBack": 0xA6, "BrowserHome": 0xAC,
	"CapsLock": 0x14, "ScrollLock": 0x91, "Pause": 0x13,
}

func init() {
	for i := 1; i <= 24; i++ {
		voiceKeyNames[fmt.Sprintf("F%d", i)] = uint32(0x70 + i - 1)
	}
}

func voiceKey(name string) uint32 { return voiceKeyNames[name] }

// voiceKeyIsModifier 修饰键当触发键时不吞（吞了 Shift/Ctrl 的状态会乱），其余单键吞掉，免得 CapsLock 跟着切大小写
func voiceKeyIsModifier(vk uint32) bool { return vk >= 0xA0 && vk <= 0xA5 }

// 只由钩子线程驱动。单击触发键开始录音，再单击结束；按着触发键时按了别的键算组合键：
// 第一下的组合键取消这次录音，结束那下的组合键忽略（继续录）。
type voiceKeys struct {
	held      uint32 // 我们接管的这次触发键按下，松开前非 0
	recording bool   // 第一下已经松开，正在录音，等结束那下
	combo     bool
	escape    bool
	pressed   [256]bool
}

// heal 把记成「按着」、实际早已松开的键纠正回来。低级钩子收不到锁屏、安全桌面、管理员窗口里的按键，
// 漏掉一次松开，之后每次按触发键都会被当成组合键（或当成触发键的连发）而不启动。
// isDown 在钩子里是这次事件之前的真实状态（GetAsyncKeyState 在钩子返回后才更新）。
func (s *voiceKeys) heal(isDown func(vk uint32) bool) (healed []uint32) {
	for vk, pressed := range s.pressed {
		if pressed && !isDown(uint32(vk)) {
			s.pressed[vk] = false
			healed = append(healed, uint32(vk))
			if uint32(vk) == s.held { // 触发键自己的松开丢了：当作这一下按成组合键作废，不动正在进行的录音
				s.held, s.combo = 0, false
			}
		}
	}
	return healed
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

// 识别完的文字怎么交出去
const (
	voicePaste     = iota // 贴进输入框，之后还原剪贴板
	voicePasteKeep        // 认不出是不是输入框：照样贴，文字留在剪贴板里
	voiceCopyCard         // 没地方贴：复制，弹卡片显示整段文字
)

// voiceOutput switched=录音期间换了前台窗口，modsDown=修饰键一直没松（发 Ctrl+V 会变成别的组合），
// input=焦点确定是输入框（UIA 可写文本或终端），caret=有系统输入光标（UIA 说不清的老程序、Qt）
func voiceOutput(switched, modsDown, input, caret bool) int {
	switch {
	case switched || modsDown:
		return voiceCopyCard
	case input:
		return voicePaste
	case caret:
		return voicePasteKeep
	}
	return voiceCopyCard
}
