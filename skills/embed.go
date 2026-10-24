// Package skills 把给 AI 用的 skill 打进程序：设置页「自定义」里展示说明、新建样式时拷模板、导出整个文件夹。
package skills

import "embed"

//go:embed mami-capsule-style
var FS embed.FS

// CapsuleStyle 胶囊样式 skill 在 FS 里的目录
const CapsuleStyle = "mami-capsule-style"
