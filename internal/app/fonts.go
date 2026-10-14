package app

import (
	_ "embed"
	"log"
	"sync"

	"github.com/egoist/mygo/ui"
)

// MiSans（小米）的子集：Regular 和 Bold，只留拉丁、西里尔、假名、标点和常用汉字，
// 两份加起来 3.4 MB。字体来自 https://hyperos.mi.com/font ，可免费商用。
// 子集里没有的字（比如韩文）会落到系统字体。
//
//go:embed fonts/MiSans-Regular.subset.ttf
var miSansRegular []byte

//go:embed fonts/MiSans-Bold.subset.ttf
var miSansBold []byte

// fontStack 是整个界面的字体：先 MiSans，缺的字交给系统界面字体
const fontStack = "MiSans, system-ui"

var fontsOnce sync.Once

// registerFonts 把内置字体登记给 MyGo，只做一次
func registerFonts() {
	fontsOnce.Do(func() {
		for _, f := range [][]byte{miSansRegular, miSansBold} {
			if err := ui.RegisterFont(f, "MiSans"); err != nil {
				log.Println("font:", err)
			}
		}
	})
}
