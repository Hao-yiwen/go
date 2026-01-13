// 版权所有 2019 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

import (
	"sync/atomic"
	"unsafe"
)

// poolDequeue 是一个无锁的固定大小单生产者多消费者队列。
// 单个生产者可以从头部推送和弹出，消费者可以从尾部弹出。
//
// 它有一个附加特性，即将未使用的槽位置空以避免不必要的对象保留。
// 这对于 sync.Pool 很重要，但在文献中通常不会考虑这个属性。
type poolDequeue struct {
	// headTail 将 32 位的 head 索引和 32 位的 tail 索引打包在一起。
	// 两者都是对 vals 取模 len(vals)-1 的索引。
	//
	// tail = 队列中最旧数据的索引
	// head = 下一个要填充的槽位的索引
	//
	// 范围 [tail, head) 内的槽位由消费者拥有。
	// 消费者继续拥有此范围外的槽位，直到它将该槽位置空，
	// 此时所有权传递给生产者。
	//
	// head 索引存储在最高有效位中，这样我们可以原子地加到它上面，
	// 溢出是无害的。
	headTail atomic.Uint64

	// vals 是存储在此双端队列中的 interface{} 值的环形缓冲区。
	// 它的大小必须是 2 的幂。
	//
	// 如果槽位为空，则 vals[i].typ 为 nil，否则为非 nil。
	// 一个槽位仍在使用中，直到 tail 索引已经移过它 *且* typ 已被设置为 nil。
	// 这由消费者原子地设置为 nil，由生产者原子地读取。
	vals []eface
}

type eface struct {
	typ, val unsafe.Pointer
}

const dequeueBits = 32

// dequeueLimit 是 poolDequeue 的最大大小。
//
// 这必须最多为 (1<<dequeueBits)/2，因为检测满状态依赖于
// 环绕环形缓冲区而不环绕索引。我们除以 4 使其在 32 位上适合 int。
const dequeueLimit = (1 << dequeueBits) / 4

// dequeueNil 用于在 poolDequeue 中表示 interface{}(nil)。
// 由于我们使用 nil 表示空槽位，我们需要一个哨兵值来表示 nil。
type dequeueNil *struct{}

func (d *poolDequeue) unpack(ptrs uint64) (head, tail uint32) {
	const mask = 1<<dequeueBits - 1
	head = uint32((ptrs >> dequeueBits) & mask)
	tail = uint32(ptrs & mask)
	return
}

func (d *poolDequeue) pack(head, tail uint32) uint64 {
	const mask = 1<<dequeueBits - 1
	return (uint64(head) << dequeueBits) |
		uint64(tail&mask)
}

// pushHead 在队列头部添加 val。如果队列已满则返回 false。
// 它只能由单个生产者调用。
func (d *poolDequeue) pushHead(val any) bool {
	ptrs := d.headTail.Load()
	head, tail := d.unpack(ptrs)
	if (tail+uint32(len(d.vals)))&(1<<dequeueBits-1) == head {
		// 队列已满。
		return false
	}
	slot := &d.vals[head&uint32(len(d.vals)-1)]

	// 检查 head 槽位是否已被 popTail 释放。
	typ := atomic.LoadPointer(&slot.typ)
	if typ != nil {
		// 另一个 goroutine 仍在清理尾部，
		// 所以队列实际上仍然是满的。
		return false
	}

	// head 槽位是空闲的，所以我们拥有它。
	if val == nil {
		val = dequeueNil(nil)
	}
	*(*any)(unsafe.Pointer(slot)) = val

	// 递增 head。这将槽位的所有权传递给 popTail，
	// 并充当写入槽位的存储屏障。
	d.headTail.Add(1 << dequeueBits)
	return true
}

// popHead 移除并返回队列头部的元素。
// 如果队列为空则返回 false。它只能由单个生产者调用。
func (d *poolDequeue) popHead() (any, bool) {
	var slot *eface
	for {
		ptrs := d.headTail.Load()
		head, tail := d.unpack(ptrs)
		if tail == head {
			// 队列为空。
			return nil, false
		}

		// 确认 tail 并递减 head。我们在读取值之前这样做，
		// 以收回此槽位的所有权。
		head--
		ptrs2 := d.pack(head, tail)
		if d.headTail.CompareAndSwap(ptrs, ptrs2) {
			// 我们成功收回了槽位。
			slot = &d.vals[head&uint32(len(d.vals)-1)]
			break
		}
	}

	val := *(*any)(unsafe.Pointer(slot))
	if val == dequeueNil(nil) {
		val = nil
	}
	// 将槽位置零。与 popTail 不同，这不会与 pushHead 竞争，
	// 所以我们在这里不需要小心。
	*slot = eface{}
	return val, true
}

