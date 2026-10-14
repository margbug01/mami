//go:build windows

package app

import (
	"encoding/base64"
	"syscall"
	"unsafe"
)

// API Key 用 DPAPI（CryptProtectData）加密后再写进 config.json：
// 只有同一台电脑上的同一个 Windows 账户解得开，配置文件被拷走也读不出 key。

var (
	crypt32             = syscall.NewLazyDLL("crypt32.dll")
	pCryptProtectData   = crypt32.NewProc("CryptProtectData")
	pCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	pLocalFree          = kernel32.NewProc("LocalFree")
)

const cryptprotectUIForbidden = 0x1

type dataBlob struct {
	cbData uint32
	pbData *byte
}

func newBlob(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{cbData: uint32(len(b)), pbData: &b[0]}
}

func (b *dataBlob) bytes() []byte {
	out := make([]byte, b.cbData)
	copy(out, unsafe.Slice(b.pbData, b.cbData))
	return out
}

// protectKey 加密并转成 base64，好放进 JSON
func protectKey(key string) (string, error) {
	var out dataBlob
	r, _, err := pCryptProtectData.Call(uintptr(unsafe.Pointer(newBlob([]byte(key)))),
		uintptr(unsafe.Pointer(u16("Mami API key"))), 0, 0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", err
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return base64.StdEncoding.EncodeToString(out.bytes()), nil
}

func unprotectKey(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	var out dataBlob
	r, _, err := pCryptUnprotectData.Call(uintptr(unsafe.Pointer(newBlob(raw))),
		0, 0, 0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return "", err
	}
	defer pLocalFree.Call(uintptr(unsafe.Pointer(out.pbData)))
	return string(out.bytes()), nil
}
