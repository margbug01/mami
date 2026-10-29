package app

import (
	"slices"
	"sync"
	"testing"
	"time"
)

// fakePlayer 假的「谁在出声」和「暂停/播放」：暂停后 quietAfter 才安静（WebKit 那样）；deaf=暂停不管用（通话、游戏）；
// others 一直在出声、不接暂停的别的程序（UU 远程这种远程桌面一直占着扬声器）
type fakePlayer struct {
	mu         sync.Mutex
	playing    bool
	pausedAt   time.Time
	quietAfter time.Duration
	deaf       bool
	others     []int32
	sent       []int
}

func (f *fakePlayer) install(t *testing.T) {
	t.Helper()
	oldP, oldS, oldN, oldT, oldW := mediaPlaying, mediaSend, mediaName, mediaTick, mediaSettle
	t.Cleanup(func() {
		mediaPlaying, mediaSend, mediaName, mediaTick, mediaSettle = oldP, oldS, oldN, oldT, oldW
		media = mediaHold{}
	})
	mediaName = func(int32) string { return "" }
	media = mediaHold{}
	mediaTick, mediaSettle = 5*time.Millisecond, 300*time.Millisecond
	mediaPlaying = func() []int32 {
		f.mu.Lock()
		defer f.mu.Unlock()
		out := slices.Clone(f.others)
		if f.playing && (f.deaf || f.pausedAt.IsZero() || time.Since(f.pausedAt) < f.quietAfter) {
			out = append(out, 7)
		}
		return out
	}
	mediaSend = func(cmd int) bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sent = append(f.sent, cmd)
		switch {
		case cmd == mediaPause && !f.deaf:
			f.pausedAt = time.Now()
		case cmd == mediaPlay:
			f.playing, f.pausedAt = true, time.Time{}
		}
		return true
	}
}

func (f *fakePlayer) commands() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}

// settle 等后台的恢复跑完（最多 mediaSettle 再多一点）
func settle() { time.Sleep(mediaSettle + 100*time.Millisecond) }

func TestMediaNothingPlaying(t *testing.T) {
	f := &fakePlayer{}
	f.install(t)
	media.hold()
	media.release()
	settle()
	if got := f.commands(); len(got) != 0 {
		t.Fatalf("nothing was playing, sent %v", got)
	}
}

// 在放：开始录就暂停，说完等它真安静了再放
func TestMediaPausesAndResumes(t *testing.T) {
	f := &fakePlayer{playing: true, quietAfter: 60 * time.Millisecond}
	f.install(t)
	media.hold()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("on start sent %v, want [pause]", got)
	}
	media.release() // 刚停就说完了：WebKit 还占着扬声器，得等它安静
	time.Sleep(20 * time.Millisecond)
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("resumed before it went quiet: %v", got)
	}
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause, mediaPlay}) {
		t.Fatalf("sent %v, want [pause play]", got)
	}
}

// 暂停不管用（通话、游戏一直在出声）：不发播放，免得把别的早先暂停的播放器放起来
func TestMediaNotResumedWhenPauseDidNothing(t *testing.T) {
	f := &fakePlayer{playing: true, deaf: true}
	f.install(t)
	media.hold()
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("sent %v, want only [pause]", got)
	}
}

// 连着说两段：第一段说完、还没恢复就又开始录，中间不能放起来，也不再暂停一次；最后只放一次
func TestMediaStaysPausedBetweenTakes(t *testing.T) {
	f := &fakePlayer{playing: true, quietAfter: 60 * time.Millisecond}
	f.install(t)
	media.hold()
	media.release()
	time.Sleep(10 * time.Millisecond)
	media.hold()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("between takes sent %v, want only [pause]", got)
	}
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause, mediaPlay}) {
		t.Fatalf("sent %v, want [pause play]", got)
	}
}

// 录的时候自己又点了播放：说完它在出声，不再发播放
func TestMediaLeftAloneWhenResumedByHand(t *testing.T) {
	f := &fakePlayer{playing: true, quietAfter: 10 * time.Millisecond}
	f.install(t)
	media.hold()
	time.Sleep(mediaSettle + 20*time.Millisecond) // 录了一会儿，早就安静了
	f.mu.Lock()
	f.pausedAt = time.Time{} // 用户自己点了播放
	f.mu.Unlock()
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("sent %v, want only [pause]", got)
	}
}

// 旁边有个一直出声、不接暂停的程序（UU 远程）：音乐停了就该放回来，不能等它也安静（它永远不会）
func TestMediaResumesBesideAnAlwaysOnApp(t *testing.T) {
	f := &fakePlayer{playing: true, quietAfter: 60 * time.Millisecond, others: []int32{9}}
	f.install(t)
	media.hold()
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause, mediaPlay}) {
		t.Fatalf("sent %v, want [pause play]", got)
	}
}

// 只有那个一直出声的程序、音乐本来没在放：暂停（不碍事）之后什么都没安静，不发播放
func TestMediaNotResumedWhenOnlyAnAlwaysOnAppPlays(t *testing.T) {
	f := &fakePlayer{others: []int32{9}}
	f.install(t)
	media.hold()
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("sent %v, want only [pause]", got)
	}
}

// 录的时候自己又点了播放、旁边还有一直出声的：说完谁都没安静，不再发播放
func TestMediaLeftAloneWhenResumedByHandBesideAnAlwaysOnApp(t *testing.T) {
	f := &fakePlayer{playing: true, quietAfter: 10 * time.Millisecond, others: []int32{9}}
	f.install(t)
	media.hold()
	time.Sleep(mediaSettle + 20*time.Millisecond)
	f.mu.Lock()
	f.pausedAt = time.Time{}
	f.mu.Unlock()
	media.release()
	settle()
	if got := f.commands(); !slices.Equal(got, []int{mediaPause}) {
		t.Fatalf("sent %v, want only [pause]", got)
	}
}

func TestMediaNames(t *testing.T) {
	old := mediaName
	t.Cleanup(func() { mediaName = old })
	mediaName = func(p int32) string {
		return map[int32]string{1: "/Applications/UURemote.app", 2: "/Applications/MySpotify.app"}[p]
	}
	if got := mediaNames([]int32{1, 2, 3}); got != "UURemote, MySpotify, 3" {
		t.Fatalf("got %q", got)
	}
}
