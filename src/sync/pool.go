// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"internal/race"
	"runtime"
	"sync/atomic"
	"unsafe"
)

// Pool 是一组可以单独保存和检索的临时对象。
//
// 存储在 Pool 中的任何项目可能在任何时候被自动删除而不会通知。
// 如果发生这种情况时 Pool 持有唯一的引用，该项目可能会被释放。
//
// Pool 可以安全地被多个 goroutine 同时使用。
//
// Pool 的目的是缓存已分配但未使用的项目以供以后重用，
// 从而减轻垃圾收集器的压力。也就是说，它使构建高效的、
// 线程安全的空闲列表变得容易。但是，它并不适合所有的空闲列表。
//
// Pool 的适当用途是管理一组临时项目，这些项目在包的并发独立客户端之间
// 静默共享，并可能被重用。Pool 提供了一种在多个客户端之间分摊分配开销的方法。
//
// Pool 的良好使用示例是 fmt 包，它维护一个动态大小的临时输出缓冲区存储。
// 该存储在负载下会扩展（当许多 goroutine 正在积极打印时），
// 并在空闲时收缩。
//
// 另一方面，作为短生命周期对象的一部分维护的空闲列表不适合使用 Pool，
// 因为在这种情况下开销无法很好地分摊。让这类对象实现自己的空闲列表更为高效。
//
// Pool 在首次使用后不得被复制。
//
// 按照 [Go 内存模型] 的术语，对 Put(x) 的调用 "同步先于"
// 返回相同值 x 的 [Pool.Get] 调用。
// 类似地，返回 x 的 New 调用 "同步先于" 返回相同值 x 的 Get 调用。
//
// [Go 内存模型]: https://go.dev/ref/mem
type Pool struct {
	noCopy noCopy

	local     unsafe.Pointer // 每个 P 的本地固定大小池，实际类型是 [P]poolLocal
	localSize uintptr        // local 数组的大小

	victim     unsafe.Pointer // 上一个周期的 local
	victimSize uintptr        // victims 数组的大小

	// New 可选地指定一个函数，用于在 Get 本应返回 nil 时生成一个值。
	// 它不能与 Get 的调用并发更改。
	New func() any
}

// 每个 P 的本地 Pool 附录。
type poolLocalInternal struct {
	private any       // 只能被相应的 P 使用。
	shared  poolChain // 本地 P 可以 pushHead/popHead；任何 P 都可以 popTail。
}

type poolLocal struct {
	poolLocalInternal

	// 在缓存行大小满足 128 mod (缓存行大小) = 0 的常见平台上防止伪共享。
	pad [128 - unsafe.Sizeof(poolLocalInternal{})%128]byte
}

// 来自 runtime
//
//go:linkname runtime_randn runtime.randn
func runtime_randn(n uint32) uint32

var poolRaceHash [128]uint64

// poolRaceAddr 返回一个用作竞态检测器逻辑的同步点的地址。
// 我们不直接使用存储在 x 中的实际指针，以免与该地址上的其他同步冲突。
// 相反，我们对指针进行哈希以获取 poolRaceHash 中的索引。
// 参见 golang.org/cl/31589 上的讨论。
func poolRaceAddr(x any) unsafe.Pointer {
	ptr := uintptr((*[2]unsafe.Pointer)(unsafe.Pointer(&x))[1])
	h := uint32((uint64(uint32(ptr)) * 0x85ebca6b) >> 16)
	return unsafe.Pointer(&poolRaceHash[h%uint32(len(poolRaceHash))])
}

// Put 将 x 添加到池中。
func (p *Pool) Put(x any) {
	if x == nil {
		return
	}
	if race.Enabled {
		if runtime_randn(4) == 0 {
			// 随机丢弃 x。
			return
		}
		race.ReleaseMerge(poolRaceAddr(x))
		race.Disable()
	}
	l, _ := p.pin()
	if l.private == nil {
		l.private = x
	} else {
		l.shared.pushHead(x)
	}
	runtime_procUnpin()
	if race.Enabled {
		race.Enable()
	}
}

// Get 从 [Pool] 中选择一个任意项目，将其从 Pool 中移除，并将其返回给调用者。
// Get 可能选择忽略池并将其视为空。
// 调用者不应假设传递给 [Pool.Put] 的值与 Get 返回的值之间存在任何关系。
//
// 如果 Get 本应返回 nil 且 p.New 非 nil，Get 将返回调用 p.New 的结果。
func (p *Pool) Get() any {
	if race.Enabled {
		race.Disable()
	}
	l, pid := p.pin()
	x := l.private
	l.private = nil
	if x == nil {
		// 尝试弹出本地分片的头部。我们优先选择头部而不是尾部，
		// 以获得重用的时间局部性。
		x, _ = l.shared.popHead()
		if x == nil {
			x = p.getSlow(pid)
		}
	}
	runtime_procUnpin()
	if race.Enabled {
		race.Enable()
		if x != nil {
			race.Acquire(poolRaceAddr(x))
		}
	}
	if x == nil && p.New != nil {
		x = p.New()
	}
	return x
}

