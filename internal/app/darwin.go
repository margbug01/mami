//go:build darwin

package app

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// macOS 这边不用 cgo：系统框架用 purego 直接调（和 MyGo 自己的 darwin 后端一样）。
// 这个文件只放加载框架和 objc/CoreFoundation 的小工具，具体功能在各个 *_darwin.go 里。

type cgPoint struct{ X, Y float64 }
type cgSize struct{ W, H float64 }
type cgRect struct {
	Origin cgPoint
	Size   cgSize
}

type cfRange struct{ Location, Length int }

var (
	libObjC, libCF, libCG, libAX, libAppKit, libSecurity, libAudio, libAVF uintptr

	msgSendAddr, poolPushFn, poolPopFn uintptr

	// objc_msgSend 带浮点、结构体参数或返回值的几种固定签名
	msgRect        func(obj objc.ID, sel objc.SEL) cgRect
	msgSetRectBool func(obj objc.ID, sel objc.SEL, r cgRect, b bool)
	msgFloat       func(obj objc.ID, sel objc.SEL) float64
	msgPoint       func(obj objc.ID, sel objc.SEL) cgPoint
	msgSize        func(obj objc.ID, sel objc.SEL) cgSize
	msgPointInt    func(obj objc.ID, sel objc.SEL, p cgPoint, n int) int

	cfRelease               func(obj uintptr)
	cfRetain                func(obj uintptr) uintptr
	cfRunLoopGetCurrent     func() uintptr
	cfRunLoopAddSource      func(rl, src, mode uintptr)
	cfRunLoopRun            func()
	cfMachPortCreateRLS     func(alloc, port uintptr, order int) uintptr
	cfGetTypeID             func(obj uintptr) uintptr
	cfStringGetTypeID       func() uintptr
	kCFRunLoopCommonModes   uintptr
	kCFBooleanTrue          uintptr
	cgEventTapCreate        func(tap, place, options uint32, mask uint64, callback, user uintptr) uintptr
	cgEventTapEnable        func(tap uintptr, enable bool)
	cgEventGetIntegerField  func(ev uintptr, field uint32) int64
	cgEventSetIntegerField  func(ev uintptr, field uint32, value int64)
	cgEventGetFlags         func(ev uintptr) uint64
	cgEventSetFlags         func(ev uintptr, flags uint64)
	cgEventGetLocation      func(ev uintptr) cgPoint
	cgEventCreate           func(source uintptr) uintptr
	cgEventCreateKeyboard   func(source uintptr, key uint16, down bool) uintptr
	cgEventPost             func(tap uint32, ev uintptr)
	cgEventSourceKeyState   func(state int32, key uint16) bool
	cgEventSourceButton     func(state int32, button uint32) bool
	cgEventSourceFlagsState func(state int32) uint64
	cgWindowListCopyInfo    func(option, relativeTo uint32) uintptr
	cgPreflightListenAccess func() bool
	cgRequestListenAccess   func() bool

	axIsProcessTrusted            func() bool
	axIsProcessTrustedWithOptions func(options uintptr) bool
	axCreateSystemWide            func() uintptr
	axCreateApplication           func(pid int32) uintptr
	axCopyAttribute               func(el, attr uintptr, value *uintptr) int32
	axCopyParamAttribute          func(el, attr, param uintptr, value *uintptr) int32
	axSetAttribute                func(el, attr, value uintptr) int32
	axSetMessagingTimeout         func(el uintptr, seconds float32) int32
	axValueGetValue               func(value uintptr, typ uint32, out unsafe.Pointer) bool
	axValueCreate                 func(typ uint32, in unsafe.Pointer) uintptr
)

