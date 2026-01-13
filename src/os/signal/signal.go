// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package signal

import (
	"context"
	"os"
	"slices"
	"sync"
)

var handlers struct {
	sync.Mutex
	// 将通道映射到应该发送给它的信号。
	m map[chan<- os.Signal]*handler
	// 将信号映射到接收它的通道数。
	ref [numSig]int64
	// 在通道被停止时将通道映射到信号。
	// 不是映射，因为条目在这里只存在很短的时间。
	// 我们需要一个单独的容器，因为我们需要 m 对应于 ref
	// 在所有时间，并且我们还需要跟踪为
	// 被停止的通道的 *handler
	// 值。参见 Stop 函数。
	stopping []stopping
}

type stopping struct {
	c chan<- os.Signal
	h *handler
}

type handler struct {
	mask [(numSig + 31) / 32]uint32
}

func (h *handler) want(sig int) bool {
	return (h.mask[sig/32]>>uint(sig&31))&1 != 0
}

func (h *handler) set(sig int) {
	h.mask[sig/32] |= 1 << uint(sig&31)
}

func (h *handler) clear(sig int) {
	h.mask[sig/32] &^= 1 << uint(sig&31)
}

// 停止将信号 sigs 中继到任何先前注册的通道
// 接收它们，并要么将信号处理程序重置为其原始值
// （action=disableSignal），要么忽略信号（action=ignoreSignal）。
func cancel(sigs []os.Signal, action func(int)) {
	handlers.Lock()
	defer handlers.Unlock()

	remove := func(n int) {
		var zerohandler handler

		for c, h := range handlers.m {
			if h.want(n) {
				handlers.ref[n]--
				h.clear(n)
				if h.mask == zerohandler.mask {
					delete(handlers.m, c)
				}
			}
		}

		action(n)
	}

	if len(sigs) == 0 {
		for n := 0; n < numSig; n++ {
			remove(n)
		}
	} else {
		for _, s := range sigs {
			remove(signum(s))
		}
	}
}

// Ignore 导致提供的信号被忽略。如果程序接收到它们，
// 将不会发生任何事情。Ignore 撤销任何先前
// 对提供的信号调用 [Notify] 的效果。
// 如果未提供信号，所有传入信号都将被忽略。
func Ignore(sig ...os.Signal) {
	cancel(sig, ignoreSignal)
}

// Ignored 报告 sig 当前是否被忽略。
func Ignored(sig os.Signal) bool {
	sn := signum(sig)
	return sn >= 0 && signalIgnored(sn)
}

var (
	// watchSignalLoopOnce 保护对条件
	// 初始化的 watchSignalLoop 的调用。如果 watchSignalLoop 非 nil，
	// 它将在调用 Notify 时在 goroutine 中懒惰地运行。
	// 参见 Issue 21576。
	watchSignalLoopOnce sync.Once
	watchSignalLoop     func()
)

// Notify 导致包 signal 将传入信号中继到 c。
// 如果未提供信号，所有传入信号都将被中继到 c。
// 否则，只有提供的信号才会。
//
// 包 signal 不会阻止向 c 发送：调用者必须确保
// c 有足够的缓冲区空间来跟上预期的
// 信号速率。对于用于仅通知一个信号值的通道，
// 大小为 1 的缓冲区就足够了。
//
// 允许用同一通道多次调用 Notify：
// 每次调用都会扩展发送到该通道的信号集合。
// 从集合中删除信号的唯一方法是调用 [Stop]。
//
// 允许用不同的通道和相同的信号多次调用 Notify：
// 每个通道独立地接收传入信号的副本。
func Notify(c chan<- os.Signal, sig ...os.Signal) {
	if c == nil {
		panic("os/signal: Notify using nil channel")
	}

	handlers.Lock()
	defer handlers.Unlock()

	h := handlers.m[c]
	if h == nil {
		if handlers.m == nil {
			handlers.m = make(map[chan<- os.Signal]*handler)
		}
		h = new(handler)
		handlers.m[c] = h
	}

	add := func(n int) {
		if n < 0 {
			return
		}
		if !h.want(n) {
			h.set(n)
			if handlers.ref[n] == 0 {
				enableSignal(n)

				// 运行时要求我们在启动监视程序之前
				// 启用信号。
				watchSignalLoopOnce.Do(func() {
					if watchSignalLoop != nil {
						go watchSignalLoop()
					}
				})
			}
			handlers.ref[n]++
		}
	}

	if len(sig) == 0 {
		for n := 0; n < numSig; n++ {
			add(n)
		}
	} else {
		for _, s := range sig {
			add(signum(s))
		}
	}
}

