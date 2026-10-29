//go:build darwin

package app

import (
	"log"
	"strings"
	"sync"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego/objc"
)

// 辅助功能（AX）：问焦点是不是输入框、选了什么、选区在哪、光标前后是什么字。
// AX 是跨进程的同步调用，哪个线程都能调；对方卡住时最多等 axTimeout，所以不用像 Windows 那样单开线程。

const axTimeout = 0.25 // 秒

const (
	axValueCGRect  = 3
	axValueCFRange = 4
)

var (
	axSystem      uintptr // AXUIElementCreateSystemWide，常驻
	axAttrs       sync.Map
	axIsSettable  func(el, attr uintptr, out *bool) int32
	axLastAppPID  int32
	axLastAppLock sync.Mutex
)

// axName 属性名的 CFString，建一次常驻
func axName(name string) uintptr {
	if v, ok := axAttrs.Load(name); ok {
		return v.(uintptr)
	}
	s := cfConst(name)
	axAttrs.Store(name, s)
	return s
}

// axCopy 读一个属性（+1 引用，调用方 cfRelease）；读不到返回 0
func axCopy(el uintptr, attr string) uintptr {
	var v uintptr
	if el == 0 || axCopyAttribute(el, axName(attr), &v) != 0 {
		return 0
	}
	return v
}

func axString(el uintptr, attr string) string {
	v := axCopy(el, attr)
	if v == 0 {
		return ""
	}
	defer cfRelease(v)
	return goString(objc.ID(v))
}

func axRange(el uintptr, attr string) (cfRange, bool) {
	var r cfRange
	v := axCopy(el, attr)
	if v == 0 {
		return r, false
	}
	defer cfRelease(v)
	return r, axValueGetValue(v, axValueCFRange, unsafe.Pointer(&r))
}

func axInt(el uintptr, attr string) (int, bool) {
	v := axCopy(el, attr)
	if v == 0 {
		return 0, false
	}
	defer cfRelease(v)
	return int(send(objc.ID(v), "integerValue")), true
}

// axBounds 一段字在屏幕上的框（AXBoundsForRange，屏幕坐标）
func axBounds(el uintptr, r cfRange) (RECT, bool) {
	param := axValueCreate(axValueCFRange, unsafe.Pointer(&r))
	if param == 0 {
		return RECT{}, false
	}
	defer cfRelease(param)
	var v uintptr
	if axCopyParamAttribute(el, axName("AXBoundsForRange"), param, &v) != 0 || v == 0 {
		return RECT{}, false
	}
	defer cfRelease(v)
	var rc cgRect
	if !axValueGetValue(v, axValueCGRect, unsafe.Pointer(&rc)) || rc.Size.W <= 0 && rc.Size.H <= 0 {
		return RECT{}, false
	}
	return RECT{round32(rc.Origin.X), round32(rc.Origin.Y), round32(rc.Origin.X + rc.Size.W), round32(rc.Origin.Y + rc.Size.H)}, true
}

// axFrame 元素自己的框（AXPosition + AXSize）
func axFrame(el uintptr) (RECT, bool) {
	var p cgPoint
	var s cgSize
	pv, sv := axCopy(el, "AXPosition"), axCopy(el, "AXSize")
	if pv != 0 {
		defer cfRelease(pv)
	}
	if sv != 0 {
		defer cfRelease(sv)
	}
	if pv == 0 || sv == 0 || !axValueGetValue(pv, 1, unsafe.Pointer(&p)) || !axValueGetValue(sv, 2, unsafe.Pointer(&s)) {
		return RECT{}, false
	}
	return RECT{round32(p.X), round32(p.Y), round32(p.X + s.W), round32(p.Y + s.H)}, true
}

func startUIA() error {
	axSystem = axCreateSystemWide()
	// 设在系统级元素上的超时对所有元素生效
	axSetMessagingTimeout(axSystem, axTimeout)
	return nil
}

// axFocused 当前焦点元素（+1 引用）。Electron/Chromium 程序默认不建无障碍树，
// 换到新的前台程序时先给它设一下 AXManualAccessibility
func axFocused() uintptr {
	if axSystem == 0 {
		return 0
	}
	if pid := frontmostPID(); pid > 0 {
		axLastAppLock.Lock()
		if pid != axLastAppPID {
			axLastAppPID = pid
			if app := axCreateApplication(pid); app != 0 {
				axSetAttribute(app, axName("AXManualAccessibility"), kCFBooleanTrue)
				cfRelease(app)
			}
		}
		axLastAppLock.Unlock()
	}
	return axCopy(axSystem, "AXFocusedUIElement")
}