func (p *Pool) getSlow(pid int) any {
	// 关于加载顺序，请参阅 pin 中的注释。
	size := runtime_LoadAcquintptr(&p.localSize) // load-acquire
	locals := p.local                            // load-consume
	// 尝试从其他处理器窃取一个元素。
	for i := 0; i < int(size); i++ {
		l := indexLocal(locals, (pid+i+1)%int(size))
		if x, _ := l.shared.popTail(); x != nil {
			return x
		}
	}

	// 尝试 victim 缓存。我们在尝试从所有主缓存窃取之后才这样做，
	// 因为我们希望 victim 缓存中的对象尽可能地老化淘汰。
	size = atomic.LoadUintptr(&p.victimSize)
	if uintptr(pid) >= size {
		return nil
	}
	locals = p.victim
	l := indexLocal(locals, pid)
	if x := l.private; x != nil {
		l.private = nil
		return x
	}
	for i := 0; i < int(size); i++ {
		l := indexLocal(locals, (pid+i)%int(size))
		if x, _ := l.shared.popTail(); x != nil {
			return x
		}
	}

	// 将 victim 缓存标记为空，这样将来的 get 操作就不会再尝试它。
	atomic.StoreUintptr(&p.victimSize, 0)

	return nil
}

// pin 将当前 goroutine 固定到 P，禁用抢占，
// 并返回该 P 的 poolLocal 池和 P 的 id。
// 调用者在使用完池后必须调用 runtime_procUnpin()。
func (p *Pool) pin() (*poolLocal, int) {
	// 检查 p 是否为 nil 以触发 panic。
	// 否则，nil 解引用会在 m 被固定时发生，
	// 导致致命错误而不是 panic。
	if p == nil {
		panic("nil Pool")
	}

	pid := runtime_procPin()
	// 在 pinSlow 中我们先存储到 local 然后存储到 localSize，这里我们以相反的顺序加载。
	// 由于我们已禁用抢占，GC 不会在中间发生。
	// 因此这里我们必须观察到 local 至少与 localSize 一样大。
	// 我们可能观察到更新/更大的 local，这没关系（我们必须观察到它的零初始化状态）。
	s := runtime_LoadAcquintptr(&p.localSize) // load-acquire
	l := p.local                              // load-consume
	if uintptr(pid) < s {
		return indexLocal(l, pid), pid
	}
	return p.pinSlow()
}

func (p *Pool) pinSlow() (*poolLocal, int) {
	// 在互斥锁下重试。
	// 固定时无法锁定互斥锁。
	runtime_procUnpin()
	allPoolsMu.Lock()
	defer allPoolsMu.Unlock()
	pid := runtime_procPin()
	// 当我们被固定时，poolCleanup 不会被调用。
	s := p.localSize
	l := p.local
	if uintptr(pid) < s {
		return indexLocal(l, pid), pid
	}
	if p.local == nil {
		allPools = append(allPools, p)
	}
	// 如果 GOMAXPROCS 在 GC 之间发生变化，我们会重新分配数组并丢失旧的。
	size := runtime.GOMAXPROCS(0)
	local := make([]poolLocal, size)
	atomic.StorePointer(&p.local, unsafe.Pointer(&local[0])) // store-release
	runtime_StoreReluintptr(&p.localSize, uintptr(size))     // store-release
	return &local[pid], pid
}

// poolCleanup 应该是一个内部细节，
// 但广泛使用的包通过 linkname 访问它。
// 著名的"耻辱堂"成员包括：
//   - github.com/bytedance/gopkg
//   - github.com/songzhibin97/gkit
//
// 不要删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname poolCleanup
func poolCleanup() {
	// 此函数在世界停止时、垃圾收集开始时被调用。
	// 它不能分配内存，可能也不应该调用任何运行时函数。

	// 因为世界已停止，没有池用户可以处于固定部分
	// （实际上，这使所有 P 都被固定）。

	// 从所有池中删除 victim 缓存。
	for _, p := range oldPools {
		p.victim = nil
		p.victimSize = 0
	}

	// 将主缓存移动到 victim 缓存。
	for _, p := range allPools {
		p.victim = p.local
		p.victimSize = p.localSize
		p.local = nil
		p.localSize = 0
	}

	// 具有非空主缓存的池现在具有非空的 victim 缓存，
	// 并且没有池具有主缓存。
	oldPools, allPools = allPools, nil
}

var (
	allPoolsMu Mutex

	// allPools 是具有非空主缓存的池的集合。
	// 受 1) allPoolsMu 和固定 或 2) STW 保护。
	allPools []*Pool

	// oldPools 是可能具有非空 victim 缓存的池的集合。
	// 受 STW 保护。
	oldPools []*Pool
)

func init() {
	runtime_registerPoolCleanup(poolCleanup)
}

func indexLocal(l unsafe.Pointer, i int) *poolLocal {
	lp := unsafe.Pointer(uintptr(l) + uintptr(i)*unsafe.Sizeof(poolLocal{}))
	return (*poolLocal)(lp)
}

// 在 runtime 中实现。
func runtime_registerPoolCleanup(cleanup func())
func runtime_procPin() int
func runtime_procUnpin()

// 以下在 internal/runtime/atomic 中实现，
// 编译器也知道将我们 linkname 到此包中的符号进行内联化。

//go:linkname runtime_LoadAcquintptr internal/runtime/atomic.LoadAcquintptr
func runtime_LoadAcquintptr(ptr *uintptr) uintptr

//go:linkname runtime_StoreReluintptr internal/runtime/atomic.StoreReluintptr
func runtime_StoreReluintptr(ptr *uintptr, val uintptr)
