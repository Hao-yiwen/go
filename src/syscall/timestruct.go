// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm) || wasip1

package syscall

// TimespecToNsec 返回存储在 ts 中的时间，以纳秒表示。
func TimespecToNsec(ts Timespec) int64 { return ts.Nano() }

// NsecToTimespec 将纳秒数转换为 [Timespec]。
func NsecToTimespec(nsec int64) Timespec {
	sec := nsec / 1e9
	nsec = nsec % 1e9
	if nsec < 0 {
		nsec += 1e9
		sec--
	}
	return setTimespec(sec, nsec)
}

// TimevalToNsec 返回存储在 tv 中的时间，以纳秒表示。
func TimevalToNsec(tv Timeval) int64 { return tv.Nano() }

// NsecToTimeval 将纳秒数转换为 [Timeval]。
func NsecToTimeval(nsec int64) Timeval {
	nsec += 999 // 向上舍入到微秒
	usec := nsec % 1e9 / 1e3
	sec := nsec / 1e9
	if usec < 0 {
		usec += 1e6
		sec--
	}
	return setTimeval(sec, usec)
}
