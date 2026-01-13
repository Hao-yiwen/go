// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// syscall 包提供了底层操作系统原语的接口。
// 具体细节因底层系统而异，默认情况下，godoc 会显示当前系统的 syscall 文档。
// 如果你想让 godoc 显示其他系统的 syscall 文档，请设置 $GOOS 和 $GOARCH 为目标系统。
// 例如，如果你想在 linux/amd64 上查看 freebsd/arm 的文档，
// 请将 $GOOS 设置为 freebsd，将 $GOARCH 设置为 arm。
// syscall 的主要用途是在其他提供更可移植系统接口的包内部使用，
// 例如 "os"、"time" 和 "net"。如果可能，请使用那些包而不是这个包。
// 有关本包中函数和数据类型的详细信息，请参阅相应操作系统的手册。
// 这些调用返回 err == nil 表示成功；否则 err 是描述失败的操作系统错误。
// 在大多数系统上，该错误的类型为 [Errno]。
//
// 注意：本包中定义的大多数函数、类型和常量也可在 [golang.org/x/sys] 包中找到。
// 该包比本包提供更多的系统调用支持，大多数新代码应尽可能优先使用该包。
// 更多信息请参阅 https://golang.org/s/go1.4-syscall。
package syscall

import "internal/bytealg"

//go:generate go run ./mksyscall_windows.go -systemdll -output zsyscall_windows.go syscall_windows.go security_windows.go

// StringByteSlice 将字符串转换为以 NUL 结尾的 []byte。
// 如果 s 包含 NUL 字节，此函数会 panic 而不是返回错误。
//
// 已弃用：请使用 ByteSliceFromString 代替。
func StringByteSlice(s string) []byte {
	a, err := ByteSliceFromString(s)
	if err != nil {
		panic("syscall: string with NUL passed to StringByteSlice")
	}
	return a
}

// ByteSliceFromString 返回一个以 NUL 结尾的字节切片，
// 包含 s 的文本内容。如果 s 在任何位置包含 NUL 字节，
// 则返回 (nil, [EINVAL])。
func ByteSliceFromString(s string) ([]byte, error) {
	if bytealg.IndexByteString(s, 0) != -1 {
		return nil, EINVAL
	}
	a := make([]byte, len(s)+1)
	copy(a, s)
	return a, nil
}

// StringBytePtr 返回指向以 NUL 结尾的字节数组的指针。
// 如果 s 包含 NUL 字节，此函数会 panic 而不是返回错误。
//
// 已弃用：请使用 [BytePtrFromString] 代替。
func StringBytePtr(s string) *byte { return &StringByteSlice(s)[0] }

// BytePtrFromString 返回指向以 NUL 结尾的字节数组的指针，
// 包含 s 的文本内容。如果 s 在任何位置包含 NUL 字节，
// 则返回 (nil, [EINVAL])。
func BytePtrFromString(s string) (*byte, error) {
	a, err := ByteSliceFromString(s)
	if err != nil {
		return nil, err
	}
	return &a[0], nil
}

// 单字零值，用于当我们需要指向 0 字节的有效指针时使用。
// 参见 mksyscall.pl。
var _zero uintptr

// Unix 返回存储在 ts 中的时间，以秒加纳秒的形式表示。
func (ts *Timespec) Unix() (sec int64, nsec int64) {
	return int64(ts.Sec), int64(ts.Nsec)
}

// Unix 返回存储在 tv 中的时间，以秒加纳秒的形式表示。
func (tv *Timeval) Unix() (sec int64, nsec int64) {
	return int64(tv.Sec), int64(tv.Usec) * 1000
}

// Nano 返回存储在 ts 中的时间，以纳秒表示。
func (ts *Timespec) Nano() int64 {
	return int64(ts.Sec)*1e9 + int64(ts.Nsec)
}

// Nano 返回存储在 tv 中的时间，以纳秒表示。
func (tv *Timeval) Nano() int64 {
	return int64(tv.Sec)*1e9 + int64(tv.Usec)*1000
}

// Getpagesize 和 Exit 由运行时提供。

func Getpagesize() int
func Exit(code int)

// runtimeSetenv 和 runtimeUnsetenv 由运行时提供。
func runtimeSetenv(k, v string)
func runtimeUnsetenv(k string)

// runtimeClearenv 由运行时提供（在没有 clearenv(3) 的平台上，
// 它只是 runtimeUnsetenv 的包装器）。
func runtimeClearenv(env map[string]int)
