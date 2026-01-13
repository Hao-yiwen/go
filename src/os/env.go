// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 通用环境变量。

package os

import (
	"internal/testlog"
	"syscall"
)

// Expand 根据映射函数替换字符串中的 ${var} 或 $var。
// 例如，[os.ExpandEnv](s) 等同于 [os.Expand](s, [os.Getenv])。
func Expand(s string, mapping func(string) string) string {
	var buf []byte
	// ${} 都是 ASCII，所以字节操作对此足够了。
	i := 0
	for j := 0; j < len(s); j++ {
		if s[j] == '$' && j+1 < len(s) {
			if buf == nil {
				buf = make([]byte, 0, 2*len(s))
			}
			buf = append(buf, s[i:j]...)
			name, w := getShellName(s[j+1:])
			if name == "" && w > 0 {
				// 遇到无效语法；吃掉这些字符。
			} else if name == "" {
				// 有效语法，但 $ 后面没有名称。
				// 保持美元符号不变。
				buf = append(buf, s[j])
			} else {
				buf = append(buf, mapping(name)...)
			}
			j += w
			i = j + 1
		}
	}
	if buf == nil {
		return s
	}
	return string(buf) + s[i:]
}

// ExpandEnv 根据当前环境变量的值替换字符串中的 ${var} 或 $var。
// 对未定义变量的引用将被替换为空字符串。
func ExpandEnv(s string) string {
	return Expand(s, Getenv)
}

// isShellSpecialVar 报告该字符是否标识特殊的
// shell 变量，如 $*。
func isShellSpecialVar(c uint8) bool {
	switch c {
	case '*', '#', '$', '@', '!', '?', '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}
	return false
}

// isAlphaNum 报告该字节是否是 ASCII 字母、数字或下划线。
func isAlphaNum(c uint8) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// getShellName 返回字符串开头的名称以及提取它所消耗的字节数。
// 如果名称被 {} 包围，它是 ${} 扩展的一部分，
// 需要比名称长度多两个字节。
func getShellName(s string) (string, int) {
	switch {
	case s[0] == '{':
		if len(s) > 2 && isShellSpecialVar(s[1]) && s[2] == '}' {
			return s[1:2], 3
		}
		// 扫描到右花括号
		for i := 1; i < len(s); i++ {
			if s[i] == '}' {
				if i == 1 {
					return "", 2 // 错误语法；吃掉 "${}"
				}
				return s[1:i], i + 1
			}
		}
		return "", 1 // 错误语法；吃掉 "${"
	case isShellSpecialVar(s[0]):
		return s[0:1], 1
	}
	// 扫描字母数字字符。
	var i int
	for i = 0; i < len(s) && isAlphaNum(s[i]); i++ {
	}
	return s[:i], i
}

// Getenv 获取以 key 为名称的环境变量的值。
// 它返回该值，如果变量不存在则为空。
// 要区分空值和未设置的值，请使用 [LookupEnv]。
func Getenv(key string) string {
	testlog.Getenv(key)
	v, _ := syscall.Getenv(key)
	return v
}

// LookupEnv 获取以 key 为名称的环境变量的值。
// 如果变量存在于环境中，则返回值（可能为空）且布尔值为 true。
// 否则返回值将为空，布尔值将为 false。
func LookupEnv(key string) (string, bool) {
	testlog.Getenv(key)
	return syscall.Getenv(key)
}

// Setenv 设置以 key 为名称的环境变量的值。
// 如果有错误则返回错误。
func Setenv(key, value string) error {
	err := syscall.Setenv(key, value)
	if err != nil {
		return NewSyscallError("setenv", err)
	}
	return nil
}

// Unsetenv 取消设置单个环境变量。
func Unsetenv(key string) error {
	return syscall.Unsetenv(key)
}

// Clearenv 删除所有环境变量。
func Clearenv() {
	syscall.Clearenv()
}

// Environ 返回表示环境的字符串副本，
// 格式为 "key=value"。
func Environ() []string {
	return syscall.Environ()
}
