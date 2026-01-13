// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package reflect

import (
	"internal/abi"
	"internal/race"
	"internal/runtime/maps"
	"internal/runtime/sys"
	"unsafe"
)

func (t *rtype) Key() Type {
	if t.Kind() != Map {
		panic("reflect: Key of non-map type " + t.String())
	}
	tt := (*abi.MapType)(unsafe.Pointer(t))
	return toType(tt.Key)
}

// MapOf 返回具有给定键和元素类型的 map 类型。
// 例如，如果 k 表示 int，e 表示 string，
// MapOf(k, e) 表示 map[int]string。
//
// 如果键类型不是有效的 map 键类型（即，如果它没有
// 实现 Go 的 == 运算符），MapOf 会 panic。
func MapOf(key, elem Type) Type {
	ktyp := key.common()
	etyp := elem.common()

	if ktyp.Equal == nil {
		panic("reflect.MapOf: invalid key type " + stringFor(ktyp))
	}

	// 在缓存中查找。
	ckey := cacheKey{Map, ktyp, etyp, 0}
	if mt, ok := lookupCache.Load(ckey); ok {
		return mt.(Type)
	}

	// 在已知类型中查找。
	s := "map[" + stringFor(ktyp) + "]" + stringFor(etyp)
	for _, tt := range typesByString(s) {
		mt := (*abi.MapType)(unsafe.Pointer(tt))
		if mt.Key == ktyp && mt.Elem == etyp {
			ti, _ := lookupCache.LoadOrStore(ckey, toRType(tt))
			return ti.(Type)
		}
	}

	group, slot := groupAndSlotOf(key, elem)

	// 创建一个 map 类型。
	// 注意：flag 值必须与 ../cmd/compile/internal/reflectdata/reflect.go:writeType
	// 中 TMAP 情况使用的值匹配。
	var imap any = (map[unsafe.Pointer]unsafe.Pointer)(nil)
	mt := **(**abi.MapType)(unsafe.Pointer(&imap))
	mt.Str = resolveReflectName(newName(s, "", false, false))
	mt.TFlag = abi.TFlagDirectIface
	mt.Hash = fnv1(etyp.Hash, 'm', byte(ktyp.Hash>>24), byte(ktyp.Hash>>16), byte(ktyp.Hash>>8), byte(ktyp.Hash))
	mt.Key = ktyp
	mt.Elem = etyp
	mt.Group = group.common()
	mt.Hasher = func(p unsafe.Pointer, seed uintptr) uintptr {
		return typehash(ktyp, p, seed)
	}
	mt.GroupSize = mt.Group.Size()
	mt.SlotSize = slot.Size()
	mt.ElemOff = slot.Field(1).Offset
	mt.Flags = 0
	if needKeyUpdate(ktyp) {
		mt.Flags |= abi.MapNeedKeyUpdate
	}
	if hashMightPanic(ktyp) {
		mt.Flags |= abi.MapHashMightPanic
	}
	if ktyp.Size_ > abi.MapMaxKeyBytes {
		mt.Flags |= abi.MapIndirectKey
	}
	if etyp.Size_ > abi.MapMaxKeyBytes {
		mt.Flags |= abi.MapIndirectElem
	}
	mt.PtrToThis = 0

	ti, _ := lookupCache.LoadOrStore(ckey, toRType(&mt.Type))
	return ti.(Type)
}

func groupAndSlotOf(ktyp, etyp Type) (Type, Type) {
	// type group struct {
	//     ctrl uint64
	//     slots [abi.MapGroupSlots]struct {
	//         key  keyType
	//         elem elemType
	//     }
	// }
	// （保留原始注释结构以说明 group 类型的布局）

	if ktyp.Size() > abi.MapMaxKeyBytes {
		ktyp = PointerTo(ktyp)
	}
	if etyp.Size() > abi.MapMaxElemBytes {
		etyp = PointerTo(etyp)
	}

	fields := []StructField{
		{
			Name: "Key",
			Type: ktyp,
		},
		{
			Name: "Elem",
			Type: etyp,
		},
	}
	slot := StructOf(fields)

	fields = []StructField{
		{
			Name: "Ctrl",
			Type: TypeFor[uint64](),
		},
		{
			Name: "Slots",
			Type: ArrayOf(abi.MapGroupSlots, slot),
		},
	}
	group := StructOf(fields)
	return group, slot
}

