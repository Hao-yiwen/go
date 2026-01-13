// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
unsafe 包包含绕过 Go 程序类型安全的操作。

导入 unsafe 的包可能是不可移植的，并且不受 Go 1 兼容性指南的保护。
*/
package unsafe

// ArbitraryType 仅用于文档目的，实际上不是 unsafe 包的一部分。
// 它表示任意 Go 表达式的类型。
type ArbitraryType int

// IntegerType 仅用于文档目的，实际上不是 unsafe 包的一部分。
// 它表示任意整数类型。
type IntegerType int

// Pointer 表示指向任意类型的指针。有四种特殊操作可用于
// Pointer 类型，而其他类型不可用：
//   - 任何类型的指针值都可以转换为 Pointer。
//   - Pointer 可以转换为任何类型的指针值。
//   - uintptr 可以转换为 Pointer。
//   - Pointer 可以转换为 uintptr。
//
// 因此，Pointer 允许程序绕过类型系统并读写任意内存。
// 应极其谨慎地使用它。
//
// 以下涉及 Pointer 的模式是有效的。
// 不使用这些模式的代码今天可能是无效的，
// 或者将来可能变得无效。
// 即使是下面的有效模式也有重要的注意事项。
//
// 运行 "go vet" 可以帮助发现不符合这些模式的 Pointer 使用，
// 但 "go vet" 的静默并不保证代码是有效的。
//
// (1) 将 *T1 转换为 Pointer 再转换为 *T2。
//
// 前提是 T2 不大于 T1 并且两者具有等效的内存布局，
// 此转换允许将一种类型的数据重新解释为另一种类型的数据。
// 一个例子是 math.Float64bits 的实现：
//
//	func Float64bits(f float64) uint64 {
//		return *(*uint64)(unsafe.Pointer(&f))
//	}
//
// (2) 将 Pointer 转换为 uintptr（但不能转换回 Pointer）。
//
// 将 Pointer 转换为 uintptr 会产生所指向值的内存地址（作为整数）。
// 这种 uintptr 的常见用途是打印它。
//
// 通常，将 uintptr 转换回 Pointer 是无效的。
//
// uintptr 是整数，不是引用。
// 将 Pointer 转换为 uintptr 会创建一个没有指针语义的整数值。
// 即使 uintptr 保存了某个对象的地址，
// 如果对象移动，垃圾收集器也不会更新该 uintptr 的值，
// 该 uintptr 也不会阻止对象被回收。
//
// 其余模式列举了从 uintptr 转换为 Pointer 的唯一有效情况。
//
// (3) 将 Pointer 转换为 uintptr 再转换回来，并进行算术运算。
//
// 如果 p 指向已分配的对象内部，可以通过转换为 uintptr、
// 添加偏移量、再转换回 Pointer 来在对象中前进。
//
//	p = unsafe.Pointer(uintptr(p) + offset)
//
// 此模式最常见的用途是访问结构体中的字段或数组的元素：
//
//	// 等效于 f := unsafe.Pointer(&s.f)
//	f := unsafe.Pointer(uintptr(unsafe.Pointer(&s)) + unsafe.Offsetof(s.f))
//
//	// 等效于 e := unsafe.Pointer(&x[i])
//	e := unsafe.Pointer(uintptr(unsafe.Pointer(&x[0])) + i*unsafe.Sizeof(x[0]))
//
// 以这种方式从指针加减偏移量都是有效的。
// 使用 &^ 对指针进行舍入（通常用于对齐）也是有效的。
// 在所有情况下，结果必须继续指向原始分配的对象内部。
//
// 与 C 不同，将指针前进到刚好超出其原始分配范围是无效的：
//
//	// 无效：end 指向已分配空间之外。
//	var s thing
//	end = unsafe.Pointer(uintptr(unsafe.Pointer(&s)) + unsafe.Sizeof(s))
//
//	// 无效：end 指向已分配空间之外。
//	b := make([]byte, n)
//	end = unsafe.Pointer(uintptr(unsafe.Pointer(&b[0])) + uintptr(n))
//
// 注意，两个转换必须出现在同一个表达式中，
// 它们之间只能有算术运算：
//
//	// 无效：uintptr 不能在转换回 Pointer 之前存储在变量中。
//	u := uintptr(p)
//	p = unsafe.Pointer(u + offset)
//
// 注意，指针必须指向已分配的对象，因此它不能是 nil。
//
//	// 无效：nil 指针的转换
//	u := unsafe.Pointer(nil)
//	p := unsafe.Pointer(uintptr(u) + offset)
//
// (4) 调用 [syscall.Syscall] 等函数时将 Pointer 转换为 uintptr。
//
// syscall 包中的 Syscall 函数将其 uintptr 参数直接传递给操作系统，
// 操作系统可能根据调用的具体情况将其中一些重新解释为指针。
// 也就是说，系统调用实现隐式地将某些参数从 uintptr 转换回指针。
//
// 如果必须将指针参数转换为 uintptr 以用作参数，
// 该转换必须出现在调用表达式本身中：
//
//	syscall.Syscall(SYS_READ, uintptr(fd), uintptr(unsafe.Pointer(p)), uintptr(n))
//
// 编译器处理在汇编实现的函数调用的参数列表中转换为 uintptr 的 Pointer，
// 方法是安排保留被引用的已分配对象（如果有）并且不移动它直到调用完成，
// 即使仅从类型来看，对象在调用期间似乎不再需要。
//
// 为了让编译器识别此模式，转换必须出现在参数列表中：
//
//	// 无效：uintptr 不能在系统调用期间隐式转换回 Pointer 之前存储在变量中。
//	u := uintptr(unsafe.Pointer(p))
//	syscall.Syscall(SYS_READ, uintptr(fd), u, uintptr(n))
//
// (5) 将 [reflect.Value.Pointer] 或 [reflect.Value.UnsafeAddr] 的结果
// 从 uintptr 转换为 Pointer。
//
// reflect 包的 Value 方法 Pointer 和 UnsafeAddr 返回类型 uintptr
// 而不是 unsafe.Pointer，以防止调用者在未先导入 "unsafe" 的情况下
// 将结果更改为任意类型。然而，这意味着结果是脆弱的，
// 必须在调用后立即在同一表达式中转换为 Pointer：
//
//	p := (*int)(unsafe.Pointer(reflect.ValueOf(new(int)).Pointer()))
//
// 与上述情况一样，在转换之前存储结果是无效的：
//
//	// 无效：uintptr 不能在转换回 Pointer 之前存储在变量中。
//	u := reflect.ValueOf(new(int)).Pointer()
//	p := (*int)(unsafe.Pointer(u))
//
// (6) 将 [reflect.SliceHeader] 或 [reflect.StringHeader] 的 Data 字段
// 转换为 Pointer 或从 Pointer 转换。
//
// 与前一种情况一样，reflect 数据结构 SliceHeader 和 StringHeader
// 将字段 Data 声明为 uintptr，以防止调用者在未先导入 "unsafe" 的情况下
// 将结果更改为任意类型。然而，这意味着 SliceHeader 和 StringHeader
// 仅在解释实际切片或字符串值的内容时有效。
//
//	var s string
//	hdr := (*reflect.StringHeader)(unsafe.Pointer(&s)) // 情况 1
//	hdr.Data = uintptr(unsafe.Pointer(p))              // 情况 6（本情况）
//	hdr.Len = n
//
// 在此用法中，hdr.Data 实际上是引用字符串头中底层指针的另一种方式，
// 而不是 uintptr 变量本身。
//
// 通常，[reflect.SliceHeader] 和 [reflect.StringHeader] 应仅用作
// *reflect.SliceHeader 和 *reflect.StringHeader 指向实际的切片或字符串，
// 而不应作为普通结构体使用。
// 程序不应声明或分配这些结构体类型的变量。
//
//	// 无效：直接声明的头不会将 Data 保持为引用。
//	var hdr reflect.StringHeader
//	hdr.Data = uintptr(unsafe.Pointer(p))
//	hdr.Len = n
//	s := *(*string)(unsafe.Pointer(&hdr)) // p 可能已经丢失
type Pointer *ArbitraryType

