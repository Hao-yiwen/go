// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package filepath

import (
	"os"
	"strings"
	"syscall"
)

// HasPrefix 出于历史兼容性而存在，不应使用。
//
// 已弃用：HasPrefix 不尊重路径边界，
// 也不在需要时忽略大小写。
func HasPrefix(p, prefix string) bool {
	if strings.HasPrefix(p, prefix) {
		return true
	}
	return strings.HasPrefix(strings.ToLower(p), strings.ToLower(prefix))
}

func splitList(path string) []string {
	// 相同的实现用于 os/exec 中的 LookPath；
	// 更改此处时考虑更改 os/exec。

	if path == "" {
		return []string{}
	}

	// 分割路径，尊重但保留引号。
	list := []string{}
	start := 0
	quo := false
	for i := 0; i < len(path); i++ {
		switch c := path[i]; {
		case c == '"':
			quo = !quo
		case c == ListSeparator && !quo:
			list = append(list, path[start:i])
			start = i + 1
		}
	}
	list = append(list, path[start:])

	// 移除引号。
	for i, s := range list {
		list[i] = strings.ReplaceAll(s, `"`, ``)
	}

	return list
}

func abs(path string) (string, error) {
	if path == "" {
		// syscall.FullPath 在空路径上返回错误，因为它不是有效路径。
		// 为了实现 Abs 在空字符串输入时返回工作目录的行为，
		// 通过将空路径改为 "." 路径来特殊处理。见 golang.org/issue/24441。
		path = "."
	}
	fullPath, err := syscall.FullPath(path)
	if err != nil {
		return "", err
	}
	return Clean(fullPath), nil
}

func join(elem []string) string {
	var b strings.Builder
	var lastChar byte
	for _, e := range elem {
		switch {
		case b.Len() == 0:
			// 不修改地添加第一个非空路径元素。
		case os.IsPathSeparator(lastChar):
			// 如果路径以斜杠结尾，从下一个路径元素中去掉任何前导斜杠，
			// 以避免从非 UNC 元素创建 UNC 路径（任何以 "\\" 开头的路径）。
			//
			// 当第一个元素是不完整的 UNC 路径（例如 "\\"）时，
			// Join 的正确行为未明确指定。我们目前连接后续元素，
			// 所以 Join("\\", "host", "share") 产生 "\\host\share"。
			for len(e) > 0 && os.IsPathSeparator(e[0]) {
				e = e[1:]
			}
			// 如果路径是 \ 且下一个路径元素是 ??，
			// 添加额外的 .\ 以创建 \.\?? 而不是 \??\
			// （根本地设备路径）。
			if b.Len() == 1 && strings.HasPrefix(e, "??") && (len(e) == len("??") || os.IsPathSeparator(e[2])) {
				b.WriteString(`.\`)
			}
		case lastChar == ':':
			// 如果路径以冒号结尾，保持路径相对于驱动器上的当前目录，
			// 不添加分隔符。保留下一个路径元素中的前导斜杠，
			// 这可能使路径成为绝对路径。
			//
			// 	Join(`C:`, `f`) = `C:f`
			//	Join(`C:`, `\f`) = `C:\f`
		default:
			// 在所有其他情况下，在元素之间添加分隔符。
			b.WriteByte('\\')
			lastChar = '\\'
		}
		if len(e) > 0 {
			b.WriteString(e)
			lastChar = e[len(e)-1]
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return Clean(b.String())
}

func sameWord(a, b string) bool {
	return strings.EqualFold(a, b)
}
