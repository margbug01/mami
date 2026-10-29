package app

// Windows 还没做「录音时暂停播放」（设置里也不显示）：要从音频会话判断谁在出声，再找系统媒体控制发暂停
func platformMediaPlaying() []int32 { return nil }

func platformMediaSend(int) bool { return false }
