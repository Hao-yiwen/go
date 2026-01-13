// Copyright 2022 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// （安全的）用户内存区域的实现。
//
// 本文件包含用户内存区域的实现，其中 Go 值可以被手动批量分配和释放。
// 手动释放内存（可能在 GC 周期之前）意味着可以延迟垃圾回收周期，
// 通过减少 GC 周期频率来提高效率。还有其他潜在的效率优势，
// 例如改善局部性和使用更高效的分配策略。
//
// 这里的内存区域之所以安全，是因为一旦被释放，访问内存区域的内存
// 将导致显式的程序错误，并且内存区域的地址空间在找不到更多指向它的
// 指针之前不会被重用。有一个例外：如果内存区域分配的内存没有耗尽，
// 它会被放回池中以供重用。这意味着不总是能保证崩溃。
//
// 虽然这看起来不安全，但它仍然防止了内存损坏，并且实际上是必要的，
// 以使 new(T) 成为内存区域的有效实现。这种属性对于允许简单的实现是
// 可取的。（它还避免了在 GC 活动时尝试将内存区域块设置为故障时
// 与 GC 同步所产生的复杂性。）
//
// 实现分层工作。在底层，内存区域以块为单位管理。每个块必须是堆内存区域
// 大小的倍数，或者堆内存区域大小必须能被内存区域块整除。每个块的地址空间，
// 以及该地址空间对应的每个 heapArena，都永久保留用作内存区域块。
// 也就是说，它们永远不能用于通用堆。每个块也由单个 mspan 表示，
// 并被建模为单个大型堆分配。必须这样，因为每个块包含可能指向堆的
// 普通 Go 值，所以它必须像任何其他对象一样被扫描。因此，指向块的
// 任何指针总是会导致在其对应的内存区域仍然存活时扫描整个块。
//
// 块可以从操作系统代表我们映射的新内存中分配，或者通过重用旧的已释放块。
// 当块被释放时，它们的底层内存被返回给操作系统，设置为访问时故障，
// 并且在程序不再指向该块之前不能重用（代码将此状态称为"隔离"），
// 这是由 GC 检查的属性。
//
// 清扫器处理将块从隔离状态移出以准备重用。当块被置于隔离状态时，
// 其对应的 span 被标记为 noscan，这样 GC 就不会尝试扫描会导致故障的内存。
//
// 在下一层是用户内存区域本身。它们由一个活动块组成，新的 Go 值被
// 碰撞分配到其中，以及在分配到内存区域时耗尽的块列表。一旦内存区域被释放，
// 它释放它引用的所有完整块，并将活动块放入重用列表供未来的内存区域使用。
// 每个内存区域在被释放之前都明确保持其引用的块列表存活。每个用户内存区域
// 也映射到一个附加了终结器的对象，该终结器确保即使内存区域本身从未被
// 显式释放，内存区域的块也都被释放。
//
// 包含指针的内存在每个块中从低地址向高地址碰撞分配，而无指针内存从高地址
// 向低地址碰撞分配。这样做的原因是利用 GC 优化，即当对象中没有更多指针时
// GC 将停止扫描对象，这也允许我们省略为分配到内存区域中的无指针 Go 值
// 清除堆位图。
//
// 注意，内存区域并发使用是不安全的。
//
// 总之，有 2 种资源：内存区域和内存区域块。它们存在于以下生命周期中：
//
// (1) 通过 newArena 创建新的内存区域。
// (2) 分配块以保存通过 new 或 slice 分配到内存区域的内存。
//    (a) 首先从部分使用的块的重用列表中分配块。
//    (b) 如果没有这样的块，则从就绪列表中获取块。
//    (c) 如果以上都失败，则为新块映射内存。
// (3) 内存区域被释放，或对它的所有引用被删除，触发其终结器。
//    (a) 如果 GC 未活动，耗尽的块被设置为故障并放入隔离列表。
//    (b) 如果 GC 活动，耗尽的块被放入故障列表，稍后将经历步骤 (a)。
//    (c) 任何剩余的部分使用的块被放入重用列表。
// (4) 一旦找不到更多指向隔离内存区域块的指针，清扫器将这些块从隔离中
//     取出并放入就绪列表。

package runtime

import (
	"internal/abi"
	"internal/goarch"
	"internal/runtime/atomic"
	"internal/runtime/math"
	"internal/runtime/sys"
	"unsafe"
)

// 以 arena_ 开头的函数旨在导出给内存区域的下游用户。
// 他们应该将这些函数包装在更高级别的 API 中。
//
// 底层内存区域及其资源通过不透明的 unsafe.Pointer 管理。

// arena_newArena 是 newUserArena 的包装器。
//
//go:linkname arena_newArena arena.runtime_arena_newArena
func arena_newArena() unsafe.Pointer {
	return unsafe.Pointer(newUserArena())
}

// arena_arena_New 是 (*userArena).new 的包装器，但 typ 是一个 any
// （仍然必须是 *_type），并且 typ 必须是指向要实际分配的类型的指针的
// 类型描述符，即传递 *T 来分配 T。这是必要的，因为此函数返回 *T。
//
//go:linkname arena_arena_New arena.runtime_arena_arena_New
func arena_arena_New(arena unsafe.Pointer, typ any) any {
	t := (*_type)(efaceOf(&typ).data)
	if t.Kind() != abi.Pointer {
		throw("arena_New: non-pointer type")
	}
	te := (*ptrtype)(unsafe.Pointer(t)).Elem
	x := ((*userArena)(arena)).new(te)
	var result any
	e := efaceOf(&result)
	e._type = t
	e.data = x
	return result
}

