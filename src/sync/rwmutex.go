// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"internal/race"
	"sync/atomic"
	"unsafe"
)

// runtime/rwmutex.go 中有此文件的修改副本。
// 如果你在这里做任何更改，请检查是否也应该在那里做更改。

// RWMutex 是一个读写互斥锁。
// 该锁可以被任意数量的读者或单个写者持有。
// RWMutex 的零值是一个未锁定的互斥锁。
//
// RWMutex 在首次使用后不得被复制。
//
// 如果任何 goroutine 在锁已被一个或多个读者持有时调用 [RWMutex.Lock]，
// 对 [RWMutex.RLock] 的并发调用将阻塞，直到写者获取（并释放）锁，
// 以确保锁最终对写者可用。
// 请注意，这禁止递归读锁定。
// [RWMutex.RLock] 不能升级为 [RWMutex.Lock]，
// [RWMutex.Lock] 也不能降级为 [RWMutex.RLock]。
//
// 按照 [Go 内存模型] 的术语，对于任何 n < m，
// 第 n 次调用 [RWMutex.Unlock] "同步先于" 第 m 次调用 Lock，
// 就像 [Mutex] 一样。
// 对于任何 RLock 调用，存在一个 n 使得
// 第 n 次调用 Unlock "同步先于" 该 RLock 调用，
// 并且相应的 [RWMutex.RUnlock] 调用 "同步先于" 第 n+1 次调用 Lock。
//
// [Go 内存模型]: https://go.dev/ref/mem
type RWMutex struct {
	w           Mutex        // 如果有等待的写者则被持有
	writerSem   uint32       // 写者等待完成中的读者的信号量
	readerSem   uint32       // 读者等待完成中的写者的信号量
	readerCount atomic.Int32 // 等待中的读者数量
	readerWait  atomic.Int32 // 离开中的读者数量
}

const rwmutexMaxReaders = 1 << 30

// 通过以下方式向竞态检测器指示 happens-before 关系：
// - Unlock  -> Lock:  readerSem
// - Unlock  -> RLock: readerSem
// - RUnlock -> Lock:  writerSem
//
// 下面的方法暂时禁用竞态同步事件的处理，
// 以便向竞态检测器提供上述更精确的模型。
//
// 例如，RLock 中的 atomic.AddInt32 不应该看起来提供
// acquire-release 语义，这会错误地同步竞争的读者，
// 从而可能遗漏竞态。

// RLock 为读取锁定 rw。
//
// 不应将其用于递归读锁定；被阻塞的 Lock 调用会阻止新读者获取锁。
// 请参阅 [RWMutex] 类型的文档。
func (rw *RWMutex) RLock() {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.Disable()
	}
	if rw.readerCount.Add(1) < 0 {
		// 有写者在等待，等待它。
		runtime_SemacquireRWMutexR(&rw.readerSem, false, 0)
	}
	if race.Enabled {
		race.Enable()
		race.Acquire(unsafe.Pointer(&rw.readerSem))
	}
}

// TryRLock 尝试为读取锁定 rw 并报告是否成功。
//
// 请注意，虽然 TryRLock 确实存在正确的使用方式，但这种情况很少见，
// 使用 TryRLock 通常是互斥锁特定用法中存在更深层问题的信号。
func (rw *RWMutex) TryRLock() bool {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.Disable()
	}
	for {
		c := rw.readerCount.Load()
		if c < 0 {
			if race.Enabled {
				race.Enable()
			}
			return false
		}
		if rw.readerCount.CompareAndSwap(c, c+1) {
			if race.Enabled {
				race.Enable()
				race.Acquire(unsafe.Pointer(&rw.readerSem))
			}
			return true
		}
	}
}

// RUnlock 撤销单个 [RWMutex.RLock] 调用；
// 它不影响其他同时进行的读者。
// 如果在调用 RUnlock 时 rw 没有被锁定用于读取，则会产生运行时错误。
func (rw *RWMutex) RUnlock() {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.ReleaseMerge(unsafe.Pointer(&rw.writerSem))
		race.Disable()
	}
	if r := rw.readerCount.Add(-1); r < 0 {
		// 将慢路径分离出来以允许快路径被内联
		rw.rUnlockSlow(r)
	}
	if race.Enabled {
		race.Enable()
	}
}

