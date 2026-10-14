package i18n

// i18nDict：中文原文 → {英文, 日文}。
// 新增界面文字时，先在代码里写 T("中文")，再回来补这张表；
// TestI18nDictComplete 会扫源码，漏了会失败。
var Dict = map[string][2]string{
	// 侧边栏和页面
	"接口":            {"API", "接続"},
	"翻译":            {"Translation", "翻訳"},
	"快捷键":           {"Shortcuts", "ショートカット"},
	"划词按钮":          {"Selection button", "選択ボタン"},
	"通用":            {"General", "一般"},
	"保存":            {"Save", "保存"},
	"接口（OpenAI 兼容）": {"API (OpenAI compatible)", "API（OpenAI 互換）"},

	// 接口页
	"接口地址":                    {"Endpoint", "エンドポイント"},
	"会自动拼上 /chat/completions": {"/chat/completions is appended automatically", "/chat/completions は自動で付きます"},
	"免费接口留空":                  {"Leave empty for free endpoints", "無料の API なら空のままで"},
	"模型":                      {"Model", "モデル"},
	"测试连接":                    {"Test connection", "接続テスト"},
	"代理":                      {"Proxy", "プロキシ"},
	"Go 不读系统代理，要走 Clash 得填这里": {"Go ignores the system proxy; set it here to go through Clash", "Go はシステムのプロキシを使いません。Clash 経由ならここに入力"},
	"▸ 高级":  {"▸ Advanced", "▸ 詳細設定"},
	"▾ 高级":  {"▾ Advanced", "▾ 詳細設定"},
	"正在请求 ": {"Requesting ", "リクエスト中 "},
	"连接失败：": {"Connection failed: ", "接続に失敗しました："},
	"连接正常（%s）：你好，世界 → %s": {"Connected (%s): 你好，世界 → %s", "接続OK（%s）：你好，世界 → %s"},

	// 翻译页
	"中文翻成":       {"Chinese to", "中国語を翻訳"},
	"选中的文字里有汉字时": {"When the selection contains Chinese characters", "選択した文字に漢字が含まれるとき"},
	"其他翻成":       {"Others to", "それ以外を翻訳"},
	"不含汉字的文字（英文、日文等）": {"Text without Chinese characters (English, Japanese, etc.)", "漢字を含まない文字（英語・日本語など）"},
	"额外要求": {"Extra instructions", "追加の指示"},
	"追加给模型的话，比如风格、术语处理": {"Extra words for the model, such as style or terminology", "モデルへの補足。文体や用語の扱いなど"},
	"比如：技术文档风格，术语保留英文":  {"e.g. technical-doc style, keep terms in English", "例：技術文書の文体、用語は英語のまま"},
	"拖动调整高度，点一下恢复自动":    {"Drag to resize, click to reset to auto", "ドラッグで高さを調整、クリックで自動に戻す"},

	// 快捷键页
	"原地替换":                    {"Replace in place", "その場で置換"},
	"写法如 Ctrl+Shift+Y、Alt+F2": {"Like Ctrl+Shift+Y or Alt+F2", "例：Ctrl+Shift+Y、Alt+F2"},
	"只弹卡片":                    {"Card only", "カードのみ表示"},
	"留空表示不用":                  {"Leave empty to disable", "空欄で無効"},
	"快捷键没选中就全选":               {"Select all if nothing is selected", "未選択なら全選択"},
	"按 Alt+T 时没选中文字，先全选整个输入框再翻译": {"When Alt+T is pressed with nothing selected, select the whole box first", "Alt+T を押したとき何も選択されていなければ、入力欄全体を選択してから翻訳"},
	"开机自启": {"Start at login", "ログイン時に起動"},
	"登录 Windows 后在托盘里自动运行": {"Runs in the tray after you sign in to Windows", "Windows にサインイン後、トレイで自動的に実行"},

	// 划词按钮页
	"划词后显示翻译按钮":              {"Show translate button after selecting", "選択後に翻訳ボタンを表示"},
	"选中文字后旁边弹出一个小图标，点它出译文":   {"A small icon appears next to the selection; click it to translate", "選択した文字の横に小さなアイコンが出ます。クリックで翻訳"},
	"只在输入框里弹":                {"Only in input boxes", "入力欄でのみ表示"},
	"网页正文、文档里划词不弹":           {"No button when selecting page text or documents", "ページ本文やドキュメントでは表示しない"},
	"点翻译按钮后":                 {"After clicking translate", "翻訳ボタンを押した後"},
	"直接替换出错时仍会弹卡片说明原因":       {"If direct replace fails, a card still explains why", "直接置換に失敗したときは、理由をカードで表示します"},
	"不弹按钮的程序":                {"Programs without the button", "ボタンを出さないアプリ"},
	"进程名，逗号分隔":               {"Process names, comma separated", "プロセス名をカンマ区切りで"},
	"▸ 兼容性":                  {"▸ Compatibility", "▸ 互換性"},
	"▾ 兼容性":                  {"▾ Compatibility", "▾ 互換性"},
	"认不出输入框的程序也弹":            {"Also show in apps that hide their input boxes", "入力欄を判別できないアプリでも表示"},
	"微信这类程序开着能用，但聊天记录里划词也会弹": {"Works in apps like WeChat, but the button also appears when selecting chat history", "WeChat などで使えますが、チャット履歴の選択でもボタンが出ます"},
	"只认文本光标":                 {"Text cursor only", "テキストカーソルのみ"},
	"按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹": {"Only counts as a selection when the pointer is an I-beam, so dragging windows or files won't trigger it", "マウスを押したときカーソルが I 形の場合のみ選択とみなします。ウィンドウやファイルのドラッグでは出ません"},
	"显示卡片（先显示「翻译中…」）":              {"Show a card (with \"Translating…\" first)", "カードを表示（先に「翻訳中…」）"},
	"不显示「翻译中」，出结果再弹":               {"No \"Translating\" card; show the result when ready", "「翻訳中」は出さず、結果が出てから表示"},
	"直接替换，不弹卡片":                    {"Replace directly, no card", "カードを出さず直接置換"},

	// 通用页
	"界面语言": {"Language", "表示言語"},
	"设置窗口、卡片和托盘菜单用哪种语言": {"Language for the settings window, cards and tray menu", "設定画面・カード・トレイメニューの言語"},
	"外观":                {"Appearance", "外観"},
	"浅色、深色，或跟随 Windows": {"Light, dark, or follow Windows", "ライト、ダーク、または Windows に合わせる"},
	"跟随系统":              {"Follow system", "システムに合わせる"},
	"浅色":                {"Light", "ライト"},
	"深色":                {"Dark", "ダーク"},

	// 保存状态
	"已保存":          {"Saved", "保存しました"},
	"保存失败：":        {"Save failed: ", "保存に失敗しました："},
	"已保存，但快捷键有问题：": {"Saved, but some shortcuts have problems: ", "保存しました。ただしショートカットに問題があります："},
	"：":            {": ", "："},
	"；":            {"; ", "；"},

	// 按钮和卡片
	"翻译中":            {"Translating", "翻訳中"},
	"翻译中…":           {"Translating…", "翻訳中…"},
	"复制":             {"Copy", "コピー"},
	"已复制":            {"Copied", "コピーしました"},
	"替换":             {"Replace", "置換"},
	"关闭":             {"Close", "閉じる"},
	"中文 → ":          {"Chinese → ", "中国語 → "},
	"自动检测 → ":        {"Auto-detect → ", "自動判別 → "},
	"中文 → English":   {"Chinese → English", "中国語 → English"},
	" · 窗口已切换，没有替换":  {" · Window changed, not replaced", " · ウィンドウが切り替わったため置換しませんでした"},
	" · 期间点了别处，没有替换": {" · You clicked elsewhere, not replaced", " · 他をクリックしたため置換しませんでした"},
	" · 终端里不替换":      {" · No replace in terminals", " · ターミナルでは置換しません"},
	"选中了 %d 个字，超过上限 %d。":                            {"%d characters selected, over the limit of %d.", "%d 文字を選択しました。上限 %d を超えています。"},
	"没取到选中的文字。这个窗口可能不支持 Ctrl+C。":                    {"Couldn't get the selected text. This window may not support Ctrl+C.", "選択した文字を取得できませんでした。このウィンドウは Ctrl+C に対応していない可能性があります。"},
	"终端里要先用鼠标选中文字。为了不误发 Ctrl+C 中断正在跑的程序，终端里不会自动全选。": {"In a terminal, select the text with the mouse first. To avoid sending Ctrl+C and interrupting a running program, Mami never auto-selects there.", "ターミナルでは先にマウスで文字を選択してください。実行中のプログラムを Ctrl+C で中断しないよう、自動全選択は行いません。"},

	// 托盘
	"设置…":   {"Settings…", "設定…"},
	"打开日志":  {"Open log", "ログを開く"},
	"退出":    {"Quit", "終了"},
	" 原地替换": {" replace in place", " でその場置換"},

	// 错误和启动提示
	"proxy 地址不对：%w":                                 {"Invalid proxy address: %w", "プロキシのアドレスが正しくありません：%w"},
	"%s 格式不对：%w":                                    {"%s is not valid: %w", "%s の形式が正しくありません：%w"},
	"还没填 API Key：托盘图标右键 → 设置":                       {"No API key yet: right-click the tray icon → Settings", "API キーが未入力です：トレイアイコンを右クリック → 設定"},
	"请求失败：%w":                                       {"Request failed: %w", "リクエストに失敗しました：%w"},
	"接口返回的不是 chat/completions 格式":                   {"The endpoint didn't return chat/completions format", "API が chat/completions 形式で返しませんでした"},
	"模型返回了空内容":                                      {"The model returned nothing", "モデルの応答が空でした"},
	"鼠标钩子挂载失败，只能用快捷键":                               {"Mouse hook failed; only shortcuts will work", "マウスフックに失敗しました。ショートカットのみ使えます"},
	"配置文件没读成功，保存会覆盖它：":                              {"Couldn't read the config file; saving will overwrite it: ", "設定ファイルを読み込めませんでした。保存すると上書きされます："},
	"默认用 B 站免费的 Index-Translate-35B，不用 Key；换别的接口再填": {"Uses Bilibili's free Index-Translate-35B by default, no key needed; fill this in if you switch endpoints", "既定は Bilibili の無料 Index-Translate-35B で、キー不要です。別の API を使う場合は入力してください"},
	"启用快捷键":                                         {"Enable shortcuts", "ショートカットを有効にする"},
	"关掉后不注册任何全局快捷键，不会和别的程序抢键；划词按钮照常能用": {"Registers no global shortcuts when off, so nothing clashes with other programs; the selection button still works", "オフにするとグローバルショートカットを一切登録せず、他のアプリと競合しません。選択ボタンは引き続き使えます"},
}
