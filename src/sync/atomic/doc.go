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

// BUG(rsc): On 386, the 64-bit functions use instructions unavailable before the Pentium MMX.
//
// On non-Linux ARM, the 64-bit functions use instructions unavailable before the ARMv6k core.
//
// On ARM, 386, and 32-bit MIPS, it is the caller's responsibility to arrange
// for 64-bit alignment of 64-bit words accessed atomically via the primitive
// atomic functions (types [Int64] and [Uint64] are automatically aligned).
// The first word in an allocated struct, array, or slice; in a global
// variable; or in a local variable (because on 32-bit architectures, the
// subject of 64-bit atomic operations will escape to the heap) can be
// relied upon to be 64-bit aligned.

// SwapInt32 atomically stores new into *addr and returns the previous *addr value.
// Consider using the more ergonomic and less error-prone [Int32.Swap] instead.
//
//go:noescape
func SwapInt32(addr *int32, new int32) (old int32)

// SwapUint32 atomically stores new into *addr and returns the previous *addr value.
// Consider using the more ergonomic and less error-prone [Uint32.Swap] instead.
//
//go:noescape
func SwapUint32(addr *uint32, new uint32) (old uint32)

// SwapUintptr atomically stores new into *addr and returns the previous *addr value.
// Consider using the more ergonomic and less error-prone [Uintptr.Swap] instead.
//
//go:noescape
func SwapUintptr(addr *uintptr, new uintptr) (old uintptr)

// SwapPointer atomically stores new into *addr and returns the previous *addr value.
// Consider using the more ergonomic and less error-prone [Pointer.Swap] instead.
func SwapPointer(addr *unsafe.Pointer, new unsafe.Pointer) (old unsafe.Pointer)

// CompareAndSwapInt32 executes the compare-and-swap operation for an int32 value.
// Consider using the more ergonomic and less error-prone [Int32.CompareAndSwap] instead.
//
//go:noescape
func CompareAndSwapInt32(addr *int32, old, new int32) (swapped bool)

// CompareAndSwapUint32 executes the compare-and-swap operation for a uint32 value.
// Consider using the more ergonomic and less error-prone [Uint32.CompareAndSwap] instead.
//
//go:noescape
func CompareAndSwapUint32(addr *uint32, old, new uint32) (swapped bool)

// CompareAndSwapUintptr executes the compare-and-swap operation for a uintptr value.
// Consider using the more ergonomic and less error-prone [Uintptr.CompareAndSwap] instead.
//
//go:noescape
func CompareAndSwapUintptr(addr *uintptr, old, new uintptr) (swapped bool)

// CompareAndSwapPointer executes the compare-and-swap operation for a unsafe.Pointer value.
// Consider using the more ergonomic and less error-prone [Pointer.CompareAndSwap] instead.
func CompareAndSwapPointer(addr *unsafe.Pointer, old, new unsafe.Pointer) (swapped bool)

// AddInt32 atomically adds delta to *addr and returns the new value.
// Consider using the more ergonomic and less error-prone [Int32.Add] instead.
//
//go:noescape
func AddInt32(addr *int32, delta int32) (new int32)

// AddUint32 atomically adds delta to *addr and returns the new value.
// To subtract a signed positive constant value c from x, do AddUint32(&x, ^uint32(c-1)).
// In particular, to decrement x, do AddUint32(&x, ^uint32(0)).
// Consider using the more ergonomic and less error-prone [Uint32.Add] instead.
//
//go:noescape
func AddUint32(addr *uint32, delta uint32) (new uint32)

// AddUintptr atomically adds delta to *addr and returns the new value.
// Consider using the more ergonomic and less error-prone [Uintptr.Add] instead.
//
//go:noescape
func AddUintptr(addr *uintptr, delta uintptr) (new uintptr)

// AndInt32 atomically performs a bitwise AND operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Int32.And] instead.
//
//go:noescape
func AndInt32(addr *int32, mask int32) (old int32)

// AndUint32 atomically performs a bitwise AND operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Uint32.And] instead.
//
//go:noescape
func AndUint32(addr *uint32, mask uint32) (old uint32)

// AndUintptr atomically performs a bitwise AND operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Uintptr.And] instead.
//
//go:noescape
func AndUintptr(addr *uintptr, mask uintptr) (old uintptr)

// OrInt32 atomically performs a bitwise OR operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Int32.Or] instead.
//
//go:noescape
func OrInt32(addr *int32, mask int32) (old int32)

// OrUint32 atomically performs a bitwise OR operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Uint32.Or] instead.
//
//go:noescape
func OrUint32(addr *uint32, mask uint32) (old uint32)

// OrUintptr atomically performs a bitwise OR operation on *addr using the bitmask provided as mask
// and returns the old value.
// Consider using the more ergonomic and less error-prone [Uintptr.Or] instead.
//
//go:noescape
func OrUintptr(addr *uintptr, mask uintptr) (old uintptr)

// LoadInt32 atomically loads *addr.
// Consider using the more ergonomic and less error-prone [Int32.Load] instead.
//
//go:noescape
func LoadInt32(addr *int32) (val int32)

// LoadUint32 atomically loads *addr.
// Consider using the more ergonomic and less error-prone [Uint32.Load] instead.
//
//go:noescape
func LoadUint32(addr *uint32) (val uint32)

// LoadUintptr atomically loads *addr.
// Consider using the more ergonomic and less error-prone [Uintptr.Load] instead.
//
//go:noescape
func LoadUintptr(addr *uintptr) (val uintptr)

// LoadPointer atomically loads *addr.
// Consider using the more ergonomic and less error-prone [Pointer.Load] instead.
func LoadPointer(addr *unsafe.Pointer) (val unsafe.Pointer)

// StoreInt32 atomically stores val into *addr.
// Consider using the more ergonomic and less error-prone [Int32.Store] instead.
//
//go:noescape
func StoreInt32(addr *int32, val int32)

// StoreUint32 atomically stores val into *addr.
// Consider using the more ergonomic and less error-prone [Uint32.Store] instead.
//
//go:noescape
func StoreUint32(addr *uint32, val uint32)

// StoreUintptr atomically stores val into *addr.
// Consider using the more ergonomic and less error-prone [Uintptr.Store] instead.
//
//go:noescape
func StoreUintptr(addr *uintptr, val uintptr)

// StorePointer atomically stores val into *addr.
// Consider using the more ergonomic and less error-prone [Pointer.Store] instead.
func StorePointer(addr *unsafe.Pointer, val unsafe.Pointer)
