// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package fs

import (
	"time"
)

// FormatFileInfo 返回 info 的格式化版本，便于人类阅读。
// [FileInfo] 的实现可以从 String 方法调用它。
// 对于名为 "hello.go"、100 字节、权限 0o644、创建于
// 1970 年 1 月 1 日中午的文件的输出为：
//
//	-rw-r--r-- 100 1970-01-01 12:00:00 hello.go
func FormatFileInfo(info FileInfo) string {
	name := info.Name()
	b := make([]byte, 0, 40+len(name))
	b = append(b, info.Mode().String()...)
	b = append(b, ' ')

	size := info.Size()
	var usize uint64
	if size >= 0 {
		usize = uint64(size)
	} else {
		b = append(b, '-')
		usize = uint64(-size)
	}
	var buf [20]byte
	i := len(buf) - 1
	for usize >= 10 {
		q := usize / 10
		buf[i] = byte('0' + usize - q*10)
		i--
		usize = q
	}
	buf[i] = byte('0' + usize)
	b = append(b, buf[i:]...)
	b = append(b, ' ')

	b = append(b, info.ModTime().Format(time.DateTime)...)
	b = append(b, ' ')

	b = append(b, name...)
	if info.IsDir() {
		b = append(b, '/')
	}

	return string(b)
}

// FormatDirEntry 返回 dir 的格式化版本，便于人类阅读。
// [DirEntry] 的实现可以从 String 方法调用它。
// 对于名为 subdir 的目录和名为 hello.go 的文件的输出为：
//
//	d subdir/
//	- hello.go
func FormatDirEntry(dir DirEntry) string {
	name := dir.Name()
	b := make([]byte, 0, 5+len(name))

	// Type 方法不返回任何权限位，
	// 因此从字符串中删除它们。
	mode := dir.Type().String()
	mode = mode[:len(mode)-9]

	b = append(b, mode...)
	b = append(b, ' ')
	b = append(b, name...)
	if dir.IsDir() {
		b = append(b, '/')
	}
	return string(b)
}
