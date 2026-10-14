//go:build windows

package app

import (
	"errors"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// 判断「当前焦点是不是一个能打字的输入框」，划词按钮只在这种地方弹。
// 浏览器/Electron 不建 Win32 系统光标，只能问 UI Automation：
//   - textarea / input：Edit，有 ValuePattern 且不只读
//   - contenteditable：Group，有 TextPattern（Chromium 只给文本框和整页 Document 挂 TextPattern）
//   - 网页正文：焦点是整页 Document，ValuePattern 只读 → 不算
// 老式 Win32 编辑框（记事本、tk 等）再用系统光标兜底。

var (
	ole32    = syscall.NewLazyDLL("ole32.dll")
	oleaut32 = syscall.NewLazyDLL("oleaut32.dll")

	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")
	pVariantClear     = oleaut32.NewProc("VariantClear")
	pGetGUIThreadInfo = user32.NewProc("GetGUIThreadInfo")
)

type comGUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	clsidCUIAutomation = comGUID{0xff48dba4, 0x60ef, 0x4201, [8]byte{0xaa, 0x87, 0x54, 0x10, 0x3e, 0xef, 0x59, 0x4e}}
	iidIUIAutomation   = comGUID{0x30cbe57d, 0xd9d0, 0x452a, [8]byte{0xab, 0x13, 0x7a, 0xc5, 0xac, 0x48, 0x25, 0xee}}
)

const (
	uiaControlTypeProperty    = 30003
	uiaIsTextPatternAvailable = 30040
	uiaIsValuePatternAvail    = 30043
	uiaValueIsReadOnly        = 30046
	uiaDocumentControlType    = 50030

	vtI4   = 3
	vtBool = 11
)

type variant struct {
	VT        uint16
	_         [3]uint16
	Val, Val2 int64
}

type GUITHREADINFO struct {
	CbSize        uint32
	Flags         uint32
	HwndActive    uintptr
	HwndFocus     uintptr
	HwndCapture   uintptr
	HwndMenuOwner uintptr
	HwndMoveSize  uintptr
	HwndCaret     uintptr
	RcCaret       RECT
}

func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	vtbl := *(*uintptr)(unsafe.Pointer(obj))
	fn := *(*uintptr)(unsafe.Pointer(vtbl + uintptr(idx)*unsafe.Sizeof(uintptr(0))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{obj}, args...)...)
	return r
}

func comRelease(obj uintptr) { comCall(obj, 2) }

type editableReq struct {
	fg    uintptr
	pt    POINT // 松开鼠标的位置：选区问不出来时，用它判断划的是不是输入框里的字
	reply chan focusInfo
}

// focusInfo：焦点是不是有选中文字的输入框；sel 是选区最后一行在屏幕上的框（物理像素）
type focusInfo struct {
	ok     bool
	sel    RECT
	hasSel bool
	blind  bool // 有输入光标但位置是假的、选区也问不出（微信 4.x 这类 Qt 程序）：分不清输入框和聊天记录
}

var uiaReqs = make(chan editableReq, 8)

// uiaTextReqs 问焦点元素里选中的完整文字（终端用：不发 Ctrl+C）
var uiaTextReqs = make(chan chan string, 4)

// selectedTextUIA 读焦点元素选中的文字；UIA 不可用、超时、没选中都返回 ""
func selectedTextUIA() string {
	reply := make(chan string, 1)
	select {
	case uiaTextReqs <- reply:
	default:
		return ""
	}
	select {
	case s := <-reply:
		return s
	case <-time.After(800 * time.Millisecond):
		log.Println("uia: selection text query timed out")
		return ""
	}
}