// popTail 移除并返回队列尾部的元素。
// 如果队列为空则返回 false。它可以被任意数量的消费者调用。
func (d *poolDequeue) popTail() (any, bool) {
	var slot *eface
	for {
		ptrs := d.headTail.Load()
		head, tail := d.unpack(ptrs)
		if tail == head {
			// 队列为空。
			return nil, false
		}

		// 确认 head 和 tail（用于我们上面的推测性检查）并递增 tail。
		// 如果成功，那么我们拥有 tail 处的槽位。
		ptrs2 := d.pack(head, tail+1)
		if d.headTail.CompareAndSwap(ptrs, ptrs2) {
			// 成功。
			slot = &d.vals[tail&uint32(len(d.vals)-1)]
			break
		}
	}

	// 我们现在拥有槽位。
	val := *(*any)(unsafe.Pointer(slot))
	if val == dequeueNil(nil) {
		val = nil
	}

	// 告诉 pushHead 我们已经完成了这个槽位。将槽位置零也很重要，
	// 这样我们就不会留下可能使这个对象存活时间超过必要时间的引用。
	//
	// 我们先写入 val，然后通过原子写入 typ 来发布我们已完成此槽位。
	slot.val = nil
	atomic.StorePointer(&slot.typ, nil)
	// 此时 pushHead 拥有该槽位。

	return val, true
}

// poolChain 是 poolDequeue 的动态大小版本。
//
// 它被实现为 poolDequeue 的双向链表队列，其中每个双端队列的大小是
// 前一个的两倍。一旦一个双端队列填满，这会分配一个新的，
// 并且只向最新的双端队列推送。弹出发生在列表的另一端，
// 一旦一个双端队列被耗尽，它就会从列表中移除。
type poolChain struct {
	// head 是要推送到的 poolDequeue。这只被生产者访问，
	// 所以不需要同步。
	head *poolChainElt

	// tail 是要从中 popTail 的 poolDequeue。这被消费者访问，
	// 所以读写必须是原子的。
	tail atomic.Pointer[poolChainElt]
}

type poolChainElt struct {
	poolDequeue

	// next 和 prev 链接到此 poolChain 中相邻的 poolChainElt。
	//
	// next 由生产者原子写入，由消费者原子读取。
	// 它只从 nil 转换为非 nil。
	//
	// prev 由消费者原子写入，由生产者原子读取。
	// 它只从非 nil 转换为 nil。
	next, prev atomic.Pointer[poolChainElt]
}

func (c *poolChain) pushHead(val any) {
	d := c.head
	if d == nil {
		// 初始化链。
		const initSize = 8 // 必须是 2 的幂
		d = new(poolChainElt)
		d.vals = make([]eface, initSize)
		c.head = d
		c.tail.Store(d)
	}

	if d.pushHead(val) {
		return
	}

	// 当前双端队列已满。分配一个两倍大小的新队列。
	newSize := len(d.vals) * 2
	if newSize >= dequeueLimit {
		// 不能再大了。
		newSize = dequeueLimit
	}

	d2 := &poolChainElt{}
	d2.prev.Store(d)
	d2.vals = make([]eface, newSize)
	c.head = d2
	d.next.Store(d2)
	d2.pushHead(val)
}

func (c *poolChain) popHead() (any, bool) {
	d := c.head
	for d != nil {
		if val, ok := d.popHead(); ok {
			return val, ok
		}
		// 前一个双端队列中可能仍有未消费的元素，所以尝试回退。
		d = d.prev.Load()
	}
	return nil, false
}

func (c *poolChain) popTail() (any, bool) {
	d := c.tail.Load()
	if d == nil {
		return nil, false
	}

	for {
		// 重要的是我们在弹出尾部 *之前* 加载 next 指针。
		// 通常，d 可能暂时为空，但如果 next 在弹出之前是非 nil
		// 且弹出失败，那么 d 是永久为空的，这是从链中安全删除 d 的唯一条件。
		d2 := d.next.Load()

		if val, ok := d.popTail(); ok {
			return val, ok
		}

		if d2 == nil {
			// 这是唯一的双端队列。它现在是空的，
			// 但将来可能会被推送。
			return nil, false
		}

		// 链的尾部已被耗尽，所以移动到下一个双端队列。
		// 尝试将它从链中删除，这样下一次弹出就不必再查看空的双端队列。
		if c.tail.CompareAndSwap(d, d2) {
			// 我们赢得了竞争。清除 prev 指针，
			// 这样垃圾收集器可以收集空的双端队列，
			// 并且 popHead 不会回退超过必要的程度。
			d2.prev.Store(nil)
		}
		d = d2
	}
}