// arena_arena_Slice 是 (*userArena).slice 的包装器。
//
//go:linkname arena_arena_Slice arena.runtime_arena_arena_Slice
func arena_arena_Slice(arena unsafe.Pointer, slice any, cap int) {
	((*userArena)(arena)).slice(slice, cap)
}

// arena_arena_Free 是 (*userArena).free 的包装器。
//
//go:linkname arena_arena_Free arena.runtime_arena_arena_Free
func arena_arena_Free(arena unsafe.Pointer) {
	((*userArena)(arena)).free()
}

// arena_heapify 获取存在于内存区域中的值，并在堆上创建它的副本。
// 不在内存区域中的值将原样返回。
//
//go:linkname arena_heapify arena.runtime_arena_heapify
func arena_heapify(s any) any {
	var v unsafe.Pointer
	e := efaceOf(&s)
	t := e._type
	switch t.Kind() {
	case abi.String:
		v = stringStructOf((*string)(e.data)).str
	case abi.Slice:
		v = (*slice)(e.data).array
	case abi.Pointer:
		v = e.data
	default:
		panic("arena: Clone only supports pointers, slices, and strings")
	}
	span := spanOf(uintptr(v))
	if span == nil || !span.isUserArenaChunk {
		// 不在用户内存区域块中存储。
		return s
	}
	// 在堆上分配存储空间用于复制。
	var x any
	switch t.Kind() {
	case abi.String:
		s1 := s.(string)
		s2, b := rawstring(len(s1))
		copy(b, s1)
		x = s2
	case abi.Slice:
		len := (*slice)(e.data).len
		et := (*slicetype)(unsafe.Pointer(t)).Elem
		sl := new(slice)
		*sl = slice{makeslicecopy(et, len, len, (*slice)(e.data).array), len, len}
		xe := efaceOf(&x)
		xe._type = t
		xe.data = unsafe.Pointer(sl)
	case abi.Pointer:
		et := (*ptrtype)(unsafe.Pointer(t)).Elem
		e2 := newobject(et)
		typedmemmove(et, e2, e.data)
		xe := efaceOf(&x)
		xe._type = t
		xe.data = e2
	}
	return x
}

const (
	// userArenaChunkBytes 是用户内存区域块的大小。
	userArenaChunkBytesMax = 8 << 20
	userArenaChunkBytes    = uintptr(int64(userArenaChunkBytesMax-heapArenaBytes)&(int64(userArenaChunkBytesMax-heapArenaBytes)>>63) + heapArenaBytes) // min(userArenaChunkBytesMax, heapArenaBytes)

	// userArenaChunkPages 是用户内存区域块使用的页数。
	userArenaChunkPages = userArenaChunkBytes / pageSize

	// userArenaChunkMaxAllocBytes 是可以从内存区域分配的对象的最大大小。
	// 选择这个数字是为了将用户内存区域的最坏情况碎片限制在 25%。
	// 更大的分配被重定向到堆。
	userArenaChunkMaxAllocBytes = userArenaChunkBytes / 4
)

func init() {
	if userArenaChunkPages*pageSize != userArenaChunkBytes {
		throw("user arena chunk size is not a multiple of the page size")
	}
	if userArenaChunkBytes%physPageSize != 0 {
		throw("user arena chunk size is not a multiple of the physical page size")
	}
	if userArenaChunkBytes < heapArenaBytes {
		if heapArenaBytes%userArenaChunkBytes != 0 {
			throw("user arena chunk size is smaller than a heap arena, but doesn't divide it")
		}
	} else {
		if userArenaChunkBytes%heapArenaBytes != 0 {
			throw("user arena chunks size is larger than a heap arena, but not a multiple")
		}
	}
	lockInit(&userArenaState.lock, lockRankUserArenaState)
}

// userArenaChunkReserveBytes 返回为堆元数据保留的额外字节数。
func userArenaChunkReserveBytes() uintptr {
	// 在分配头实验中，我们为指针/标量位图保留块的末尾。
	// 我们还为引用位图的虚拟 _type 保留空间。
	// 虚拟 _type 的 PtrBytes 字段指示这些位中有多少是有效的。
	return userArenaChunkBytes/goarch.PtrSize/8 + unsafe.Sizeof(_type{})
}

type userArena struct {
	// fullList 是没有足够剩余空闲内存的完整块列表，
	// 一旦这个用户内存区域被释放，我们就会释放它们。
	//
	// 这里不能使用 mSpanList，因为它不在堆中。
	fullList *mspan

	// active 是我们当前正在分配的用户内存区域块。
	active *mspan

	// refs 是对内存区域块的引用集合，以便它们保持存活。
	//
	// 列表中的最后一个引用总是指向 active，而其余的对应 fullList。
	// 具体来说，fullList 的头是倒数第二个，fullList.next 是倒数第三个，依此类推。
	//
	// 换句话说，每次新块变为活动块时，它都会被追加到这个列表中。
	refs []unsafe.Pointer

	// 如果已在此内存区域上调用 free，则 defunct 为 true。
	//
	// 这只是发现并发分配和释放的尽力而为的方式。
	// 也用于检测重复释放。
	defunct atomic.Bool
}

