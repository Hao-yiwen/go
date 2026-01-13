// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package time

import (
	"internal/godebug"
	"unsafe"
)

// Sleep 暂停当前 goroutine 至少 d 时长。
// 负数或零时长会导致 Sleep 立即返回。
func Sleep(d Duration)

var asynctimerchan = godebug.New("asynctimerchan")

// syncTimer 将 c 作为 unsafe.Pointer 返回，用于传递给 newTimer。
// 如果 GODEBUG asynctimerchan 禁用了异步定时器通道代码，
// 则 syncTimer 始终返回 nil，以禁用运行时中的特殊通道代码路径。
func syncTimer(c chan Time) unsafe.Pointer {
	// 如果 asynctimerchan=1，我们甚至不告诉运行时
	// 关于通道定时器的事，这样我们就能获得 Go 1.23 之前的代码路径。
	if asynctimerchan.Value() == "1" {
		asynctimerchan.IncNonDefault()
		return nil
	}

	// 否则传递给运行时。
	// 这处理 asynctimerchan=0（Go 1.23 的默认行为），
	// 以及 asynctimerchan=2（类似于 asynctimerchan=1，
	// 但完全由运行时实现）。
	// 使用 asynctimerchan=2 的唯一原因是调试
	// 由 asynctimerchan=1 修复的问题：它启用新的
	// 可被 GC 的定时器通道（#61542）但不启用同步通道（#37196）。
	//
	// 如果我们决定回滚同步通道，我们仍然会有
	// 一个经过完整测试的异步运行时实现（asynctimerchan=2），
	// 并且可以使这个函数始终返回 c。
	//
	// 如果我们决定保留同步通道，我们可以删除运行时中
	// 所有 asynctimerchan 的处理，只保留这个函数
	// 来处理 asynctimerchan=1。
	return *(*unsafe.Pointer)(unsafe.Pointer(&c))
}

// when 是设置 runtimeTimer 的 'when' 字段的辅助函数。
// 它返回未来 Duration d 之后的时间（以纳秒为单位）。
// 如果 d 是负数，它会被忽略。如果由于溢出导致返回值
// 小于零，则返回 MaxInt64。
func when(d Duration) int64 {
	if d <= 0 {
		return runtimeNano()
	}
	t := runtimeNano() + int64(d)
	if t < 0 {
		// 注意：runtimeNano() 和 d 始终为正，因此加法
		// （包括溢出）永远不会导致 t == 0。
		t = 1<<63 - 1 // math.MaxInt64
	}
	return t
}

// 这些函数从 runtime 包推送到 time 包。

// 参数 cp 是 chan Time，但运行时中的声明使用指针，
// 所以我们这里也使用指针。这使得一些积极比较
// linkname 符号定义的工具更满意。
//
//go:linkname newTimer
func newTimer(when, period int64, f func(any, uintptr, int64), arg any, cp unsafe.Pointer) *Timer

//go:linkname stopTimer
func stopTimer(*Timer) bool

//go:linkname resetTimer
func resetTimer(t *Timer, when, period int64) bool

// 注意：运行时知道 struct Timer 的布局，因为 newTimer 分配它。
// 运行时也知道 Ticker 和 Timer 具有相同的布局。
// 通道后面有额外的字段，保留给运行时使用，用户无法访问。

// Timer 类型表示单个事件。
// 当 Timer 到期时，当前时间将被发送到 C，
// 除非 Timer 是由 [AfterFunc] 创建的。
// Timer 必须使用 [NewTimer] 或 AfterFunc 创建。
type Timer struct {
	C         <-chan Time
	initTimer bool
}

// Stop 阻止 [Timer] 触发。
// 如果调用停止了定时器，返回 true；如果定时器已经到期或已被停止，返回 false。
//
// 对于使用 [AfterFunc](d, f) 创建的基于函数的定时器，
// 如果 t.Stop 返回 false，则定时器已经到期，
// 函数 f 已经在自己的 goroutine 中启动；
// Stop 不会在返回前等待 f 完成。
// 如果调用者需要知道 f 是否已完成，
// 必须与 f 显式协调。
//
// 对于使用 NewTimer(d) 创建的基于通道的定时器，从 Go 1.23 开始，
// Stop 返回后从 t.C 的任何接收都保证会阻塞，
// 而不是接收 Stop 之前的过时时间值；
// 如果程序尚未从 t.C 接收且定时器正在运行，
// Stop 保证返回 true。
// 在 Go 1.23 之前，安全使用 Stop 的唯一方法是
// 如果 Stop 返回 false 则插入额外的 <-t.C 来排空潜在的过时值。
// 更多详情请参阅 [NewTimer] 文档。
func (t *Timer) Stop() bool {
	if !t.initTimer {
		panic("time: Stop called on uninitialized Timer")
	}
	return stopTimer(t)
}