func (rw *RWMutex) rUnlockSlow(r int32) {
	if r+1 == 0 || r+1 == -rwmutexMaxReaders {
		race.Enable()
		fatal("sync: RUnlock of unlocked RWMutex")
	}
	// 有写者在等待。
	if rw.readerWait.Add(-1) == 0 {
		// 最后一个读者解除写者的阻塞。
		runtime_Semrelease(&rw.writerSem, false, 1)
	}
}

// Lock 为写入锁定 rw。
// 如果锁已被锁定用于读取或写入，Lock 将阻塞直到锁可用。
func (rw *RWMutex) Lock() {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.Disable()
	}
	// 首先，解决与其他写者的竞争。
	rw.w.Lock()
	// 向读者宣布有一个等待中的写者。
	r := rw.readerCount.Add(-rwmutexMaxReaders) + rwmutexMaxReaders
	// 等待活跃的读者。
	if r != 0 && rw.readerWait.Add(r) != 0 {
		runtime_SemacquireRWMutex(&rw.writerSem, false, 0)
	}
	if race.Enabled {
		race.Enable()
		race.Acquire(unsafe.Pointer(&rw.readerSem))
		race.Acquire(unsafe.Pointer(&rw.writerSem))
	}
}

// TryLock 尝试为写入锁定 rw 并报告是否成功。
//
// 请注意，虽然 TryLock 确实存在正确的使用方式，但这种情况很少见，
// 使用 TryLock 通常是互斥锁特定用法中存在更深层问题的信号。
func (rw *RWMutex) TryLock() bool {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.Disable()
	}
	if !rw.w.TryLock() {
		if race.Enabled {
			race.Enable()
		}
		return false
	}
	if !rw.readerCount.CompareAndSwap(0, -rwmutexMaxReaders) {
		rw.w.Unlock()
		if race.Enabled {
			race.Enable()
		}
		return false
	}
	if race.Enabled {
		race.Enable()
		race.Acquire(unsafe.Pointer(&rw.readerSem))
		race.Acquire(unsafe.Pointer(&rw.writerSem))
	}
	return true
}

// Unlock 解锁 rw 的写锁定。如果在调用 Unlock 时 rw 没有被锁定用于写入，
// 则会产生运行时错误。
//
// 与 Mutex 一样，锁定的 [RWMutex] 不与特定的 goroutine 关联。
// 一个 goroutine 可以 [RWMutex.RLock]（[RWMutex.Lock]）一个 RWMutex，
// 然后安排另一个 goroutine 来 [RWMutex.RUnlock]（[RWMutex.Unlock]）它。
func (rw *RWMutex) Unlock() {
	if race.Enabled {
		race.Read(unsafe.Pointer(&rw.w))
		race.Release(unsafe.Pointer(&rw.readerSem))
		race.Disable()
	}

	// 向读者宣布没有活跃的写者。
	r := rw.readerCount.Add(rwmutexMaxReaders)
	if r >= rwmutexMaxReaders {
		race.Enable()
		fatal("sync: Unlock of unlocked RWMutex")
	}
	// 解除被阻塞的读者（如果有）。
	for i := 0; i < int(r); i++ {
		runtime_Semrelease(&rw.readerSem, false, 0)
	}
	// 允许其他写者继续。
	rw.w.Unlock()
	if race.Enabled {
		race.Enable()
	}
}

// syscall_hasWaitingReaders 报告是否有任何 goroutine 正在等待获取 rw 的读锁。
// 这个函数存在是因为 syscall.ForkLock 是一个 RWMutex，
// 我们不能在不破坏兼容性的情况下更改它。
// 我们不需要也不想要 ForkLock 的 RWMutex 语义，我们使用这个私有 API
// 来避免必须更改 ForkLock 的类型。
// 有关更多详细信息，请参阅 syscall 包。
//
//go:linkname syscall_hasWaitingReaders syscall.hasWaitingReaders
func syscall_hasWaitingReaders(rw *RWMutex) bool {
	r := rw.readerCount.Load()
	return r < 0 && r+rwmutexMaxReaders > 0
}

// RLocker 返回一个 [Locker] 接口，该接口通过调用 rw.RLock 和 rw.RUnlock
// 实现 [Locker.Lock] 和 [Locker.Unlock] 方法。
func (rw *RWMutex) RLocker() Locker {
	return (*rlocker)(rw)
}

type rlocker RWMutex

func (r *rlocker) Lock()   { (*RWMutex)(r).RLock() }
func (r *rlocker) Unlock() { (*RWMutex)(r).RUnlock() }
