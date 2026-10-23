//go:build darwin

package app

// macOS 的隐私权限：辅助功能（监听和模拟按键、读输入框）、麦克风。

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