// Reset 撤销任何先前调用 [Notify] 对提供的
// 信号的影响。
// 如果未提供信号，所有信号处理程序都将被重置。
func Reset(sig ...os.Signal) {
	cancel(sig, disableSignal)
}

// Stop 导致包 signal 停止将传入信号中继到 c。
// 它撤销使用 c 的所有先前调用 [Notify] 的影响。
// 当 Stop 返回时，保证 c 不会再接收信号。
func Stop(c chan<- os.Signal) {
	handlers.Lock()

	h := handlers.m[c]
	if h == nil {
		handlers.Unlock()
		return
	}
	delete(handlers.m, c)

	for n := 0; n < numSig; n++ {
		if h.want(n) {
			handlers.ref[n]--
			if handlers.ref[n] == 0 {
				disableSignal(n)
			}
		}
	}

	// 信号将不再被发送到通道。
	// 我们想避免 SIGINT 等信号的竞争：
	// 它应该要么被发送到通道，
	// 要么程序应该采取默认操作（即退出）。
	// 为了避免信号被发送、信号处理程序被调用，
	// 然后 Stop 在 process 函数下方有机会
	// 在通道上发送它之前注销通道的可能性，
	// 将通道放在被停止的
	// 通道列表上，并等待信号发送
	// 平静后再完全移除它。

	handlers.stopping = append(handlers.stopping, stopping{c, h})

	handlers.Unlock()

	signalWaitUntilIdle()

	handlers.Lock()

	for i, s := range handlers.stopping {
		if s.c == c {
			handlers.stopping = slices.Delete(handlers.stopping, i, i+1)
			break
		}
	}

	handlers.Unlock()
}

// 等待直到没有更多信号等待被发送。
// 由运行时包定义。
func signalWaitUntilIdle()

func process(sig os.Signal) {
	n := signum(sig)
	if n < 0 {
		return
	}

	handlers.Lock()
	defer handlers.Unlock()

	for c, h := range handlers.m {
		if h.want(n) {
			// 发送但不为其阻塞
			select {
			case c <- sig:
			default:
			}
		}
	}

	// 避免 Stop 中提到的竞争。
	for _, d := range handlers.stopping {
		if d.h.want(n) {
			select {
			case d.c <- sig:
			default:
			}
		}
	}
}

// NotifyContext 返回父上下文的副本，该上下文被标记为完成
// （其 Done 通道已关闭），当列出的信号之一到达、
// 返回的停止函数被调用或父上下文的
// Done 通道被关闭时（以先发生的为准）。
//
// 停止函数注销信号行为，这与 [signal.Reset] 类似，
// 可能会恢复给定信号的默认行为。例如，Go 程序接收 [os.Interrupt] 的默认
// 行为是退出。调用
// NotifyContext(parent, os.Interrupt) 将改变行为以取消
// 返回的上下文。接收到的未来中断将不会触发默认
// （退出）行为，直到返回的停止函数被调用。
//
// 如果信号导致返回的上下文被取消，调用
// [context.Cause] 会返回一个描述该信号的错误。
//
// 停止函数释放与其关联的资源，所以代码应该
// 在此上下文中运行的操作完成且
// 信号不再需要被转向到上下文时立即调用停止。
func NotifyContext(parent context.Context, signals ...os.Signal) (ctx context.Context, stop context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	c := &signalCtx{
		Context: ctx,
		cancel:  cancel,
		signals: signals,
	}
	c.ch = make(chan os.Signal, 1)
	Notify(c.ch, c.signals...)
	if ctx.Err() == nil {
		go func() {
			select {
			case s := <-c.ch:
				c.cancel(signalError(s.String() + " signal received"))
			case <-c.Done():
			}
		}()
	}
	return c, c.stop
}

type signalCtx struct {
	context.Context

	cancel  context.CancelCauseFunc
	signals []os.Signal
	ch      chan os.Signal
}

func (c *signalCtx) stop() {
	c.cancel(nil)
	Stop(c.ch)
}

type stringer interface {
	String() string
}

func (c *signalCtx) String() string {
	var buf []byte
	// 我们知道 c.Context 的类型是 context.cancelCtx，我们知道
	// cancelCtx 的 String 方法返回以".WithCancel"结尾的字符串。
	name := c.Context.(stringer).String()
	name = name[:len(name)-len(".WithCancel")]
	buf = append(buf, "signal.NotifyContext("+name...)
	if len(c.signals) != 0 {
		buf = append(buf, ", ["...)
		for i, s := range c.signals {
			buf = append(buf, s.String()...)
			if i != len(c.signals)-1 {
				buf = append(buf, ' ')
			}
		}
		buf = append(buf, ']')
	}
	buf = append(buf, ')')
	return string(buf)
}

type signalError string

func (s signalError) Error() string {
	return string(s)
}
