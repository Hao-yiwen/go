// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"errors"
	"internal/bytealg"
	"internal/strconv"
	_ "unsafe" // for go:linkname
)

// 由运行时提供的随机数源。
// 我们生成随机临时文件名，使得文件很可能还不存在——
// 可以将 TempFile 中的尝试次数保持在最少。
//
//go:linkname runtime_rand runtime.rand
func runtime_rand() uint64

func nextRandom() string {
	return strconv.FormatUint(uint64(uint32(runtime_rand())), 10)
}

// CreateTemp 在目录 dir 中创建一个新的临时文件，
// 打开该文件以进行读写操作，并返回生成的文件。
// 文件名是通过获取 pattern 并在末尾添加随机字符串生成的。
// 如果 pattern 包含 "*"，则随机字符串替换最后的 "*"。
// 该文件以模式 0o600 创建（在 umask 之前）。
// 如果 dir 是空字符串，CreateTemp 使用默认临时文件目录，如 [TempDir] 所示。
// 同时调用 CreateTemp 的多个程序或 goroutine 不会选择同一个文件。
// 调用者可以使用文件的 Name 方法找到文件的路径名。
// 调用者有责任在不需要时删除该文件。
func CreateTemp(dir, pattern string) (*File, error) {
	if dir == "" {
		dir = TempDir()
	}

	prefix, suffix, err := prefixAndSuffix(pattern)
	if err != nil {
		return nil, &PathError{Op: "createtemp", Path: pattern, Err: err}
	}
	prefix = joinPath(dir, prefix)

	try := 0
	for {
		name := prefix + nextRandom() + suffix
		f, err := OpenFile(name, O_RDWR|O_CREATE|O_EXCL, 0600)
		if IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return nil, &PathError{Op: "createtemp", Path: prefix + "*" + suffix, Err: ErrExist}
		}
		return f, err
	}
}

var errPatternHasSeparator = errors.New("pattern contains path separator")

// prefixAndSuffix 按最后一个通配符 "*"（如果适用）分割 pattern，
// 返回前缀为 "*" 之前的部分，后缀为 "*" 之后的部分。
func prefixAndSuffix(pattern string) (prefix, suffix string, err error) {
	for i := 0; i < len(pattern); i++ {
		if IsPathSeparator(pattern[i]) {
			return "", "", errPatternHasSeparator
		}
	}
	if pos := bytealg.LastIndexByteString(pattern, '*'); pos != -1 {
		prefix, suffix = pattern[:pos], pattern[pos+1:]
	} else {
		prefix = pattern
	}
	return prefix, suffix, nil
}

// MkdirTemp 在目录 dir 中创建一个新的临时目录，
// 并返回新目录的路径名。
// 新目录的名称是通过在 pattern 末尾添加随机字符串生成的。
// 如果 pattern 包含 "*"，则随机字符串将替换最后的 "*"。
// 该目录以模式 0o700 创建（在 umask 之前）。
// 如果 dir 是空字符串，MkdirTemp 使用默认临时文件目录，如 TempDir 所示。
// 同时调用 MkdirTemp 的多个程序或 goroutine 不会选择同一个目录。
// 调用者有责任在不需要时删除该目录。
func MkdirTemp(dir, pattern string) (string, error) {
	if dir == "" {
		dir = TempDir()
	}

	prefix, suffix, err := prefixAndSuffix(pattern)
	if err != nil {
		return "", &PathError{Op: "mkdirtemp", Path: pattern, Err: err}
	}
	prefix = joinPath(dir, prefix)

	try := 0
	for {
		name := prefix + nextRandom() + suffix
		err := Mkdir(name, 0700)
		if err == nil {
			return name, nil
		}
		if IsExist(err) {
			if try++; try < 10000 {
				continue
			}
			return "", &PathError{Op: "mkdirtemp", Path: prefix + "*" + suffix, Err: ErrExist}
		}
		if IsNotExist(err) {
			if _, err := Stat(dir); IsNotExist(err) {
				return "", err
			}
		}
		return "", err
	}
}

func joinPath(dir, name string) string {
	if len(dir) > 0 && IsPathSeparator(dir[len(dir)-1]) {
		return dir + name
	}
	return dir + string(PathSeparator) + name
}
