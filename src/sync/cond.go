// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"sync/atomic"
	"unsafe"
)

// Cond 实现了一个条件变量，它是 goroutine 等待或宣布事件发生的汇合点。
//
// 每个 Cond 都有一个关联的 Locker L（通常是 [*Mutex] 或 [*RWMutex]），
// 在更改条件和调用 [Cond.Wait] 方法时必须持有该锁。
//
// Cond 在首次使用后不得被复制。
//
// 按照 [Go 内存模型] 的术语，Cond 保证对 [Cond.Broadcast] 或 [Cond.Signal]
// 的调用 "同步先于" 它所唤醒的任何 Wait 调用。
//
// 对于许多简单的使用场景，用户使用 channel 会比使用 Cond 更好
// （Broadcast 对应于关闭 channel，Signal 对应于向 channel 发送）。
//
// 有关 [sync.Cond] 替代方案的更多信息，请参阅 [Roberto Clapis 的高级并发模式系列文章]
// 以及 [Bryan Mills 的并发模式演讲]。
//
// [Go 内存模型]: https://go.dev/ref/mem
// [Roberto Clapis 的高级并发模式系列文章]: https://blogtitle.github.io/categories/concurrency/
// [Bryan Mills 的并发模式演讲]: https://drive.google.com/file/d/1nPdvhB0PutEJzdCq5ms6UI58dp50fcAN/view
type Cond struct {
	noCopy noCopy

	// L 在观察或更改条件时被持有
	L Locker

	notify  notifyList
	checker copyChecker
}

// NewCond 返回一个使用 Locker l 的新 Cond。
func NewCond(l Locker) *Cond {
	return &Cond{L: l}
}

// Wait 原子地解锁 c.L 并暂停调用 goroutine 的执行。
// 在稍后恢复执行后，Wait 在返回前会锁定 c.L。
// 与其他系统不同，除非被 [Cond.Broadcast] 或 [Cond.Signal] 唤醒，
// 否则 Wait 不会返回。
//
// 因为在 Wait 等待期间 c.L 未被锁定，调用者通常不能假定
// Wait 返回时条件为真。相反，调用者应该在循环中调用 Wait：
//
//	c.L.Lock()
//	for !condition() {
//	    c.Wait()
//	}
//	... 使用条件 ...
//	c.L.Unlock()
func (c *Cond) Wait() {
	c.checker.check()
	t := runtime_notifyListAdd(&c.notify)
	c.L.Unlock()
	runtime_notifyListWait(&c.notify, t)
	c.L.Lock()
}

// Signal 唤醒一个等待 c 的 goroutine（如果有的话）。
//
// 调用者在调用期间可以持有 c.L，但这不是必需的。
//
// Signal() 不影响 goroutine 的调度优先级；如果其他 goroutine
// 正在尝试锁定 c.L，它们可能会在"等待中"的 goroutine 之前被唤醒。
func (c *Cond) Signal() {
	c.checker.check()
	runtime_notifyListNotifyOne(&c.notify)
}

// Broadcast 唤醒所有等待 c 的 goroutine。
//
// 调用者在调用期间可以持有 c.L，但这不是必需的。
func (c *Cond) Broadcast() {
	c.checker.check()
	runtime_notifyListNotifyAll(&c.notify)
}

// copyChecker 保存指向自身的反向指针以检测对象复制。
type copyChecker uintptr

func (c *copyChecker) check() {
	// 通过三个步骤检查 c 是否已被复制：
	// 1. 第一个比较是快速路径。如果 c 已初始化且未被复制，这将立即返回。否则，c 要么未初始化，要么已被复制。
	// 2. 确保 c 已初始化。如果 CAS 成功，我们就完成了。如果失败，c 要么是被并发初始化而我们输掉了竞争，要么 c 已被复制。
	// 3. 再次执行步骤 1。现在 c 肯定已初始化，如果这次失败，说明 c 已被复制。
	if uintptr(*c) != uintptr(unsafe.Pointer(c)) &&
		!atomic.CompareAndSwapUintptr((*uintptr)(c), 0, uintptr(unsafe.Pointer(c))) &&
		uintptr(*c) != uintptr(unsafe.Pointer(c)) {
		panic("sync.Cond is copied")
	}
}

// noCopy 可以添加到首次使用后不得复制的结构体中。
//
// 详情请参阅 https://golang.org/issues/8005#issuecomment-190753527。
//
// 注意，由于 Lock 和 Unlock 方法的存在，它不能被嵌入。
type noCopy struct{}

// Lock 是一个空操作，被 `go vet` 的 -copylocks 检查器使用。
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
