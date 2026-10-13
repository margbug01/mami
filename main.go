//go:build windows

package main

import (
	_ "embed"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
)

//go:embed assets/icon.png
var iconPNG []byte

func main() {
	demo := flag.Bool("demo", false, "只画出按钮和一张示例卡片，8 秒后退出（不挂钩子、不注册快捷键）")
	flag.Parse()

	mygo.App.SetName("Huaci")
	if !*demo && !mygo.App.RequestSingleInstanceLock() {
		return // 已经在跑了，那边会打开设置窗口
	}

	a := &App{pid: uint32(os.Getpid())}
	// HUACI_CONFIG 让测试用自己的配置，不碰真实的
	if p := os.Getenv("HUACI_CONFIG"); p != "" {
		a.cfgPath = p
		a.logPath = filepath.Join(filepath.Dir(p), "huaci.log")
	} else {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			dir = "."
		}
		a.cfgPath = filepath.Join(dir, "config.json")
		logDir, err := mygo.App.Path(mygo.PathLogs)
		if err != nil {
			logDir = dir
		}
		a.logPath = filepath.Join(logDir, "huaci.log")
	}
	if f := openLog(a.logPath); f != nil {
		log.SetOutput(f)
		defer f.Close()
	} else {
		log.SetOutput(io.Discard)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	cfg, created, cfgErr := loadConfig(a.cfgPath)
	if err := a.setConfig(cfg); err != nil {
		log.Println(err)
		a.setConfig(defaultConfig())
	}
	d, _, _ := pGetDoubleClickTime.Call()
	a.dblTime = uint32(d)
	a.avatar = loadAvatar()

	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWindowAllClosed(func() {}) // 只剩托盘时也接着跑
	mygo.App.OnSecondInstance(func([]string, string) { a.openSettings() })

	mygo.App.WhenReady(func() {
		if _, err := startWorker(); err != nil {
			log.Fatal("worker: ", err)
		}
		a.createWindows()
		log.Printf("start: model=%s base=%s hotkey=%s config=%s", cfg.Model, cfg.BaseURL, cfg.Hotkey, a.cfgPath)
		if *demo {
			a.runDemo()
			return
		}
		a.createTray()
		probs := a.registerHotkeys(cfg)
		if len(probs) > 0 {
			log.Println("hotkeys:", probs)
		}
		if err := startUIA(); err != nil {
			log.Println("uia:", err, "— 只能靠系统光标判断输入框")
		} else {
			a.uiaOK = true
		}
		if err := startHook(a.isOurs); err != nil {
			log.Println(err)
			probs = append(probs, "鼠标钩子挂载失败，只能用快捷键")
		}
		go a.mouseLoop()
		switch {
		case cfgErr != nil:
			a.openSettings()
			a.set.status, a.set.statusErr = "配置文件没读成功，保存会覆盖它："+cfgErr.Error(), true
		case created:
			a.openSettings()
			a.set.status = "默认用 B 站免费的 Index-Translate-35B，不用 Key；换别的接口再填"
		case len(probs) > 0:
			a.openSettings()
			a.set.status, a.set.statusErr = strings.Join(probs, "；"), true
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	log.Println("exit")
}
