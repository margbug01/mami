//go:build windows

package app

import (
	_ "embed"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"mami/internal/i18n"

	"github.com/egoist/mygo"
)

//go:embed assets/icon.png
var iconPNG []byte

// Run 是程序入口：读配置、挂钩子、开托盘，一直跑到退出
func Run() {
	if len(os.Args) > 1 && os.Args[1] == updateFlag { // 更新助手：不开界面、不抢单实例锁
		runUpdateHelper(os.Args[2:])
		return
	}
	demo := flag.Bool("demo", false, "只画出按钮和一张示例卡片，8 秒后退出（不挂钩子、不注册快捷键）")
	demoVoice := flag.Bool("demo-voice", false, "只放一遍语音胶囊的几种状态（假音量，不开麦克风），然后退出")
	demoStyle := flag.String("demo-style", "", "配合 -demo-voice：只放这几种样式（逗号分隔，如 mercury），空着就全放")
	flag.String("voice", "", "让已经在跑的 Mami 开关语音输入：toggle | start | stop | cancel（给遥控器、脚踏板、宏工具用）")
	flag.Parse()

	mygo.App.SetName("Mami")
	if !*demo && !*demoVoice && !mygo.App.RequestSingleInstanceLock() {
		return // 已经在跑了，那边会打开设置窗口（带 -voice 时改成开关语音）
	}
	if _, ok := voiceCommandArg(os.Args[1:]); ok && !*demo && !*demoVoice {
		log.Printf("voice: -voice given but Mami is not running")
		return // 没有在跑的 Mami 可以转；别因为一个遥控器按键把整个程序启动起来
	}

	a := &App{pid: uint32(os.Getpid())}
	// MAMI_CONFIG 让测试用自己的配置，不碰真实的
	if p := os.Getenv("MAMI_CONFIG"); p != "" {
		a.cfgPath = p
		a.logPath = filepath.Join(filepath.Dir(p), "mami.log")
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
		a.logPath = filepath.Join(logDir, "mami.log")
	}
	a.hist = &history{dir: filepath.Join(filepath.Dir(a.cfgPath), "history")}
	a.learn = newLearner(filepath.Join(filepath.Dir(a.cfgPath), "learned.json"))
	a.samples = newSampleLog(filepath.Join(filepath.Dir(a.cfgPath), "samples.jsonl"))
	a.logPaths = []string{a.logPath}
	if os.Getenv("MAMI_CONFIG") == "" { // 改名前（Huaci）的日志也算进去
		a.logPaths = append(a.logPaths, filepath.Join(filepath.Dir(filepath.Dir(a.cfgPath)), "Huaci", "logs", "huaci.log"))
	}
	if f := openLog(a.logPath); f != nil {
		log.SetOutput(f)
		defer f.Close()
	} else {
		log.SetOutput(io.Discard)
	}
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	if os.Getenv("MAMI_CONFIG") == "" {
		migrateFromHuaci(filepath.Dir(a.cfgPath))
	}

	cfg, created, cfgErr := loadConfig(a.cfgPath)
	if created { // 全新安装才默认开语音（老配置没有这个字段，升级上来不能悄悄打开）
		cfg.VoiceEnabled = true
		if rightAltTaken() { // 韩文键盘、带 AltGr 的欧洲键盘：右 Alt 另有用处，默认改右 Ctrl
			cfg.VoiceHotkey = "RCtrl"
			log.Printf("voice: right Alt is used by this keyboard layout, trigger set to RCtrl")
		}
		if err := saveConfig(a.cfgPath, cfg); err != nil {
			log.Println(err)
		}
	}
	if len(cfg.ASRHotwords) > 0 { // 老配置里的热词搬进词库，和学到的词一起管
		var words []learnedWord
		for _, h := range cfg.ASRHotwords {
			words = append(words, learnedWord{Right: h})
		}
		a.learn.add(words)
		cfg.ASRHotwords = []string{}
		if err := saveConfig(a.cfgPath, cfg); err != nil {
			log.Println(err)
		}
	}
	if err := a.setConfig(cfg); err != nil {
		log.Println(err)
		a.setConfig(defaultConfig())
	}
	d, _, _ := pGetDoubleClickTime.Call()
	a.dblTime = uint32(d)
	a.avatar = loadAvatar()

	mygo.Bind(&VoiceWeb{a})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.quitting = true })
	mygo.App.OnWindowAllClosed(func() {}) // 只剩托盘时也接着跑
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		if cmd, ok := voiceCommandArg(args); ok {
			if !voiceCommand(cmd) {
				log.Printf("voice: unknown -voice %q (toggle/start/stop/cancel)", cmd)
			}
			return
		}
		a.openSettings()
	})

	mygo.App.WhenReady(func() {
		if _, err := startWorker(); err != nil {
			log.Fatal("worker: ", err)
		}
		sysLocale = mygo.App.Locale()
		i18n.Set(i18n.Resolve(cfg.Language, sysLocale))
		a.createWindows()
		a.warmWebVoice(cfg.VoiceStyle)
		log.Printf("start: model=%s base=%s hotkey=%s config=%s", cfg.Model, cfg.BaseURL, cfg.Hotkey, a.cfgPath)
		if *demo {
			a.runDemo()
			return
		}
		if *demoVoice {
			go a.runVoiceDemo(*demoStyle)
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
		if err := startHook(a.isOurs, a.onCardKey); err != nil {
			log.Println(err)
			probs = append(probs, T("鼠标钩子挂载失败，只能用快捷键"))
		}
		go a.mouseLoop()
		go a.voiceLoop()
		go a.pruneLoop()
		go a.updateLoop()
		go cleanupUpdate()
		switch {
		case cfgErr != nil:
			a.openSettings()
			a.set.status, a.set.statusErr = T("配置文件没读成功，保存会覆盖它：")+cfgErr.Error(), true
		case created:
			a.openSettings()
			a.set.guide = true // 第一次启动先放新手引导，看完或跳过再带去填 Key（endGuide）
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
