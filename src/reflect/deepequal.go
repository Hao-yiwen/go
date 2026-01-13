// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 通过反射进行深度相等测试

package reflect

import (
	"internal/bytealg"
	"unsafe"
)

// 在 deepValueEqual 期间，必须跟踪正在进行的检查。
// 比较算法假设当再次遇到正在进行的检查时，它们都为真。
// 已访问的比较存储在以 visit 为索引的 map 中。
type visit struct {
	a1  unsafe.Pointer
	a2  unsafe.Pointer
	typ Type
}

// 使用反射类型测试深度相等。map 参数跟踪已经见过的比较，
// 这允许在递归类型上进行短路处理。
func deepValueEqual(v1, v2 Value, visited map[visit]bool) bool {
	if !v1.IsValid() || !v2.IsValid() {
		return v1.IsValid() == v2.IsValid()
	}
	if v1.Type() != v2.Type() {
		return false
	}

	// 我们希望避免在 visited map 中放入超过需要的内容。
	// 对于可能遇到的任何引用循环，hard(v1, v2) 需要对循环中的
	// 至少一种类型返回 true，并且获取 Value 的内部指针是安全有效的。
	hard := func(v1, v2 Value) bool {
		switch v1.Kind() {
		case Pointer:
			if !v1.typ().Pointers() {
				// 不在堆中的指针不可能是循环的。
				// 至少，我们当前对 internal/runtime/sys.NotInHeap 的所有使用
				// 都具有这个特性。运行时的不是循环的（而且我们也不会对它们
				// 使用 DeepEqual），cgo 生成的都是空结构体。
				return false
			}
			fallthrough
		case Map, Slice, Interface:
			// nil 指针不可能是循环的。避免将它们放入 visited map。
			return !v1.IsNil() && !v2.IsNil()
		}
		return false
	}

	if hard(v1, v2) {
		// 对于 Pointer 或 Map 值，我们需要检查 flagIndir，
		// 我们通过调用 pointer 方法来做到这一点。
		// 对于 Slice 或 Interface，flagIndir 始终被设置，
		// 使用 v.ptr 就足够了。
		ptrval := func(v Value) unsafe.Pointer {
			switch v.Kind() {
			case Pointer, Map:
				return v.pointer()
			default:
				return v.ptr
			}
		}
		addr1 := ptrval(v1)
		addr2 := ptrval(v2)
		if uintptr(addr1) > uintptr(addr2) {
			// 规范化顺序以减少 visited 中的条目数量。
			// 假设是非移动的垃圾收集器。
			addr1, addr2 = addr2, addr1
		}

		// 如果引用已经被看到过，则短路。
		typ := v1.Type()
		v := visit{addr1, addr2, typ}
		if visited[v] {
			return true
		}

		// 记住以备后用。
		visited[v] = true
	}

	switch v1.Kind() {
	case Array:
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case Slice:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		// []byte 的特殊情况，这很常见。
		if v1.Type().Elem().Kind() == Uint8 {
			return bytealg.Equal(v1.Bytes(), v2.Bytes())
		}
		for i := 0; i < v1.Len(); i++ {
			if !deepValueEqual(v1.Index(i), v2.Index(i), visited) {
				return false
			}
		}
		return true
	case Interface:
		if v1.IsNil() || v2.IsNil() {
			return v1.IsNil() == v2.IsNil()
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case Pointer:
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		return deepValueEqual(v1.Elem(), v2.Elem(), visited)
	case Struct:
		for i, n := 0, v1.NumField(); i < n; i++ {
			if !deepValueEqual(v1.Field(i), v2.Field(i), visited) {
				return false
			}
		}
		return true
	case Map:
		if v1.IsNil() != v2.IsNil() {
			return false
		}
		if v1.Len() != v2.Len() {
			return false
		}
		if v1.UnsafePointer() == v2.UnsafePointer() {
			return true
		}
		iter := v1.MapRange()
		for iter.Next() {
			val1 := iter.Value()
			val2 := v2.MapIndex(iter.Key())
			if !val1.IsValid() || !val2.IsValid() || !deepValueEqual(val1, val2, visited) {
				return false
			}
		}
		return true
	case Func:
		if v1.IsNil() && v2.IsNil() {
			return true
		}
		// 没法做得更好了：
		return false
	case Int, Int8, Int16, Int32, Int64:
		return v1.Int() == v2.Int()
	case Uint, Uint8, Uint16, Uint32, Uint64, Uintptr:
		return v1.Uint() == v2.Uint()
	case String:
		return v1.String() == v2.String()
	case Bool:
		return v1.Bool() == v2.Bool()
	case Float32, Float64:
		return v1.Float() == v2.Float()
	case Complex64, Complex128:
		return v1.Complex() == v2.Complex()
	default:
		// 普通的相等比较就足够了
		return valueInterface(v1, false) == valueInterface(v2, false)
	}
}

// DeepEqual 报告 x 和 y 是否"深度相等"，定义如下。
// 如果以下情况之一适用，则两个相同类型的值深度相等。
// 不同类型的值永远不会深度相等。
//
// 当数组值的对应元素深度相等时，数组值深度相等。
//
// 如果结构体值的对应字段（包括导出和未导出的）深度相等，则结构体值深度相等。
//
// 如果两个函数值都是 nil，则它们深度相等；否则它们不深度相等。
//
// 如果接口值持有深度相等的具体值，则接口值深度相等。
//
// 当以下所有条件都为真时，map 值深度相等：
// 它们都是 nil 或都不是 nil，它们具有相同的长度，
// 并且它们是同一个 map 对象或它们的对应键（使用 Go 相等性匹配）
// 映射到深度相等的值。
//
// 如果指针值使用 Go 的 == 运算符相等，或者它们指向深度相等的值，
// 则指针值深度相等。
//
// 当以下所有条件都为真时，切片值深度相等：
// 它们都是 nil 或都不是 nil，它们具有相同的长度，
// 并且它们指向同一底层数组的同一初始条目（即 &x[0] == &y[0]）
// 或它们的对应元素（直到长度）深度相等。
// 注意，非 nil 的空切片和 nil 切片（例如 []byte{} 和 []byte(nil)）
// 不是深度相等的。
//
// 其他值 - 数字、布尔值、字符串和通道 - 如果使用 Go 的 == 运算符相等，
// 则它们深度相等。
//
// 一般来说，DeepEqual 是 Go 的 == 运算符的递归放松。
// 然而，这个想法不可能在没有一些不一致的情况下实现。
// 具体来说，一个值可能与自身不相等，
// 要么因为它是 func 类型（通常不可比较），
// 要么因为它是浮点 NaN 值（在浮点比较中不等于自身），
// 要么因为它是包含这样值的数组、结构体或接口。
// 另一方面，指针值始终等于自身，
// 即使它们指向或包含这样有问题的值，
// 因为它们使用 Go 的 == 运算符比较相等，这是深度相等的充分条件，
// 与内容无关。
// DeepEqual 被定义为使得相同的快捷方式适用于切片和 map：
// 如果 x 和 y 是同一个切片或同一个 map，
// 则无论内容如何，它们都深度相等。
//
// 当 DeepEqual 遍历数据值时，它可能会发现循环。
// 第二次及后续 DeepEqual 比较之前已经比较过的两个指针值时，
// 它将这些值视为相等，而不是检查它们所指向的值。
// 这确保了 DeepEqual 会终止。
func DeepEqual(x, y any) bool {
	if x == nil || y == nil {
		return x == y
	}
	v1 := ValueOf(x)
	v2 := ValueOf(y)
	if v1.Type() != v2.Type() {
		return false
	}
	return deepValueEqual(v1, v2, make(map[visit]bool))
}