// focusedSelectionText 在 UIA 线程上：焦点元素 TextPattern 的全部选区拼起来
func focusedSelectionText(auto uintptr) string {
	var el uintptr
	if hr := comCall(auto, 8, uintptr(unsafe.Pointer(&el))); hr != 0 || el == 0 { // GetFocusedElement
		return ""
	}
	defer comRelease(el)
	var tp uintptr
	if hr := comCall(el, 14, uiaTextPatternId, uintptr(unsafe.Pointer(&iidTextPattern)), uintptr(unsafe.Pointer(&tp))); hr != 0 || tp == 0 { // GetCurrentPatternAs
		return ""
	}
	defer comRelease(tp)
	var arr uintptr
	if hr := comCall(tp, 5, uintptr(unsafe.Pointer(&arr))); hr != 0 || arr == 0 { // GetSelection
		return ""
	}
	defer comRelease(arr)
	var n int32
	comCall(arr, 3, uintptr(unsafe.Pointer(&n))) // get_Length
	var parts []string
	for i := int32(0); i < n; i++ {
		var r uintptr
		if hr := comCall(arr, 4, uintptr(i), uintptr(unsafe.Pointer(&r))); hr != 0 || r == 0 { // GetElement
			continue
		}
		var bstr uintptr
		comCall(r, 12, ^uintptr(0), uintptr(unsafe.Pointer(&bstr))) // GetText(-1) 不限长度
		comRelease(r)
		if bstr == 0 {
			continue
		}
		l, _, _ := pSysStringLen.Call(bstr)
		parts = append(parts, string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(bstr)), l))))
		pSysFreeString.Call(bstr)
	}
	return strings.Join(parts, "\n")
}

// startUIA 起一条 COM(MTA) 线程专门做跨进程的 UIA 查询，慢了也不卡 UI 和钩子。
func startUIA() error {
	ready := make(chan error)
	go func() {
		runtime.LockOSThread()
		pCoInitializeEx.Call(0, 0) // COINIT_MULTITHREADED
		var auto uintptr
		hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, 1, // CLSCTX_INPROC_SERVER
			uintptr(unsafe.Pointer(&iidIUIAutomation)), uintptr(unsafe.Pointer(&auto)))
		if hr != 0 || auto == 0 {
			ready <- errors.New("CoCreateInstance(CUIAutomation) failed")
			return
		}
		ready <- nil
		for {
			select {
			case req := <-uiaReqs:
				req.reply <- focusIsEditable(auto, req.fg, req.pt)
			case reply := <-uiaTextReqs:
				reply <- focusedSelectionText(auto)
			}
		}
	}()
	return <-ready
}

func uiaProp(el uintptr, id int) variant {
	var v variant
	comCall(el, 10, uintptr(id), uintptr(unsafe.Pointer(&v))) // GetCurrentPropertyValue
	return v
}

func (v variant) boolean() bool { return v.VT == vtBool && int16(v.Val) != 0 }

// focusIsEditable 焦点是可编辑的输入框，并且里面确实选中了非空白的文字。
// 双击空输入框也算「双击选词」手势，但选区是空的，这里拦掉。
// 顺序：老式 Edit 能直接问选区 → UIA 能问就问 UIA → 都问不出（微信这类 Qt 自绘程序）
// 就只认「鼠标松开的位置和输入光标在同一行」，免得在聊天记录里双击也弹。
func focusIsEditable(auto, fg uintptr, pt POINT) focusInfo {
	gti, ok := guiThreadInfo(fg)
	hasCaret := ok && gti.HwndCaret != 0
	if hasCaret {
		if st := win32Selection(gti.HwndFocus); st != selUnknown {
			fi := caretInfo(gti)
			fi.ok = st == selText
			return fi
		}
	}
	if auto != 0 {
		if fi, known := uiaFocus(auto, pt); known {
			return fi
		}
	}
	if hasCaret {
		fi := caretInfo(gti)
		fi.ok = fi.hasSel && nearCaret(fi.sel, pt)
		fi.blind = !fi.hasSel
		if !fi.ok && !fi.blind {
			log.Printf("no button: %s 问不出选区，鼠标 %v 不在输入光标那行（光标 %+v）", filepath.Base(processName(windowPID(fg))), pt, fi.sel)
		}
		return fi
	}
	log.Printf("no button: %s 问不出选区、也没有输入光标", filepath.Base(processName(windowPID(fg))))
	return focusInfo{}
}

// caretInfo 把系统光标换成屏幕坐标，当作选区末尾
func caretInfo(gti GUITHREADINFO) focusInfo {
	var fi focusInfo
	c := gti.RcCaret
	tl, br := POINT{c.Left, c.Top}, POINT{c.Right, c.Bottom}
	pClientToScreen.Call(gti.HwndCaret, uintptr(unsafe.Pointer(&tl)))
	pClientToScreen.Call(gti.HwndCaret, uintptr(unsafe.Pointer(&br)))
	if br.Y-tl.Y >= 4 { // Tk 这类只为输入法建 1 像素的假光标，位置不跟文字走，不能用
		fi.sel, fi.hasSel = RECT{tl.X, tl.Y, br.X, br.Y}, true
	}
	return fi
}

