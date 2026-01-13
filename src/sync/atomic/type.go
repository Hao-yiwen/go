// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package atomic

import "unsafe"

// Bool 是一个原子布尔值。
// 零值是 false。
//
// Bool 在首次使用后不能被复制。
type Bool struct {
	_ noCopy
	v uint32
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Bool) Load() bool { return LoadUint32(&x.v) != 0 }

// Store 原子地将 val 存储到 x 中。
func (x *Bool) Store(val bool) { StoreUint32(&x.v, b32(val)) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Bool) Swap(new bool) (old bool) { return SwapUint32(&x.v, b32(new)) != 0 }

// CompareAndSwap 对布尔值 x 执行比较并交换操作。
func (x *Bool) CompareAndSwap(old, new bool) (swapped bool) {
	return CompareAndSwapUint32(&x.v, b32(old), b32(new))
}

// b32 返回表示 b 的 uint32 值 0 或 1。
func b32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// 用于测试 *Pointer[T] 的方法可以被内联。
// 与 cmd/compile/internal/test/inl_test.go:TestIntendedInlining 保持同步。
var _ = &Pointer[int]{}

// Pointer 是类型 *T 的原子指针。零值是 nil *T。
//
// Pointer 在首次使用后不能被复制。
type Pointer[T any] struct {
	// 在字段中提及 *T 以禁止 Pointer 类型之间的转换。
	// 有关更多详情，请参见 go.dev/issue/56603。
	// 使用 *T，而不是 T，以避免虚假的递归类型定义错误。
	_ [0]*T

	_ noCopy
	v unsafe.Pointer
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Pointer[T]) Load() *T { return (*T)(LoadPointer(&x.v)) }

// Store 原子地将 val 存储到 x 中。
func (x *Pointer[T]) Store(val *T) { StorePointer(&x.v, unsafe.Pointer(val)) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Pointer[T]) Swap(new *T) (old *T) { return (*T)(SwapPointer(&x.v, unsafe.Pointer(new))) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Pointer[T]) CompareAndSwap(old, new *T) (swapped bool) {
	return CompareAndSwapPointer(&x.v, unsafe.Pointer(old), unsafe.Pointer(new))
}

// Int32 是一个原子 int32。零值是 0。
//
// Int32 在首次使用后不能被复制。
type Int32 struct {
	_ noCopy
	v int32
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Int32) Load() int32 { return LoadInt32(&x.v) }

// Store 原子地将 val 存储到 x 中。
func (x *Int32) Store(val int32) { StoreInt32(&x.v, val) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Int32) Swap(new int32) (old int32) { return SwapInt32(&x.v, new) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Int32) CompareAndSwap(old, new int32) (swapped bool) {
	return CompareAndSwapInt32(&x.v, old, new)
}

// Add 原子地将 delta 加到 x 并返回新值。
func (x *Int32) Add(delta int32) (new int32) { return AddInt32(&x.v, delta) }

// And 使用提供的位掩码对 x 执行原子按位 AND 操作
// 并返回旧值。
func (x *Int32) And(mask int32) (old int32) { return AndInt32(&x.v, mask) }

// Or 使用提供的位掩码对 x 执行原子按位 OR 操作
// 并返回旧值。
func (x *Int32) Or(mask int32) (old int32) { return OrInt32(&x.v, mask) }

// Int64 是一个原子 int64。零值是 0。
//
// Int64 在首次使用后不能被复制。
type Int64 struct {
	_ noCopy
	_ align64
	v int64
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Int64) Load() int64 { return LoadInt64(&x.v) }

// Store 原子地将 val 存储到 x 中。
func (x *Int64) Store(val int64) { StoreInt64(&x.v, val) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Int64) Swap(new int64) (old int64) { return SwapInt64(&x.v, new) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Int64) CompareAndSwap(old, new int64) (swapped bool) {
	return CompareAndSwapInt64(&x.v, old, new)
}

// Add 原子地将 delta 加到 x 并返回新值。
func (x *Int64) Add(delta int64) (new int64) { return AddInt64(&x.v, delta) }

// And 使用提供的位掩码对 x 执行原子按位 AND 操作
// 并返回旧值。
func (x *Int64) And(mask int64) (old int64) { return AndInt64(&x.v, mask) }

// Or 使用提供的位掩码对 x 执行原子按位 OR 操作
// 并返回旧值。
func (x *Int64) Or(mask int64) (old int64) { return OrInt64(&x.v, mask) }