// 系统库都在这里打开、函数都在这里注册。包里的 init 按文件名先后跑，ax_darwin.go 这种排在
// darwin.go 前面的文件要是自己在 init 里注册，那时 libAX 还是 0，一启动就 panic
func init() {
	open := func(path string) uintptr {
		h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			panic("mami: cannot load " + path + ": " + err.Error())
		}
		return h
	}
	libObjC = open("/usr/lib/libobjc.A.dylib")
	libCF = open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	libCG = open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	libAX = open("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
	libAppKit = open("/System/Library/Frameworks/AppKit.framework/AppKit")
	libSecurity = open("/System/Library/Frameworks/Security.framework/Security")
	libAudio = open("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox")
	libAVF = open("/System/Library/Frameworks/AVFoundation.framework/AVFoundation")

	msgSendAddr = dlsym(libObjC, "objc_msgSend")
	stret := msgSendAddr // amd64 上返回大于 16 字节的结构体要走 _stret
	if runtime.GOARCH == "amd64" {
		stret = dlsym(libObjC, "objc_msgSend_stret")
	}
	purego.RegisterFunc(&msgRect, stret)
	purego.RegisterFunc(&msgSetRectBool, msgSendAddr)
	purego.RegisterFunc(&msgFloat, msgSendAddr)
	purego.RegisterFunc(&msgPoint, msgSendAddr)
	purego.RegisterFunc(&msgSize, msgSendAddr)
	purego.RegisterFunc(&msgPointInt, msgSendAddr)
	poolPushFn = dlsym(libObjC, "objc_autoreleasePoolPush")
	poolPopFn = dlsym(libObjC, "objc_autoreleasePoolPop")

	reg := purego.RegisterLibFunc
	reg(&cfRelease, libCF, "CFRelease")
	reg(&cfRetain, libCF, "CFRetain")
	reg(&cfRunLoopGetCurrent, libCF, "CFRunLoopGetCurrent")
	reg(&cfRunLoopAddSource, libCF, "CFRunLoopAddSource")
	reg(&cfRunLoopRun, libCF, "CFRunLoopRun")
	reg(&cfMachPortCreateRLS, libCF, "CFMachPortCreateRunLoopSource")
	reg(&cfGetTypeID, libCF, "CFGetTypeID")
	reg(&cfStringGetTypeID, libCF, "CFStringGetTypeID")
	kCFRunLoopCommonModes = dlvar(libCF, "kCFRunLoopCommonModes")
	kCFBooleanTrue = dlvar(libCF, "kCFBooleanTrue")

	reg(&cgEventTapCreate, libCG, "CGEventTapCreate")
	reg(&cgEventTapEnable, libCG, "CGEventTapEnable")
	reg(&cgEventGetIntegerField, libCG, "CGEventGetIntegerValueField")
	reg(&cgEventSetIntegerField, libCG, "CGEventSetIntegerValueField")
	reg(&cgEventGetFlags, libCG, "CGEventGetFlags")
	reg(&cgEventSetFlags, libCG, "CGEventSetFlags")
	reg(&cgEventGetLocation, libCG, "CGEventGetLocation")
	reg(&cgEventCreate, libCG, "CGEventCreate")
	reg(&cgEventCreateKeyboard, libCG, "CGEventCreateKeyboardEvent")
	reg(&cgEventPost, libCG, "CGEventPost")
	reg(&cgEventSourceKeyState, libCG, "CGEventSourceKeyState")
	reg(&cgEventSourceButton, libCG, "CGEventSourceButtonState")
	reg(&cgEventSourceFlagsState, libCG, "CGEventSourceFlagsState")
	reg(&cgWindowListCopyInfo, libCG, "CGWindowListCopyWindowInfo")
	reg(&cgPreflightListenAccess, libCG, "CGPreflightListenEventAccess")
	reg(&cgRequestListenAccess, libCG, "CGRequestListenEventAccess")

	reg(&axIsProcessTrusted, libAX, "AXIsProcessTrusted")
	reg(&axIsProcessTrustedWithOptions, libAX, "AXIsProcessTrustedWithOptions")
	reg(&axCreateSystemWide, libAX, "AXUIElementCreateSystemWide")
	reg(&axCreateApplication, libAX, "AXUIElementCreateApplication")
	reg(&axCopyAttribute, libAX, "AXUIElementCopyAttributeValue")
	reg(&axCopyParamAttribute, libAX, "AXUIElementCopyParameterizedAttributeValue")
	reg(&axIsSettable, libAX, "AXUIElementIsAttributeSettable")
	reg(&axSetAttribute, libAX, "AXUIElementSetAttributeValue")
	reg(&axSetMessagingTimeout, libAX, "AXUIElementSetMessagingTimeout")
	reg(&axValueGetValue, libAX, "AXValueGetValue")
	reg(&axValueCreate, libAX, "AXValueCreate")
}