// nearCaret 鼠标在光标那一行上下各留 1.5 行。在输入框里拖选/双击，光标会跟到鼠标那里；
// 在聊天记录里双击，输入框的光标不动，离鼠标好几行远。
func nearCaret(caret RECT, pt POINT) bool {
	h := max(caret.Bottom-caret.Top, scale(16, dpiAt(pt)))
	return pt.Y >= caret.Top-h*3/2 && pt.Y <= caret.Bottom+h*3/2
}

// uiaFocus 问 UIA。known=false 表示 UIA 说不清，交给调用方兜底。
func uiaFocus(auto uintptr, pt POINT) (fi focusInfo, known bool) {
	var el uintptr
	if hr := comCall(auto, 8, uintptr(unsafe.Pointer(&el))); hr != 0 || el == 0 { // GetFocusedElement
		return fi, false
	}
	defer comRelease(el)
	ct := uiaProp(el, uiaControlTypeProperty)
	valuePat := uiaProp(el, uiaIsValuePatternAvail).boolean()
	readOnly := uiaProp(el, uiaValueIsReadOnly)
	textPat := uiaProp(el, uiaIsTextPatternAvailable).boolean()
	isDoc := ct.VT == vtI4 && int32(ct.Val) == uiaDocumentControlType
	editable := (valuePat && readOnly.VT == vtBool && !readOnly.boolean()) || (textPat && !isDoc)
	if readOnly.VT != vtBool && readOnly.VT != 0 {
		pVariantClear.Call(uintptr(unsafe.Pointer(&readOnly)))
	}
	if !editable {
		return fi, isDoc // 网页正文这种明确不是输入框的，直接定论；其他交给系统光标兜底
	}
	state, rc, has := uiaSelection(el)
	fi = focusInfo{ok: state != selEmpty, sel: rc, hasSel: has}
	if state == selUnknown {
		// 选区问不出来：至少鼠标得落在这个输入框上
		var box RECT
		if hr := comCall(el, 43, uintptr(unsafe.Pointer(&box))); hr == 0 { // get_CurrentBoundingRectangle
			m := scale(8, dpiAt(pt))
			fi.ok = pt.X >= box.Left-m && pt.X <= box.Right+m && pt.Y >= box.Top-m && pt.Y <= box.Bottom+m
		}
	}
	return fi, true
}

const (
	selUnknown = iota // 问不出来就放行，宁可多弹也别在能用的地方不弹
	selEmpty
	selText

	uiaTextPatternId = 10014
	emGetSel         = 0x00B0
)

var (
	iidTextPattern       = comGUID{0x32eba289, 0x3583, 0x42c9, [8]byte{0x9c, 0x59, 0x3b, 0x6d, 0x9a, 0x1e, 0x9b, 0x6a}}
	pSysStringLen        = oleaut32.NewProc("SysStringLen")
	pSysFreeString       = oleaut32.NewProc("SysFreeString")
	pSendMessageTimeoutW = user32.NewProc("SendMessageTimeoutW")
	pClientToScreen      = user32.NewProc("ClientToScreen")

	pSafeArrayGetUBound    = oleaut32.NewProc("SafeArrayGetUBound")
	pSafeArrayAccessData   = oleaut32.NewProc("SafeArrayAccessData")
	pSafeArrayUnaccessData = oleaut32.NewProc("SafeArrayUnaccessData")
	pSafeArrayDestroy      = oleaut32.NewProc("SafeArrayDestroy")
)

