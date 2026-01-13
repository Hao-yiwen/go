// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// sync 包提供了基本的同步原语，如互斥锁。
// 除了 [Once] 和 [WaitGroup] 类型外，大多数原语都是供底层库例程使用的。
// 更高级的同步最好通过 channel 和通信来完成。
//
// 包含本包中定义的类型的值不应被复制。
package sync

import (
	isync "internal/sync"
)

// Mutex 是一个互斥锁。
// Mutex 的零值是一个未锁定的互斥锁。
//
// Mutex 在首次使用后不得被复制。
//
// 按照 [Go 内存模型] 的术语，对于任何 n < m，
// 第 n 次调用 [Mutex.Unlock] "同步先于" 第 m 次调用 [Mutex.Lock]。
// 成功调用 [Mutex.TryLock] 等同于调用 Lock。
// 失败的 TryLock 调用不会建立任何 "同步先于" 关系。
//
// [Go 内存模型]: https://go.dev/ref/mem
type Mutex struct {
	_ noCopy

	mu isync.Mutex
}

// Locker 表示一个可以被锁定和解锁的对象。
type Locker interface {
	Lock()
	Unlock()
}

// Lock 锁定 m。
// 如果锁已被使用，调用的 goroutine 会阻塞直到互斥锁可用。
func (m *Mutex) Lock() {
	m.mu.Lock()
}

// TryLock 尝试锁定 m 并报告是否成功。
//
// 请注意，虽然 TryLock 确实存在正确的使用方式，但这种情况很少见，
// 使用 TryLock 通常是互斥锁特定用法中存在更深层问题的信号。
func (m *Mutex) TryLock() bool {
	return m.mu.TryLock()
}

// Unlock 解锁 m。
// 如果在调用 Unlock 时 m 未被锁定，则会产生运行时错误。
//
// 锁定的 [Mutex] 不与特定的 goroutine 关联。
// 允许一个 goroutine 锁定 Mutex，然后安排另一个 goroutine 来解锁它。
func (m *Mutex) Unlock() {
	m.mu.Unlock()
}
