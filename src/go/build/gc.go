// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build gc

package build

import (
	"path/filepath"
	"runtime"
)

// getToolDir 返回the default value of ToolDir.
func getToolDir() string {
	return filepath.Join(runtime.GOROOT(), "pkg/tool/"+runtime.GOOS+"_"+runtime.GOARCH)
}
