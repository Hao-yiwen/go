// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/goarch"
	"internal/goexperiment"
	"internal/runtime/math"
	"internal/runtime/sys"
	"unsafe"
)

type slice struct {
	array unsafe.Pointer
	len   int
	cap   int
}

// notInHeapSlice 是由 internal/runtime/sys.NotInHeap 内存支持的切片。
type notInHeapSlice struct {
	array *notInHeap
	len   int
	cap   int
}

func panicmakeslicelen() {
	panic(errorString("makeslice: len out of range"))
}

func panicmakeslicecap() {
	panic(errorString("makeslice: cap out of range"))
}

// makeslicecopy 分配一个包含 "tolen" 个类型为 "et" 的元素的切片，
// 然后将 "fromlen" 个类型为 "et" 的元素从 "from" 复制到该新分配中。
func makeslicecopy(et *_type, tolen int, fromlen int, from unsafe.Pointer) unsafe.Pointer {
	var tomem, copymem uintptr
	if uintptr(tolen) > uintptr(fromlen) {
		var overflow bool
		tomem, overflow = math.MulUintptr(et.Size_, uintptr(tolen))
		if overflow || tomem > maxAlloc || tolen < 0 {
			panicmakeslicelen()
		}
		copymem = et.Size_ * uintptr(fromlen)
	} else {
		// fromlen 是一个已知的良好长度，提供并等于或大于 tolen，
		// 因此也使 tolen 成为一个好的切片长度，因为 from 和 to 切片具有
		// 相同的元素宽度。
		tomem = et.Size_ * uintptr(tolen)
		copymem = tomem
	}

	var to unsafe.Pointer
	if !et.Pointers() {
		to = mallocgc(tomem, nil, false)
		if copymem < tomem {
			memclrNoHeapPointers(add(to, copymem), tomem-copymem)
		}
	} else {
		// 注意：不能使用 rawmem（避免内存清零），因为那样 GC 可以扫描未初始化的内存。
		to = mallocgc(tomem, et, true)
		if copymem > 0 && writeBarrier.enabled {
			// 仅对 old.array 中的指针进行着色，因为我们知道目标切片
			// 仅包含 nil 指针，因为它在分配期间已清除。
			//
			// 将类型传递给此函数作为优化是安全的，因为
			// from 和 to 仅引用表示
			// 类型 et 的整个值的内存。参见 bulkBarrierPreWrite 上的注释。
			bulkBarrierPreWriteSrcOnly(uintptr(to), uintptr(from), copymem, et)
		}
	}

	if raceenabled {
		callerpc := sys.GetCallerPC()
		pc := abi.FuncPCABIInternal(makeslicecopy)
		racereadrangepc(from, copymem, callerpc, pc)
	}
	if msanenabled {
		msanread(from, copymem)
	}
	if asanenabled {
		asanread(from, copymem)
	}

	memmove(to, from, copymem)

	return to
}

// makeslice 应该是一个内部细节，
// 但广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/bytedance/sonic
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname makeslice
func makeslice(et *_type, len, cap int) unsafe.Pointer {
	mem, overflow := math.MulUintptr(et.Size_, uintptr(cap))
	if overflow || mem > maxAlloc || len < 0 || len > cap {
		// 注意：当某人执行 make([]T, bignumber) 时，产生"len 超出范围"错误而不是
		// "cap 超出范围"错误。
		// "cap 超出范围"也是真的，但由于上限仅隐含地
		// 供应，说 len 更清楚。
		// 参见 golang.org/issue/4085。
		mem, overflow := math.MulUintptr(et.Size_, uintptr(len))
		if overflow || mem > maxAlloc || len < 0 {
			panicmakeslicelen()
		}
		panicmakeslicecap()
	}

	return mallocgc(mem, et, true)
}

func makeslice64(et *_type, len64, cap64 int64) unsafe.Pointer {
	len := int(len64)
	if int64(len) != len64 {
		panicmakeslicelen()
	}

	cap := int(cap64)
	if int64(cap) != cap64 {
		panicmakeslicecap()
	}

	return makeslice(et, len, cap)
}

