// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import (
	"path"
)

// GlobFS 是一个具有 Glob 方法的文件系统。
type GlobFS interface {
	FS

	// Glob 返回所有与模式匹配的文件名，
	// 提供了顶级 Glob 函数的实现。
	Glob(pattern string) ([]string, error)
}

// Glob 返回与模式匹配的所有文件的名称，
// 如果没有匹配的文件则返回 nil。
// 模式的语法与 [path.Match] 中的相同。
// 该模式可以描述分层名称，如 usr/*/bin/ed。
//
// Glob 忽略文件系统错误，如读取目录的 I/O 错误。
// 唯一可能的返回错误是 [path.ErrBadPattern]，报告
// 该模式格式不正确。
//
// 如果 fs 实现了 [GlobFS]，Glob 将调用 fs.Glob。
// 否则，Glob 使用 [ReadDir] 遍历目录树
// 并查找与模式匹配的项。
func Glob(fsys FS, pattern string) (matches []string, err error) {
	return globWithLimit(fsys, pattern, 0)
}

func globWithLimit(fsys FS, pattern string, depth int) (matches []string, err error) {
	// 添加此限制以防止堆栈耗尽问题。参见
	// CVE-2022-30630。
	const pathSeparatorsLimit = 10000
	if depth > pathSeparatorsLimit {
		return nil, path.ErrBadPattern
	}
	if fsys, ok := fsys.(GlobFS); ok {
		return fsys.Glob(pattern)
	}

	// 检查模式格式是否正确。
	if _, err := path.Match(pattern, ""); err != nil {
		return nil, err
	}
	if !hasMeta(pattern) {
		if _, err = Stat(fsys, pattern); err != nil {
			return nil, nil
		}
		return []string{pattern}, nil
	}

	dir, file := path.Split(pattern)
	dir = cleanGlobPath(dir)

	if !hasMeta(dir) {
		return glob(fsys, dir, file, nil)
	}

	// 防止无限递归。参见问题 15879。
	if dir == pattern {
		return nil, path.ErrBadPattern
	}

	var m []string
	m, err = globWithLimit(fsys, dir, depth+1)
	if err != nil {
		return nil, err
	}
	for _, d := range m {
		matches, err = glob(fsys, d, file, matches)
		if err != nil {
			return
		}
	}
	return
}

// cleanGlobPath 为 glob 匹配准备路径。
func cleanGlobPath(path string) string {
	switch path {
	case "":
		return "."
	default:
		return path[0 : len(path)-1] // 删除末尾分隔符
	}
}

// glob 在目录 dir 中搜索与模式匹配的文件，
// 并将它们追加到 matches，返回更新的切片。
// 如果无法打开目录，glob 返回现有的匹配项。
// 新匹配项按字典顺序添加。
func glob(fs FS, dir, pattern string, matches []string) (m []string, e error) {
	m = matches
	infos, err := ReadDir(fs, dir)
	if err != nil {
		return // 忽略 I/O 错误
	}

	for _, info := range infos {
		n := info.Name()
		matched, err := path.Match(pattern, n)
		if err != nil {
			return m, err
		}
		if matched {
			m = append(m, path.Join(dir, n))
		}
	}
	return
}

// hasMeta 报告路径是否包含 path.Match 识别的任何魔法字符。
func hasMeta(path string) bool {
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '*', '?', '[', '\\':
			return true
		}
	}
	return false
}
