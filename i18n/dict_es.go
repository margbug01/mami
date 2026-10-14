package i18n

// Español
var dictES = map[string]string{
	"接口": "API", "翻译": "Traducción", "快捷键": "Atajos", "划词按钮": "Botón de selección", "通用": "General", "保存": "Guardar",
	"接口（OpenAI 兼容）": "API (compatible con OpenAI)",

	"接口地址": "Endpoint", "会自动拼上 /chat/completions": "Se añade /chat/completions automáticamente", "免费接口留空": "Déjalo vacío en APIs gratuitas",
	"模型": "Modelo", "测试连接": "Probar conexión", "代理": "Proxy",
	"Go 不读系统代理，要走 Clash 得填这里": "Go no usa el proxy del sistema; para usar Clash, indícalo aquí",
	"▸ 高级": "▸ Avanzado", "▾ 高级": "▾ Avanzado",
	"正在请求 ": "Solicitando ", "连接失败：": "Error de conexión: ", "连接正常（%s）：你好，世界 → %s": "Conexión correcta (%s): 你好，世界 → %s",

	"中文翻成": "Chino a", "选中的文字里有汉字时": "Cuando la selección contiene caracteres chinos",
	"其他翻成": "Otros a", "不含汉字的文字（英文、日文等）": "Texto sin caracteres chinos (inglés, japonés, etc.)",
	"额外要求": "Instrucciones extra", "追加给模型的话，比如风格、术语处理": "Indicaciones adicionales para el modelo, como estilo o terminología",
	"比如：技术文档风格，术语保留英文": "p. ej.: estilo de documentación técnica, términos en inglés",
	"拖动调整高度，点一下恢复自动":   "Arrastra para cambiar la altura; clic para volver a automático",

	"原地替换": "Reemplazar en el sitio", "写法如 Ctrl+Shift+Y、Alt+F2": "Por ejemplo Ctrl+Shift+Y o Alt+F2",
	"只弹卡片": "Solo tarjeta", "留空表示不用": "Vacío para desactivar",
	"快捷键没选中就全选":                  "Seleccionar todo si no hay selección",
	"按 Alt+T 时没选中文字，先全选整个输入框再翻译": "Si pulsas Alt+T sin texto seleccionado, se selecciona todo el cuadro antes de traducir",
	"开机自启": "Iniciar con la sesión",
	"登录 Windows 后在托盘里自动运行": "Se ejecuta en la bandeja al iniciar sesión en Windows",

	"划词后显示翻译按钮":            "Mostrar botón de traducir al seleccionar",
	"选中文字后旁边弹出一个小图标，点它出译文": "Al seleccionar texto aparece un icono pequeño; púlsalo para traducir",
	"只在输入框里弹":              "Solo en cuadros de texto",
	"网页正文、文档里划词不弹":         "No aparece al seleccionar texto de páginas web o documentos",
	"点翻译按钮后":               "Tras pulsar traducir",
	"直接替换出错时仍会弹卡片说明原因":     "Si el reemplazo directo falla, una tarjeta explica el motivo",
	"不弹按钮的程序":              "Programas sin botón",
	"进程名，逗号分隔":             "Nombres de proceso, separados por comas",
	"▸ 兼容性":                "▸ Compatibilidad", "▾ 兼容性": "▾ Compatibilidad",
	"认不出输入框的程序也弹":                  "Mostrar también en programas que ocultan sus cuadros de texto",
	"微信这类程序开着能用，但聊天记录里划词也会弹":       "Funciona en programas como WeChat, pero el botón también aparece al seleccionar el historial del chat",
	"只认文本光标":                       "Solo cursor de texto",
	"按下鼠标时光标是 I 形才算划词，拖窗口、拖文件不会误弹": "Solo cuenta como selección si el puntero es una I al pulsar; arrastrar ventanas o archivos no lo activa",
	"显示卡片（先显示「翻译中…」）":              "Mostrar tarjeta (primero «Traduciendo…»)",
	"不显示「翻译中」，出结果再弹":               "Sin «Traduciendo»; mostrar al tener el resultado",
	"直接替换，不弹卡片":                    "Reemplazar directamente, sin tarjeta",

	"界面语言": "Idioma", "设置窗口、卡片和托盘菜单用哪种语言": "Idioma de la ventana de ajustes, las tarjetas y el menú de la bandeja",
	"外观": "Apariencia", "浅色、深色，或跟随 Windows": "Claro, oscuro o según Windows",
	"跟随系统": "Seguir el sistema", "浅色": "Claro", "深色": "Oscuro",

	"已保存": "Guardado", "保存失败：": "Error al guardar: ", "已保存，但快捷键有问题：": "Guardado, pero hay problemas con los atajos: ", "：": ": ", "；": "; ",

	"翻译中": "Traduciendo", "翻译中…": "Traduciendo…", "复制": "Copiar", "已复制": "Copiado", "替换": "Reemplazar", "关闭": "Cerrar",
	"中文 → ": "Chino → ", "自动检测 → ": "Detección automática → ", "中文 → English": "Chino → English",
	" · 窗口已切换，没有替换":              " · La ventana cambió; no se reemplazó",
	" · 期间点了别处，没有替换":             " · Hiciste clic en otro lugar; no se reemplazó",
	" · 终端里不替换":                  " · No se reemplaza en terminales",
	"选中了 %d 个字，超过上限 %d。":         "Se seleccionaron %d caracteres; el límite es %d.",
	"没取到选中的文字。这个窗口可能不支持 Ctrl+C。": "No se pudo obtener el texto seleccionado. Es posible que esta ventana no admita Ctrl+C.",
	"终端里要先用鼠标选中文字。为了不误发 Ctrl+C 中断正在跑的程序，终端里不会自动全选。": "En una terminal, selecciona primero el texto con el ratón. Para no enviar Ctrl+C e interrumpir un programa en ejecución, no se selecciona todo automáticamente.",

	"设置…": "Ajustes…", "打开日志": "Abrir registro", "退出": "Salir", " 原地替换": " reemplazar en el sitio",

	"proxy 地址不对：%w":                                 "Dirección de proxy no válida: %w",
	"%s 格式不对：%w":                                    "%s no es válido: %w",
	"还没填 API Key：托盘图标右键 → 设置":                       "Falta la API Key: clic derecho en el icono de la bandeja → Ajustes",
	"请求失败：%w":                                       "Solicitud fallida: %w",
	"接口返回的不是 chat/completions 格式":                   "La API no devolvió el formato chat/completions",
	"模型返回了空内容":                                      "El modelo devolvió una respuesta vacía",
	"鼠标钩子挂载失败，只能用快捷键":                               "No se pudo instalar el gancho del ratón; solo funcionarán los atajos",
	"配置文件没读成功，保存会覆盖它：":                              "No se pudo leer el archivo de configuración; al guardar se sobrescribirá: ",
	"默认用 B 站免费的 Index-Translate-35B，不用 Key；换别的接口再填": "Por defecto usa el Index-Translate-35B gratuito de Bilibili, sin Key; rellénalo si cambias de API",
}