// growslice 为切片分配新的备份存储。
//
// 参数：
//
//	oldPtr = 指向切片的备份数组的指针
//	newLen = 新长度（= oldLen + num）
//	oldCap = 原始切片的容量。
//	   num = 被添加的元素数
//	    et = 元素类型
//
// 返回值：
//
//	newPtr = 指向新备份存储的指针
//	newLen = 与参数相同的值
//	newCap = 新备份存储的容量
//
// 要求 uint(newLen) > uint(oldCap)。
// 假设原始切片长度为 newLen - num
//
// 分配新的备份存储，至少有 newLen 个元素的空间。
// 现有条目 [0, oldLen) 被复制到新的备份存储。
// 添加的条目 [oldLen, newLen) 不由 growslice 初始化
// （尽管对于包含指针的元素类型，它们被清零）。它们
// 必须由调用者初始化。
// 尾随条目 [newLen, newCap) 被清零。
//
// growslice 的奇怪调用约定使得调用
// 此函数的生成代码更简单。特别是，它接受并返回
// 新长度，以便旧长度不是活跃的（不需要
// 溢出/恢复），并且新长度被返回（也不需要
// 溢出/恢复）。
//
// growslice 应该是一个内部细节，
// 但广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/bytedance/sonic
//   - github.com/chenzhuoyu/iasm
//   - github.com/cloudwego/dynamicgo
//   - github.com/ugorji/go/codec
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname growslice
func growslice(oldPtr unsafe.Pointer, newLen, oldCap, num int, et *_type) slice {
	oldLen := newLen - num
	if raceenabled {
		callerpc := sys.GetCallerPC()
		racereadrangepc(oldPtr, uintptr(oldLen*int(et.Size_)), callerpc, abi.FuncPCABIInternal(growslice))
	}
	if msanenabled {
		msanread(oldPtr, uintptr(oldLen*int(et.Size_)))
	}
	if asanenabled {
		asanread(oldPtr, uintptr(oldLen*int(et.Size_)))
	}

	if newLen < 0 {
		panic(errorString("growslice: len out of range"))
	}

	if et.Size_ == 0 {
		// append 不应该创建一个具有 nil 指针但非零 len 的切片。
		// 我们假设 append 在这种情况下不需要保留 oldPtr。
		return slice{unsafe.Pointer(&zerobase), newLen, newLen}
	}

	newcap := nextslicecap(newLen, oldCap)

	var overflow bool
	var lenmem, newlenmem, capmem uintptr
	// 对 et.Size 的常见值进行专业化。
	// 对于 1，我们不需要任何除法/乘法。
	// 对于 goarch.PtrSize，编译器会优化除法/乘法为一个常数的移位。
	// 对于 2 的幂，使用变量移位。
	noscan := !et.Pointers()
	switch {
	case et.Size_ == 1:
		lenmem = uintptr(oldLen)
		newlenmem = uintptr(newLen)
		capmem = roundupsize(uintptr(newcap), noscan)
		overflow = uintptr(newcap) > maxAlloc
		newcap = int(capmem)
	case et.Size_ == goarch.PtrSize:
		lenmem = uintptr(oldLen) * goarch.PtrSize
		newlenmem = uintptr(newLen) * goarch.PtrSize
		capmem = roundupsize(uintptr(newcap)*goarch.PtrSize, noscan)
		overflow = uintptr(newcap) > maxAlloc/goarch.PtrSize
		newcap = int(capmem / goarch.PtrSize)
	case isPowerOfTwo(et.Size_):
		var shift uintptr
		if goarch.PtrSize == 8 {
			// 掩码移位以更好的代码生成。
			shift = uintptr(sys.TrailingZeros64(uint64(et.Size_))) & 63
		} else {
			shift = uintptr(sys.TrailingZeros32(uint32(et.Size_))) & 31
		}
		lenmem = uintptr(oldLen) << shift
		newlenmem = uintptr(newLen) << shift
		capmem = roundupsize(uintptr(newcap)<<shift, noscan)
		overflow = uintptr(newcap) > (maxAlloc >> shift)
		newcap = int(capmem >> shift)
		capmem = uintptr(newcap) << shift
	default:
		lenmem = uintptr(oldLen) * et.Size_
		newlenmem = uintptr(newLen) * et.Size_
		capmem, overflow = math.MulUintptr(et.Size_, uintptr(newcap))
		capmem = roundupsize(capmem, noscan)
		newcap = int(capmem / et.Size_)
		capmem = uintptr(newcap) * et.Size_
	}

	// 除了 capmem > maxAlloc 之外，overflow 的检查是必要的
	// 以防止可用于触发的溢出
	// 在 32 位体系结构上使用此示例程序出现分段错误：
	//
	// type T [1<<27 + 1]int64
	//
	// var d T
	// var s []T
	//
	// func main() {
	//   s = append(s, d, d, d, d)
	//   print(len(s), "\n")
	// }
	if overflow || capmem > maxAlloc {
		panic(errorString("growslice: len out of range"))
	}

	var p unsafe.Pointer
	if !et.Pointers() {
		p = mallocgc(capmem, nil, false)
		// The append() that calls growslice is going to overwrite from oldLen to newLen.
		// Only clear the part that will not be overwritten.
		// The reflect_growslice() that calls growslice will manually clear
		// the region not cleared here.
		memclrNoHeapPointers(add(p, newlenmem), capmem-newlenmem)
	} else {
		// Note: can't use rawmem (which avoids zeroing of memory), because then GC can scan uninitialized memory.
		p = mallocgc(capmem, et, true)
		if lenmem > 0 && writeBarrier.enabled {
			// Only shade the pointers in oldPtr since we know the destination slice p
			// only contains nil pointers because it has been cleared during alloc.
			//
			// It's safe to pass a type to this function as an optimization because
			// from and to only ever refer to memory representing whole values of
			// type et. See the comment on bulkBarrierPreWrite.
			bulkBarrierPreWriteSrcOnly(uintptr(p), uintptr(oldPtr), lenmem-et.Size_+et.PtrBytes, et)
		}
	}
	memmove(p, oldPtr, lenmem)

	return slice{p, newLen, newcap}
}

