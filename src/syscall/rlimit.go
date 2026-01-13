// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix

package syscall

import (
	"sync/atomic"
)

// origRlimitNofile，如果非空，是原始的 RLIMIT_NOFILE 软限制。
var origRlimitNofile atomic.Pointer[Rlimit]

// 一些系统为了与使用 select 及其硬编码最大文件描述符
// （受 fd_set 大小限制）的代码兼容，
// 设置了人为的低软限制打开文件数。
//
// Go 不使用 select，所以不应该受这些限制约束。
// 在某些系统上限制是 256，这很容易达到，
// 即使在像 gofmt 这样简单的程序中，当它们并行遍历文件树时也是如此。
//
// 经过 go.dev/issue/46279 的长时间讨论后，我们决定
// 最好的方法是让 Go 无条件地为自己提高限制，
// 然后让旧软件根据需要将限制设回去。
// 真正希望 Go 保持限制不变的代码可以设置硬限制，
// Go 当然别无选择只能遵守。
func init() {
	var lim Rlimit
	if err := Getrlimit(RLIMIT_NOFILE, &lim); err == nil && lim.Max > 0 && lim.Cur < lim.Max-1 {
		origRlimitNofile.Store(&lim)
		nlim := lim

		// 我们将 Cur 设置为 Max - 1，这样我们更有可能
		// 检测到另一个进程使用 prlimit 来更改我们的资源限制的情况。
		// 理论是使用 prlimit 更改为 Cur == Max 比
		// 更改为 Cur == Max - 1 更有可能。
		// 我们检查这一点的地方在 exec_linux.go 中。
		nlim.Cur = nlim.Max - 1

		adjustFileLimit(&nlim)
		setrlimit(RLIMIT_NOFILE, &nlim)
	}
}

func Setrlimit(resource int, rlim *Rlimit) error {
	if resource == RLIMIT_NOFILE {
		// 在 origRlimitNofile 中存储 nil，告诉 StartProcess
		// 不要在子进程中调整 rlimit。
		origRlimitNofile.Store(nil)
	}
	return setrlimit(resource, rlim)
}
