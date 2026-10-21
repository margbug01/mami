package app

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSampleOf(t *testing.T) {
	pasted := "请帮我写一个函数，把用户上传的图片压缩到一百 KB 以内，然后保存到对象存储里，返回访问链接。"
	edited := "请帮我写一个 Go 函数，把用户上传的图片压缩到 100KB 以内，然后存到 S3，返回访问链接。"
	final, changed, ok := sampleOf(pasted, "前面别的字"+edited+"后面接着打的")
	if !ok || final != edited || changed == 0 || !worthSample(pasted, final, changed) {
		t.Fatalf("edited: ok=%v changed=%d final=%q", ok, changed, final)
	}
	if _, _, ok := sampleOf(pasted, ""); ok {
		t.Error("empty box (already sent) must not count")
	}
	if _, _, ok := sampleOf(pasted, "晚上吃什么，要不要一起去楼下那家面馆"); ok {
		t.Error("unrelated text must not count")
	}
	if f, c, _ := sampleOf("好的明天见。", "好的，明天见！"); worthSample("好的明天见。", f, c) {
		t.Error("short chat must not be kept as a sample")
	}
	if f, c, _ := sampleOf(pasted, pasted); worthSample(pasted, f, c) {
		t.Error("unchanged text must not be kept")
	}
}

func TestSampleMinimumLength(t *testing.T) {
	for _, length := range []int{9, 10, 11, 39, 40, 1500, 1501} {
		pasted := strings.Repeat("字", length-1) + "甲"
		edited := strings.Repeat("字", length-1) + "乙"
		final, changed, ok := sampleOf(pasted, edited)
		want := length >= 10 && length <= 1500
		if got := ok && worthSample(pasted, final, changed); got != want {
			t.Errorf("length %d: got %v, want %v", length, got, want)
		}
		if worthSample(pasted, pasted, 0) {
			t.Errorf("length %d: unchanged text must not be kept", length)
		}
	}
}

func TestSampleLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.jsonl")
	s := newSampleLog(path)
	now := time.Now()
	old := rewriteSample{Time: now.Add(-sampleMaxAge - time.Hour).Format(time.RFC3339), Pasted: "旧", Final: "旧的"}
	if err := s.add(old, now.Add(-sampleMaxAge-time.Hour)); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < sampleKeep+3; i++ {
		if err := s.add(rewriteSample{Time: now.Format(time.RFC3339), Pasted: "p", Final: fmt.Sprint(i)}, now); err != nil {
			t.Fatal(err)
		}
	}
	all := newSampleLog(path).load()
	if len(all) != sampleKeep || s.count() != sampleKeep || newSampleLog(path).count() != sampleKeep {
		t.Fatalf("kept %d, count %d", len(all), s.count())
	}
	if all[0].Final != "3" || strings.Contains(all[len(all)-1].Final, "旧") {
		t.Fatalf("oldest kept %q", all[0].Final)
	}
	if err := s.clear(); err != nil || s.count() != 0 || len(s.load()) != 0 {
		t.Fatal("clear")
	}
}