// NewTimer 创建一个新的 Timer，它将在至少 duration d 之后
// 在其通道上发送当前时间。
//
// 在 Go 1.23 之前，垃圾收集器不会回收
// 尚未到期或未被停止的定时器，因此代码通常
// 在调用 NewTimer 后立即 defer t.Stop，以便在
// 不再需要时使定时器可回收。
// 从 Go 1.23 开始，垃圾收集器可以回收未引用的定时器，
// 即使它们尚未到期或未被停止。
// Stop 方法不再需要帮助垃圾收集器。
// （代码当然仍然可以出于其他原因调用 Stop 来停止定时器。）
//
// 在 Go 1.23 之前，与 Timer 关联的通道是
// 异步的（带缓冲，容量为 1），这意味着
// 即使在 [Timer.Stop] 或 [Timer.Reset] 返回后
// 仍可能收到过时的时间值。
// 从 Go 1.23 开始，通道是同步的（无缓冲，容量为 0），
// 消除了这些过时值的可能性。
//
// GODEBUG 设置 asynctimerchan=1 恢复 Go 1.23 之前的两种行为：
// 设置后，未到期的定时器不会被垃圾回收，
// 通道将具有带缓冲的容量。此设置可能会在
// Go 1.27 或更高版本中删除。
func NewTimer(d Duration) *Timer {
	c := make(chan Time, 1)
	t := newTimer(when(d), 0, sendTime, c, syncTimer(c))
	t.C = c
	return t
}

// Reset 将定时器更改为在 duration d 之后到期。
// 如果定时器之前是活动的，返回 true；如果定时器已到期或已被停止，返回 false。
//
// 对于使用 [AfterFunc](d, f) 创建的基于函数的定时器，Reset 要么重新安排
// f 运行的时间（此时 Reset 返回 true），要么安排 f
// 再次运行（此时返回 false）。
// 当 Reset 返回 false 时，Reset 既不会在返回前等待
// 先前的 f 完成，也不保证随后运行 f 的 goroutine
// 不会与先前的并发运行。如果调用者需要知道
// 先前的 f 执行是否已完成，必须与 f 显式协调。
//
// 对于使用 NewTimer 创建的基于通道的定时器，从 Go 1.23 开始，
// Reset 返回后从 t.C 的任何接收都保证不会收到
// 与先前定时器设置对应的时间值；
// 如果程序尚未从 t.C 接收且定时器正在运行，
// Reset 保证返回 true。
// 在 Go 1.23 之前，安全使用 Reset 的唯一方法是先调用 [Timer.Stop]
// 并显式排空定时器。
// 更多详情请参阅 [NewTimer] 文档。
func (t *Timer) Reset(d Duration) bool {
	if !t.initTimer {
		panic("time: Reset called on uninitialized Timer")
	}
	w := when(d)
	return resetTimer(t, w, 0)
}

// sendTime 在 c 上非阻塞地发送当前时间。
func sendTime(c any, seq uintptr, delta int64) {
	// delta 是通道发送本应发生多久之前。
	// 当前时间可以任意远到未来，因为运行时
	// 可以延迟 sendTime 调用，直到 goroutine 尝试从
	// 通道接收。减去 delta 以回到我们过去发送的旧时间。
	select {
	case c.(chan Time) <- Now().Add(Duration(-delta)):
	default:
	}
}

// After 等待时长过去，然后在返回的通道上发送当前时间。
// 它等同于 [NewTimer](d).C。
//
// 在 Go 1.23 之前，此文档警告说底层 [Timer]
// 在定时器触发之前不会被垃圾收集器回收，
// 如果效率是一个问题，代码应该使用 NewTimer 代替，
// 并在不再需要定时器时调用 [Timer.Stop]。
// 从 Go 1.23 开始，垃圾收集器可以回收未引用的、
// 未停止的定时器。当 After 可以满足需求时，没有理由优先使用 NewTimer。
func After(d Duration) <-chan Time {
	return NewTimer(d).C
}

// AfterFunc 等待时长过去，然后在自己的 goroutine 中调用 f。
// 它返回一个 [Timer]，可以使用其 Stop 方法取消调用。
// 返回的 Timer 的 C 字段未使用，将为 nil。
func AfterFunc(d Duration, f func()) *Timer {
	return newTimer(when(d), 0, goFunc, f, nil)
}

func goFunc(arg any, seq uintptr, delta int64) {
	go arg.(func())()
}