// Uint32 是一个原子 uint32。零值是 0。
//
// Uint32 在首次使用后不能被复制。
type Uint32 struct {
	_ noCopy
	v uint32
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Uint32) Load() uint32 { return LoadUint32(&x.v) }

// Store 原子地将 val 存储到 x 中。
func (x *Uint32) Store(val uint32) { StoreUint32(&x.v, val) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Uint32) Swap(new uint32) (old uint32) { return SwapUint32(&x.v, new) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Uint32) CompareAndSwap(old, new uint32) (swapped bool) {
	return CompareAndSwapUint32(&x.v, old, new)
}

// Add 原子地将 delta 加到 x 并返回新值。
func (x *Uint32) Add(delta uint32) (new uint32) { return AddUint32(&x.v, delta) }

// And 使用提供的位掩码对 x 执行原子按位 AND 操作
// 并返回旧值。
func (x *Uint32) And(mask uint32) (old uint32) { return AndUint32(&x.v, mask) }

// Or 使用提供的位掩码对 x 执行原子按位 OR 操作
// 并返回旧值。
func (x *Uint32) Or(mask uint32) (old uint32) { return OrUint32(&x.v, mask) }

// Uint64 是一个原子 uint64。零值是 0。
//
// Uint64 在首次使用后不能被复制。
type Uint64 struct {
	_ noCopy
	_ align64
	v uint64
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Uint64) Load() uint64 { return LoadUint64(&x.v) }

// Store 原子地将 val 存储到 x 中。
func (x *Uint64) Store(val uint64) { StoreUint64(&x.v, val) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Uint64) Swap(new uint64) (old uint64) { return SwapUint64(&x.v, new) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Uint64) CompareAndSwap(old, new uint64) (swapped bool) {
	return CompareAndSwapUint64(&x.v, old, new)
}

// Add 原子地将 delta 加到 x 并返回新值。
func (x *Uint64) Add(delta uint64) (new uint64) { return AddUint64(&x.v, delta) }

// And 使用提供的位掩码对 x 执行原子按位 AND 操作
// 并返回旧值。
func (x *Uint64) And(mask uint64) (old uint64) { return AndUint64(&x.v, mask) }

// Or 使用提供的位掩码对 x 执行原子按位 OR 操作
// 并返回旧值。
func (x *Uint64) Or(mask uint64) (old uint64) { return OrUint64(&x.v, mask) }

// Uintptr 是一个原子 uintptr。零值是 0。
//
// Uintptr 在首次使用后不能被复制。
type Uintptr struct {
	_ noCopy
	v uintptr
}

// Load 原子地加载并返回存储在 x 中的值。
func (x *Uintptr) Load() uintptr { return LoadUintptr(&x.v) }

// Store 原子地将 val 存储到 x 中。
func (x *Uintptr) Store(val uintptr) { StoreUintptr(&x.v, val) }

// Swap 原子地将 new 存储到 x 中并返回之前的值。
func (x *Uintptr) Swap(new uintptr) (old uintptr) { return SwapUintptr(&x.v, new) }

// CompareAndSwap 对 x 执行比较并交换操作。
func (x *Uintptr) CompareAndSwap(old, new uintptr) (swapped bool) {
	return CompareAndSwapUintptr(&x.v, old, new)
}

// Add 原子地将 delta 加到 x 并返回新值。
func (x *Uintptr) Add(delta uintptr) (new uintptr) { return AddUintptr(&x.v, delta) }

// And 使用提供的位掩码对 x 执行原子按位 AND 操作
// 并返回旧值。
func (x *Uintptr) And(mask uintptr) (old uintptr) { return AndUintptr(&x.v, mask) }

// Or 使用提供的位掩码对 x 执行原子按位 OR 操作
// 并返回旧值。
func (x *Uintptr) Or(mask uintptr) (old uintptr) { return OrUintptr(&x.v, mask) }

// noCopy 可以添加到首次使用后不得被复制的结构体中。
//
// 有关详情，请参见 https://golang.org/issues/8005#issuecomment-190753527。
//
// 注意，由于 Lock 和 Unlock 方法的存在，它不能被嵌入。
type noCopy struct{}

// Lock 是由 `go vet` 的 -copylocks 检查器使用的无操作。
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// align64 可以添加到必须 64 位对齐的结构体中。
// 这个结构体由编译器中的特殊情况识别
// 并且如果复制到任何其他包都不会工作。
type align64 struct{}