// growsliceNoAlias is like growslice but only for the case where
// we know that oldPtr is not aliased.
//
// In other words, the caller must know that there are no other references
// to the backing memory of the slice being grown aside from the slice header
// that will be updated with new backing memory when growsliceNoAlias
// returns, and therefore oldPtr must be the only pointer to its referent
// aside from the slice header updated by the returned slice.
//
// In addition, oldPtr must point to the start of the allocation and match
// the pointer that was returned by mallocgc. In particular, oldPtr must not
// be an interior pointer, such as after a reslice.
//
// See freegc for details.
func growsliceNoAlias(oldPtr unsafe.Pointer, newLen, oldCap, num int, et *_type) slice {
	s := growslice(oldPtr, newLen, oldCap, num, et)
	if goexperiment.RuntimeFreegc && oldPtr != nil && oldPtr != s.array {
		if gp := getg(); uintptr(oldPtr) < gp.stack.lo || gp.stack.hi <= uintptr(oldPtr) {
			// oldPtr does not point into the current stack, and it is not
			// the data pointer for s after the grow, so attempt to free it.
			// (Note that freegc also verifies that oldPtr does not point into our stack,
			// but checking here first is slightly cheaper for the case when
			// oldPtr is on the stack and freegc would be a no-op.)
			//
			// TODO(thepudds): it may be that oldPtr==s.array only when elemsize==0,
			// so perhaps we could prohibit growsliceNoAlias being called in that case
			// and eliminate that check here, or alternatively, we could lean into
			// freegc being a no-op for zero-sized allocations (that is, no check of
			// oldPtr != s.array here and just let freegc return quickly).
			noscan := !et.Pointers()
			freegc(oldPtr, uintptr(oldCap)*et.Size_, noscan)
		}
	}
	return s
}

// nextslicecap computes the next appropriate slice length.
func nextslicecap(newLen, oldCap int) int {
	newcap := oldCap
	doublecap := newcap + newcap
	if newLen > doublecap {
		return newLen
	}

	const threshold = 256
	if oldCap < threshold {
		return doublecap
	}
	for {
		// Transition from growing 2x for small slices
		// to growing 1.25x for large slices. This formula
		// gives a smooth-ish transition between the two.
		newcap += (newcap + 3*threshold) >> 2

		// We need to check `newcap >= newLen` and whether `newcap` overflowed.
		// newLen is guaranteed to be larger than zero, hence
		// when newcap overflows then `uint(newcap) > uint(newLen)`.
		// This allows to check for both with the same comparison.
		if uint(newcap) >= uint(newLen) {
			break
		}
	}

	// Set newcap to the requested cap when
	// the newcap calculation overflowed.
	if newcap <= 0 {
		return newLen
	}
	return newcap
}