// Sizeof 接受任何类型的表达式 x，并返回假设变量 v 的字节大小，
// 就好像 v 是通过 var v = x 声明的。
// 大小不包括 x 可能引用的任何内存。
// 例如，如果 x 是切片，Sizeof 返回切片描述符的大小，
// 而不是切片引用的内存大小；
// 如果 x 是接口，Sizeof 返回接口值本身的大小，
// 而不是接口中存储的值的大小。
// 对于结构体，大小包括由字段对齐引入的任何填充。
// 如果参数 x 的类型没有可变大小，则 Sizeof 的返回值是 Go 常量。
// （如果类型是类型参数，或者是具有可变大小元素的数组或结构体类型，
// 则该类型具有可变大小）。
func Sizeof(x ArbitraryType) uintptr

// Offsetof 返回由 x 表示的字段在结构体中的偏移量，
// x 必须是 structValue.field 的形式。换句话说，
// 它返回结构体开始和字段开始之间的字节数。
// 如果参数 x 的类型没有可变大小，则 Offsetof 的返回值是 Go 常量。
// （有关可变大小类型的定义，请参见 [Sizeof] 的描述。）
func Offsetof(x ArbitraryType) uintptr

// Alignof 接受任何类型的表达式 x，并返回假设变量 v 所需的对齐方式，
// 就好像 v 是通过 var v = x 声明的。
// 它是最大值 m，使得 v 的地址始终是 m 的整数倍。
// 它与 [reflect.TypeOf](x).Align() 返回的值相同。
// 作为特例，如果变量 s 是结构体类型且 f 是该结构体中的字段，
// 则 Alignof(s.f) 将返回该类型字段在结构体中所需的对齐方式。
// 此情况与 [reflect.TypeOf](s.f).FieldAlign() 返回的值相同。
// 如果参数的类型没有可变大小，则 Alignof 的返回值是 Go 常量。
// （有关可变大小类型的定义，请参见 [Sizeof] 的描述。）
func Alignof(x ArbitraryType) uintptr