var stringType = rtypeOf("")

// MapIndex 返回 map v 中与 key 关联的值。
// 如果 v 的 Kind 不是 [Map]，它会 panic。
// 如果在 map 中找不到 key 或者 v 表示一个 nil map，它返回零值 Value。
// 与 Go 中一样，key 的值必须可赋值给 map 的键类型。
func (v Value) MapIndex(key Value) Value {
	v.mustBe(Map)
	tt := (*abi.MapType)(unsafe.Pointer(v.typ()))

	// 不要求 key 是导出的，这样 DeepEqual 和其他程序可以使用
	// MapKeys 返回的所有键作为 MapIndex 的参数。但是，如果 map
	// 或 key 是未导出的，结果将被视为未导出。这与结构体的行为
	// 一致，结构体允许读取但不允许写入未导出的字段。

	var e unsafe.Pointer
	if (tt.Key == stringType || key.kind() == String) && tt.Key == key.typ() && tt.Elem.Size() <= abi.MapMaxElemBytes {
		k := *(*string)(key.ptr)
		e = mapaccess_faststr(v.typ(), v.pointer(), k)
	} else {
		key = key.assignTo("reflect.Value.MapIndex", tt.Key, nil)
		var k unsafe.Pointer
		if key.flag&flagIndir != 0 {
			k = key.ptr
		} else {
			k = unsafe.Pointer(&key.ptr)
		}
		e = mapaccess(v.typ(), v.pointer(), k)
	}
	if e == nil {
		return Value{}
	}
	typ := tt.Elem
	fl := (v.flag | key.flag).ro()
	fl |= flag(typ.Kind())
	return copyVal(typ, fl, e)
}

// 等同于 runtime.mapIterStart。
//
//go:noinline
func mapIterStart(t *abi.MapType, m *maps.Map, it *maps.Iter) {
	if race.Enabled && m != nil {
		callerpc := sys.GetCallerPC()
		race.ReadPC(unsafe.Pointer(m), callerpc, abi.FuncPCABIInternal(mapIterStart))
	}

	it.Init(t, m)
	it.Next()
}

// 等同于 runtime.mapIterNext。
//
//go:noinline
func mapIterNext(it *maps.Iter) {
	if race.Enabled {
		callerpc := sys.GetCallerPC()
		race.ReadPC(unsafe.Pointer(it.Map()), callerpc, abi.FuncPCABIInternal(mapIterNext))
	}

	it.Next()
}

// MapKeys 返回包含 map 中所有键的切片，顺序不确定。
// 如果 v 的 Kind 不是 [Map]，它会 panic。
// 如果 v 表示一个 nil map，它返回空切片。
func (v Value) MapKeys() []Value {
	v.mustBe(Map)
	tt := (*abi.MapType)(unsafe.Pointer(v.typ()))
	keyType := tt.Key

	fl := v.flag.ro() | flag(keyType.Kind())

	// 逃逸分析看不出 map 不会逃逸。它看到从 maps.IterStart 逃逸，
	// 通过赋值进入它，尽管它并没有逃逸这个函数。
	mptr := abi.NoEscape(v.pointer())
	m := (*maps.Map)(mptr)
	mlen := int(0)
	if m != nil {
		mlen = maplen(mptr)
	}
	var it maps.Iter
	mapIterStart(tt, m, &it)
	a := make([]Value, mlen)
	var i int
	for i = 0; i < len(a); i++ {
		key := it.Key()
		if key == nil {
			// 自从我们上面调用 maplen 以来，有人从 map 中删除了一个条目。
			// 这是一个数据竞争，但我们对此无能为力。
			break
		}
		a[i] = copyVal(keyType, fl, key)
		mapIterNext(&it)
	}
	return a[:i]
}

// MapIter 是用于遍历 map 的迭代器。
// 参见 [Value.MapRange]。
type MapIter struct {
	m     Value
	hiter maps.Iter
}

