// 版权所有 2014 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package atomic

import (
	"unsafe"
)

// Value 提供了对一致类型的值的原子加载和存储。
// Value 的零值从 [Value.Load] 返回 nil。
// 一旦调用了 [Value.Store]，Value 就不能被复制。
//
// Value 在首次使用后不能被复制。
type Value struct {
	v any
}

// efaceWords 是 interface{} 的内部表示。
type efaceWords struct {
	typ  unsafe.Pointer
	data unsafe.Pointer
}

// Load 返回由最近一次 Store 设置的值。
// 如果没有调用过 Store，则返回 nil。
func (v *Value) Load() (val any) {
	vp := (*efaceWords)(unsafe.Pointer(v))
	typ := LoadPointer(&vp.typ)
	if typ == nil || typ == unsafe.Pointer(&firstStoreInProgress) {
		// 第一次存储还未完成。
		return nil
	}
	data := LoadPointer(&vp.data)
	vlp := (*efaceWords)(unsafe.Pointer(&val))
	vlp.typ = typ
	vlp.data = data
	return
}

var firstStoreInProgress byte

// Store 将 [Value] v 的值设置为 val。
// 对给定 Value 的所有 Store 调用必须使用相同具体类型的值。
// 不同类型的 Store 会导致 panic，Store(nil) 也会。
func (v *Value) Store(val any) {
	if val == nil {
		panic("sync/atomic: store of nil value into Value")
	}
	vp := (*efaceWords)(unsafe.Pointer(v))
	vlp := (*efaceWords)(unsafe.Pointer(&val))
	for {
		typ := LoadPointer(&vp.typ)
		if typ == nil {
			// 尝试开始第一次存储。
			// 禁用抢占，以便其他 goroutine 可以使用
			// 主动自旋等待来等待完成。
			runtime_procPin()
			if !CompareAndSwapPointer(&vp.typ, nil, unsafe.Pointer(&firstStoreInProgress)) {
				runtime_procUnpin()
				continue
			}
			// 完成第一次存储。
			StorePointer(&vp.data, vlp.data)
			StorePointer(&vp.typ, vlp.typ)
			runtime_procUnpin()
			return
		}
		if typ == unsafe.Pointer(&firstStoreInProgress) {
			// 第一次存储进行中。等待。
			// 由于我们在第一次存储周围禁用了抢占，
			// 我们可以通过主动自旋来等待。
			continue
		}
		// 第一次存储完成。检查类型并覆盖数据。
		if typ != vlp.typ {
			panic("sync/atomic: store of inconsistently typed value into Value")
		}
		StorePointer(&vp.data, vlp.data)
		return
	}
}

// Swap 将 new 存储到 Value 中并返回之前的值。如果 Value 为空则返回 nil。
//
// 对给定 Value 的所有 Swap 调用必须使用相同具体类型的值。
// 不同类型的 Swap 会导致 panic，Swap(nil) 也会。
func (v *Value) Swap(new any) (old any) {
	if new == nil {
		panic("sync/atomic: swap of nil value into Value")
	}
	vp := (*efaceWords)(unsafe.Pointer(v))
	np := (*efaceWords)(unsafe.Pointer(&new))
	for {
		typ := LoadPointer(&vp.typ)
		if typ == nil {
			// 尝试开始第一次存储。
			// 禁用抢占，以便其他 goroutine 可以使用
			// 主动自旋等待来等待完成。
			runtime_procPin()
			if !CompareAndSwapPointer(&vp.typ, nil, unsafe.Pointer(&firstStoreInProgress)) {
				runtime_procUnpin()
				continue
			}
			// 完成第一次存储。
			StorePointer(&vp.data, np.data)
			StorePointer(&vp.typ, np.typ)
			runtime_procUnpin()
			return nil
		}
		if typ == unsafe.Pointer(&firstStoreInProgress) {
			// 第一次存储进行中。等待。
			// 由于我们在第一次存储周围禁用了抢占，
			// 我们可以通过主动自旋来等待。
			continue
		}
		// 第一次存储完成。检查类型并覆盖数据。
		if typ != np.typ {
			panic("sync/atomic: swap of inconsistently typed value into Value")
		}
		op := (*efaceWords)(unsafe.Pointer(&old))
		op.typ, op.data = np.typ, SwapPointer(&vp.data, np.data)
		return old
	}
}

// CompareAndSwap 对 [Value] 执行比较并交换操作。
//
// 对给定 Value 的所有 CompareAndSwap 调用必须使用相同具体类型的值。
// 不同类型的 CompareAndSwap 会导致 panic，CompareAndSwap(old, nil) 也会。
func (v *Value) CompareAndSwap(old, new any) (swapped bool) {
	if new == nil {
		panic("sync/atomic: compare and swap of nil value into Value")
	}
	vp := (*efaceWords)(unsafe.Pointer(v))
	np := (*efaceWords)(unsafe.Pointer(&new))
	op := (*efaceWords)(unsafe.Pointer(&old))
	if op.typ != nil && np.typ != op.typ {
		panic("sync/atomic: compare and swap of inconsistently typed values")
	}
	for {
		typ := LoadPointer(&vp.typ)
		if typ == nil {
			if old != nil {
				return false
			}
			// 尝试开始第一次存储。
			// 禁用抢占，以便其他 goroutine 可以使用
			// 主动自旋等待来等待完成。
			runtime_procPin()
			if !CompareAndSwapPointer(&vp.typ, nil, unsafe.Pointer(&firstStoreInProgress)) {
				runtime_procUnpin()
				continue
			}
			// 完成第一次存储。
			StorePointer(&vp.data, np.data)
			StorePointer(&vp.typ, np.typ)
			runtime_procUnpin()
			return true
		}
		if typ == unsafe.Pointer(&firstStoreInProgress) {
			// 第一次存储进行中。等待。
			// 由于我们在第一次存储周围禁用了抢占，
			// 我们可以通过主动自旋来等待。
			continue
		}
		// 第一次存储完成。检查类型并覆盖数据。
		if typ != np.typ {
			panic("sync/atomic: compare and swap of inconsistently typed value into Value")
		}
		// 通过运行时相等性检查来比较旧值和当前值。
		// 这允许比较值类型，这是
		// 包函数不提供的功能。
		// 下面的 CompareAndSwapPointer 只能确保 vp.data
		// 自 LoadPointer 以来没有改变。
		data := LoadPointer(&vp.data)
		var i any
		(*efaceWords)(unsafe.Pointer(&i)).typ = typ
		(*efaceWords)(unsafe.Pointer(&i)).data = data
		if i != old {
			return false
		}
		return CompareAndSwapPointer(&vp.data, data, np.data)
	}
}

// 禁用/启用抢占，在运行时中实现。
func runtime_procPin() int
func runtime_procUnpin()