func dlsym(lib uintptr, name string) uintptr {
	p, err := purego.Dlsym(lib, name)
	if err != nil {
		panic("mami: " + err.Error())
	}
	return p
}

// dlvar 读框架导出的全局常量（CFStringRef、CFBooleanRef 这类指针变量）
func dlvar(lib uintptr, name string) uintptr {
	return *(*uintptr)(cPointer(dlsym(lib, name)))
}

var (
	selMu    sync.Mutex
	selCache = map[string]objc.SEL{}
	clsCache = map[string]objc.ID{}
)

func sel(name string) objc.SEL {
	selMu.Lock()
	defer selMu.Unlock()
	s, ok := selCache[name]
	if !ok {
		s = objc.RegisterName(name)
		selCache[name] = s
	}
	return s
}

func class(name string) objc.ID {
	selMu.Lock()
	defer selMu.Unlock()
	c, ok := clsCache[name]
	if !ok {
		c = objc.ID(objc.GetClass(name))
		clsCache[name] = c
	}
	return c
}

// send 发一条参数和返回值都是整数/指针的消息（不走反射，钩子线程上也能用）
func send(obj objc.ID, name string, args ...uintptr) objc.ID {
	var a [8]uintptr
	a[0], a[1] = uintptr(obj), uintptr(sel(name))
	n := copy(a[2:], args)
	r, _, _ := purego.SyscallN(msgSendAddr, a[:n+2]...)
	return objc.ID(r)
}

func sendBool(obj objc.ID, name string, args ...uintptr) bool {
	return send(obj, name, args...)&0xff != 0
}

// withPool 给这段 objc 调用一个自动释放池；goroutine 在池子里不能换线程
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool, _, _ := purego.SyscallN(poolPushFn)
	defer purego.SyscallN(poolPopFn, pool)
	fn()
}

const nsUTF8 = 4

// nsString 自动释放的 NSString（要在 withPool 里用）
func nsString(s string) objc.ID {
	b := []byte(s)
	var p uintptr
	if len(b) > 0 {
		p = uintptr(unsafe.Pointer(&b[0]))
	}
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:", p, uintptr(len(b)), nsUTF8)
	runtime.KeepAlive(b)
	return send(str, "autorelease")
}

// cfConst 常驻的 CFString/NSString（键名、属性名），不释放
func cfConst(s string) uintptr {
	b := []byte(s)
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:", uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), nsUTF8)
	runtime.KeepAlive(b)
	return uintptr(str)
}

// goString 把 NSString/CFString 拷成 Go 字符串；不是字符串就返回 ""
func goString(str objc.ID) string {
	if str == 0 || cfGetTypeID(uintptr(str)) != cfStringGetTypeID() {
		return ""
	}
	p := send(str, "UTF8String")
	if p == 0 {
		return ""
	}
	n := int(send(str, "lengthOfBytesUsingEncoding:", nsUTF8))
	return string(unsafe.Slice((*byte)(cPointer(uintptr(p))), n))
}

// cPointer 只接收系统框架拥有的地址，不能用来暂存 Go 指针
func cPointer(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

// nsData 自动释放的 NSData，拷一份 b
func nsData(b []byte) objc.ID {
	var p uintptr
	if len(b) > 0 {
		p = uintptr(unsafe.Pointer(&b[0]))
	}
	d := send(class("NSData"), "dataWithBytes:length:", p, uintptr(len(b)))
	runtime.KeepAlive(b)
	return d
}

// goBytes 拷出 NSData 的内容
func goBytes(d objc.ID) []byte {
	if d == 0 {
		return nil
	}
	n := int(send(d, "length"))
	if n == 0 {
		return []byte{}
	}
	p := send(d, "bytes")
	return append([]byte(nil), unsafe.Slice((*byte)(cPointer(uintptr(p))), n)...)
}
