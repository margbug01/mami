package i18n

// Deutsch
var dictDE = map[string]string{
	"接口": "API", "翻译": "Übersetzung", "快捷键": "Tastenkürzel", "划词按钮": "Auswahl-Button", "通用": "Allgemein", "保存": "Speichern",
	"接口（OpenAI 兼容）": "API (OpenAI-kompatibel)",

	"接口地址": "Endpunkt", "会自动拼上 /chat/completions": "/chat/completions wird automatisch angehängt", "免费接口留空": "Bei kostenlosen APIs leer lassen",
	"模型": "Modell", "测试连接": "Verbindung testen", "代理": "Proxy",
	"Go 不读系统代理，要走 Clash 得填这里": "Go ignoriert den Systemproxy; für Clash hier eintragen",
	"▸ 高级": "▸ Erweitert", "▾ 高级": "▾ Erweitert",
	"正在请求 ": "Anfrage an ", "连接失败：": "Verbindung fehlgeschlagen: ", "连接正常（%s）：你好，世界 → %s": "Verbindung OK (%s): 你好，世界 → %s",

	"中文翻成": "Chinesisch nach", "选中的文字里有汉字时": "Wenn die Auswahl chinesische Schriftzeichen enthält",
	"其他翻成": "Anderes nach", "不含汉字的文字（英文、日文等）": "Text ohne chinesische Schriftzeichen (Englisch, Japanisch usw.)",
	"额外要求": "Zusätzliche Anweisungen", "追加给模型的话，比如风格、术语处理": "Zusätzliche Hinweise an das Modell, etwa zu Stil oder Terminologie",
	"比如：技术文档风格，术语保留英文": "z. B. Stil technischer Dokumentation, Fachbegriffe auf Englisch lassen",
	"拖动调整高度，点一下恢复自动":   "Ziehen, um die Höhe zu ändern; Klick setzt auf automatisch zurück",

	"原地替换": "Direkt ersetzen", "写法如 Ctrl+Shift+Y、Alt+F2": "Zum Beispiel Strg+Shift+Y oder Alt+F2",
	"只弹卡片": "Nur Karte", "留空表示不用": "Leer lassen zum Deaktivieren",
	"快捷键没选中就全选":                  "Alles auswählen, wenn nichts markiert ist",
	"按 Alt+T 时没选中文字，先全选整个输入框再翻译": "Wenn bei Alt+T nichts markiert ist, wird zuerst das ganze Eingabefeld ausgewählt und dann übersetzt",
	"开机自启": "Beim Anmelden starten",
	"登录 Windows 后在托盘里自动运行": "Läuft nach der Windows-Anmeldung im Infobereich",

	"划词后显示翻译按钮":            "Übersetzen-Button nach der Auswahl anzeigen",
	"选中文字后旁边弹出一个小图标，点它出译文": "Neben der Auswahl erscheint ein kleines Symbol; ein Klick darauf übersetzt",
	"只在输入框里弹":              "Nur in Eingabefeldern",
	"网页正文、文档里划词不弹":         "Kein Button bei Auswahl in Webseitentext oder Dokumenten",
	"点翻译按钮后":               "Nach Klick auf Übersetzen",
	"直接替换出错时仍会弹卡片说明原因":     "Schlägt das direkte Ersetzen fehl, erklärt eine Karte den Grund",
	"不弹按钮的程序":              "Programme ohne Button",
	"进程名，逗号分隔":             "Prozessnamen, durch Kommas getrennt",
	"▸ 兼容性":                "▸ Kompatibilität", "▾ 兼容性": "▾ Kompatibilität",
	"认不出输入框的程序也弹":                  "Auch in Programmen anzeigen, die ihre Eingabefelder nicht preisgeben",
	"微信这类程序开着能用，但聊天记录里划词也会弹":       "Funktioniert in Programmen wie WeChat, der Button erscheint aber auch bei Auswahl im Chatverlauf",
	"只认文本光标":                       "Nur Textcursor",
	"按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹": "Zählt nur als Auswahl, wenn der Mauszeiger beim Drücken ein I-Cursor ist; Ziehen von Fenstern oder Dateien löst nichts aus",
	"显示卡片（先显示「翻译中…」）":              "Karte anzeigen (zuerst „Übersetze …“)",
	"不显示「翻译中」，出结果再弹":               "Kein „Übersetze“; anzeigen, sobald das Ergebnis da ist",
	"直接替换，不弹卡片":                    "Direkt ersetzen, ohne Karte",

	"界面语言": "Sprache", "设置窗口、卡片和托盘菜单用哪种语言": "Sprache für Einstellungsfenster, Karten und Infobereich-Menü",
	"外观": "Darstellung", "浅色、深色，或跟随 Windows": "Hell, dunkel oder wie Windows",
	"跟随系统": "Systemeinstellung", "浅色": "Hell", "深色": "Dunkel",

	"已保存": "Gespeichert", "保存失败：": "Speichern fehlgeschlagen: ", "已保存，但快捷键有问题：": "Gespeichert, aber mit Problemen bei den Tastenkürzeln: ", "：": ": ", "；": "; ",

	"翻译中": "Übersetze", "翻译中…": "Übersetze …", "复制": "Kopieren", "已复制": "Kopiert", "替换": "Ersetzen", "关闭": "Schließen",
	"中文 → ": "Chinesisch → ", "自动检测 → ": "Automatisch erkannt → ", "中文 → English": "Chinesisch → English",
	" · 窗口已切换，没有替换":              " · Fenster gewechselt, nicht ersetzt",
	" · 期间点了别处，没有替换":             " · Zwischenzeitlich woanders geklickt, nicht ersetzt",
	" · 终端里不替换":                  " · In Terminals wird nicht ersetzt",
	"选中了 %d 个字，超过上限 %d。":         "%d Zeichen ausgewählt, das Limit liegt bei %d.",
	"没取到选中的文字。这个窗口可能不支持 Ctrl+C。": "Der ausgewählte Text konnte nicht abgerufen werden. Dieses Fenster unterstützt Strg+C möglicherweise nicht.",
	"终端里要先用鼠标选中文字。为了不误发 Ctrl+C 中断正在跑的程序，终端里不会自动全选。": "Wähle im Terminal den Text zuerst mit der Maus aus. Damit kein Strg+C ein laufendes Programm unterbricht, wird dort nicht automatisch alles ausgewählt.",

	"设置…": "Einstellungen …", "打开日志": "Protokoll öffnen", "退出": "Beenden", " 原地替换": " direkt ersetzen",

	"proxy 地址不对：%w":                                 "Ungültige Proxy-Adresse: %w",
	"%s 格式不对：%w":                                    "%s ist ungültig: %w",
	"还没填 API Key：托盘图标右键 → 设置":                       "Noch kein API-Key: Rechtsklick auf das Symbol im Infobereich → Einstellungen",
	"请求失败：%w":                                       "Anfrage fehlgeschlagen: %w",
	"接口返回的不是 chat/completions 格式":                   "Die API hat nicht im chat/completions-Format geantwortet",
	"模型返回了空内容":                                      "Das Modell hat eine leere Antwort geliefert",
	"鼠标钩子挂载失败，只能用快捷键":                               "Maus-Hook konnte nicht installiert werden; nur Tastenkürzel funktionieren",
	"配置文件没读成功，保存会覆盖它：":                              "Konfigurationsdatei konnte nicht gelesen werden; Speichern überschreibt sie: ",
	"默认用 B 站免费的 Index-Translate-35B，不用 Key；换别的接口再填": "Standardmäßig wird das kostenlose Index-Translate-35B von Bilibili ohne Key verwendet; bei einer anderen API hier eintragen",
}