// Add 函数将 len 添加到 ptr 并返回更新后的指针
// [Pointer](uintptr(ptr) + uintptr(len))。
// len 参数必须是整数类型或无类型常量。
// 常量 len 参数必须可以用 int 类型的值表示；
// 如果它是无类型常量，则被赋予 int 类型。
// Pointer 的有效使用规则仍然适用。
func Add(ptr Pointer, len IntegerType) Pointer

// Slice 函数返回一个切片，其底层数组从 ptr 开始，
// 长度和容量都是 len。
// Slice(ptr, len) 等效于
//
//	(*[len]ArbitraryType)(unsafe.Pointer(ptr))[:]
//
// 但作为特例，如果 ptr 是 nil 且 len 是零，
// Slice 返回 nil。
//
// len 参数必须是整数类型或无类型常量。
// 常量 len 参数必须是非负的且可以用 int 类型的值表示；
// 如果它是无类型常量，则被赋予 int 类型。
// 在运行时，如果 len 是负数，或者如果 ptr 是 nil 且 len 不是零，
// 则会发生运行时 panic。
func Slice(ptr *ArbitraryType, len IntegerType) []ArbitraryType

// SliceData 返回指向参数切片的底层数组的指针。
//   - 如果 cap(slice) > 0，SliceData 返回 &slice[:1][0]。
//   - 如果 slice == nil，SliceData 返回 nil。
//   - 否则，SliceData 返回一个指向未指定内存地址的非 nil 指针。
func SliceData(slice []ArbitraryType) *ArbitraryType

// String 返回一个字符串值，其底层字节从 ptr 开始，长度为 len。
//
// len 参数必须是整数类型或无类型常量。
// 常量 len 参数必须是非负的且可以用 int 类型的值表示；
// 如果它是无类型常量，则被赋予 int 类型。
// 在运行时，如果 len 是负数，或者如果 ptr 是 nil 且 len 不是零，
// 则会发生运行时 panic。
//
// 由于 Go 字符串是不可变的，传递给 String 的字节
// 在返回的字符串值存在期间不得被修改。
func String(ptr *byte, len IntegerType) string

// StringData 返回指向 str 底层字节的指针。
// 对于空字符串，返回值是未指定的，可能是 nil。
//
// 由于 Go 字符串是不可变的，StringData 返回的字节不得被修改。
func StringData(str string) *byte
