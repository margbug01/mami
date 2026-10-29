//go:build darwin

package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// API Key 用 AES-GCM 加密后写进 config.json，密钥（32 字节随机数）放在登录钥匙串里：
// 配置文件被拷走、进了备份也读不出 key。从 Windows 拷来的配置解不开，当没填，和换了电脑的 Windows 一样。
//
// 钥匙串由系统的 security 工具来存取（和 MySpotify、Shoes 一样）：那一项认的是 security 这个苹果自己的工具，
// 不是这一次编译出来的 Mami，所以重新编译、应用内更新以后不会再弹「输入登录钥匙串的密码」。
// 早先 Mami 直接调钥匙串接口存在「config-key」里，钥匙串按每次编译的指纹认人，每换一次程序就问一次密码；
// 那一项不再读，配置里用它加密的 Key 解不开、当没填

const (
	keychainService = "dev.margbug.mami"
	keychainAccount = "master-key"
)

var (
	masterMu  sync.Mutex
	masterKey []byte
)

// security 跑一次 /usr/bin/security，给它 stdin；返回输出和退出码（没跑起来是 -1）。测试里换成假的
var security = func(stdin string, args ...string) ([]byte, int, error) {
	cmd := exec.Command("/usr/bin/security", args...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	out, err := cmd.Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out, 0, nil
	case errors.As(err, &exit):
		return out, exit.ExitCode(), err
	}
	return out, -1, err
}

// loadMasterKey 读钥匙串里的密钥，第一次没有就生成一把存进去
func loadMasterKey() ([]byte, error) {
	masterMu.Lock()
	defer masterMu.Unlock()
	if masterKey != nil {
		return masterKey, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	chain := filepath.Join(home, "Library", "Keychains", "login.keychain-db")
	find := []string{"find-generic-password", "-s", keychainService, "-a", keychainAccount, "-w", chain}
	out, code, err := security("", find...)
	switch code {
	case 0:
		key, err := hex.DecodeString(strings.TrimSpace(string(out)))
		if err != nil || len(key) != 32 {
			return nil, errors.New("keychain: the stored key is not 32 bytes")
		}
		masterKey = key
		return key, nil
	case 44: // 没有这一项：第一次
	default:
		return nil, fmt.Errorf("keychain: %v", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// 从 security 自己的提示符里存：密钥不出现在任何命令行上（ps 看得到命令行）
	add := fmt.Sprintf("add-generic-password -s %s -a %s -w %s %s\n",
		strconv.Quote(keychainService), keychainAccount, hex.EncodeToString(key), strconv.Quote(chain))
	if _, code, err := security(add, "-i"); code != 0 {
		return nil, fmt.Errorf("keychain: keeping the key: %v", err)
	}
	// 提示符里的命令失败了也不一定报：读回来对一下
	if out, code, _ := security("", find...); code != 0 || strings.TrimSpace(string(out)) != hex.EncodeToString(key) {
		return nil, errors.New("keychain: the key kept does not read back")
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
