// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build js && wasm

package os

import (
	"errors"
	"slices"
	"syscall"
)

// checkPathEscapes 报告 name 是否逃离了根目录。
//
// 由于缺乏 openat，当符号链接在解析过程中改变时，
// checkPathEscapes 容易受到 TOCTOU 竞争。
func checkPathEscapes(r *Root, name string) error {
	return checkPathEscapesInternal(r, name, false)
}

// checkPathEscapesLstat 报告 name 是否逃离了根目录。
// 它不解析最后路径组件中的符号链接。
//
// 由于缺乏 openat，当符号链接在解析过程中改变时，
// checkPathEscapes 容易受到 TOCTOU 竞争。
func checkPathEscapesLstat(r *Root, name string) error {
	return checkPathEscapesInternal(r, name, true)
}

func checkPathEscapesInternal(r *Root, name string, lstat bool) error {
	if r.root.closed.Load() {
		return ErrClosed
	}
	parts, suffixSep, err := splitPathInRoot(name, nil, nil)
	if err != nil {
		return err
	}

	i := 0
	symlinks := 0
	base := r.root.name
	for i < len(parts) {
		if parts[i] == ".." {
			// 解决一个或多个父 ("..") 路径分量。
			end := i + 1
			for end < len(parts) && parts[end] == ".." {
				end++
			}
			count := end - i
			if count > i {
				return errPathEscapes
			}
			parts = slices.Delete(parts, i-count, end)
			i -= count
			base = r.root.name
			for j := range i {
				base = joinPath(base, parts[j])
			}
			continue
		}

		part := parts[i]
		if i == len(parts)-1 {
			if lstat {
				break
			}
			part += suffixSep
		}

		next := joinPath(base, part)
		fi, err := Lstat(next)
		if err != nil {
			if IsNotExist(err) {
				return nil
			}
			return underlyingError(err)
		}
		if fi.Mode()&ModeSymlink != 0 {
			link, err := Readlink(next)
			if err != nil {
				return errPathEscapes
			}
			symlinks++
			if symlinks > rootMaxSymlinks {
				return errors.New("too many symlinks")
			}
			newparts, newSuffixSep, err := splitPathInRoot(link, parts[:i], parts[i+1:])
			if err != nil {
				return err
			}
			if i == len(parts) {
				// suffixSep 包含链接目标中的任何尾随路径分隔符。
				// 如果我们要替换路径的其余部分，保留这些。
				// 如果我们要替换路径的某个中间分量，
				// 忽略它们，因为中间分量必须始终是
				// 目录。
				suffixSep = newSuffixSep
			}
			parts = newparts
			continue
		}
		if !fi.IsDir() && i < len(parts)-1 {
			return syscall.ENOTDIR
		}

		base = next
		i++
	}
	return nil
}
