package i18n

func init() {
	entries := map[string][8]string{
		"收起时":    {"Collapsed", "折りたたみ時", "收起時", "접었을 때", "Plegado", "Replié", "Eingeklappt", "Свёрнуто"},
		"悬停展开":   {"Expanded on hover", "ホバーで展開", "懸停展開", "마우스를 올리면 확장", "Expandido al pasar el cursor", "Déplié au survol", "Bei Mauszeigerkontakt ausgeklappt", "Развёрнуто при наведении"},
		"展开面板外观": {"Expanded panel", "展開パネルの外観", "展開面板外觀", "확장 패널 모양", "Panel desplegado", "Panneau déplié", "Ausgeklapptes Panel", "Развёрнутая панель"},
		"录音浮窗":   {"Recording overlay", "録音フロート", "錄音浮窗", "녹음 오버레이", "Ventana de grabación", "Fenêtre d’enregistrement", "Aufnahmefenster", "Окно записи"},
		"浮窗样式":   {"Overlay style", "フロートのスタイル", "浮窗樣式", "오버레이 스타일", "Estilo de ventana", "Style de fenêtre", "Fensterstil", "Стиль окна"},
		"球体配色":   {"Orb colors", "オーブの配色", "球體配色", "구체 색상", "Colores del orbe", "Couleurs de l’orbe", "Kugelfarben", "Цвета сферы"},
		"浮窗外观":   {"Overlay appearance", "フロートの外観", "浮窗外觀", "오버레이 모양", "Aspecto de ventana", "Apparence de fenêtre", "Fensterdarstellung", "Оформление окна"},
		"结果条外观":  {"Result bar", "結果バーの外観", "結果條外觀", "결과 표시줄 모양", "Barra de resultados", "Barre de résultat", "Ergebnisleiste", "Панель результата"},
		"结果待复制":  {"Ready to copy", "コピー待ち", "結果待複製", "복사할 결과", "Listo para copiar", "Prêt à copier", "Bereit zum Kopieren", "Готово к копированию"},
	}
	for key, translations := range entries {
		Dict[key] = [2]string{translations[0], translations[1]}
		for index, language := range []int{TW, KO, ES, FR, DE, RU} {
			dicts[language][key] = translations[index+2]
		}
	}
}
