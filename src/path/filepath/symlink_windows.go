// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package filepath

import (
	"strings"
	"syscall"
)

// normVolumeName 类似于 VolumeName，但将驱动器字母转为大写。
// EvalSymlinks 的结果必须是唯一的，所以我们有
// EvalSymlinks(`c:\a`) == EvalSymlinks(`C:\a`)。
func normVolumeName(path string) string {
	volume := VolumeName(path)

	if len(volume) > 2 { // isUNC
		return volume
	}

	return strings.ToUpper(volume)
}

// normBase 返回路径的最后一个元素，带有正确的大小写。
func normBase(path string) (string, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}

	var data syscall.Win32finddata

	h, err := syscall.FindFirstFile(p, &data)
	if err != nil {
		return "", err
	}
	syscall.FindClose(h)

	return syscall.UTF16ToString(data.FileName[:]), nil
}

// baseIsDotDot 报告路径的最后一个元素是否是 ".."。
// 给定的路径应该事先经过 'Clean' 处理。
func baseIsDotDot(path string) bool {
	i := strings.LastIndexByte(path, Separator)
	return path[i+1:] == ".."
}

// toNorm 返回保证唯一的规范化路径。
// 它应该接受以下格式：
//   - UNC 路径                               （例如 \\server\share\foo\bar）
//   - 绝对路径                               （例如 C:\foo\bar）
//   - 以驱动器字母开头的相对路径             （例如 C:foo\bar、C:..\foo\bar、C:..、C:.）
//   - 以 '\' 开头的相对路径                  （例如 \foo\bar）
//   - 不以 '\' 开头的相对路径                （例如 foo\bar、..\foo\bar、..、.）
//
// 返回的规范化路径将与输入路径具有相同的形式（上述5种之一）。
// 如果两个路径 A 和 B 以相同格式指向同一文件，toNorm(A) 应该等于 toNorm(B)。
// normBase 参数应该等于 normBase 函数，除了在测试中。参见 normBase 函数的文档。
func toNorm(path string, normBase func(string) (string, error)) (string, error) {
	if path == "" {
		return path, nil
	}

	volume := normVolumeName(path)
	path = path[len(volume):]

	// 跳过特殊情况
	if path == "" || path == "." || path == `\` {
		return volume + path, nil
	}

	var normPath string

	for {
		if baseIsDotDot(path) {
			normPath = path + `\` + normPath

			break
		}

		name, err := normBase(volume + path)
		if err != nil {
			return "", err
		}

		normPath = name + `\` + normPath

		i := strings.LastIndexByte(path, Separator)
		if i == -1 {
			break
		}
		if i == 0 { // `\Go` or `C:\Go`
			normPath = `\` + normPath

			break
		}

		path = path[:i]
	}

	normPath = normPath[:len(normPath)-1] // 移除尾部 '\'

	return volume + normPath, nil
}

func evalSymlinks(path string) (string, error) {
	newpath, err := walkSymlinks(path)
	if err != nil {
		return "", err
	}
	newpath, err = toNorm(newpath, normBase)
	if err != nil {
		return "", err
	}
	return newpath, nil
}
