// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unique

import (
	"internal/abi"
	isync "internal/sync"
	"unsafe"
)

var zero uintptr

// Handle 是类型 T 的某个值的全局唯一标识。
//
// 两个句柄相等当且仅当用于创建句柄的两个值也相等。
// 两个句柄的比较是简单的，通常比比较用于创建它们的值更高效。
type Handle[T comparable] struct {
	value *T
}

// Value 返回生成该 Handle 的 T 值的浅拷贝。
// Value 可以被多个 goroutine 安全地并发使用。
func (h Handle[T]) Value() T {
	return *h.value
}

// Make 返回类型 T 值的全局唯一句柄。
// 句柄相等当且仅当用于生成它们的值相等。
// Make 可以被多个 goroutine 安全地并发使用。
func Make[T comparable](value T) Handle[T] {
	// 查找类型 T 的映射。
	typ := abi.TypeFor[T]()
	if typ.Size() == 0 {
		return Handle[T]{(*T)(unsafe.Pointer(&zero))}
	}
	ma, ok := uniqueMaps.Load(typ)
	if !ok {
		m := &uniqueMap[T]{canonMap: newCanonMap[T](), cloneSeq: makeCloneSeq(typ)}
		ma, _ = uniqueMaps.LoadOrStore(typ, m)
	}
	m := ma.(*uniqueMap[T])

	// 在映射中查找值。
	ptr := m.Load(value)
	if ptr == nil {
		// 将新值插入映射。
		ptr = m.LoadOrStore(clone(value, &m.cloneSeq))
	}
	return Handle[T]{ptr}
}

// uniqueMaps 是用于 unique.Make 的类型特定并发映射的索引。
//
// 两级映射乍一看可能很奇怪，因为 HashTrieMap 可以将 "any" 作为其键类型，
// 但问题在于逃逸分析。我们不想强制查找使参数逃逸，使用类型特定的映射
// 可以让我们在可能的情况下避免这种情况（例如，对于字符串和普通数据结构体）。
// 我们还获得了不将每种不同类型都塞入单个映射的好处，
// 但这当然不足以抵消两次映射查找的成本。然而，值得的是节省这些分配。
var uniqueMaps isync.HashTrieMap[*abi.Type, any] // any 始终是 *uniqueMap[T]。

type uniqueMap[T comparable] struct {
	*canonMap[T]
	cloneSeq
}