// uiaSelection 读 TextPattern 的选区：有没有非空白字符，以及最后一行在屏幕上的框
func uiaSelection(el uintptr) (state int, last RECT, hasRect bool) {
	var tp uintptr
	if hr := comCall(el, 14, uiaTextPatternId, uintptr(unsafe.Pointer(&iidTextPattern)), uintptr(unsafe.Pointer(&tp))); hr != 0 || tp == 0 { // GetCurrentPatternAs
		return selUnknown, last, false
	}
	defer comRelease(tp)
	var arr uintptr
	if hr := comCall(tp, 5, uintptr(unsafe.Pointer(&arr))); hr != 0 || arr == 0 { // GetSelection
		return selUnknown, last, false
	}
	defer comRelease(arr)
	var n int32
	comCall(arr, 3, uintptr(unsafe.Pointer(&n))) // get_Length
	state = selEmpty
	for i := int32(0); i < n; i++ {
		var r uintptr
		if hr := comCall(arr, 4, uintptr(i), uintptr(unsafe.Pointer(&r))); hr != 0 || r == 0 { // GetElement
			continue
		}
		var psa uintptr
		if hr := comCall(r, 10, uintptr(unsafe.Pointer(&psa))); hr == 0 && psa != 0 { // GetBoundingRectangles
			if rc, ok := lastRect(psa); ok {
				last, hasRect = rc, true
			}
			pSafeArrayDestroy.Call(psa)
		}
		var bstr uintptr
		comCall(r, 12, 64, uintptr(unsafe.Pointer(&bstr))) // GetText(maxLength=64)
		comRelease(r)
		if bstr == 0 {
			continue
		}
		l, _, _ := pSysStringLen.Call(bstr)
		text := string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(bstr)), l)))
		pSysFreeString.Call(bstr)
		if strings.TrimSpace(text) != "" {
			state = selText
		}
	}
	return state, last, hasRect
}

// lastRect 读 SAFEARRAY(double)：每 4 个数是一行的 left/top/width/height，取最后一个非空的
func lastRect(psa uintptr) (rc RECT, ok bool) {
	var ub int32
	if hr, _, _ := pSafeArrayGetUBound.Call(psa, 1, uintptr(unsafe.Pointer(&ub))); hr != 0 || ub < 3 {
		return rc, false
	}
	var p uintptr
	if hr, _, _ := pSafeArrayAccessData.Call(psa, uintptr(unsafe.Pointer(&p))); hr != 0 || p == 0 {
		return rc, false
	}
	defer pSafeArrayUnaccessData.Call(psa)
	d := unsafe.Slice((*float64)(unsafe.Pointer(p)), int(ub)+1)
	for i := 0; i+3 < len(d); i += 4 {
		if d[i+2] > 0 && d[i+3] > 0 {
			rc, ok = RECT{int32(d[i]), int32(d[i+1]), int32(d[i] + d[i+2]), int32(d[i+1] + d[i+3])}, true
		}
	}
	return rc, ok
}

// isEditClass 只有 Edit / RichEdit（含 WinForms 包的）才认 EM_GETSEL。
// 跨进程发给别的窗口时，系统会把两个出参清零再拷回来，看着像「选区为空」，不能信。
func isEditClass(h uintptr) bool {
	var buf [128]uint16
	n, _, _ := pGetClassNameW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return n > 0 && strings.Contains(strings.ToLower(syscall.UTF16ToString(buf[:n])), "edit")
}

// win32Selection 对 Edit/RichEdit 发 EM_GETSEL；别的控件不认这条消息，起止值不会被改写 → 未知
func win32Selection(h uintptr) int {
	if h == 0 || !isEditClass(h) {
		return selUnknown
	}
	start, end := ^uint32(0), ^uint32(0)
	var res uintptr
	if r, _, _ := pSendMessageTimeoutW.Call(h, emGetSel, uintptr(unsafe.Pointer(&start)), uintptr(unsafe.Pointer(&end)), 2, 200, uintptr(unsafe.Pointer(&res))); r == 0 {
		return selUnknown
	}
	if start == ^uint32(0) || end == ^uint32(0) {
		return selUnknown
	}
	if start == end {
		return selEmpty
	}
	return selText
}

func guiThreadInfo(fg uintptr) (GUITHREADINFO, bool) {
	tid, _, _ := pGetWindowThreadProcessId.Call(fg, 0)
	gti := GUITHREADINFO{CbSize: uint32(unsafe.Sizeof(GUITHREADINFO{}))}
	r, _, _ := pGetGUIThreadInfo.Call(tid, uintptr(unsafe.Pointer(&gti)))
	return gti, r != 0
}

func systemCaret(fg uintptr) bool {
	gti, ok := guiThreadInfo(fg)
	return ok && gti.HwndCaret != 0
}

// focusEditable 查询超时（对方进程卡住、第一次建无障碍树）就当不是输入框
func focusEditable(fg uintptr, pt POINT) focusInfo {
	reply := make(chan focusInfo, 1)
	select {
	case uiaReqs <- editableReq{fg, pt, reply}:
	default:
		return focusInfo{}
	}
	select {
	case fi := <-reply:
		return fi
	case <-time.After(600 * time.Millisecond):
		log.Println("uia: focus query timed out")
		return focusInfo{}
	}
}
