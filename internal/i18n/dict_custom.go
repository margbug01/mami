package i18n

// 设置页「自定义」：网页胶囊样式和做样式的说明（en, ja, tw, ko, es, fr, de, ru）
func init() {
	entries := map[string][8]string{
		"我的胶囊样式": {"My capsule styles", "マイカプセルスタイル", "我的膠囊樣式", "내 캡슐 스타일", "Mis estilos de cápsula", "Mes styles de capsule", "Meine Kapsel-Stile", "Мои стили капсулы"},
		"还没有。点「新建样式」从模板开始，或者把别人做好的样式文件夹放进样式文件夹。": {
			"None yet. Click New style to start from the template, or drop a style folder someone made into the styles folder.",
			"まだありません。「新しいスタイル」でテンプレートから始めるか、誰かが作ったスタイルのフォルダーをスタイルフォルダーに入れてください。",
			"還沒有。點「新建樣式」從範本開始，或者把別人做好的樣式資料夾放進樣式資料夾。",
			"아직 없습니다. '새 스타일'로 템플릿에서 시작하거나, 다른 사람이 만든 스타일 폴더를 스타일 폴더에 넣으세요.",
			"Aún no hay. Pulsa Nuevo estilo para empezar desde la plantilla, o pon en la carpeta de estilos la carpeta de un estilo hecho por otra persona.",
			"Aucun pour l'instant. Cliquez sur Nouveau style pour partir du modèle, ou déposez le dossier d'un style dans le dossier des styles.",
			"Noch keine. Mit „Neuer Stil“ von der Vorlage starten oder den Ordner eines fertigen Stils in den Stilordner legen.",
			"Пока нет. Нажмите «Новый стиль», чтобы начать с шаблона, или положите папку готового стиля в папку стилей.",
		},
		"开发模式":   {"dev mode", "開発モード", "開發模式", "개발 모드", "modo desarrollo", "mode dév", "Entwicklermodus", "режим разработки"},
		"打开":     {"Open", "開く", "打開", "열기", "Abrir", "Ouvrir", "Öffnen", "Открыть"},
		"正在用":    {"In use", "使用中", "正在用", "사용 중", "En uso", "Utilisé", "Aktiv", "Используется"},
		"用这个":    {"Use", "使う", "用這個", "사용", "Usar", "Utiliser", "Verwenden", "Выбрать"},
		"记得点保存":  {"Click Save to apply", "保存を押すと反映されます", "記得點儲存", "저장을 눌러 적용하세요", "Pulsa Guardar para aplicar", "Cliquez sur Enregistrer pour appliquer", "Zum Übernehmen auf Speichern klicken", "Нажмите «Сохранить», чтобы применить"},
		"新建样式":   {"New style", "新しいスタイル", "新建樣式", "새 스타일", "Nuevo estilo", "Nouveau style", "Neuer Stil", "Новый стиль"},
		"做样式的说明": {"How to make a style", "スタイルの作り方", "做樣式的說明", "스타일 만드는 법", "Cómo hacer un estilo", "Créer un style", "So baust du einen Stil", "Как сделать стиль"},
		"胶囊就是一张透明网页，想怎么画都行。把这份说明交给 Claude、Cursor 之类的 AI，说清想要什么样子，它就能照着写；也可以导出整个 skill 文件夹装进 AI 工具。": {
			"The capsule is just a transparent web page, so you can draw anything. Give this guide to an AI such as Claude or Cursor, describe the look you want, and it can write it; or export the whole skill folder into your AI tool.",
			"カプセルは透明な Web ページなので、何でも描けます。このガイドを Claude や Cursor などの AI に渡して欲しい見た目を伝えれば作ってくれます。skill フォルダーごと書き出して AI ツールに入れることもできます。",
			"膠囊就是一張透明網頁，想怎麼畫都行。把這份說明交給 Claude、Cursor 之類的 AI，說清想要什麼樣子，它就能照著寫；也可以匯出整個 skill 資料夾裝進 AI 工具。",
			"캡슐은 투명한 웹 페이지라서 무엇이든 그릴 수 있습니다. 이 안내를 Claude나 Cursor 같은 AI에 주고 원하는 모습을 말하면 만들어 줍니다. skill 폴더 전체를 내보내 AI 도구에 넣을 수도 있습니다.",
			"La cápsula es solo una página web transparente: puedes dibujar lo que quieras. Pasa esta guía a una IA como Claude o Cursor, describe el aspecto que buscas y la escribirá; o exporta la carpeta del skill a tu herramienta de IA.",
			"La capsule n'est qu'une page web transparente : tout est permis. Donnez ce guide à une IA comme Claude ou Cursor, décrivez le rendu voulu et elle l'écrira ; ou exportez tout le dossier du skill dans votre outil d'IA.",
			"Die Kapsel ist einfach eine transparente Webseite – alles ist möglich. Gib diese Anleitung einer KI wie Claude oder Cursor, beschreib den gewünschten Look, und sie schreibt ihn; oder exportiere den ganzen Skill-Ordner in dein KI-Werkzeug.",
			"Капсула — это просто прозрачная веб-страница, рисовать можно что угодно. Отдайте это руководство ИИ вроде Claude или Cursor, опишите нужный вид — и он напишет; или экспортируйте папку skill целиком в свой ИИ-инструмент.",
		},
		"复制说明":          {"Copy guide", "ガイドをコピー", "複製說明", "안내 복사", "Copiar guía", "Copier le guide", "Anleitung kopieren", "Скопировать руководство"},
		"已复制，粘贴给 AI 就行": {"Copied. Paste it to your AI", "コピーしました。AI に貼り付けてください", "已複製，貼給 AI 就行", "복사했습니다. AI에 붙여 넣으세요", "Copiado. Pégalo en tu IA", "Copié. Collez-le dans votre IA", "Kopiert. Einfach der KI einfügen", "Скопировано. Вставьте в ИИ"},
		"导出 skill 文件夹":  {"Export skill folder", "skill フォルダーを書き出す", "匯出 skill 資料夾", "skill 폴더 내보내기", "Exportar carpeta del skill", "Exporter le dossier du skill", "Skill-Ordner exportieren", "Экспортировать папку skill"},
		"新建失败：":         {"Couldn't create: ", "作成できませんでした：", "新建失敗：", "만들지 못했습니다: ", "No se pudo crear: ", "Création impossible : ", "Erstellen fehlgeschlagen: ", "Не удалось создать: "},
		"已建好 %s，把「做样式的说明」交给 AI 改它": {
			"Created %s. Give “How to make a style” to an AI to change it",
			"%s を作りました。「スタイルの作り方」を AI に渡して直してもらいましょう",
			"已建好 %s，把「做樣式的說明」交給 AI 改它",
			"%s 을(를) 만들었습니다. '스타일 만드는 법'을 AI에 주고 고치게 하세요",
			"Creado %s. Pasa «Cómo hacer un estilo» a una IA para cambiarlo",
			"%s créé. Donnez « Créer un style » à une IA pour le modifier",
			"%s erstellt. Gib „So baust du einen Stil“ einer KI zum Anpassen",
			"Создан %s. Отдайте «Как сделать стиль» ИИ, чтобы изменить его",
		},
		"导出失败：":   {"Export failed: ", "書き出せませんでした：", "匯出失敗：", "내보내기 실패: ", "Error al exportar: ", "Échec de l'export : ", "Export fehlgeschlagen: ", "Ошибка экспорта: "},
		"已导出到 %s": {"Exported to %s", "%s に書き出しました", "已匯出到 %s", "%s 에 내보냈습니다", "Exportado a %s", "Exporté vers %s", "Exportiert nach %s", "Экспортировано в %s"},
		"自定义的网页样式，效果在说话时看；在「自定义」里管理和新建。": {
			"A custom web style: you'll see it when you speak. Manage or create styles under Custom.",
			"カスタムの Web スタイルです。話したときに表示されます。管理や作成は「カスタム」で。",
			"自訂的網頁樣式，效果在說話時看；在「自訂」裡管理和新建。",
			"사용자 웹 스타일입니다. 말할 때 보입니다. '사용자 지정'에서 관리하고 만듭니다.",
			"Estilo web personalizado: lo verás al hablar. Gestiona o crea estilos en Personalizado.",
			"Style web personnalisé : visible quand vous parlez. Gérez ou créez des styles dans Personnalisé.",
			"Eigener Web-Stil: zu sehen, wenn du sprichst. Verwalten und anlegen unter Benutzerdefiniert.",
			"Свой веб-стиль: его видно, когда вы говорите. Управлять и создавать — в разделе «Своё».",
		},
		"去自定义": {"Open Custom", "カスタムへ", "去自訂", "사용자 지정으로", "Ir a Personalizado", "Aller à Personnalisé", "Zu Benutzerdefiniert", "Перейти в «Своё»"},
		"传给网页，样式自己决定怎么用": {"Passed to the page; the style decides how to use it", "ページに渡され、使い方はスタイル次第です", "傳給網頁，樣式自己決定怎麼用", "페이지에 전달되며 사용 방식은 스타일이 정합니다", "Se pasa a la página; el estilo decide cómo usarlo", "Transmis à la page ; le style décide comment l'utiliser", "Wird an die Seite übergeben; der Stil entscheidet, wie", "Передаётся странице; стиль сам решает, как использовать"},
	}
	for key, translations := range entries {
		Dict[key] = [2]string{translations[0], translations[1]}
		for index, language := range []int{TW, KO, ES, FR, DE, RU} {
			dicts[language][key] = translations[index+2]
		}
	}
}