// Key 返回 iter 当前 map 条目的键。
func (iter *MapIter) Key() Value {
	if !iter.hiter.Initialized() {
		panic("MapIter.Key called before Next")
	}
	iterkey := iter.hiter.Key()
	if iterkey == nil {
		panic("MapIter.Key called on exhausted iterator")
	}

	t := (*abi.MapType)(unsafe.Pointer(iter.m.typ()))
	ktype := t.Key
	return copyVal(ktype, iter.m.flag.ro()|flag(ktype.Kind()), iterkey)
}

// SetIterKey 将 iter 当前 map 条目的键赋值给 v。
// 它等同于 v.Set(iter.Key())，但避免分配新的 Value。
// 与 Go 中一样，键必须可赋值给 v 的类型，
// 且不能派生自未导出的字段。
// 如果 [Value.CanSet] 返回 false，它会 panic。
func (v Value) SetIterKey(iter *MapIter) {
	if !iter.hiter.Initialized() {
		panic("reflect: Value.SetIterKey called before Next")
	}
	iterkey := iter.hiter.Key()
	if iterkey == nil {
		panic("reflect: Value.SetIterKey called on exhausted iterator")
	}

	v.mustBeAssignable()
	var target unsafe.Pointer
	if v.kind() == Interface {
		target = v.ptr
	}

	t := (*abi.MapType)(unsafe.Pointer(iter.m.typ()))
	ktype := t.Key

	iter.m.mustBeExported() // 不要让未导出的 m 泄漏
	key := Value{ktype, iterkey, iter.m.flag | flag(ktype.Kind()) | flagIndir}
	key = key.assignTo("reflect.MapIter.SetKey", v.typ(), target)
	typedmemmove(v.typ(), v.ptr, key.ptr)
}

// Value 返回 iter 当前 map 条目的值。
func (iter *MapIter) Value() Value {
	if !iter.hiter.Initialized() {
		panic("MapIter.Value called before Next")
	}
	iterelem := iter.hiter.Elem()
	if iterelem == nil {
		panic("MapIter.Value called on exhausted iterator")
	}

	t := (*abi.MapType)(unsafe.Pointer(iter.m.typ()))
	vtype := t.Elem
	return copyVal(vtype, iter.m.flag.ro()|flag(vtype.Kind()), iterelem)
}

// SetIterValue 将 iter 当前 map 条目的值赋值给 v。
// 它等同于 v.Set(iter.Value())，但避免分配新的 Value。
// 与 Go 中一样，值必须可赋值给 v 的类型，
// 且不能派生自未导出的字段。
// 如果 [Value.CanSet] 返回 false，它会 panic。
func (v Value) SetIterValue(iter *MapIter) {
	if !iter.hiter.Initialized() {
		panic("reflect: Value.SetIterValue called before Next")
	}
	iterelem := iter.hiter.Elem()
	if iterelem == nil {
		panic("reflect: Value.SetIterValue called on exhausted iterator")
	}

	v.mustBeAssignable()
	var target unsafe.Pointer
	if v.kind() == Interface {
		target = v.ptr
	}

	t := (*abi.MapType)(unsafe.Pointer(iter.m.typ()))
	vtype := t.Elem

	iter.m.mustBeExported() // 不要让未导出的 m 泄漏
	elem := Value{vtype, iterelem, iter.m.flag | flag(vtype.Kind()) | flagIndir}
	elem = elem.assignTo("reflect.MapIter.SetValue", v.typ(), target)
	typedmemmove(v.typ(), v.ptr, elem.ptr)
}

// Next 推进 map 迭代器并报告是否还有另一个条目。
// 当 iter 耗尽时返回 false；后续调用 [MapIter.Key]、
// [MapIter.Value] 或 [MapIter.Next] 会 panic。
func (iter *MapIter) Next() bool {
	if !iter.m.IsValid() {
		panic("MapIter.Next called on an iterator that does not have an associated map Value")
	}
	if !iter.hiter.Initialized() {
		t := (*abi.MapType)(unsafe.Pointer(iter.m.typ()))
		m := (*maps.Map)(iter.m.pointer())
		mapIterStart(t, m, &iter.hiter)
	} else {
		if iter.hiter.Key() == nil {
			panic("MapIter.Next called on exhausted iterator")
		}
		mapIterNext(&iter.hiter)
	}
	return iter.hiter.Key() != nil
}

