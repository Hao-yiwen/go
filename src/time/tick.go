// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package time

import "unsafe"

// 注意：运行时知道 struct Ticker 的布局，因为 newTimer 分配它。
// 另请注意 Ticker 和 Timer 具有相同的布局，因此 newTimer 可以处理两者。
// initTimer 和 initTicker 字段命名不同，
// 这样用户就不能在没有 unsafe 的情况下在两者之间转换。

// Ticker 持有一个通道，该通道按间隔传递时钟的 "tick"。
type Ticker struct {
	C          <-chan Time // tick 传递的通道。
	initTicker bool
}

// NewTicker 返回一个新的 [Ticker]，其中包含一个通道，该通道将在每次 tick 后
// 在通道上发送当前时间。tick 的周期由 duration 参数指定。
// ticker 将调整时间间隔或丢弃 tick 以弥补慢接收者。
// duration d 必须大于零；否则 NewTicker 将 panic。
//
// 在 Go 1.23 之前，垃圾收集器不会回收
// 尚未到期或未被停止的 ticker，因此代码通常
// 在调用 NewTicker 后立即 defer t.Stop，以便在
// 不再需要时使 ticker 可回收。
// 从 Go 1.23 开始，垃圾收集器可以回收未引用的 ticker，
// 即使它们尚未被停止。
// Stop 方法不再需要帮助垃圾收集器。
// （代码当然仍然可以出于其他原因调用 Stop 来停止 ticker。）
func NewTicker(d Duration) *Ticker {
	if d <= 0 {
		panic("non-positive interval for NewTicker")
	}
	// 给通道一个 1 元素的时间缓冲区。
	// 如果客户端在读取时落后，我们会丢弃 tick，
	// 直到客户端赶上。
	c := make(chan Time, 1)
	t := (*Ticker)(unsafe.Pointer(newTimer(when(d), int64(d), sendTime, c, syncTimer(c))))
	t.C = c
	return t
}

// Stop 关闭 ticker。Stop 之后，不会再发送 tick。
// Stop 不会关闭通道，以防止并发的从通道读取的 goroutine
// 看到错误的 "tick"。
func (t *Ticker) Stop() {
	if !t.initTicker {
		// 这是误用，对于 time.Timer 同样会 panic，
		// 但这并不总是 panic，我们保持它不 panic
		// 以避免破坏旧程序。参见 issue 21874。
		return
	}
	stopTimer((*Timer)(unsafe.Pointer(t)))
}

// Reset 停止 ticker 并将其周期重置为指定的 duration。
// 下一个 tick 将在新周期过去后到达。duration d
// 必须大于零；否则 Reset 将 panic。
func (t *Ticker) Reset(d Duration) {
	if d <= 0 {
		panic("non-positive interval for Ticker.Reset")
	}
	if !t.initTicker {
		panic("time: Reset called on uninitialized Ticker")
	}
	resetTimer((*Timer)(unsafe.Pointer(t)), when(d), int64(d))
}

// Tick 是 [NewTicker] 的便捷包装器，仅提供对 tick 通道的访问。
// 与 NewTicker 不同，如果 d <= 0，Tick 将返回 nil。
//
// 在 Go 1.23 之前，此文档警告说底层 [Ticker]
// 永远不会被垃圾收集器回收，如果效率是一个问题，
// 代码应该使用 NewTicker 代替，并在不再需要 ticker 时
// 调用 [Ticker.Stop]。
// 从 Go 1.23 开始，垃圾收集器可以回收未引用的 ticker，
// 即使它们尚未被停止。
// Stop 方法不再需要帮助垃圾收集器。
// 当 Tick 可以满足需求时，不再有任何理由优先使用 NewTicker。
func Tick(d Duration) <-chan Time {
	if d <= 0 {
		return nil
	}
	return NewTicker(d).C
}
