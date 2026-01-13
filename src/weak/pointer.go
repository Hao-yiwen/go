// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package weak

import (
	"internal/abi"
	"runtime"
	"unsafe"
)

// Pointer 是指向类型 T 值的弱指针。
//
// 与普通指针一样，Pointer 可以引用对象的任何部分，
// 例如结构体的字段或数组的元素。
// 仅被弱指针指向的对象不被视为可达，一旦对象变得不可达，
// [Pointer.Value] 可能返回 nil。
//
// 弱指针的主要用例是实现缓存、规范化映射（如 unique 包）
// 以及将不同值的生命周期绑定在一起（例如，通过具有弱键的映射）。
//
// 两个 Pointer 值相等当且仅当创建它们的指针相等。
// 即使在用于创建弱引用的指针所引用的对象被回收后，
// 此属性仍然保持。
// 如果多个弱指针指向同一对象内的不同偏移量
// （例如，指向同一结构体的不同字段的指针），
// 这些指针将不相等。
// 换句话说，弱指针映射到对象和对象内的偏移量，而不是普通地址。
// 如果从一个变得不可达的对象创建了弱指针，但该对象随后
// 因终结器而复活，那么该弱指针将不等于复活后创建的弱指针。
//
// 使用 nil 指针调用 [Make] 会返回一个 [Pointer.Value] 始终返回 nil 的弱指针。
// Pointer 的零值表现得如同它是通过将 nil 传递给 [Make] 创建的，
// 并与此类指针相等。
//
// 不保证 [Pointer.Value] 最终会返回 nil。
// 一旦对象变得不可达，[Pointer.Value] 可能立即返回 nil。
// 存储在全局变量中的值，或者可以通过从全局变量追踪指针找到的值，
// 是可达的。函数参数或接收者可能在函数最后一次提及它的位置变得不可达。
// 为确保 [Pointer.Value] 不返回 nil，
// 请在对象必须保持可达的最后一个位置之后，
// 将指向该对象的指针传递给 [runtime.KeepAlive] 函数。
//
// 注意，由于不保证 [Pointer.Value] 最终会返回 nil，
// 即使对象不再被引用，运行时也允许执行一种节省空间的优化，
// 将对象批量放入单个分配槽中。如果未引用对象的弱指针
// 始终与被引用对象在同一批次中，则该弱指针可能永远不会变为 nil。
// 通常，这种批处理只发生在微小的（大约 16 字节或更少）且无指针的对象上。
type Pointer[T any] struct {
	// 在类型定义中提及 T 以防止 Pointer 类型之间的转换，
	// 就像我们对 sync/atomic.Pointer 所做的那样。
	_ [0]*T
	u unsafe.Pointer
}

// Make 从指向某个类型 T 值的指针创建弱指针。
func Make[T any](ptr *T) Pointer[T] {
	// 显式强制 ptr 逃逸到堆上。
	ptr = abi.Escape(ptr)

	var u unsafe.Pointer
	if ptr != nil {
		u = runtime_registerWeakPointer(unsafe.Pointer(ptr))
	}
	runtime.KeepAlive(ptr)
	return Pointer[T]{u: u}
}

// Value 返回用于创建弱指针的原始指针。
// 如果原始指针指向的值已被垃圾收集器回收，则返回 nil。
// 如果弱指针指向一个带有终结器的对象，那么一旦该对象的终结器
// 被排入执行队列，Value 就会返回 nil。
func (p Pointer[T]) Value() *T {
	if p.u == nil {
		return nil
	}
	return (*T)(runtime_makeStrongFromWeak(p.u))
}

// 在 runtime 中实现。

//go:linkname runtime_registerWeakPointer
func runtime_registerWeakPointer(unsafe.Pointer) unsafe.Pointer

//go:linkname runtime_makeStrongFromWeak
func runtime_makeStrongFromWeak(unsafe.Pointer) unsafe.Pointer
