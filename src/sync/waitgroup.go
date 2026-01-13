// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"internal/race"
	"internal/synctest"
	"sync/atomic"
	"unsafe"
)

// WaitGroup 是一个计数信号量，通常用于等待一组 goroutine 或任务完成。
//
// 通常，主 goroutine 会通过调用 [WaitGroup.Go] 启动任务（每个任务在一个新的 goroutine 中），
// 然后通过调用 [WaitGroup.Wait] 等待所有任务完成。例如：
//
//	var wg sync.WaitGroup
//	wg.Go(task1)
//	wg.Go(task2)
//	wg.Wait()
//
// WaitGroup 也可以通过使用 [WaitGroup.Add] 和 [WaitGroup.Done] 来跟踪任务，
// 而不使用 Go 启动新的 goroutine。
//
// 前面的示例可以使用显式创建的 goroutine 以及 Add 和 Done 来重写：
//
//	var wg sync.WaitGroup
//	wg.Add(1)
//	go func() {
//		defer wg.Done()
//		task1()
//	}()
//	wg.Add(1)
//	go func() {
//		defer wg.Done()
//		task2()
//	}()
//	wg.Wait()
//
// 这种模式在 [WaitGroup.Go] 出现之前的代码中很常见。
//
// WaitGroup 在首次使用后不得被复制。
type WaitGroup struct {
	noCopy noCopy

	// 位（从高到低）：
	//   bits[0:32]  计数器
	//   bits[32]    标志：synctest bubble 成员资格
	//   bits[33:64] 等待计数
	state atomic.Uint64
	sema  uint32
}

// waitGroupBubbleFlag 表示 WaitGroup 与 synctest bubble 关联。
const waitGroupBubbleFlag = 0x8000_0000

// Add 将 delta（可以为负数）添加到 [WaitGroup] 任务计数器。
// 如果计数器变为零，所有在 [WaitGroup.Wait] 上阻塞的 goroutine 都会被释放。
// 如果计数器变为负数，Add 会 panic。
//
// 调用者应该优先使用 [WaitGroup.Go]。
//
// 请注意，当计数器为零时发生的正 delta 调用必须在 Wait 之前发生。
// 负 delta 的调用，或者当计数器大于零时开始的正 delta 调用，可以在任何时候发生。
// 通常这意味着对 Add 的调用应该在创建 goroutine 或其他要等待的事件的语句之前执行。
// 如果 WaitGroup 被重用于等待多个独立的事件集，
// 新的 Add 调用必须在所有先前的 Wait 调用返回之后发生。
// 请参阅 WaitGroup 示例。
func (wg *WaitGroup) Add(delta int) {
	if race.Enabled {
		if delta < 0 {
			// 与 Wait 同步递减。
			race.ReleaseMerge(unsafe.Pointer(wg))
		}
		race.Disable()
		defer race.Enable()
	}
	bubbled := false
	if synctest.IsInBubble() {
		// 如果从 bubble 内调用 Add，则所有 Add 调用必须来自同一个 bubble。
		switch synctest.Associate(wg) {
		case synctest.Unbubbled:
		case synctest.OtherBubble:
			// wg 已经与不同的 bubble 关联。
			fatal("sync: WaitGroup.Add called from multiple synctest bubbles")
		case synctest.CurrentBubble:
			bubbled = true
			state := wg.state.Or(waitGroupBubbleFlag)
			if state != 0 && state&waitGroupBubbleFlag == 0 {
				// Add 已从此 bubble 外部调用。
				fatal("sync: WaitGroup.Add called from inside and outside synctest bubble")
			}
		}
	}
	state := wg.state.Add(uint64(delta) << 32)
	if state&waitGroupBubbleFlag != 0 && !bubbled {
		// Add 已从 synctest bubble 内调用（而我们不在其中）。
		fatal("sync: WaitGroup.Add called from inside and outside synctest bubble")
	}
	v := int32(state >> 32)
	w := uint32(state & 0x7fffffff)
	if race.Enabled && delta > 0 && v == int32(delta) {
		// 第一次递增必须与 Wait 同步。
		// 需要将其建模为读取，因为可能有多个并发的 wg.counter 从 0 转换。
		race.Read(unsafe.Pointer(&wg.sema))
	}
	if v < 0 {
		panic("sync: negative WaitGroup counter")
	}
	if w != 0 && delta > 0 && v == int32(delta) {
		panic("sync: WaitGroup misuse: Add called concurrently with Wait")
	}
	if v > 0 || w == 0 {
		return
	}
	// 当 waiters > 0 时，此 goroutine 已将计数器设置为 0。
	// 现在不可能有状态的并发修改：
	// - Add 不能与 Wait 并发发生，
	// - 如果 Wait 看到 counter == 0，则不会递增 waiters。
	// 仍然进行廉价的健全性检查以检测 WaitGroup 的误用。
	if wg.state.Load() != state {
		panic("sync: WaitGroup misuse: Add called concurrently with Wait")
	}
	// 将 waiters 计数重置为 0。
	wg.state.Store(0)
	if bubbled {
		// 当 counter 为 0 时，Add 不能与 wait 并发发生，
		// 所以我们可以安全地将 wg 与其当前 bubble 解除关联。
		synctest.Disassociate(wg)
	}
	for ; w != 0; w-- {
		runtime_Semrelease(&wg.sema, false, 0)
	}
}

