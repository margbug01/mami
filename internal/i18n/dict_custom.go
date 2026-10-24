package i18n

// 设置页「自定义」：网页胶囊样式和做样式的说明（en, ja, tw, ko, es, fr, de, ru）
func init() {
	entries := map[string][8]string{
		"胶囊样式":         {"Capsule styles", "カプセルスタイル", "膠囊樣式", "캡슐 스타일", "Estilos de cápsula", "Styles de capsule", "Kapsel-Stile", "Стили капсулы"},
		"给 AI 的 skill": {"Skills for AI", "AI 用の skill", "給 AI 的 skill", "AI용 skill", "Skills para IA", "Skills pour l'IA", "Skills für KI", "Skills для ИИ"},
		"复制给 AI，或者导出文件夹装进 Claude Code、Cursor 这类工具。": {"Copy one to your AI, or export the folder into tools like Claude Code or Cursor.", "AI にコピーするか、フォルダーを書き出して Claude Code や Cursor などに入れてください。", "複製給 AI，或者匯出資料夾裝進 Claude Code、Cursor 這類工具。", "AI에 복사하거나 폴더를 내보내 Claude Code, Cursor 같은 도구에 넣으세요.", "Cópialo a tu IA o exporta la carpeta a herramientas como Claude Code o Cursor.", "Copiez-le dans votre IA, ou exportez le dossier dans un outil comme Claude Code ou Cursor.", "Kopiere ihn in deine KI oder exportiere den Ordner in Werkzeuge wie Claude Code oder Cursor.", "Скопируйте в ИИ или экспортируйте папку в Claude Code, Cursor и подобные инструменты."},
		"导出": {"Export", "書き出す", "匯出", "내보내기", "Exportar", "Exporter", "Exportieren", "Экспорт"},
		"已建好 %s，复制胶囊样式的 skill 给 AI 改它": {"Created %s. Copy the capsule style skill to an AI to change it", "%s を作りました。カプセルスタイルの skill を AI にコピーして直してもらいましょう", "已建好 %s，複製膠囊樣式的 skill 給 AI 改它", "%s 을(를) 만들었습니다. 캡슐 스타일 skill을 AI에 복사해 고치게 하세요", "Creado %s. Copia el skill de estilos de cápsula a una IA para cambiarlo", "%s créé. Copiez le skill des styles de capsule dans une IA pour le modifier", "%s erstellt. Kopiere den Kapsel-Stil-Skill in eine KI zum Anpassen", "Создан %s. Скопируйте skill стилей капсулы в ИИ, чтобы изменить его"},
		"开发模式":          {"dev mode", "開発モード", "開發模式", "개발 모드", "modo desarrollo", "mode dév", "Entwicklermodus", "режим разработки"},
		"打开":            {"Open", "開く", "打開", "열기", "Abrir", "Ouvrir", "Öffnen", "Открыть"},
		"正在用":           {"In use", "使用中", "正在用", "사용 중", "En uso", "Utilisé", "Aktiv", "Используется"},
		"记得点保存":         {"Click Save to apply", "保存を押すと反映されます", "記得點儲存", "저장을 눌러 적용하세요", "Pulsa Guardar para aplicar", "Cliquez sur Enregistrer pour appliquer", "Zum Übernehmen auf Speichern klicken", "Нажмите «Сохранить», чтобы применить"},
		"新建样式":          {"New style", "新しいスタイル", "新建樣式", "새 스타일", "Nuevo estilo", "Nouveau style", "Neuer Stil", "Новый стиль"},
		"已复制，粘贴给 AI 就行": {"Copied. Paste it to your AI", "コピーしました。AI に貼り付けてください", "已複製，貼給 AI 就行", "복사했습니다. AI에 붙여 넣으세요", "Copiado. Pégalo en tu IA", "Copié. Collez-le dans votre IA", "Kopiert. Einfach der KI einfügen", "Скопировано. Вставьте в ИИ"},
		"新建失败：":         {"Couldn't create: ", "作成できませんでした：", "新建失敗：", "만들지 못했습니다: ", "No se pudo crear: ", "Création impossible : ", "Erstellen fehlgeschlagen: ", "Не удалось создать: "},
		"导出失败：":         {"Export failed: ", "書き出せませんでした：", "匯出失敗：", "내보내기 실패: ", "Error al exportar: ", "Échec de l'export : ", "Export fehlgeschlagen: ", "Ошибка экспорта: "},
		"已导出到 %s":       {"Exported to %s", "%s に書き出しました", "已匯出到 %s", "%s 에 내보냈습니다", "Exportado a %s", "Exporté vers %s", "Exportiert nach %s", "Экспортировано в %s"},
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
