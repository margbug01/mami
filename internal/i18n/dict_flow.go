package i18n

func init() {
	entries := map[string][8]string{
		"流光": {"Flow", "流光", "流光", "흐르는 빛", "Flujo", "Flux", "Lichtfluss", "Поток"},
		"冰海": {"Ice Sea", "氷海", "冰海", "얼음 바다", "Mar helado", "Mer de glace", "Eismeer", "Ледяное море"},
		"落日": {"Sunset", "夕日", "落日", "노을", "Atardecer", "Coucher de soleil", "Sonnenuntergang", "Закат"},
		"极光": {"Aurora", "オーロラ", "極光", "오로라", "Aurora", "Aurore", "Polarlicht", "Северное сияние"},
		"莓果": {"Berry", "ベリー", "莓果", "베리", "Bayas", "Baies", "Beeren", "Ягоды"},
		"翡翠": {"Jade", "翡翠", "翡翠", "비취", "Jade", "Jade", "Jade", "Нефрит"},
	}
	for key, translations := range entries {
		Dict[key] = [2]string{translations[0], translations[1]}
		for index, language := range []int{TW, KO, ES, FR, DE, RU} {
			dicts[language][key] = translations[index+2]
		}
	}
}
