package app

func syncDockDraft(draft *Config, before, after Config) {
	if before.VoiceDock != after.VoiceDock {
		draft.VoiceDock = after.VoiceDock
	}
	if before.VoiceDockTone != after.VoiceDockTone {
		draft.VoiceDockTone = after.VoiceDockTone
	}
	if before.VoiceDockPosition != after.VoiceDockPosition {
		draft.VoiceDockPosition = after.VoiceDockPosition
	}
	if before.VoiceTranslate != after.VoiceTranslate {
		draft.VoiceTranslate = after.VoiceTranslate
	}
	if before.VoiceTarget != after.VoiceTarget {
		draft.VoiceTarget = after.VoiceTarget
	}
	if before.PolishStyle != after.PolishStyle {
		draft.PolishStyle = after.PolishStyle
	}
	if before.PolishAuto != after.PolishAuto {
		draft.PolishAuto = after.PolishAuto
	}
	if before.VoiceLiveText != after.VoiceLiveText {
		draft.VoiceLiveText = after.VoiceLiveText
	}
	if before.VoiceLearn != after.VoiceLearn {
		draft.VoiceLearn = after.VoiceLearn
	}
	if before.VoiceRewrite != after.VoiceRewrite {
		draft.VoiceRewrite = after.VoiceRewrite
	}
	if before.VoiceSound != after.VoiceSound {
		draft.VoiceSound = after.VoiceSound
	}
	if before.VoiceStyle != after.VoiceStyle {
		draft.VoiceStyle = after.VoiceStyle
	}
	if before.VoiceTone != after.VoiceTone {
		draft.VoiceTone = after.VoiceTone
	}
}

// voiceStyleName / voiceToneName 贴边面板里显示的名字，和设置页同一套叫法
func voiceStyleName(s string) string {
	switch s {
	case "orb":
		return T("果冻球")
	case "glass":
		return T("磨砂玻璃")
	case "pixel":
		return T("复古像素")
	case "mercury":
		return T("水银")
	case "islands":
		return T("三岛连桥")
	case "leds":
		return T("分段电平")
	}
	return T("渐变波柱")
}

func voiceToneName(s string) string {
	switch s {
	case "light":
		return T("浅色")
	case "auto":
		return T("跟随主题")
	}
	return T("深色")
}
