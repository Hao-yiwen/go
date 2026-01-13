// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"sync/atomic"
)

// Once 是一个只会执行一次操作的对象。
//
// Once 在首次使用后不得被复制。
//
// 按照 [Go 内存模型] 的术语，
// f 的返回 "同步先于" once.Do(f) 的任何调用的返回。
//
// [Go 内存模型]: https://go.dev/ref/mem
type Once struct {
	_ noCopy

	// done 表示操作是否已执行。
	// 它放在结构体的第一个位置是因为它在热路径中被使用。
	// 热路径在每个调用点都会被内联。
	// 将 done 放在第一个位置可以在某些架构（amd64/386）上生成更紧凑的指令，
	// 在其他架构上生成更少的指令（用于计算偏移量）。
	done atomic.Bool
	m    Mutex
}

// Do 当且仅当这是该 [Once] 实例第一次调用 Do 时才调用函数 f。
// 换句话说，给定
//
//	var once Once
//
// 如果多次调用 once.Do(f)，只有第一次调用会执行 f，
// 即使每次调用中 f 的值不同。每个要执行的函数都需要一个新的 Once 实例。
//
// Do 用于必须只运行一次的初始化。由于 f 是无参的，
// 可能需要使用函数字面量来捕获要由 Do 调用的函数的参数：
//
//	config.once.Do(func() { config.init(filename) })
//
// 因为在 f 返回之前，Do 的调用不会返回，所以如果 f 导致 Do 被调用，
// 将会发生死锁。
//
// 如果 f 发生 panic，Do 认为它已返回；之后对 Do 的调用将直接返回，不再调用 f。
func (o *Once) Do(f func()) {
	// 注意：这是 Do 的一个错误实现：
	//
	//	if o.done.CompareAndSwap(false, true) {
	//		f()
	//	}
	//
	// Do 保证当它返回时，f 已经完成。
	// 这个实现不会实现该保证：
	// 给定两个同时的调用，CAS 的获胜者会调用 f，
	// 而第二个会立即返回，而不会等待第一个对 f 的调用完成。
	// 这就是为什么慢路径要回退到互斥锁，
	// 以及为什么 o.done.Store 必须延迟到 f 返回之后。

	if !o.done.Load() {
		// 将慢路径分离出来以允许快路径被内联。
		o.doSlow(f)
	}
}

func (o *Once) doSlow(f func()) {
	o.m.Lock()
	defer o.m.Unlock()
	if !o.done.Load() {
		defer o.done.Store(true)
		f()
	}
}
