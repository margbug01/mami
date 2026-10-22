package i18n

// 新手引导（guide.go）的文案：{英, 日, 繁, 韩, 西, 法, 德, 俄}。
// 要念的那句里 [方括号] 是口头禅和改口，整理时去掉：去掉后剩下的得是一句通顺的话（i18n 和 app 的测试都会查）。
func init() {
	entries := map[string][8]string{
		"新手引导…": {"Tutorial…", "使い方ガイド…", "新手導覽…", "사용 안내…", "Guía de inicio…", "Visite guidée…", "Einführung …", "Знакомство…"},
		"跳过引导":  {"Skip tour", "ガイドをスキップ", "略過導覽", "안내 건너뛰기", "Saltar guía", "Passer la visite", "Einführung überspringen", "Пропустить"},
		"继续":    {"Continue", "次へ", "繼續", "계속", "Continuar", "Continuer", "Weiter", "Далее"},
		"开始":    {"Start", "はじめる", "開始", "시작", "Empezar", "Commencer", "Los geht’s", "Начать"},
		"跳过":    {"Skip", "スキップ", "略過", "건너뛰기", "Saltar", "Passer", "Überspringen", "Пропустить"},
		"开始使用":  {"Get started", "使い始める", "開始使用", "시작하기", "Empezar a usar", "C’est parti", "Loslegen", "Начать работу"},
		"再试一次":  {"Try again", "もう一度", "再試一次", "다시 해 보기", "Otra vez", "Réessayer", "Nochmal", "Ещё раз"},
		"说话":    {"Speak", "話す", "說話", "말하기", "Hablar", "Parler", "Sprechen", "Говорить"},

		"你好，我是 Mami":       {"Hi, I'm Mami", "こんにちは、Mami です", "你好，我是 Mami", "안녕하세요, Mami예요", "Hola, soy Mami", "Bonjour, je suis Mami", "Hallo, ich bin Mami", "Привет, я Mami"},
		"在你打字的地方，帮你说，也帮你译": {"Speak and translate right where you type", "入力しているその場所で、話して、訳す", "在你打字的地方，幫你說，也幫你譯", "입력하는 바로 그곳에서, 말하고 번역하세요", "Habla y traduce justo donde escribes", "Dictez et traduisez là où vous tapez", "Sprechen und übersetzen, genau wo du tippst", "Диктуйте и переводите прямо там, где печатаете"},

		"试试用说的":         {"Try saying it", "話してみましょう", "試試用說的", "말로 해 보세요", "Prueba a decirlo", "Essayez de le dire", "Sag es einfach", "Попробуйте сказать"},
		"按一下 %s，念出这句话":  {"Tap %s and read this out", "%s を押して、この文を読み上げます", "按一下 %s，唸出這句話", "%s 키를 누르고 이 문장을 읽어 보세요", "Pulsa %s y lee esta frase", "Appuyez sur %s et lisez cette phrase", "%s antippen und diesen Satz vorlesen", "Нажмите %s и прочитайте эту фразу"},
		"点一下麦克风，念出这句话":  {"Click the mic and read this out", "マイクをクリックして、この文を読み上げます", "按一下麥克風，唸出這句話", "마이크를 누르고 이 문장을 읽어 보세요", "Pulsa el micrófono y lee esta frase", "Cliquez sur le micro et lisez cette phrase", "Mikrofon anklicken und diesen Satz vorlesen", "Нажмите на микрофон и прочитайте эту фразу"},
		"念完再按一下 %s":     {"Done? Tap %s again", "読み終えたら、もう一度 %s", "唸完再按一下 %s", "다 읽었으면 %s 키를 한 번 더", "¿Listo? Pulsa %s otra vez", "Fini ? Rappuyez sur %s", "Fertig? Nochmal %s antippen", "Готово? Нажмите %s ещё раз"},
		"念完再点一下麦克风":     {"Done? Click the mic again", "読み終えたら、もう一度マイクをクリック", "唸完再按一下麥克風", "다 읽었으면 마이크를 한 번 더", "¿Listo? Pulsa el micrófono otra vez", "Fini ? Recliquez sur le micro", "Fertig? Nochmal aufs Mikrofon klicken", "Готово? Нажмите на микрофон ещё раз"},
		"整理中…":          {"Tidying up…", "整えています…", "整理中…", "다듬는 중…", "Puliendo…", "Mise au propre…", "Wird geglättet …", "Привожу в порядок…"},
		"口头禅和改口，都帮你去掉了": {"Fillers and do-overs, gone", "口癖も言い直しも、取り除きました", "口頭禪和改口，都幫你去掉了", "군말과 말 바꾸기는 지웠어요", "Muletillas y rectificaciones, fuera", "Tics de langage et reprises, envolés", "Füllwörter und Versprecher sind weg", "Слова-паразиты и оговорки убраны"},
		"[嗯，那个，]帮我把这个接口改成用 Go 写，然后[三点开会，不对，是]四点开会。": {
			"Rewrite this endpoint in Go, [um, ]and the meeting's at [three, no, wait, ]four.",
			"[えーと、あの、]この API を Go で書き直して、会議は[3時、いや、]4時です。",
			"[嗯，那個，]幫我把這個介面改成用 Go 寫，然後[三點開會，不對，是]四點開會。",
			"[음, 그러니까, ]이 API를 Go로 다시 짜 주고, 회의는 [세 시, 아니, ]네 시야.",
			"Reescribe este endpoint en Go, [eh, ]y la reunión es a las [tres, no, perdón, ]cuatro.",
			"Réécris cet endpoint en Go, [euh, ]et la réunion est à [trois heures, non, attends, ]quatre heures.",
			"Schreib diesen Endpunkt in Go um, [äh, ]und das Meeting ist um [drei, nein, ]vier.",
			"Перепиши этот эндпоинт на Go, [э-э, ]а встреча в [три, нет, ]четыре.",
		},

		"选中，就能翻译":    {"Select to translate", "選ぶだけで翻訳", "選取，就能翻譯", "선택하면 번역", "Selecciona y traduce", "Sélectionnez, c’est traduit", "Markieren und übersetzen", "Выделите — и переведите"},
		"用鼠标选中下面这句话": {"Select this sentence with the mouse", "マウスでこの文を選択してください", "用滑鼠選取下面這句話", "마우스로 아래 문장을 선택해 보세요", "Selecciona esta frase con el ratón", "Sélectionnez cette phrase à la souris", "Markiere diesen Satz mit der Maus", "Выделите эту фразу мышью"},
		"点一下末尾的「文A」": {"Click the 文A at the end", "末尾の「文A」をクリック", "按一下末尾的「文A」", "끝에 있는 '文A'를 누르세요", "Pulsa el 文A del final", "Cliquez sur le 文A au bout", "Klick auf das 文A am Ende", "Нажмите 文A в конце"},
		"按 %s 替换":    {"Press %s to replace", "%s で置換", "按 %s 取代", "%s 키로 바꾸기", "Pulsa %s para reemplazar", "Appuyez sur %s pour remplacer", "%s zum Ersetzen", "Нажмите %s, чтобы заменить"},
		"换好了":        {"Replaced", "置き換えました", "換好了", "바꿨어요", "Reemplazado", "Remplacé", "Ersetzt", "Заменено"},
		"换好了。下次选中后按 %s，直接就换": {
			"Replaced. Next time, select and press %s to swap it in one go",
			"置き換えました。次は選択して %s で一発置換",
			"換好了。下次選取後按 %s，直接就換",
			"바꿨어요. 다음엔 선택하고 %s 키로 바로 바꾸세요",
			"Hecho. La próxima vez, selecciona y pulsa %s para cambiarlo directamente",
			"C’est fait. La prochaine fois, sélectionnez et appuyez sur %s pour remplacer d’un coup",
			"Ersetzt. Nächstes Mal markieren und %s drücken, dann geht’s direkt",
			"Готово. В следующий раз выделите и нажмите %s — заменится сразу",
		},
		"能帮我看看这个插件为什么启动后不生效吗？": {
			"Could you take a look at why this plugin doesn't work after startup?",
			"このプラグインが起動後に効かない理由を見てもらえますか？",
			"能幫我看看這個外掛為什麼啟動後不生效嗎？",
			"이 플러그인이 시작 후에 왜 작동하지 않는지 봐 줄 수 있어요?",
			"¿Puedes ver por qué este plugin no funciona después de arrancar?",
			"Tu peux regarder pourquoi ce plugin ne marche pas après le démarrage ?",
			"Kannst du schauen, warum dieses Plugin nach dem Start nicht greift?",
			"Можешь посмотреть, почему этот плагин не работает после запуска?",
		},

		"好了，就这么简单":            {"That's it", "以上です。かんたんでしょ", "好了，就這麼簡單", "끝, 이게 전부예요", "Eso es todo", "C’est tout", "Das war’s schon", "Вот и всё"},
		"Mami 住在托盘里，单击图标打开设置": {"Mami lives in the tray; click its icon for settings", "Mami はトレイにいます。アイコンをクリックすると設定が開きます", "Mami 住在系統匣裡，按一下圖示開啟設定", "Mami는 트레이에 있어요. 아이콘을 누르면 설정이 열려요", "Mami vive en la bandeja; haz clic en su icono para los ajustes", "Mami vit dans la zone de notification ; cliquez sur son icône pour les réglages", "Mami wohnt im Infobereich; ein Klick aufs Symbol öffnet die Einstellungen", "Mami живёт в трее; щёлкните значок, чтобы открыть настройки"},
	}
	langs := []int{TW, KO, ES, FR, DE, RU}
	for key, tr := range entries {
		Dict[key] = [2]string{tr[0], tr[1]}
		for i, lang := range langs {
			dicts[lang][key] = tr[2+i]
		}
	}
}
