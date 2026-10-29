package app

import (
	"log"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// 录音时暂停正在放的音乐、视频，说完接着放（设置「录音时暂停播放」）。
//
// mediaPlaying 现在往扬声器出声的程序（进程号按负责的 app 算，不含 Mami 自己）；平台不支持时 nil。
// mediaSend 给系统里正在放的那个 app 发「暂停」或「播放」：是明确的暂停、播放，不是 ⏯ 那样的切换，
// 没在放的不会被「暂停」放起来。测试里换成假的
var (
	mediaPlaying = platformMediaPlaying
	mediaSend    = platformMediaSend
	mediaName    = func(pid int32) string { return processName(uint32(pid)) }
	mediaTick    = 250 * time.Millisecond
	// mediaSettle 暂停后过这么久还在出声，就算没停住。WebKit 里的播放器（MySpotify、Safari）停了以后
	// 约 2.5 秒才放开扬声器（实测），所以不能一暂停就看
	mediaSettle = 4 * time.Second
)

const (
	mediaPlay  = 0 // MRMediaRemoteCommand：kMRPlay
	mediaPause = 1 // kMRPause
)

type mediaHold struct {
	mu     sync.Mutex
	held   bool      // 发过暂停，还没收尾
	owners []int32   // 暂停时在出声的程序
	at     time.Time // 发暂停的时候
	gen    uint64    // 每次 hold 加一：作废还在等着的恢复
}

var media mediaHold

// hold 开始录音：有程序在出声就让它暂停。上一段暂停的还没恢复（连着说下一段）就接着停着
func (m *mediaHold) hold() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gen++
	if m.held {
		return
	}
	owners := mediaPlaying()
	if len(owners) == 0 {
		return
	}
	if !mediaSend(mediaPause) {
		log.Printf("voice: couldn't pause what's playing")
		return
	}
	m.held, m.owners, m.at = true, owners, time.Now()
	log.Printf("voice: paused what's playing (%s)", mediaNames(owners))
}

// release 录音结束：暂停时在出声的程序里有一个安静下来了（暂停管用了）就发「播放」。
// 不等全部安静：「暂停」只发给系统里正在放的那一个 app，别的出声的——远程桌面（UU 远程一直占着扬声器）、
// 通话、游戏——本来就不会停，等它们会永远放不回来。过了 mediaSettle 一个都没安静，说明暂停没管用，
// 就不发「播放」——不然会把你早先自己暂停的别的播放器放起来。等的时候又开始录，就接着停着
func (m *mediaHold) release() {
	m.mu.Lock()
	if !m.held {
		m.mu.Unlock()
		return
	}
	gen := m.gen
	m.mu.Unlock()
	go func() {
		for {
			m.mu.Lock()
			if m.gen != gen || !m.held {
				m.mu.Unlock()
				return
			}
			playing := mediaPlaying()
			var stopped, still []int32
			for _, p := range m.owners {
				if slices.Contains(playing, p) {
					still = append(still, p)
				} else {
					stopped = append(stopped, p)
				}
			}
			if len(stopped) > 0 || time.Since(m.at) >= mediaSettle {
				m.held = false
				m.mu.Unlock()
				if len(stopped) > 0 {
					mediaSend(mediaPlay)
					log.Printf("voice: resumed playing (%s had stopped)", mediaNames(stopped))
				} else {
					log.Printf("voice: still playing after the pause (%s); not resuming", mediaNames(still))
				}
				return
			}
			m.mu.Unlock()
			time.Sleep(mediaTick)
		}
	}()
}

// mediaNames 日志里写程序名（UURemote, MySpotify），读不到名字的写进程号
func mediaNames(pids []int32) string {
	names := make([]string, 0, len(pids))
	for _, p := range pids {
		n := strings.TrimSuffix(filepath.Base(mediaName(p)), ".app")
		if n == "" || n == "." {
			n = strconv.Itoa(int(p))
		}
		names = append(names, n)
	}
	return strings.Join(names, ", ")
}
