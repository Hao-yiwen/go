// 版权所有 2018 The Go 作者。保留所有权利。
// 本源代码的使用受 BSD 风格许可证管制，
// 该许可证可在 LICENSE 文件中找到。

package plugin_test

import (
	_ "plugin"
	"testing"
)

func TestPlugin(t *testing.T) {
	// 这个测试确保导入 plugin
	// 包的可执行文件能够实际运行。详见问题 #28789。
}
