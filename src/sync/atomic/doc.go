// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// atomic 包提供了用于实现同步算法的底层原子内存原语。
//
// 这些函数需要非常小心才能正确使用。
// 除了特殊的底层应用外，同步最好通过 channel 或 [sync] 包的功能来完成。
// 通过通信来共享内存；不要通过共享内存来通信。
//
// 交换操作由 SwapT 函数实现，是以下操作的原子等价物：
//
//	old = *addr
//	*addr = new
//	return old
//
// 比较并交换操作由 CompareAndSwapT 函数实现，是以下操作的原子等价物：
//
//	if *addr == old {
//		*addr = new
//		return true
//	}
//	return false
//
// 加法操作由 AddT 函数实现，是以下操作的原子等价物：
//
//	*addr += delta
//	return *addr
//
// 加载和存储操作由 LoadT 和 StoreT 函数实现，
// 是 "return *addr" 和 "*addr = val" 的原子等价物。
//
// 按照 [Go 内存模型] 的术语，如果原子操作 A 的效果被原子操作 B 观察到，
// 则 A "同步先于" B。
// 此外，程序中执行的所有原子操作表现得好像是按某种顺序一致的顺序执行的。
// 此定义提供了与 C++ 的顺序一致原子操作和 Java 的 volatile 变量相同的语义。
//
// [Go 内存模型]: https://go.dev/ref/mem
package atomic

import (
	"unsafe"
)

// BUG(rsc): 在 386 上，64 位函数使用 Pentium MMX 之前不可用的指令。
//
// 在非 Linux ARM 上，64 位函数使用 ARMv6k 核心之前不可用的指令。
//
// 在 ARM、386 和 32 位 MIPS 上，调用者有责任安排
// 通过原始原子函数原子访问的 64 位字的 64 位对齐
// （[Int64] 和 [Uint64] 类型自动对齐）。
// 已分配结构体、数组或切片中的第一个字；在全局
// 变量中；或在局部变量中（因为在 32 位架构上，
// 64 位原子操作的主体会溢出到堆）可以
// 被确保是 64 位对齐的。

// SwapInt32 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Int32.Swap]。
//
//go:noescape
func SwapInt32(addr *int32, new int32) (old int32)

// SwapUint32 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Uint32.Swap]。
//
//go:noescape
func SwapUint32(addr *uint32, new uint32) (old uint32)

// SwapUintptr 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Uintptr.Swap]。
//
//go:noescape
func SwapUintptr(addr *uintptr, new uintptr) (old uintptr)

// SwapPointer 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Pointer.Swap]。
func SwapPointer(addr *unsafe.Pointer, new unsafe.Pointer) (old unsafe.Pointer)

// CompareAndSwapInt32 对 int32 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Int32.CompareAndSwap]。
//
//go:noescape
func CompareAndSwapInt32(addr *int32, old, new int32) (swapped bool)

// CompareAndSwapUint32 对 uint32 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Uint32.CompareAndSwap]。
//
//go:noescape
func CompareAndSwapUint32(addr *uint32, old, new uint32) (swapped bool)

// CompareAndSwapUintptr 对 uintptr 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Uintptr.CompareAndSwap]。
//
//go:noescape
func CompareAndSwapUintptr(addr *uintptr, old, new uintptr) (swapped bool)

// CompareAndSwapPointer 对 unsafe.Pointer 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Pointer.CompareAndSwap]。
func CompareAndSwapPointer(addr *unsafe.Pointer, old, new unsafe.Pointer) (swapped bool)

// AddInt32 原子地将 delta 加到 *addr 中并返回新值。
// 改用更符合人体工学且不易出错的 [Int32.Add]。
//
//go:noescape
func AddInt32(addr *int32, delta int32) (new int32)

// AddUint32 原子地将 delta 加到 *addr 中并返回新值。
// 要从 x 减去有符号正常量值 c，请执行 AddUint32(&x, ^uint32(c-1))。
// 特别是，要递减 x，请执行 AddUint32(&x, ^uint32(0))。
// 改用更符合人体工学且不易出错的 [Uint32.Add]。
//
//go:noescape
func AddUint32(addr *uint32, delta uint32) (new uint32)

// AddUintptr 原子地将 delta 加到 *addr 中并返回新值。
// 改用更符合人体工学且不易出错的 [Uintptr.Add]。
//
//go:noescape
func AddUintptr(addr *uintptr, delta uintptr) (new uintptr)

// AndInt32 使用提供的位掩码对 *addr 执行原子按位 AND 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Int32.And]。
//
//go:noescape
func AndInt32(addr *int32, mask int32) (old int32)

// AndUint32 使用提供的位掩码对 *addr 执行原子按位 AND 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uint32.And]。
//
//go:noescape
func AndUint32(addr *uint32, mask uint32) (old uint32)

// AndUintptr 使用提供的位掩码对 *addr 执行原子按位 AND 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uintptr.And]。
//
//go:noescape
func AndUintptr(addr *uintptr, mask uintptr) (old uintptr)

// OrInt32 使用提供的位掩码对 *addr 执行原子按位 OR 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Int32.Or]。
//
//go:noescape
func OrInt32(addr *int32, mask int32) (old int32)

// OrUint32 使用提供的位掩码对 *addr 执行原子按位 OR 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uint32.Or]。
//
//go:noescape
func OrUint32(addr *uint32, mask uint32) (old uint32)

// OrUintptr 使用提供的位掩码对 *addr 执行原子按位 OR 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uintptr.Or]。
//
//go:noescape
func OrUintptr(addr *uintptr, mask uintptr) (old uintptr)

// LoadInt32 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Int32.Load]。
//
//go:noescape
func LoadInt32(addr *int32) (val int32)

// LoadUint32 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Uint32.Load]。
//
//go:noescape
func LoadUint32(addr *uint32) (val uint32)

// LoadUintptr 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Uintptr.Load]。
//
//go:noescape
func LoadUintptr(addr *uintptr) (val uintptr)

// LoadPointer 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Pointer.Load]。
func LoadPointer(addr *unsafe.Pointer) (val unsafe.Pointer)

// StoreInt32 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Int32.Store]。
//
//go:noescape
func StoreInt32(addr *int32, val int32)

// StoreUint32 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Uint32.Store]。
//
//go:noescape
func StoreUint32(addr *uint32, val uint32)

// StoreUintptr 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Uintptr.Store]。
//
//go:noescape
func StoreUintptr(addr *uintptr, val uintptr)

// StorePointer 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Pointer.Store]。
func StorePointer(addr *unsafe.Pointer, val unsafe.Pointer)
