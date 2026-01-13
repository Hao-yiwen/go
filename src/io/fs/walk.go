// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import (
	"errors"
	"path"
)

// SkipDir 用作 [WalkDirFunc] 的返回值，以指示
// 调用中命名的目录应该被跳过。
// 任何函数都不会将其作为错误返回。
var SkipDir = errors.New("skip this directory")

// SkipAll 用作 [WalkDirFunc] 的返回值，以指示
// 所有剩余的文件和目录都应该被跳过。
// 任何函数都不会将其作为错误返回。
var SkipAll = errors.New("skip everything and stop the walk")

// WalkDirFunc 是由 [WalkDir] 调用以访问
// 每个文件或目录的函数的类型。
//
// path 参数包含作为前缀的 [WalkDir] 的参数。
// 也就是说，如果 WalkDir 使用根参数 "dir" 调用
// 并在该目录中找到名为 "a" 的文件，
// 将使用参数 "dir/a" 调用 walk 函数。
//
// d 参数是命名路径的 [DirEntry]。
//
// 函数返回的错误结果控制 [WalkDir] 如何继续。
// 如果函数返回特殊值 [SkipDir]，WalkDir 会跳过
// 当前目录（如果 d.IsDir() 为 true 则为 path，
// 否则为 path 的父目录）。
// 如果函数返回特殊值 [SkipAll]，WalkDir 会跳过
// 所有剩余的文件和目录。否则，
// 如果函数返回非 nil 错误，WalkDir 将完全停止
// 并返回该错误。
//
// err 参数报告与 path 相关的错误，表示
// [WalkDir] 不会进入该目录。
// 该函数可以决定如何处理该错误；
// 如前所述，返回错误将导致 WalkDir 停止
// 遍历整个树。
//
// [WalkDir] 在两种情况下使用非 nil err 参数调用该函数。
//
// 首先，如果根目录的初始 [Stat] 失败，WalkDir
// 将调用该函数，path 设置为 root，d 设置为 nil，
// 并将 err 设置为来自 [fs.Stat] 的错误。
//
// 其次，如果目录的 ReadDir 方法（参见 [ReadDirFile]）失败，
// WalkDir 将调用该函数，path 设置为目录的路径，
// d 设置为描述该目录的 [DirEntry]，
// 并将 err 设置为来自 ReadDir 的错误。
// 在这第二种情况下，该函数将以目录路径调用两次：
// 第一次调用是在尝试读取目录之前进行的，
// err 设置为 nil，给予该函数机会返回 [SkipDir]
// 或 [SkipAll] 并完全避免 ReadDir。
// 第二次调用是在 ReadDir 失败后，报告来自 ReadDir 的错误。
// （如果 ReadDir 成功，则没有第二次调用。）
//
// WalkDirFunc 与 [path/filepath.WalkFunc] 的区别：
//
//   - 第二个参数的类型是 [DirEntry] 而不是 [FileInfo]。
//   - 该函数在读取目录之前被调用，
//     以允许 [SkipDir] 或 [SkipAll]
//     分别完全绕过目录读取或跳过所有剩余文件和目录。
//   - 如果目录读取失败，
//     该函数将被第二次调用以报告错误。
type WalkDirFunc func(path string, d DirEntry, err error) error

// walkDir 递归下降 path，调用 walkDirFn。
func walkDir(fsys FS, name string, d DirEntry, walkDirFn WalkDirFunc) error {
	if err := walkDirFn(name, d, nil); err != nil || !d.IsDir() {
		if err == SkipDir && d.IsDir() {
			// 成功跳过目录。
			err = nil
		}
		return err
	}

	dirs, err := ReadDir(fsys, name)
	if err != nil {
		// 第二次调用，以报告 ReadDir 错误。
		err = walkDirFn(name, d, err)
		if err != nil {
			if err == SkipDir && d.IsDir() {
				err = nil
			}
			return err
		}
	}

	for _, d1 := range dirs {
		name1 := path.Join(name, d1.Name())
		if err := walkDir(fsys, name1, d1, walkDirFn); err != nil {
			if err == SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

// WalkDir 遍历以 root 为根的文件树，
// 为树中的每个文件或目录（包括 root）调用 fn。
//
// 访问文件和目录时出现的所有错误都由 fn 过滤：
// 有关详细信息，请参阅 [fs.WalkDirFunc] 文档。
//
// 文件按字典顺序遍历，这使输出具有确定性，
// 但需要 WalkDir 在继续遍历该目录之前
// 将整个目录读入内存。
//
// WalkDir 不会跟随在目录中找到的符号链接，
// 但如果 root 本身是符号链接，则将遍历其目标。
func WalkDir(fsys FS, root string, fn WalkDirFunc) error {
	info, err := Stat(fsys, root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fsys, root, FileInfoToDirEntry(info), fn)
	}
	if err == SkipDir || err == SkipAll {
		return nil
	}
	return err
}
