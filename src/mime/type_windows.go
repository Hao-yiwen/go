// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package mime

import (
	"internal/syscall/windows/registry"
)

func init() {
	osInitMime = initMimeWindows
}

func initMimeWindows() {
	names, err := registry.CLASSES_ROOT.ReadSubKeyNames()
	if err != nil {
		return
	}
	for _, name := range names {
		if len(name) < 2 || name[0] != '.' { // 只查找扩展名
			continue
		}
		k, err := registry.OpenKey(registry.CLASSES_ROOT, name, registry.READ)
		if err != nil {
			continue
		}
		v, _, err := k.GetStringValue("Content Type")
		k.Close()
		if err != nil {
			continue
		}

		// Windows 上有一个长期存在的问题：注册表有时会记录
		// ".js" 扩展名应该是 "text/plain"。参见 issue #32350。
		// 虽然通常本地配置应该覆盖默认值，但这个问题足够常见，
		// 我们在这里通过忽略该注册表设置来处理它。
		if name == ".js" && (v == "text/plain" || v == "text/plain; charset=utf-8") {
			continue
		}

		setExtensionType(name, v)
	}
}

func initMimeForTests() map[string]string {
	return map[string]string{
		".PnG": "image/png",
	}
}