// newUserArena 创建一个准备好使用的新 userArena。
func newUserArena() *userArena {
	a := new(userArena)
	SetFinalizer(a, func(a *userArena) {
		// 如果内存区域句柄在未释放的情况下被丢弃，则对内存区域调用 free，
		// 这样内存区域块就永远不会被垃圾回收器回收。
		a.free()
	})
	a.refill()
	return a
}

// new 将提供类型的新对象分配到内存区域中，并返回其指针。
//
// 此操作与同一内存区域上的其他操作并发调用是不安全的。
func (a *userArena) new(typ *_type) unsafe.Pointer {
	return a.alloc(typ, -1)
}

// slice 分配一个新的切片后备存储。slice 必须是指向切片的指针
// （即 *[]T），因为 userArenaSlice 将直接更新切片。
//
// cap 确定切片后备存储的容量，必须为非负数。
//
// 此操作与同一内存区域上的其他操作并发调用是不安全的。
func (a *userArena) slice(sl any, cap int) {
	if cap < 0 {
		panic("userArena.slice: negative cap")
	}
	i := efaceOf(&sl)
	typ := i._type
	if typ.Kind() != abi.Pointer {
		panic("slice result of non-ptr type")
	}
	typ = (*ptrtype)(unsafe.Pointer(typ)).Elem
	if typ.Kind() != abi.Slice {
		panic("slice of non-ptr-to-slice type")
	}
	typ = (*slicetype)(unsafe.Pointer(typ)).Elem
	// t 现在是我们要分配的切片的元素类型。

	*((*slice)(i.data)) = slice{a.alloc(typ, cap), cap, cap}
}

// free 将 userArena 的块返回给 mheap 并将其标记为已废弃。
//
// 对于任何给定的内存区域最多只能调用一次。
//
// 此操作与同一内存区域上的其他操作并发调用是不安全的。
func (a *userArena) free() {
	// 检查是否重复释放。
	if a.defunct.Load() {
		panic("arena double free")
	}

	// 将自己标记为已废弃。
	a.defunct.Store(true)
	SetFinalizer(a, nil)

	// 释放所有完整的内存区域。
	//
	// 此列表上的引用从倒数第二个开始按相反顺序排列。
	s := a.fullList
	i := len(a.refs) - 2
	for s != nil {
		a.fullList = s.next
		s.next = nil
		freeUserArenaChunk(s, a.refs[i])
		s = a.fullList
		i--
	}
	if a.fullList != nil || i >= 0 {
		// 完整列表上还有剩余，或者我们未能实际遍历整个 refs 列表。
		throw("full list doesn't match refs list in length")
	}

	// 将活动块放入重用列表。
	//
	// 注意 active 的引用始终是 refs 中的最后一个引用。
	s = a.active
	if s != nil {
		if raceenabled || msanenabled || asanenabled {
			// 启用了清理器时不要重用内存区域。我们希望积极地捕获
			// 任何释放后使用的错误。
			freeUserArenaChunk(s, a.refs[len(a.refs)-1])
		} else {
			lock(&userArenaState.lock)
			userArenaState.reuse = append(userArenaState.reuse, liveUserArenaChunk{s, a.refs[len(a.refs)-1]})
			unlock(&userArenaState.lock)
		}
	}
	// 将 a.active 置为 nil，这样与释放的竞争更可能导致崩溃。
	a.active = nil
	a.refs = nil
}

// alloc 在当前块中保留空间，或调用 refill 并在新块中保留空间。
// 如果 cap 为负，类型将按字面意思理解，否则它将被视为
// 容量为 cap 的切片后备存储的元素类型。
func (a *userArena) alloc(typ *_type, cap int) unsafe.Pointer {
	s := a.active
	var x unsafe.Pointer
	for {
		x = s.userArenaNextFree(typ, cap)
		if x != nil {
			break
		}
		s = a.refill()
	}
	return x
}

// refill 将当前内存区域块插入完整列表，并从部分列表或通过
// 分配新块（两者都来自 mheap）获取新块。
func (a *userArena) refill() *mspan {
	// 如果有活动块，假设它已满。
	s := a.active
	if s != nil {
		if s.userArenaChunkFree.size() > userArenaChunkMaxAllocBytes {
			// 很难判断块中何时实际内存不足，
			// 因为失败的分配可能仍然留下一些可用的空闲空间。
			// 然而，空闲空间的数量永远不应超过最大分配大小。
			throw("wasted too much memory in an arena chunk")
		}
		s.next = a.fullList
		a.fullList = s
		a.active = nil
		s = nil
	}
	var x unsafe.Pointer

	// 检查部分使用的列表。
	lock(&userArenaState.lock)
	if len(userArenaState.reuse) > 0 {
		// 从列表中取出最后一个内存区域块。
		n := len(userArenaState.reuse) - 1
		x = userArenaState.reuse[n].x
		s = userArenaState.reuse[n].mspan
		userArenaState.reuse[n].x = nil
		userArenaState.reuse[n].mspan = nil
		userArenaState.reuse = userArenaState.reuse[:n]
	}
	unlock(&userArenaState.lock)
	if s == nil {
		// 分配一个新的。
		x, s = newUserArenaChunk()
		if s == nil {
			throw("out of memory")
		}
	}
	a.refs = append(a.refs, x)
	a.active = s
	return s
}