// reflect_growslice should be an internal detail,
// but widely used packages access it using linkname.
// Notable members of the hall of shame include:
//   - github.com/cloudwego/dynamicgo
//
// Do not remove or change the type signature.
// See go.dev/issue/67401.
//
//go:linkname reflect_growslice reflect.growslice
func reflect_growslice(et *_type, old slice, num int) slice {
	// Semantically equivalent to slices.Grow, except that the caller
	// is responsible for ensuring that old.len+num > old.cap.
	num -= old.cap - old.len // preserve memory of old[old.len:old.cap]
	new := growslice(old.array, old.cap+num, old.cap, num, et)
	// growslice does not zero out new[old.cap:new.len] since it assumes that
	// the memory will be overwritten by an append() that called growslice.
	// Since the caller of reflect_growslice is not append(),
	// zero out this region before returning the slice to the reflect package.
	if !et.Pointers() {
		oldcapmem := uintptr(old.cap) * et.Size_
		newlenmem := uintptr(new.len) * et.Size_
		memclrNoHeapPointers(add(new.array, oldcapmem), newlenmem-oldcapmem)
	}
	new.len = old.len // preserve the old length
	return new
}

func isPowerOfTwo(x uintptr) bool {
	return x&(x-1) == 0
}

// slicecopy is used to copy from a string or slice of pointerless elements into a slice.
func slicecopy(toPtr unsafe.Pointer, toLen int, fromPtr unsafe.Pointer, fromLen int, width uintptr) int {
	if fromLen == 0 || toLen == 0 {
		return 0
	}

	n := fromLen
	if toLen < n {
		n = toLen
	}

	if width == 0 {
		return n
	}

	size := uintptr(n) * width
	if raceenabled {
		callerpc := sys.GetCallerPC()
		pc := abi.FuncPCABIInternal(slicecopy)
		racereadrangepc(fromPtr, size, callerpc, pc)
		racewriterangepc(toPtr, size, callerpc, pc)
	}
	if msanenabled {
		msanread(fromPtr, size)
		msanwrite(toPtr, size)
	}
	if asanenabled {
		asanread(fromPtr, size)
		asanwrite(toPtr, size)
	}

	if size == 1 { // common case worth about 2x to do here
		// TODO: is this still worth it with new memmove impl?
		*(*byte)(toPtr) = *(*byte)(fromPtr) // known to be a byte pointer
	} else {
		memmove(toPtr, fromPtr, size)
	}
	return n
}

//go:linkname bytealg_MakeNoZero internal/bytealg.MakeNoZero
func bytealg_MakeNoZero(len int) []byte {
	if uintptr(len) > maxAlloc {
		panicmakeslicelen()
	}
	cap := roundupsize(uintptr(len), true)
	return unsafe.Slice((*byte)(mallocgc(cap, nil, false)), cap)[:len]
}

// moveSlice copies the input slice to the heap and returns it.
// et is the element type of the slice.
func moveSlice(et *_type, old unsafe.Pointer, len, cap int) (unsafe.Pointer, int, int) {
	if cap == 0 {
		if old != nil {
			old = unsafe.Pointer(&zerobase)
		}
		return old, 0, 0
	}
	capmem := uintptr(cap) * et.Size_
	new := mallocgc(capmem, et, true)
	bulkBarrierPreWriteSrcOnly(uintptr(new), uintptr(old), capmem, et)
	memmove(new, old, capmem)
	return new, len, cap
}

// moveSliceNoScan is like moveSlice except the element type is known to
// not have any pointers. We instead pass in the size of the element.
func moveSliceNoScan(elemSize uintptr, old unsafe.Pointer, len, cap int) (unsafe.Pointer, int, int) {
	if cap == 0 {
		if old != nil {
			old = unsafe.Pointer(&zerobase)
		}
		return old, 0, 0
	}
	capmem := uintptr(cap) * elemSize
	new := mallocgc(capmem, nil, false)
	memmove(new, old, capmem)
	return new, len, cap
}

