//go:build darwin

package app

import (
	"context"
	"os"
	"strings"
	"testing"
)

// macOS 键码和 VK 码互换：语音触发键、⌘C/⌘V 都靠这张表
func TestMacKeyMapping(t *testing.T) {
	for _, c := range []struct {
		code uint16
		vk   uint32
	}{{0x08, 'C'}, {0x09, 'V'}, {0x00, 'A'}, {0x06, 'Z'}, {0x24, vkReturn}, {0x35, vkEscape}, {0x7C, 0x27}, {0x3D, 0xA5}, {0x36, VK_RWIN}, {0x3F, vkFn}, {0x60, 0x74}} {
		if got := macToVK[c.code]; got != c.vk {
			t.Errorf("macToVK[%#x] = %#x, want %#x", c.code, got, c.vk)
		}
		if got := vkToMac[c.vk]; got != c.code {
			t.Errorf("vkToMac[%#x] = %#x, want %#x", c.vk, got, c.code)
		}
	}
	if vkToMac[vkReturn] != 0x24 { // 小键盘回车也映射到 VK_RETURN，发键要用主键盘那个
		t.Errorf("vkToMac[RETURN] = %#x", vkToMac[vkReturn])
	}
	// 右 ⌥：flagsChanged 里看它自己那一位判断按下/松开
	if vk, down := macKeyVK(evFlagsChanged, 0x3D, 0x40|flagAlt); vk != 0xA5 || !down {
		t.Errorf("right option down = %#x %v", vk, down)
	}
	if vk, down := macKeyVK(evFlagsChanged, 0x3D, 0x20|flagAlt); vk != 0xA5 || down { // 只剩左 ⌥ 按着
		t.Errorf("right option up = %#x %v", vk, down)
	}
	if vk, _ := macKeyVK(evFlagsChanged, 0x39, 0x10000); vk != 0 { // CapsLock 只报切换，不当触发键
		t.Errorf("caps lock mapped to %#x", vk)
	}
	if vk, down := macKeyVK(evKeyUp, 0x11, 0); vk != 'T' || down {
		t.Errorf("T up = %#x %v", vk, down)
	}
}

func TestMacKeyLabels(t *testing.T) {
	if got := keysLabel("Alt+Shift+T"); got != "⌥⇧T" {
		t.Errorf("keysLabel = %q", got)
	}
	if got := keyLabel("RAlt"); !strings.HasSuffix(got, " ⌥") {
		t.Errorf("keyLabel(RAlt) = %q", got)
	}
	if got := keyLabel("F5"); got != "F5" {
		t.Errorf("keyLabel(F5) = %q", got)
	}
	if voiceKey("RWin") != VK_RWIN || voiceKey("Fn") != vkFn || !voiceKeyIsModifier(vkFn) {
		t.Error("right ⌘ / fn should be modifier voice keys on macOS")
	}
}

// TestMain 测试不碰真的钥匙串：共用的测试（translate_test 这类）存配置时会加密 Key，原来会在登录钥匙串里
// 建出 dev.margbug.mami 那一项（Mami 真正用的密钥就成了测试生成的），每个测试程序还被加进它的允许列表。
// 换成假密钥，钥匙串函数换成一调用就让测试挂掉的。不能写在 init 里：本文件排在 keychain_darwin.go 前面，
// 它的 init 会把真函数注册回来
func TestMain(m *testing.M) {
	masterKey = []byte(strings.Repeat("t", 32))
	secItemCopyMatching = func(uintptr, *uintptr) int32 { panic("test touched the real keychain") }
	secItemAdd = func(uintptr, *uintptr) int32 { panic("test touched the real keychain") }
	os.Exit(m.Run())
}

// API Key 加解密（密钥直接给，不碰真的钥匙串）
func TestKeyCipherRoundTrip(t *testing.T) {
	masterMu.Lock()
	saved := masterKey
	masterKey = []byte(strings.Repeat("k", 32))
	masterMu.Unlock()
	defer func() { masterMu.Lock(); masterKey = saved; masterMu.Unlock() }()

	enc, err := protectKey("sk-test-密钥")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(enc, "sk-test") {
		t.Fatal("ciphertext leaks the key")
	}
	if got, err := unprotectKey(enc); err != nil || got != "sk-test-密钥" {
		t.Fatalf("round trip = %q, %v", got, err)
	}
	b := []byte(enc)
	b[len(b)-3] ^= 1
	if _, err := unprotectKey(string(b)); err == nil {
		t.Fatal("tampered ciphertext decrypted")
	}
	if _, err := unprotectKey("AQIDBA=="); err == nil { // Windows 的 DPAPI 密文这类
		t.Fatal("foreign blob decrypted")
	}
}

// 按着右 ⌥ 时系统报的样子（实测）：CGEventSourceKeyState 说左 ⌥ 按着、右 ⌥ 没按，flags 里是右 ⌥ 那一位 0x40。
// keyDown 得分清左右，不然触发键右 ⌥ 每次都被当成和左 ⌥ 一起按的组合键，语音起不来
func TestKeyDownTellsModifierSidesApart(t *testing.T) {
	defer func(f func() uint64, k func(uint16) bool) { hidFlags, hidKey = f, k }(hidFlags, hidKey)
	hidFlags = func() uint64 { return 0x20080140 }
	hidKey = func(code uint16) bool { return code == 0x3A }
	if !keyDown(0xA5) {
		t.Error("right option held, keyDown(VK_RMENU) = false")
	}
	if !keyDown(VK_MENU) {
		t.Error("keyDown(VK_MENU) = false")
	}
	for _, vk := range []uintptr{0xA0, 0xA1, 0xA2, 0xA3, 0xA4, VK_LWIN, VK_RWIN} {
		if keyDown(vk) {
			t.Errorf("only right option held, keyDown(%#x) = true", vk)
		}
	}
	hidFlags = func() uint64 { return 0x20000100 }
	hidKey = func(code uint16) bool { return code == 0x04 } // H
	if !keyDown('H') || keyDown('J') {
		t.Error("letters should come from the key state")
	}
}

// 没接麦克风（Mac mini 自己没有）：直接说没找到麦克风，不去开 AudioQueue
func TestRecordWithoutMicSaysSo(t *testing.T) {
	defer func(f func() bool, n func(*audioFormat, uintptr, uintptr, uintptr, uintptr, uint32, *uintptr) int32) {
		hasMic, aqNewInput = f, n
	}(hasMic, aqNewInput)
	hasMic = func() bool { return false }
	aqNewInput = func(*audioFormat, uintptr, uintptr, uintptr, uintptr, uint32, *uintptr) int32 {
		panic("opened an audio queue without a microphone")
	}
	pcm, err := recordVoice(context.Background(), make(chan struct{}), func() { t.Error("started without a microphone") }, nil)
	if err == nil || !strings.Contains(err.Error(), "没找到麦克风") || pcm != nil {
		t.Fatalf("recordVoice = %d bytes, %v; want the no-microphone error", len(pcm), err)
	}
}
