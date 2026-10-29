//go:build darwin

package app

import (
	"log"
	"os/exec"
	"sync/atomic"

	"github.com/ebitengine/purego/objc"
)

// macOS 的隐私权限：辅助功能（监听和模拟按键、读输入框）、麦克风。

const needPermissions = true

var axPrompted atomic.Bool // 这次启动弹过系统的辅助功能对话框

func accessibilityOK() bool { return axIsProcessTrusted() }
func micOK() bool           { return micPermission() == micGranted }

// askAccessibility 第一次弹系统对话框（它会把 Mami 加进列表、带「打开系统设置」按钮），之后直接打开那一页
func askAccessibility() {
	if !axPrompted.Load() {
		promptAccessibility()
		return
	}
	openPrivacy("Privacy_Accessibility")
}

// askMicrophone 没问过就弹系统的麦克风对话框；拒绝过只能去系统设置里打开
func askMicrophone() {
	if micPermission() == micUndetermined {
		withPool(func() {
			blk := objc.NewBlock(func(_ objc.Block, granted bool) { log.Printf("mic: granted=%v", granted) })
			send(class("AVCaptureDevice"), "requestAccessForMediaType:completionHandler:", dlvar(libAVF, "AVMediaTypeAudio"), uintptr(blk))
			blk.Release() // AVFoundation 自己拷了一份
		})
		return
	}
	openPrivacy("Privacy_Microphone")
}

func openPrivacy(anchor string) {
	if err := exec.Command("/usr/bin/open", "x-apple.systempreferences:com.apple.preference.security?"+anchor).Start(); err != nil {
		log.Println("open privacy settings:", err)
	}
}

const (
	micUndetermined = iota // 还没问过：第一次录音时系统会弹窗
	micGranted
	micDenied // 拒绝过，或者被家长控制/公司策略限制
)

// micPermission AVCaptureDevice 的麦克风授权状态
func micPermission() int {
	st := 0
	withPool(func() {
		st = int(send(class("AVCaptureDevice"), "authorizationStatusForMediaType:", dlvar(libAVF, "AVMediaTypeAudio")))
	})
	switch st {
	case 3: // AVAuthorizationStatusAuthorized
		return micGranted
	case 1, 2: // Restricted, Denied
		return micDenied
	}
	return micUndetermined
}