// moveSliceNoCap is like moveSlice, but can pick any appropriate capacity
// for the returned slice.
// Elements between len and cap in the returned slice will be zeroed.
func moveSliceNoCap(et *_type, old unsafe.Pointer, len int) (unsafe.Pointer, int, int) {
	if len == 0 {
		if old != nil {
			old = unsafe.Pointer(&zerobase)
		}
		return old, 0, 0
	}
	lenmem := uintptr(len) * et.Size_
	capmem := roundupsize(lenmem, false)
	new := mallocgc(capmem, et, true)
	bulkBarrierPreWriteSrcOnly(uintptr(new), uintptr(old), lenmem, et)
	memmove(new, old, lenmem)
	return new, len, int(capmem / et.Size_)
}

// moveSliceNoCapNoScan is a combination of moveSliceNoScan and moveSliceNoCap.
func moveSliceNoCapNoScan(elemSize uintptr, old unsafe.Pointer, len int) (unsafe.Pointer, int, int) {
	if len == 0 {
		if old != nil {
			old = unsafe.Pointer(&zerobase)
		}
		return old, 0, 0
	}
	lenmem := uintptr(len) * elemSize
	capmem := roundupsize(lenmem, true)
	new := mallocgc(capmem, nil, false)
	memmove(new, old, lenmem)
	if capmem > lenmem {
		memclrNoHeapPointers(add(new, lenmem), capmem-lenmem)
	}
	return new, len, int(capmem / elemSize)
}

// growsliceBuf is like growslice, but we can use the given buffer
// as a backing store if we want. bufPtr must be on the stack.
func growsliceBuf(oldPtr unsafe.Pointer, newLen, oldCap, num int, et *_type, bufPtr unsafe.Pointer, bufLen int) slice {
	if newLen > bufLen {
		// Doesn't fit, process like a normal growslice.
		return growslice(oldPtr, newLen, oldCap, num, et)
	}
	oldLen := newLen - num
	if oldPtr != bufPtr && oldLen != 0 {
		// Move data to start of buffer.
		// Note: bufPtr is on the stack, so no write barrier needed.
		memmove(bufPtr, oldPtr, uintptr(oldLen)*et.Size_)
	}
	// Pick a new capacity.
	//
	// Unlike growslice, we don't need to double the size each time.
	// The work done here is not proportional to the length of the slice.
	// (Unless the memmove happens above, but that is rare, and in any
	// case there are not many elements on this path.)
	//
	// Instead, we try to just bump up to the next size class.
	// This will ensure that we don't waste any space when we eventually
	// call moveSlice with the resulting slice.
	newCap := int(roundupsize(uintptr(newLen)*et.Size_, !et.Pointers()) / et.Size_)

	// Zero slice beyond newLen.
	// The buffer is stack memory, so NoHeapPointers is ok.
	// Caller will overwrite [oldLen:newLen], so we don't need to zero that portion.
	// If et.Pointers(), buffer is at least initialized so we don't need to
	// worry about the caller overwriting junk in [oldLen:newLen].
	if newLen < newCap {
		memclrNoHeapPointers(add(bufPtr, uintptr(newLen)*et.Size_), uintptr(newCap-newLen)*et.Size_)
	}

	return slice{bufPtr, newLen, newCap}
}

// growsliceBufNoAlias is a combination of growsliceBuf and growsliceNoAlias.
// bufPtr must be on the stack.
func growsliceBufNoAlias(oldPtr unsafe.Pointer, newLen, oldCap, num int, et *_type, bufPtr unsafe.Pointer, bufLen int) slice {
	s := growsliceBuf(oldPtr, newLen, oldCap, num, et, bufPtr, bufLen)
	if goexperiment.RuntimeFreegc && oldPtr != bufPtr && oldPtr != nil && oldPtr != s.array {
		// oldPtr is not bufPtr (the stack buffer) and it is not
		// the data pointer for s after the grow, so attempt to free it.
		// (Note that freegc does a broader check that oldPtr does not point into our stack,
		// but checking here first is slightly cheaper for a common case when oldPtr is bufPtr
		// and freegc would be a no-op.)
		//
		// TODO(thepudds): see related TODO in growsliceNoAlias about possibly eliminating
		// the oldPtr != s.array check.
		noscan := !et.Pointers()
		freegc(oldPtr, uintptr(oldCap)*et.Size_, noscan)
	}
	return s
}
