package i18n

// 语音页的新文案与现有各语言词典保持对应。
func init() {
	entries := map[string][6]string{
		"样式":         {"樣式", "스타일", "Estilo", "Style", "Stil", "Стиль"},
		"录音时屏幕底部的样子": {"錄音時螢幕底部的樣子", "녹음 중 화면 아래에 표시되는 모양", "Lo que aparece abajo mientras grabas", "Ce qui s'affiche en bas pendant l'enregistrement", "Anzeige unten am Bildschirm während der Aufnahme", "Что видно внизу экрана во время записи"},
		"渐变波柱":       {"漸層波柱", "그라데이션 막대", "Barras degradadas", "Barres en dégradé", "Verlaufsbalken", "Градиентные столбики"},
		"果冻球":        {"果凍球", "젤리 구슬", "Bola de gelatina", "Bulle de gelée", "Gelee-Kugel", "Желейный шарик"},
		"提示音":        {"提示音", "효과음", "Sonidos", "Sons", "Töne", "Звуки"},
		"开始、结束、取消时轻响一声": {"開始、結束、取消時輕響一聲", "시작, 완료, 취소 시 부드러운 알림음", "Un sonido suave al empezar, terminar y cancelar", "Un léger carillon au début, à la fin et à l'annulation", "Ein leiser Ton bei Start, Ende und Abbruch", "Тихий сигнал при начале, завершении и отмене"},
		"点一下完成，Esc 取消":  {"點一下完成，Esc 取消", "클릭하면 완료, Esc는 취소", "Haz clic para terminar, Esc para cancelar", "Cliquez pour terminer, Échap pour annuler", "Klicken zum Beenden, Esc zum Abbrechen", "Щёлкните, чтобы закончить, Esc — отмена"},
		"取消":            {"取消", "취소", "Cancelar", "Annuler", "Abbrechen", "Отмена"},
		"完成":            {"完成", "완료", "Listo", "Terminé", "Fertig", "Готово"},
		"语音输入":          {"語音輸入", "음성 입력", "Entrada de voz", "Saisie vocale", "Spracheingabe", "Голосовой ввод"},
		"启用语音输入":        {"啟用語音輸入", "음성 입력 사용", "Activar entrada de voz", "Activer la saisie vocale", "Spracheingabe aktivieren", "Включить голосовой ввод"},
		"按一下开始说话，再按一下贴入；Esc 取消": {"按一下開始說話，再按一下貼入；Esc 取消", "한 번 눌러 말하고 다시 누르면 붙여넣기; Esc는 취소", "Pulsa una vez para hablar y otra para pegar; Esc cancela", "Appuyez une fois pour parler, encore pour coller ; Échap annule", "Einmal drücken zum Sprechen, nochmal zum Einfügen; Esc bricht ab", "Нажмите раз для записи, ещё раз для вставки; Esc отменяет"},
		"触发键":              {"觸發鍵", "실행 키", "Tecla de voz", "Touche vocale", "Sprechtaste", "Клавиша записи"},
		"快捷键总开关关闭时不响应":     {"快捷鍵總開關關閉時不回應", "전역 단축키가 꺼지면 작동하지 않음", "Inactivo si los atajos globales están desactivados", "Inactif si les raccourcis globaux sont désactivés", "Bei deaktivierten globalen Tastenkürzeln inaktiv", "Не работает при отключённых горячих клавишах"},
		"语音接口地址":           {"語音介面位址", "음성 API 주소", "URL de ASR", "URL de reconnaissance vocale", "ASR-Adresse", "Адрес API распознавания"},
		"语音始终直连，不使用代理":     {"語音一律直連，不使用代理", "음성은 프록시 없이 직접 연결", "La voz se conecta directamente, sin proxy", "La voix se connecte directement, sans proxy", "Sprache verbindet sich direkt, ohne Proxy", "Голос подключается напрямую, без прокси"},
		"热词":               {"熱詞", "인식 용어", "Palabras clave", "Termes favoris", "Erkennungsbegriffe", "Ключевые слова"},
		"逗号或换行分隔":          {"以逗號或換行分隔", "쉼표 또는 줄바꿈으로 구분", "Separa con comas o saltos de línea", "Séparez par des virgules ou retours à la ligne", "Durch Kommas oder Zeilenumbrüche trennen", "Разделяйте запятыми или переносами строк"},
		"测试语音接口":           {"測試語音介面", "음성 API 테스트", "Probar ASR", "Tester la reconnaissance", "ASR testen", "Проверить API речи"},
		"语音接口正常":           {"語音介面正常", "음성 API 정상", "ASR conectado", "Connexion vocale OK", "ASR-Verbindung OK", "API речи работает"},
		"请先填写语音 API Key":   {"請先填寫語音 API Key", "먼저 음성 API 키를 입력하세요", "Introduce primero la clave API de voz", "Saisissez d’abord la clé API vocale", "Zuerst den Sprach-API-Schlüssel eingeben", "Сначала укажите ключ API речи"},
		"语音连接中断，未收到最终结果":   {"語音連線中斷，未收到最終結果", "음성 연결 중단, 최종 결과 없음", "ASR desconectado sin resultado final", "Connexion vocale interrompue sans résultat final", "ASR getrennt, kein Endergebnis", "Соединение прервано без итогового результата"},
		"麦克风操作失败：%s（%d）%s": {"麥克風操作失敗：%s（%d）%s", "마이크 오류: %s (%d) %s", "Error de micrófono: %s (%d) %s", "Échec du microphone : %s (%d) %s", "Mikrofonfehler: %s (%d) %s", "Ошибка микрофона: %s (%d) %s"},
		"录音中":              {"錄音中", "녹음 중", "Grabando", "Enregistrement", "Aufnahme", "Запись"},
		"识别中…":             {"辨識中…", "인식 중…", "Transcribiendo…", "Transcription…", "Erkennung…", "Распознавание…"},
		"没听清，请再说一次":        {"沒聽清，請再說一次", "인식하지 못했습니다. 다시 말해주세요", "No se reconoció voz. Inténtalo de nuevo", "Aucune parole reconnue. Réessayez", "Nichts erkannt. Bitte erneut sprechen", "Речь не распознана. Повторите"},
		"无法写入剪贴板":          {"無法寫入剪貼簿", "클립보드에 쓸 수 없음", "No se puede escribir en el portapapeles", "Impossible d’écrire dans le presse-papiers", "Zwischenablage nicht beschreibbar", "Не удалось записать в буфер обмена"},
	}
	langs := []int{TW, KO, ES, FR, DE, RU}
	for key, translations := range entries {
		for index, lang := range langs {
			dicts[lang][key] = translations[index]
		}
	}
}