// Reset 修改 iter 以遍历 v。
// 如果 v 的 Kind 不是 [Map] 且 v 不是零值，它会 panic。
// Reset(Value{}) 使 iter 不再引用任何 map，
// 这可能允许先前迭代的 map 被垃圾回收。
func (iter *MapIter) Reset(v Value) {
	if v.IsValid() {
		v.mustBe(Map)
	}
	iter.m = v
	iter.hiter = maps.Iter{}
}

// MapRange 返回 map 的 range 迭代器。
// 如果 v 的 Kind 不是 [Map]，它会 panic。
//
// 调用 [MapIter.Next] 推进迭代器，调用 [MapIter.Key]/[MapIter.Value] 访问每个条目。
// 当迭代器耗尽时 [MapIter.Next] 返回 false。
// MapRange 遵循与 range 语句相同的迭代语义。
//
// 示例：
//
//	iter := reflect.ValueOf(m).MapRange()
//	for iter.Next() {
//		k := iter.Key()
//		v := iter.Value()
//		...
//	}
func (v Value) MapRange() *MapIter {
	// 这是可内联的，以利用"函数外联"。
	// 如果调用者不允许它逃逸，MapIter 的分配可以在栈上分配。
	// 参见 https://blog.filippo.io/efficient-go-apis-with-the-inliner/
	if v.kind() != Map {
		v.panicNotMap()
	}
	return &MapIter{m: v}
}

// SetMapIndex 将 map v 中与 key 关联的元素设置为 elem。
// 如果 v 的 Kind 不是 [Map]，它会 panic。
// 如果 elem 是零值 Value，SetMapIndex 会从 map 中删除 key。
// 否则，如果 v 持有一个 nil map，SetMapIndex 会 panic。
// 与 Go 中一样，key 的值必须可赋值给 map 的键类型，
// elem 的值必须可赋值给 map 的元素类型。
func (v Value) SetMapIndex(key, elem Value) {
	v.mustBe(Map)
	v.mustBeExported()
	key.mustBeExported()
	tt := (*abi.MapType)(unsafe.Pointer(v.typ()))

	if (tt.Key == stringType || key.kind() == String) && tt.Key == key.typ() && tt.Elem.Size() <= abi.MapMaxElemBytes {
		k := *(*string)(key.ptr)
		if elem.typ() == nil {
			mapdelete_faststr(v.typ(), v.pointer(), k)
			return
		}
		elem.mustBeExported()
		elem = elem.assignTo("reflect.Value.SetMapIndex", tt.Elem, nil)
		var e unsafe.Pointer
		if elem.flag&flagIndir != 0 {
			e = elem.ptr
		} else {
			e = unsafe.Pointer(&elem.ptr)
		}
		mapassign_faststr(v.typ(), v.pointer(), k, e)
		return
	}

	key = key.assignTo("reflect.Value.SetMapIndex", tt.Key, nil)
	var k unsafe.Pointer
	if key.flag&flagIndir != 0 {
		k = key.ptr
	} else {
		k = unsafe.Pointer(&key.ptr)
	}
	if elem.typ() == nil {
		mapdelete(v.typ(), v.pointer(), k)
		return
	}
	elem.mustBeExported()
	elem = elem.assignTo("reflect.Value.SetMapIndex", tt.Elem, nil)
	var e unsafe.Pointer
	if elem.flag&flagIndir != 0 {
		e = elem.ptr
	} else {
		e = unsafe.Pointer(&elem.ptr)
	}
	mapassign(v.typ(), v.pointer(), k, e)
}

// 强制慢速 panic 路径不内联，这样它就不会增加调用者的内联预算。
// TODO: 当内联器不再仅是自底向上时撤销此操作。
//
//go:noinline
func (f flag) panicNotMap() {
	f.mustBe(Map)
}
