// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

// VisibleFields 返回 t 中所有可见的字段，t 必须是结构体类型。
// 如果一个字段可以通过 FieldByName 调用直接访问，则定义为可见的。
// 返回的字段包括匿名结构体成员内部的字段和未导出的字段。
// 它们遵循与结构体中相同的顺序，匿名字段后面紧跟着它们的提升字段。
//
// 对于返回切片的每个元素 e，可以通过调用 v.FieldByIndex(e.Index)
// 从类型为 t 的值 v 中检索相应的字段。
func VisibleFields(t Type) []StructField {
	if t == nil {
		panic("reflect: VisibleFields(nil)")
	}
	if t.Kind() != Struct {
		panic("reflect.VisibleFields of non-struct type")
	}
	w := &visibleFieldsWalker{
		byName:   make(map[string]int),
		visiting: make(map[Type]bool),
		fields:   make([]StructField, 0, t.NumField()),
		index:    make([]int, 0, 2),
	}
	w.walk(t)
	// 移除所有已被隐藏的字段。
	// 使用原地移除，在没有隐藏字段的常见情况下避免复制。
	j := 0
	for i := range w.fields {
		f := &w.fields[i]
		if f.Name == "" {
			continue
		}
		if i != j {
			// 一个字段已被移除。我们需要将所有后续元素向上移动。
			w.fields[j] = *f
		}
		j++
	}
	return w.fields[:j]
}

type visibleFieldsWalker struct {
	byName   map[string]int
	visiting map[Type]bool
	fields   []StructField
	index    []int
}

// walk 遍历结构体类型 t 中的所有字段，按索引前序访问字段
// 并将它们追加到 w.fields（这保持了所需的顺序）。
// 已被覆盖的字段的 Name 字段会被清空。
func (w *visibleFieldsWalker) walk(t Type) {
	if w.visiting[t] {
		return
	}
	w.visiting[t] = true
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		w.index = append(w.index, i)
		add := true
		if oldIndex, ok := w.byName[f.Name]; ok {
			old := &w.fields[oldIndex]
			if len(w.index) == len(old.Index) {
				// 相同深度的同名字段相互抵消。将字段名设置为空
				// 以表示这种情况已发生，不需要添加此字段。
				old.Name = ""
				add = false
			} else if len(w.index) < len(old.Index) {
				// 旧字段输了，因为它比新字段更深。
				old.Name = ""
			} else {
				// 旧字段赢了，因为它比新字段更浅。
				add = false
			}
		}
		if add {
			// 复制索引，这样它就不会被其他追加操作覆盖。
			f.Index = append([]int(nil), w.index...)
			w.byName[f.Name] = len(w.fields)
			w.fields = append(w.fields, f)
		}
		if f.Anonymous {
			if f.Type.Kind() == Pointer {
				f.Type = f.Type.Elem()
			}
			if f.Type.Kind() == Struct {
				w.walk(f.Type)
			}
		}
		w.index = w.index[:len(w.index)-1]
	}
	delete(w.visiting, t)
}
