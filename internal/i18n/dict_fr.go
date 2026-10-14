package i18n

// Français
var dictFR = map[string]string{
	"接口": "API", "翻译": "Traduction", "快捷键": "Raccourcis", "划词按钮": "Bouton de sélection", "通用": "Général", "保存": "Enregistrer",
	"接口（OpenAI 兼容）": "API (compatible OpenAI)",

	"接口地址": "Point d'accès", "会自动拼上 /chat/completions": "/chat/completions est ajouté automatiquement", "免费接口留空": "Laisser vide pour une API gratuite",
	"模型": "Modèle", "测试连接": "Tester la connexion", "代理": "Proxy",
	"Go 不读系统代理，要走 Clash 得填这里": "Go ignore le proxy du système ; pour passer par Clash, renseignez-le ici",
	"▸ 高级": "▸ Avancé", "▾ 高级": "▾ Avancé",
	"正在请求 ": "Requête en cours ", "连接失败：": "Échec de la connexion : ", "连接正常（%s）：你好，世界 → %s": "Connexion réussie (%s) : 你好，世界 → %s",

	"中文翻成": "Chinois vers", "选中的文字里有汉字时": "Quand la sélection contient des caractères chinois",
	"其他翻成": "Autres vers", "不含汉字的文字（英文、日文等）": "Texte sans caractères chinois (anglais, japonais, etc.)",
	"额外要求": "Consignes supplémentaires", "追加给模型的话，比如风格、术语处理": "Précisions pour le modèle, comme le style ou la terminologie",
	"比如：技术文档风格，术语保留英文": "ex. : style documentation technique, termes en anglais",
	"拖动调整高度，点一下恢复自动":   "Faites glisser pour redimensionner, cliquez pour revenir en automatique",

	"原地替换": "Remplacer sur place", "写法如 Ctrl+Shift+Y、Alt+F2": "Par exemple Ctrl+Shift+Y ou Alt+F2",
	"只弹卡片": "Carte seulement", "留空表示不用": "Laisser vide pour désactiver",
	"快捷键没选中就全选":                  "Tout sélectionner si rien n'est sélectionné",
	"按 Alt+T 时没选中文字，先全选整个输入框再翻译": "Si Alt+T est pressé sans sélection, tout le champ est d'abord sélectionné avant la traduction",
	"开机自启": "Lancer à l'ouverture de session",
	"登录 Windows 后在托盘里自动运行": "S'exécute dans la zone de notification après l'ouverture de session Windows",

	"划词后显示翻译按钮":            "Afficher le bouton de traduction après sélection",
	"选中文字后旁边弹出一个小图标，点它出译文": "Une petite icône apparaît à côté de la sélection ; cliquez dessus pour traduire",
	"只在输入框里弹":              "Uniquement dans les champs de saisie",
	"网页正文、文档里划词不弹":         "Pas de bouton pour le texte des pages web ou des documents",
	"点翻译按钮后":               "Après un clic sur traduire",
	"直接替换出错时仍会弹卡片说明原因":     "Si le remplacement direct échoue, une carte en explique la raison",
	"不弹按钮的程序":              "Programmes sans bouton",
	"进程名，逗号分隔":             "Noms de processus, séparés par des virgules",
	"▸ 兼容性":                "▸ Compatibilité", "▾ 兼容性": "▾ Compatibilité",
	"认不出输入框的程序也弹":                  "Afficher aussi dans les programmes qui masquent leurs champs de saisie",
	"微信这类程序开着能用，但聊天记录里划词也会弹":       "Fonctionne dans des programmes comme WeChat, mais le bouton apparaît aussi en sélectionnant l'historique des discussions",
	"只认文本光标":                       "Curseur de texte seulement",
	"按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹": "Ne compte comme sélection que si le pointeur est un I au moment du clic ; déplacer des fenêtres ou des fichiers ne le déclenche pas",
	"显示卡片（先显示「翻译中…」）":              "Afficher une carte (d'abord « Traduction… »)",
	"不显示「翻译中」，出结果再弹":               "Pas de « Traduction… » ; afficher quand le résultat arrive",
	"直接替换，不弹卡片":                    "Remplacer directement, sans carte",

	"界面语言": "Langue", "设置窗口、卡片和托盘菜单用哪种语言": "Langue de la fenêtre de réglages, des cartes et du menu de la zone de notification",
	"外观": "Apparence", "浅色、深色，或跟随 Windows": "Clair, sombre ou selon Windows",
	"跟随系统": "Suivre le système", "浅色": "Clair", "深色": "Sombre",

	"已保存": "Enregistré", "保存失败：": "Échec de l'enregistrement : ", "已保存，但快捷键有问题：": "Enregistré, mais des raccourcis posent problème : ", "：": " : ", "；": " ; ",

	"翻译中": "Traduction", "翻译中…": "Traduction…", "复制": "Copier", "已复制": "Copié", "替换": "Remplacer", "关闭": "Fermer",
	"中文 → ": "Chinois → ", "自动检测 → ": "Détection auto → ", "中文 → English": "Chinois → English",
	" · 窗口已切换，没有替换":              " · La fenêtre a changé ; rien n'a été remplacé",
	" · 期间点了别处，没有替换":             " · Vous avez cliqué ailleurs ; rien n'a été remplacé",
	" · 终端里不替换":                  " · Pas de remplacement dans les terminaux",
	"选中了 %d 个字，超过上限 %d。":         "%d caractères sélectionnés, la limite est de %d.",
	"没取到选中的文字。这个窗口可能不支持 Ctrl+C。": "Impossible de récupérer le texte sélectionné. Cette fenêtre ne prend peut-être pas en charge Ctrl+C.",
	"终端里要先用鼠标选中文字。为了不误发 Ctrl+C 中断正在跑的程序，终端里不会自动全选。": "Dans un terminal, sélectionnez d'abord le texte à la souris. Pour ne pas envoyer Ctrl+C et interrompre un programme en cours, la sélection automatique est désactivée.",

	"设置…": "Réglages…", "打开日志": "Ouvrir le journal", "退出": "Quitter", " 原地替换": " remplacer sur place",

	"proxy 地址不对：%w":                                 "Adresse de proxy invalide : %w",
	"%s 格式不对：%w":                                    "%s n'est pas valide : %w",
	"还没填 API Key：托盘图标右键 → 设置":                       "Clé API manquante : clic droit sur l'icône de la zone de notification → Réglages",
	"请求失败：%w":                                       "Échec de la requête : %w",
	"接口返回的不是 chat/completions 格式":                   "L'API n'a pas renvoyé le format chat/completions",
	"模型返回了空内容":                                      "Le modèle a renvoyé une réponse vide",
	"鼠标钩子挂载失败，只能用快捷键":                               "Échec du crochet souris ; seuls les raccourcis fonctionneront",
	"配置文件没读成功，保存会覆盖它：":                              "Impossible de lire le fichier de configuration ; l'enregistrement l'écrasera : ",
	"默认用 B 站免费的 Index-Translate-35B，不用 Key；换别的接口再填": "Utilise par défaut l'Index-Translate-35B gratuit de Bilibili, sans clé ; à renseigner si vous changez d'API",
	"启用快捷键":                                         "Activer les raccourcis",
	"关掉后不注册任何全局快捷键，不会和别的程序抢键；划词按钮照常能用": "Désactivé, aucun raccourci global n'est enregistré : pas de conflit avec d'autres programmes ; le bouton de sélection fonctionne toujours",
}
