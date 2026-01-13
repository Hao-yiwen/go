// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package filepath

import (
	"errors"
	"internal/filepathlite"
	"io/fs"
	"os"
	"runtime"
	"syscall"
)

func walkSymlinks(path string) (string, error) {
	volLen := filepathlite.VolumeNameLen(path)
	pathSeparator := string(os.PathSeparator)

	if volLen < len(path) && os.IsPathSeparator(path[volLen]) {
		volLen++
	}
	vol := path[:volLen]
	dest := vol
	linksWalked := 0
	for start, end := volLen, volLen; start < len(path); start = end {
		for start < len(path) && os.IsPathSeparator(path[start]) {
			start++
		}
		end = start
		for end < len(path) && !os.IsPathSeparator(path[end]) {
			end++
		}

		// 在 Windows 上，"." 可以是符号链接。
		// 我们查找它，如果是绝对路径则使用该值。
		// 如果不是，我们只返回 "."。
		isWindowsDot := runtime.GOOS == "windows" && path[filepathlite.VolumeNameLen(path):] == "."

		// 下一个路径组件在 path[start:end] 中。
		if end == start {
			// 没有更多路径组件。
			break
		} else if path[start:end] == "." && !isWindowsDot {
			// 忽略路径组件 "."。
			continue
		} else if path[start:end] == ".." {
			// 如果可能，回退到前一个组件。
			// 注意 volLen 包含任何前导斜杠。

			// 将 r 设置为 dest 中卷之后最后一个斜杠的索引。
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if os.IsPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen || dest[r+1:] == ".." {
				// 路径没有斜杠（它是空的或只是 "C:"）
				// 或者它以我们必须保留的 ".." 结尾。
				// 无论如何，保留这个 ".."。
				if len(dest) > volLen {
					dest += pathSeparator
				}
				dest += ".."
			} else {
				// 丢弃最后一个斜杠之后的所有内容。
				dest = dest[:r]
			}
			continue
		}

		// 普通路径组件。将其添加到结果中。

		if len(dest) > filepathlite.VolumeNameLen(dest) && !os.IsPathSeparator(dest[len(dest)-1]) {
			dest += pathSeparator
		}

		dest += path[start:end]

		// 解析符号链接。

		fi, err := os.Lstat(dest)
		if err != nil {
			return "", err
		}

		if fi.Mode()&fs.ModeSymlink == 0 {
			if !fi.Mode().IsDir() && end < len(path) {
				return "", syscall.ENOTDIR
			}
			continue
		}

		// 找到符号链接。

		linksWalked++
		if linksWalked > 255 {
			return "", errors.New("EvalSymlinks: too many links")
		}

		link, err := os.Readlink(dest)
		if err != nil {
			return "", err
		}

		if isWindowsDot && !IsAbs(link) {
			// 在 Windows 上，如果 "." 是相对符号链接，
			// 只返回 "."。
			break
		}

		path = link + path[end:]

		v := filepathlite.VolumeNameLen(link)
		if v > 0 {
			// 符号链接到驱动器名称是绝对路径。
			if v < len(link) && os.IsPathSeparator(link[v]) {
				v++
			}
			vol = link[:v]
			dest = vol
			end = len(vol)
		} else if len(link) > 0 && os.IsPathSeparator(link[0]) {
			// 符号链接到绝对路径。
			dest = link[:1]
			end = 1
			vol = link[:1]
			volLen = 1
		} else {
			// 符号链接到相对路径；替换 dest 中的
			// 最后一个路径组件。
			var r int
			for r = len(dest) - 1; r >= volLen; r-- {
				if os.IsPathSeparator(dest[r]) {
					break
				}
			}
			if r < volLen {
				dest = vol
			} else {
				dest = dest[:r]
			}
			end = 0
		}
	}
	return Clean(dest), nil
}