type liveUserArenaChunk struct {
	*mspan // 必须表示一个用户内存区域块。

	// 对 mspan.base() 的引用以保持块存活。
	x unsafe.Pointer
}

var userArenaState struct {
	lock mutex

	// reuse 包含可以快速重用于另一个内存区域的
	// 部分使用且已存活的用户内存区域块列表。
	//
	// 由 lock 保护。
	reuse []liveUserArenaChunk

	// fault 包含需要设置为故障的完整用户内存区域块。
	//
	// 由 lock 保护。
	fault []liveUserArenaChunk
}

// userArenaNextFree 在用户内存区域中为指定类型的项保留空间。
// 如果 cap 不是 -1，则这是用于类型 t 的 cap 个元素的数组。
func (s *mspan) userArenaNextFree(typ *_type, cap int) unsafe.Pointer {
	size := typ.Size_
	if cap > 0 {
		if size > ^uintptr(0)/uintptr(cap) {
			// 溢出。
			throw("out of memory")
		}
		size *= uintptr(cap)
	}
	if size == 0 || cap == 0 {
		return unsafe.Pointer(&zerobase)
	}
	if size > userArenaChunkMaxAllocBytes {
		// 将无法很好地放入块中的分配直接重定向到堆。
		if cap >= 0 {
			return newarray(typ, cap)
		}
		return newobject(typ)
	}

	// 在为新对象设置空间时防止抢占。
	//
	// 表现得像我们正在分配一样。
	mp := acquirem()
	if mp.mallocing != 0 {
		throw("malloc deadlock")
	}
	if mp.gsignal == getg() {
		throw("malloc during signal")
	}
	mp.mallocing = 1

	var ptr unsafe.Pointer
	if !typ.Pointers() {
		// 从块的尾端分配无指针对象。
		v, ok := s.userArenaChunkFree.takeFromBack(size, typ.Align_)
		if ok {
			ptr = unsafe.Pointer(v)
		}
	} else {
		v, ok := s.userArenaChunkFree.takeFromFront(size, typ.Align_)
		if ok {
			ptr = unsafe.Pointer(v)
		}
	}
	if ptr == nil {
		// 分配失败。
		mp.mallocing = 0
		releasem(mp)
		return nil
	}
	if s.needzero != 0 {
		throw("arena chunk needs zeroing, but should already be zeroed")
	}
	// 设置堆位图并进行额外的记账。
	if typ.Pointers() {
		if cap >= 0 {
			userArenaHeapBitsSetSliceType(typ, cap, ptr, s)
		} else {
			userArenaHeapBitsSetType(typ, ptr, s)
		}
		c := getMCache(mp)
		if c == nil {
			throw("mallocgc called without a P or outside bootstrapping")
		}
		if cap > 0 {
			c.scanAlloc += size - (typ.Size_ - typ.PtrBytes)
		} else {
			c.scanAlloc += typ.PtrBytes
		}
	}

	// 确保上面将 x 初始化为类型安全内存并设置堆位的存储
	// 在调用者可以使 ptr 对垃圾回收器可见之前发生。
	// 否则，在弱排序机器上，垃圾回收器可能会跟随指向 x 的指针，
	// 但看到未初始化的内存或过时的堆位。
	publicationBarrier()

	mp.mallocing = 0
	releasem(mp)

	return ptr
}

// userArenaHeapBitsSetSliceType 是 heapBitsSetType 的等效函数，但用于
// 在用户内存区域块中分配的 Go 切片后备存储值。它为在地址 ptr 处
// 分配的 n 个连续的 typ 类型值设置堆位图。
func userArenaHeapBitsSetSliceType(typ *_type, n int, ptr unsafe.Pointer, s *mspan) {
	mem, overflow := math.MulUintptr(typ.Size_, uintptr(n))
	if overflow || n < 0 || mem > maxAlloc {
		panic(plainError("runtime: allocation size out of range"))
	}
	for i := 0; i < n; i++ {
		userArenaHeapBitsSetType(typ, add(ptr, uintptr(i)*typ.Size_), s)
	}
}

// userArenaHeapBitsSetType 是 heapSetType 的等效函数，但用于
// 在用户内存区域块中分配的非切片后备存储 Go 值。它为在地址 ptr 处
// 分配的 typ 类型值设置类型元数据。base 是内存区域块的基地址。
func userArenaHeapBitsSetType(typ *_type, ptr unsafe.Pointer, s *mspan) {
	base := s.base()
	h := s.writeUserArenaHeapBits(uintptr(ptr))

	p := getGCMask(typ) // 1 位指针掩码的开始
	nb := typ.PtrBytes / goarch.PtrSize

	for i := uintptr(0); i < nb; i += ptrBits {
		k := nb - i
		if k > ptrBits {
			k = ptrBits
		}
		// 注意：在大端平台上，我们对从 GCData 读取的数据进行字节交换，
		// GCData 始终由编译器以小端顺序存储。writeUserArenaHeapBits
		// 以平台顺序方式处理数据以提高效率，但以小端顺序存储回数据，
		// 因为我们通过虚拟类型公开位图。
		h = h.write(s, readUintptr(addb(p, i/8)), k)
	}
	// 注意：我们在这里调用 pad 以确保为对象的无指针尾部
	// 发出显式的 0 位。这确保下一个对象只有一个 noMorePtrs 标记
	// 需要清除。我们不需要这样做来清除之前使用的陈旧 noMorePtrs 标记，
	// 因为内存区域块指针位图在重用时总是完全清除。
	h = h.pad(s, typ.Size_-typ.PtrBytes)
	h.flush(s, uintptr(ptr), typ.Size_)

	// 更新类型信息中的 PtrBytes 值。在此之后，GC 将观察到新的位图。
	s.largeType.PtrBytes = uintptr(ptr) - base + typ.PtrBytes

	// 仔细检查位图是否正确写出。
	const doubleCheck = false
	if doubleCheck {
		doubleCheckHeapPointersInterior(uintptr(ptr), uintptr(ptr), typ.Size_, typ.Size_, typ, &s.largeType, s)
	}
}

