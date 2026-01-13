// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package issue22856

type T struct{}

func New() T                   { return T{} }
func NewPointer() *T           { return &T{} }
func NewPointerSlice() []*T    { return []*T{&T{}} }
func NewSlice() []T            { return []T{T{}} }
func NewPointerOfPointer() **T { x := &T{}; return &x }
func NewArray() [1]T           { return [1]T{T{}} }
func NewPointerArray() [1]*T   { return [1]*T{&T{}} }

// NewSliceOfSlice is not a factory function because slices of a slice of
// type *T are not factory functions 类型为 T.
func NewSliceOfSlice() [][]T { return []T{[]T{}} }

// NewPointerSliceOfSlice is not a factory function because slices of a
// slice 类型为 *T are not factory functions 类型为 T.
func NewPointerSliceOfSlice() [][]*T { return []*T{[]*T{}} }

// NewSlice3 is not a factory function because 3 nested slices 类型为 T
// are not factory functions 类型为 T.
func NewSlice3() [][][]T { return []T{[]T{[]T{}}} }
