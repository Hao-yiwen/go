// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build dragonfly || freebsd || linux || netbsd || openbsd || solaris

package syscall

import "sync"

// forkExecPipe 原子地打开一个管道，并在两个文件描述符上设置 O_CLOEXEC。
func forkExecPipe(p []int) error {
	return Pipe2(p, O_CLOEXEC)
}

var (
	// 保护 forking 变量。
	forkingLock sync.Mutex
	// 当前正在 fork 的 goroutine 数量，也就是持有 ForkLock 概念写锁的 goroutine 数量。
	forking int
)

// hasWaitingReaders 报告是否有任何 goroutine 正在等待获取 rw 的读锁。
// 它在 sync 包中定义。
func hasWaitingReaders(rw *sync.RWMutex) bool

// acquireForkLock 获取 ForkLock 的写锁。
// ForkLock 是导出的，我们承诺在 fork 期间会调用 ForkLock.Lock，
// 这样在我们 fork 之前，其他线程不会创建尚未设置 close-on-exec 的新 fd。
// 但这会强制所有 fork 调用串行化，这是不好的。
// 但我们没有承诺这种串行化，而且其他 ForkLock 用户基本上无法检测到，这是好的。
// 通过确保 ForkLock 在第一次 fork 时锁定，并在没有更多 fork 时解锁，来避免串行化。
func acquireForkLock() {
	forkingLock.Lock()
	defer forkingLock.Unlock()

	if forking == 0 {
		// 当前 ForkLock 没有写锁。
		ForkLock.Lock()
		forking++
		return
	}

	// ForkLock 当前被写锁定。

	if hasWaitingReaders(&ForkLock) {
		// ForkLock 被写锁定，至少有一个 goroutine 正在等待读取。
		// 为了避免锁饥饿，允许读者继续。
		// 简单的方法是让我们获取一个读锁。
		// 这将阻塞我们直到所有当前概念写锁被释放。
		//
		// 注意，在具有 O_CLOEXEC 和 SOCK_CLOEXEC 的现代系统上，
		// 这种情况是不寻常的。在这些系统上，
		// 标准库不应该在 ForkLock 上获取读锁。

		forkingLock.Unlock()

		ForkLock.RLock()
		ForkLock.RUnlock()

		forkingLock.Lock()

		// 读者有机会了，所以现在获取写锁。

		if forking == 0 {
			ForkLock.Lock()
		}
	}

	forking++
}

// releaseForkLock 释放由 acquireForkLock 获取的 ForkLock 概念写锁。
func releaseForkLock() {
	forkingLock.Lock()
	defer forkingLock.Unlock()

	if forking <= 0 {
		panic("syscall.releaseForkLock: negative count")
	}

	forking--

	if forking == 0 {
		// 没有更多的概念写锁。
		ForkLock.Unlock()
	}
}