type writeUserArenaHeapBits struct {
	offset uintptr // span 中 mask 低位表示指针状态的偏移量。
	mask   uintptr // 从地址 addr 开始的一些指针位。
	valid  uintptr // buf 中有效的位数（包括低位）
	low    uintptr // 不覆盖的低位数
}

func (s *mspan) writeUserArenaHeapBits(addr uintptr) (h writeUserArenaHeapBits) {
	offset := addr - s.base()

	// 我们可能从堆位图字的中间开始写入位。
	// 记住我们从字中的哪一位开始，这样我们就可以确保不会覆盖之前的位。
	h.low = offset / goarch.PtrSize % ptrBits

	// 向下取整到开始位图字的堆字。
	h.offset = offset - h.low*goarch.PtrSize

	// 我们还没有任何位。
	h.mask = 0
	h.valid = h.low

	return
}

// write 使用 bits 的低 valid 位追加下一个有效指针槽的指针性。
// 1=指针，0=标量。
func (h writeUserArenaHeapBits) write(s *mspan, bits, valid uintptr) writeUserArenaHeapBits {
	if h.valid+valid <= ptrBits {
		// 快速路径 - 只是累积位。
		h.mask |= bits << h.valid
		h.valid += valid
		return h
	}
	// 位太多无法放入这个字。写出当前字并移动到下一个字。

	data := h.mask | bits<<h.valid       // 这个字的掩码
	h.mask = bits >> (ptrBits - h.valid) // 下一个字的剩余
	h.valid += valid - ptrBits           // 有 h.valid+valid 位，写入其中 ptrBits 个

	// 将掩码刷新到内存位图。
	idx := h.offset / (ptrBits * goarch.PtrSize)
	m := uintptr(1)<<h.low - 1
	bitmap := s.heapBits()
	bitmap[idx] = bswapIfBigEndian(bswapIfBigEndian(bitmap[idx])&m | data)
	// 注意：此写入不需要同步，因为分配器对页面有独占访问权，
	// 并且位图条目都用于单个页面。此外，这些写入的可见性
	// 由 mallocgc 中的发布屏障保证。

	// 移动到位图的下一个字。
	h.offset += ptrBits * goarch.PtrSize
	h.low = 0
	return h
}

// 添加 size 字节的填充。
func (h writeUserArenaHeapBits) pad(s *mspan, size uintptr) writeUserArenaHeapBits {
	if size == 0 {
		return h
	}
	words := size / goarch.PtrSize
	for words > ptrBits {
		h = h.write(s, 0, ptrBits)
		words -= ptrBits
	}
	return h.write(s, 0, words)
}

// flush 刷新已写入的位，并根据需要添加零以覆盖完整对象 [addr, addr+size)。
func (h writeUserArenaHeapBits) flush(s *mspan, addr, size uintptr) {
	offset := addr - s.base()

	// zeros 计算表示对象所需的位数减去我们已经写入的位数。
	// 这是需要添加的 0 位数。
	zeros := (offset+size-h.offset)/goarch.PtrSize - h.valid

	// 将零位添加到位图字边界
	if zeros > 0 {
		z := ptrBits - h.valid
		if z > zeros {
			z = zeros
		}
		h.valid += z
		zeros -= z
	}

	// 找到我们要写入的位图中的字。
	bitmap := s.heapBits()
	idx := h.offset / (ptrBits * goarch.PtrSize)

	// 写入剩余的位。
	if h.valid != h.low {
		m := uintptr(1)<<h.low - 1      // 不清除 "low" 以下的现有位
		m |= ^(uintptr(1)<<h.valid - 1) // 不清除 "valid" 以上的现有位
		bitmap[idx] = bswapIfBigEndian(bswapIfBigEndian(bitmap[idx])&m | h.mask)
	}
	if zeros == 0 {
		return
	}

	// 前进到下一个位图字。
	h.offset += ptrBits * goarch.PtrSize

	// 继续为对象的其余部分写入零。
	// 对于 ptr 位的标准使用，这不是必需的，
	// 因为位是从对象的开头读取的。某些用途，
	// 如 noscan span、oblet、批量写屏障和 cgocheck，
	// 可能从对象中间开始，所以这些写入仍然是必需的。
	for {
		// 写入零位。
		idx := h.offset / (ptrBits * goarch.PtrSize)
		if zeros < ptrBits {
			bitmap[idx] = bswapIfBigEndian(bswapIfBigEndian(bitmap[idx]) &^ (uintptr(1)<<zeros - 1))
			break
		} else if zeros == ptrBits {
			bitmap[idx] = 0
			break
		} else {
			bitmap[idx] = 0
			zeros -= ptrBits
		}
		h.offset += ptrBits * goarch.PtrSize
	}
}

