package app

import (
	"embed"

	"github.com/egoist/mygo/ui"
)

// 服务商 logo 来自 LobeHub Icons（MIT，github.com/lobehub/lobe-icons），宽高 1em 改成了 24
//
//go:embed assets/logos/*.svg
var logoFS embed.FS

func logo(name string) *ui.SVG {
	b, err := logoFS.ReadFile("assets/logos/" + name + ".svg")
	if err != nil {
		panic(err)
	}
	return ui.MustParseSVG(b)
}

var (
	logoDeepSeek   = logo("deepseek")
	logoBilibili   = logo("bilibili")
	logoOpenRouter = logo("openrouter")
	logoStepFun    = logo("stepfun")
	logoBailian    = logo("bailian")
	logoTencent    = logo("tencentcloud")
)