// axEditable 能打字的输入框：文本框/文本区/组合框，或者 AXValue 可写（网页里的 contenteditable）。
// 整页网页（AXWebArea）明确不算
func axEditable(el uintptr) (editable, known bool) {
	switch axString(el, "AXRole") {
	case "AXTextField", "AXTextArea", "AXComboBox", "AXSearchField":
		return true, true
	case "AXWebArea":
		return false, true
	}
	var ok bool
	if axIsSettable(el, axName("AXValue"), &ok) == 0 && ok {
		return true, true
	}
	return false, false
}

// axSelection 选区：有没有非空白的字，以及选区最后一个字的框
func axSelection(el uintptr) (state int, last RECT, hasRect bool) {
	r, ok := axRange(el, "AXSelectedTextRange")
	if !ok {
		return selUnknown, last, false
	}
	if r.Length == 0 {
		return selEmpty, last, false
	}
	state = selEmpty
	if strings.TrimSpace(axString(el, "AXSelectedText")) != "" {
		state = selText
	}
	last, hasRect = axBounds(el, cfRange{r.Location + r.Length - 1, 1})
	return state, last, hasRect
}

// focusEditable 焦点是可编辑的输入框，并且里面确实选中了非空白的文字
func focusEditable(fg uintptr, pt POINT) focusInfo {
	el := axFocused()
	if el == 0 {
		return focusInfo{}
	}
	defer cfRelease(el)
	editable, known := axEditable(el)
	if !editable {
		if !known {
			log.Printf("no button: focus role %q 认不出是不是输入框", axString(el, "AXRole"))
		}
		return focusInfo{}
	}
	state, rc, has := axSelection(el)
	fi := focusInfo{ok: state != selEmpty, sel: rc, hasSel: has}
	if state == selUnknown {
		// 选区问不出来：至少鼠标得落在这个输入框上
		if box, ok := axFrame(el); ok {
			fi.ok = pt.X >= box.Left-8 && pt.X <= box.Right+8 && pt.Y >= box.Top-8 && pt.Y <= box.Bottom+8
		}
	}
	return fi
}

// focusedInput 问不出都当不是输入框：宁可不全选，也别把整页内容选上
func focusedInput() inputState {
	el := axFocused()
	if el == 0 {
		return inputState{}
	}
	defer cfRelease(el)
	editable, _ := axEditable(el)
	if !editable {
		return inputState{}
	}
	r, ok := axRange(el, "AXSelectedTextRange")
	return inputState{input: true, empty: ok && r.Length == 0}
}

// selectedTextUIA 焦点元素里选中的字（终端、改写用：不发 ⌘C）
func selectedTextUIA() string {
	el := axFocused()
	if el == 0 {
		return ""
	}
	defer cfRelease(el)
	return axString(el, "AXSelectedText")
}

// systemCaret 焦点元素有插入点（有选区属性）：认不出是不是输入框时，当它「可能能贴」
func systemCaret(uintptr) bool {
	el := axFocused()
	if el == 0 {
		return false
	}
	defer cfRelease(el)
	_, ok := axRange(el, "AXSelectedTextRange")
	return ok
}

// caretText 光标（选区）前 before 个字、后 after 个字。AX 的位置按 UTF-16 算
func caretText(before, after int) (pre, post string, ok bool) {
	el := axFocused()
	if el == 0 {
		return "", "", false
	}
	defer cfRelease(el)
	r, ok := axRange(el, "AXSelectedTextRange")
	if !ok {
		return "", "", false
	}
	u := utf16.Encode([]rune(axString(el, "AXValue")))
	start, end := min(max(r.Location, 0), len(u)), min(max(r.Location+r.Length, 0), len(u))
	pre = string(utf16.Decode(u[max(0, start-before):start]))
	post = string(utf16.Decode(u[end:min(len(u), end+after)]))
	return pre, post, pre != "" || post != ""
}

// promptAccessibility 弹系统的「Mami 想控制这台电脑」对话框（只在没授权时有效）
func promptAccessibility() {
	axPrompted.Store(true)
	withPool(func() {
		opts := send(class("NSMutableDictionary"), "dictionary")
		send(opts, "setObject:forKey:", kCFBooleanTrue, dlvar(libAX, "kAXTrustedCheckOptionPrompt"))
		axIsProcessTrustedWithOptions(uintptr(opts))
	})
}