// bswapIfBigEndian 在 goarch.BigEndian 平台上交换 uintptr 的字节顺序，
// 在其他地方保持不变。
func bswapIfBigEndian(x uintptr) uintptr {
	if goarch.BigEndian {
		if goarch.PtrSize == 8 {
			return uintptr(sys.Bswap64(uint64(x)))
		}
		return uintptr(sys.Bswap32(uint32(x)))
	}
	return x
}

// newUserArenaChunk 分配一个用户内存区域块，它映射到单个堆内存区域和单个 span。
// 返回指向块基址的指针（这非常重要：我们需要保持块存活）和 span。
func newUserArenaChunk() (unsafe.Pointer, *mspan) {
	if gcphase == _GCmarktermination {
		throw("newUserArenaChunk called with gcphase == _GCmarktermination")
	}

	// 扣除辅助积分。因为用户内存区域块被建模为一个巨大的堆对象，
	// 计入 heapLive，我们有义务按比例辅助 GC（值得注意的是，
	// 内存区域确实代表了 GC 的额外工作，但在我们实际将东西分配到
	// 内存区域之前，我们也不知道那看起来像什么）。
	if gcBlackenEnabled != 0 {
		deductAssistCredit(userArenaChunkBytes)
	}

	// 设置 mp.mallocing 以防止被 GC 抢占。
	mp := acquirem()
	if mp.mallocing != 0 {
		throw("malloc deadlock")
	}
	if mp.gsignal == getg() {
		throw("malloc during signal")
	}
	mp.mallocing = 1

	// 分配一个新的用户内存区域。
	var span *mspan
	systemstack(func() {
		span = mheap_.allocUserArenaChunk()
	})
	if span == nil {
		throw("out of memory")
	}
	x := unsafe.Pointer(span.base())

	// 在 GC 期间分配黑色。
	// 所有槽都持有 nil，所以不需要扫描。
	// 这可能与 GC 竞争，所以如果可能存在标记位的竞争，则以原子方式进行。
	if gcphase != _GCoff {
		gcmarknewobject(span, span.base())
	}

	if raceenabled {
		// TODO(mknyszek): 跟踪单个对象。
		racemalloc(unsafe.Pointer(span.base()), span.elemsize)
	}

	if msanenabled {
		// TODO(mknyszek): 跟踪单个对象。
		msanmalloc(unsafe.Pointer(span.base()), span.elemsize)
	}

	if asanenabled {
		// TODO(mknyszek): 跟踪单个对象。
		// 注意：span.elemsize 已经包含了红区。
		rzStart := span.base() + span.elemsize
		asanpoison(unsafe.Pointer(rzStart), span.limit-rzStart)
		asanunpoison(unsafe.Pointer(span.base()), span.elemsize)
	}

	if rate := MemProfileRate; rate > 0 {
		c := getMCache(mp)
		if c == nil {
			throw("newUserArenaChunk called without a P or outside bootstrapping")
		}
		// 注意缓存 c 仅在获取 m 期间有效；参见 #47302
		if rate != 1 && int64(userArenaChunkBytes) < c.nextSample {
			c.nextSample -= int64(userArenaChunkBytes)
		} else {
			profilealloc(mp, unsafe.Pointer(span.base()), userArenaChunkBytes)
		}
	}
	mp.mallocing = 0
	releasem(mp)

	// 同样，因为这个块计入 heapLive，可能触发 GC。
	if t := (gcTrigger{kind: gcTriggerHeap}); t.test() {
		gcStart(t)
	}

	if debug.malloc {
		if inittrace.active && inittrace.id == getg().goid {
			// 初始化函数在单个 goroutine 中顺序执行。
			inittrace.bytes += uint64(userArenaChunkBytes)
		}
	}

	// 仔细检查它是否对齐到物理页大小。基于当前实现，这是显而易见的，
	// 但将来可能不是。然而，如果它没有对齐到物理页大小，
	// 那么我们以后就无法正确地将其设置为故障。
	if uintptr(x)%physPageSize != 0 {
		throw("user arena chunk is not aligned to the physical page size")
	}

	return x, span
}

// isUnusedUserArenaChunk 表示内存区域块已被设置为故障，
// 并且不再包含任何可扫描的内存。然而，它可能仍然是 mSpanInUse，
// 因为它位于隔离列表上，因为它需要被清扫。
//
// 除非调用者拥有 mspan 的所有权或世界已停止
// （在相关状态更改时阻止抢占），否则执行是不安全的。
//
// 这实际上只是用于运行时中的记账测试，
// 以区分何时不应该计数 span（因为 mSpanInUse 可能不够）。
func (s *mspan) isUnusedUserArenaChunk() bool {
	return s.isUserArenaChunk && s.spanclass == makeSpanClass(0, true)
}

