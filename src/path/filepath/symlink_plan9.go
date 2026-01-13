// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package filepath

import (
	"os"
	"strings"
	"syscall"
)

func evalSymlinks(path string) (string, error) {
	// Plan 9 没有符号链接，所以不需要替换。
	if len(path) > 0 {
		// 检查路径有效性
		_, err := os.Lstat(path)
		if err != nil {
			// 返回与其他操作系统相同的错误值。
			if strings.Contains(err.Error(), "not a directory") {
				err = syscall.ENOTDIR
			}
			return "", err
		}
	}
	return Clean(path), nil
}
