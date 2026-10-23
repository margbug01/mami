//go:build darwin

package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// API Key 用 AES-GCM 加密后写进 config.json，密钥（32 字节随机数）放在钥匙串里：
// 只有这台 Mac 的这个用户、而且是同一个签名的 Mami 才拿得到，配置文件被拷走也读不出 key。
// 从 Windows 拷来的配置解不开，当没填，和换了电脑的 Windows 一样。

var (
	secItemCopyMatching func(query uintptr, out *uintptr) int32
	secItemAdd          func(attrs uintptr, out *uintptr) int32

	masterMu  sync.Mutex
	masterKey []byte
)

func init() {
	purego.RegisterLibFunc(&secItemCopyMatching, libSecurity, "SecItemCopyMatching")
	purego.RegisterLibFunc(&secItemAdd, libSecurity, "SecItemAdd")
}

const (
	errSecItemNotFound = -25300
	errSecDuplicate    = -25299
)

// keychainQuery 钥匙串里那一项的查找条件（自动释放的字典，要在 withPool 里用）
func keychainQuery() objc.ID {
	q := send(class("NSMutableDictionary"), "dictionary")
	set := func(k string, v uintptr) { send(q, "setObject:forKey:", v, dlvar(libSecurity, k)) }
	set("kSecClass", dlvar(libSecurity, "kSecClassGenericPassword"))
	set("kSecAttrService", uintptr(nsString("dev.margbug.mami")))
	set("kSecAttrAccount", uintptr(nsString("config-key")))
	return q
}

// loadMasterKey 读钥匙串里的密钥，没有就生成一把存进去
func loadMasterKey() ([]byte, error) {
	masterMu.Lock()
	defer masterMu.Unlock()
	if masterKey != nil {
		return masterKey, nil
	}
	var key []byte
	var err error
	withPool(func() {
		for attempt := 0; attempt < 2 && key == nil && err == nil; attempt++ {
			q := keychainQuery()
			send(q, "setObject:forKey:", kCFBooleanTrue, dlvar(libSecurity, "kSecReturnData"))
			send(q, "setObject:forKey:", dlvar(libSecurity, "kSecMatchLimitOne"), dlvar(libSecurity, "kSecMatchLimit"))
			var out uintptr
			switch st := secItemCopyMatching(uintptr(q), &out); st {
			case 0:
				if b := goBytes(objc.ID(out)); len(b) == 32 {
					key = b
				} else {
					err = errors.New("keychain: bad key length")
				}
				cfRelease(out)
				continue
			case errSecItemNotFound:
			default:
				err = fmt.Errorf("keychain: SecItemCopyMatching %d", st)
				continue
			}
			fresh := make([]byte, 32)
			if _, err = rand.Read(fresh); err != nil {
				continue
			}
			add := keychainQuery()
			send(add, "setObject:forKey:", uintptr(nsData(fresh)), dlvar(libSecurity, "kSecValueData"))
			send(add, "setObject:forKey:", dlvar(libSecurity, "kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly"), dlvar(libSecurity, "kSecAttrAccessible"))
			switch st := secItemAdd(uintptr(add), nil); st {
			case 0:
				key = fresh
			case errSecDuplicate: // 别的线程刚存了一把：下一轮读它
			default:
				err = fmt.Errorf("keychain: SecItemAdd %d", st)
			}
		}
	})
	if err == nil && key == nil {
		err = errors.New("keychain: no key")
	}
	if err != nil {
		return nil, err
	}
	masterKey = key
	return key, nil
}

func keyCipher() (cipher.AEAD, error) {
	key, err := loadMasterKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// protectKey 加密并转成 base64（随机 nonce 放在密文前面），好放进 JSON
func protectKey(key string) (string, error) {
	gcm, err := keyCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(key), nil)), nil
}

func unprotectKey(enc string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return "", err
	}
	gcm, err := keyCipher()
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("encrypted key too short")
	}
	out, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