// setUserArenaChunkToFault 将用户内存区域块的地址空间设置为故障
// 并释放任何底层内存资源。
//
// 必须处于不可抢占状态以确保导出到 MemStats 的统计数据的一致性。
func (s *mspan) setUserArenaChunkToFault() {
	if !s.isUserArenaChunk {
		throw("invalid span in heapArena for user arena")
	}
	if s.npages*pageSize != userArenaChunkBytes {
		throw("span on userArena.faultList has invalid size")
	}

	// 将 span 类更新为 noscan。我们希望发生的是：
	// 任何指向 span 的指针都会阻止它被回收，所以我们希望设置标记位，
	// 但我们即将将地址空间设置为故障，所以我们必须阻止 GC 扫描这块内存。
	//
	// 在这里设置是可以的，因为 (1) GC 没有在进行中，所以扫描代码
	// 不会做出错误的决定，(2) 我们当前是不可抢占的并且在运行时中，
	// 所以 GC 被阻止启动。我们可能与清扫竞争，这可能会将其放入
	// "错误的"清扫列表，但实际上并不关心，因为块被视为大对象 span，
	// 清扫器中扫描和 noscan 大对象之间没有有意义的区别。
	// GC 开始时的 STW 充当此更新的屏障。
	s.spanclass = makeSpanClass(0, true)

	// 实际将内存区域块设置为故障，这样我们就会得到悬空指针错误。
	// sysFault 当前在每个操作系统上使用一种方法，强制它撤出
	// 支持该块的所有内存。
	sysFault(unsafe.Pointer(s.base()), s.npages*pageSize)

	// 列表上的所有内容都被计为正在使用，然而 sysFault 转换为
	// Reserved 而不是 Prepared，所以我们跳过更新 heapFree 或 heapReleased，
	// 只是将内存从总数中完全删除；现在它只是地址空间。
	gcController.heapInUse.add(-int64(s.npages * pageSize))

	// 将此计为对象的立即释放，而不是 span 从隔离列表中移除时。
	// 主要原因是分配的字节量不超过计为"已映射就绪"的量，
	// 这可能导致步调器中的死锁。
	gcController.totalFree.Add(int64(s.elemsize))

	// 更新一致的统计数据以匹配。
	//
	// 我们是不可抢占的，所以更新一致的统计数据是安全的
	// （我们的 P 不会从我们下面改变）。
	stats := memstats.heapStats.acquire()
	atomic.Xaddint64(&stats.committed, -int64(s.npages*pageSize))
	atomic.Xaddint64(&stats.inHeap, -int64(s.npages*pageSize))
	atomic.Xadd64(&stats.largeFreeCount, 1)
	atomic.Xadd64(&stats.largeFree, int64(s.elemsize))
	memstats.heapStats.release()

	// 这算作一次释放，所以更新 heapLive。
	gcController.update(-int64(s.elemsize), 0)

	// 为竞态检测器将其标记为空闲。
	if raceenabled {
		racefree(unsafe.Pointer(s.base()), s.elemsize)
	}

	systemstack(func() {
		// 将用户内存区域添加到隔离列表。
		lock(&mheap_.lock)
		mheap_.userArena.quarantineList.insert(s)
		unlock(&mheap_.lock)
	})
}

// inUserArenaChunk 如果 p 指向用户内存区域块则返回 true。
func inUserArenaChunk(p uintptr) bool {
	s := spanOf(p)
	if s == nil {
		return false
	}
	return s.isUserArenaChunk
}

// freeUserArenaChunk 将 s 表示的用户内存区域释放回运行时。
//
// x 必须是 s 内的活动指针。
//
// 一旦安全（GC 不再运行），运行时将把用户内存区域设置为故障，
// 然后一旦应用程序不再引用用户内存区域，将允许重用它。
func freeUserArenaChunk(s *mspan, x unsafe.Pointer) {
	if !s.isUserArenaChunk {
		throw("span is not for a user arena")
	}
	if s.npages*pageSize != userArenaChunkBytes {
		throw("invalid user arena span size")
	}

	// 立即将区域标记为对各种清理器空闲，而不是在清扫时处理它们。
	if raceenabled {
		racefree(unsafe.Pointer(s.base()), s.elemsize)
	}
	if msanenabled {
		msanfree(unsafe.Pointer(s.base()), s.elemsize)
	}
	if asanenabled {
		asanpoison(unsafe.Pointer(s.base()), s.elemsize)
	}
	if valgrindenabled {
		valgrindFree(unsafe.Pointer(s.base()))
	}

	// 在操作状态和统计数据时使自己不可抢占。
	//
	// 也是 setUserArenaChunksToFault 所需的。
	mp := acquirem()

	// 只有在 _GCoff 阶段我们才能将用户内存区域设置为故障。
	if gcphase == _GCoff {
		lock(&userArenaState.lock)
		faultList := userArenaState.fault
		userArenaState.fault = nil
		unlock(&userArenaState.lock)

		s.setUserArenaChunkToFault()
		for _, lc := range faultList {
			lc.mspan.setUserArenaChunkToFault()
		}

		// 在块被设置为故障之前，通过故障列表保持它们存活。
		KeepAlive(x)
		KeepAlive(faultList)
	} else {
		// 将用户内存区域放入故障列表。
		lock(&userArenaState.lock)
		userArenaState.fault = append(userArenaState.fault, liveUserArenaChunk{s, x})
		unlock(&userArenaState.lock)
	}
	releasem(mp)
}