// Done 将 [WaitGroup] 任务计数器减一。
// 它等同于 Add(-1)。
//
// 调用者应该优先使用 [WaitGroup.Go]。
//
// 按照 [Go 内存模型] 的术语，对 Done 的调用
// "同步先于" 它解除阻塞的任何 Wait 调用的返回。
//
// [Go 内存模型]: https://go.dev/ref/mem
func (wg *WaitGroup) Done() {
	wg.Add(-1)
}

// Wait 阻塞直到 [WaitGroup] 任务计数器为零。
func (wg *WaitGroup) Wait() {
	if race.Enabled {
		race.Disable()
	}
	for {
		state := wg.state.Load()
		v := int32(state >> 32)
		w := uint32(state & 0x7fffffff)
		if v == 0 {
			// 计数器为 0，无需等待。
			if race.Enabled {
				race.Enable()
				race.Acquire(unsafe.Pointer(wg))
			}
			if w == 0 && state&waitGroupBubbleFlag != 0 && synctest.IsAssociated(wg) {
				// 当 counter 为 0 时，Add 不能与 wait 并发发生，
				// 所以我们可以将 wg 与其当前 bubble 解除关联。
				if wg.state.CompareAndSwap(state, 0) {
					synctest.Disassociate(wg)
				}
			}
			return
		}
		// 递增 waiters 计数。
		if wg.state.CompareAndSwap(state, state+1) {
			if race.Enabled && w == 0 {
				// Wait 必须与第一个 Add 同步。
				// 需要将其建模为与 Add 中的读取竞争的写入。
				// 因此，只能为第一个等待者进行写入，
				// 否则并发的 Wait 会相互竞争。
				race.Write(unsafe.Pointer(&wg.sema))
			}
			synctestDurable := false
			if state&waitGroupBubbleFlag != 0 && synctest.IsInBubble() {
				if race.Enabled {
					race.Enable()
				}
				if synctest.IsAssociated(wg) {
					// Add 是在当前 bubble 内调用的，
					// 所以这个 Wait 是持久阻塞的。
					synctestDurable = true
				}
				if race.Enabled {
					race.Disable()
				}
			}
			runtime_SemacquireWaitGroup(&wg.sema, synctestDurable)
			isReset := wg.state.Load() != 0
			if race.Enabled {
				race.Enable()
				race.Acquire(unsafe.Pointer(wg))
			}
			if isReset {
				panic("sync: WaitGroup is reused before previous Wait has returned")
			}
			return
		}
	}
}

// Go 在新的 goroutine 中调用 f 并将该任务添加到 [WaitGroup]。
// 当 f 返回时，任务从 WaitGroup 中移除。
//
// 函数 f 不得 panic。
//
// 如果 WaitGroup 为空，Go 必须在 [WaitGroup.Wait] 之前发生。
// 通常，这只是意味着在调用 Wait 之前调用 Go 来启动任务。
// 如果 WaitGroup 不为空，Go 可以在任何时候发生。
// 这意味着由 Go 启动的 goroutine 本身可以调用 Go。
// 如果 WaitGroup 被重用于等待多个独立的任务集，
// 新的 Go 调用必须在所有先前的 Wait 调用返回之后发生。
//
// 按照 [Go 内存模型] 的术语，f 的返回
// "同步先于" 它解除阻塞的任何 Wait 调用的返回。
//
// [Go 内存模型]: https://go.dev/ref/mem
func (wg *WaitGroup) Go(f func()) {
	wg.Add(1)
	go func() {
		defer func() {
			if x := recover(); x != nil {
				// f 发生了 panic，这将是致命的，因为这是一个新的 goroutine。
				//
				// 调用 Done 将解除主 goroutine 中 Wait 的阻塞，
				// 允许它与致命 panic 竞争，
				// 甚至可能在 panic 完成之前退出进程（os.Exit(0)）。
				//
				// 这几乎肯定是不希望发生的，
				// 所以避免调用 Done，直接 panic。
				panic(x)
			}

			// f 正常完成，或者使用 goexit 突然终止。
			// 无论哪种方式，都递减信号量。
			wg.Done()
		}()
		f()
	}()
}