// allocUserArenaChunk 尝试重用表示为 span 的空闲用户内存区域块。
//
// 必须处于不可抢占状态以确保导出到 MemStats 的统计数据的一致性。
//
// 获取堆锁。因此必须在系统栈上运行。
//
//go:systemstack
func (h *mheap) allocUserArenaChunk() *mspan {
	var s *mspan
	var base uintptr

	// 首先检查空闲列表。
	lock(&h.lock)
	if !h.userArena.readyList.isEmpty() {
		s = h.userArena.readyList.first
		h.userArena.readyList.remove(s)
		base = s.base()
	} else {
		// 空闲列表为空，所以分配一个新的内存区域。
		hintList := &h.userArena.arenaHints
		if raceenabled {
			// 在竞态模式下只使用常规堆提示。我们可能会碎片化
			// 地址空间，但竞态检测器要求堆连续映射。
			hintList = &h.arenaHints
		}
		v, size := h.sysAlloc(userArenaChunkBytes, hintList, &mheap_.userArenaArenas)
		if size%userArenaChunkBytes != 0 {
			throw("sysAlloc size is not divisible by userArenaChunkBytes")
		}
		if size > userArenaChunkBytes {
			// 我们得到了比请求更多的。如果 heapArenaSize > userArenaChunkSize，
			// 或者 sysAlloc 作为尝试找到对齐区域的结果返回了一些额外的，
			// 就会发生这种情况。
			//
			// 将其分割并放入就绪列表。
			for i := userArenaChunkBytes; i < size; i += userArenaChunkBytes {
				s := h.allocMSpanLocked()
				s.init(uintptr(v)+i, userArenaChunkPages)
				h.userArena.readyList.insertBack(s)
			}
			size = userArenaChunkBytes
		}
		base = uintptr(v)
		if base == 0 {
			// 内存不足。
			unlock(&h.lock)
			return nil
		}
		s = h.allocMSpanLocked()
	}
	unlock(&h.lock)

	// sysAlloc 返回 Reserved 地址空间，我们重用的任何 span 都设置为
	// 故障（所以也是 Reserved），所以将其转换为 Prepared 然后 Ready。
	//
	// 与 (*mheap).grow 不同，只需映射我们请求的所有内容。
	// 我们很可能会全部使用。
	sysMap(unsafe.Pointer(base), userArenaChunkBytes, &gcController.heapReleased, "user arena chunk")
	sysUsed(unsafe.Pointer(base), userArenaChunkBytes, userArenaChunkBytes)

	// 将用户内存区域建模为大对象的堆 span。
	spc := makeSpanClass(0, false)
	// 用户内存区域块总是从操作系统新获取的。它要么是通过 sysAlloc() 新分配的，
	// 要么是在 sysFault() 之后从 readyList 重用的。然后内存通过 sysMap() 重新映射，
	// 所以我们可以安全地将其视为已清理的；内核保证在下次使用时将其填零。
	h.initSpan(s, spanAllocHeap, spc, base, userArenaChunkPages, userArenaChunkBytes)
	s.isUserArenaChunk = true
	s.elemsize -= userArenaChunkReserveBytes()
	s.freeindex = 1
	s.allocCount = 1

	// 将 s.limit 向下调整到 span 的对象包含部分。
	//
	// 这只是为了在限制上创建一个稍微更紧的边界。
	// 如果垃圾回收器，特别是保守扫描，可以暂时观察到一个膨胀的限制，
	// 这完全没问题。它只会标记整个块或者跳过它，因为我们无论如何都在标记阶段。
	s.limit = s.base() + s.elemsize

	// 调整大小以包含红区。
	if asanenabled {
		s.elemsize -= redZoneSize(s.elemsize)
	}

	// 记账这个新的内存区域块内存。
	gcController.heapInUse.add(int64(userArenaChunkBytes))
	gcController.heapReleased.add(-int64(userArenaChunkBytes))

	stats := memstats.heapStats.acquire()
	atomic.Xaddint64(&stats.inHeap, int64(userArenaChunkBytes))
	atomic.Xaddint64(&stats.committed, int64(userArenaChunkBytes))

	// 将内存区域建模为单个大型 malloc。
	atomic.Xadd64(&stats.largeAlloc, int64(s.elemsize))
	atomic.Xadd64(&stats.largeAllocCount, 1)
	memstats.heapStats.release()

	// 在不一致的内部统计中计算分配。
	gcController.totalAlloc.Add(int64(s.elemsize))

	// 更新 heapLive。
	gcController.update(int64(s.elemsize), 0)

	// 这必须清除整个堆位图，以便在不写入任何内容的情况下
	// 分配 noscan 数据是安全的。
	s.initHeapBits()

	// 预先清除 span。它是一个内存区域块，所以让我们假设
	// 所有内容都将被使用。
	//
	// 这似乎也对 Linux 是否决定用透明大页支持这块内存产生巨大差异。
	// 这个清零涉及延迟，但大页的收益几乎总是值得的。
	// 注意：即使它是新映射的并且我们知道清零没有意义，
	// 清除也很重要，因为*那*是使用大页的关键信号。
	memclrNoHeapPointers(unsafe.Pointer(s.base()), s.elemsize)
	s.needzero = 0

	s.freeIndexForScan = 1

	// 设置分配范围。
	s.userArenaChunkFree = makeAddrRange(base, base+s.elemsize)

	// 将大 span 放入 mcentral 已清扫列表，以便后台清扫器可见。
	h.central[spc].mcentral.fullSwept(h.sweepgen).push(s)

	// 设置分配头。这里避免写屏障，因为这个类型不是真正的类型，
	// 并且它存在于无效位置。
	*(*uintptr)(unsafe.Pointer(&s.largeType)) = uintptr(unsafe.Pointer(s.limit))
	*(*uintptr)(unsafe.Pointer(&s.largeType.GCData)) = s.limit + unsafe.Sizeof(_type{})
	s.largeType.PtrBytes = 0
	s.largeType.Size_ = s.elemsize

	return s
}
