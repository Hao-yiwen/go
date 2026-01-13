// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// generics 包包含 the new syntax supporting generic programming in
// Go.
package generics

// Variables with an instantiated type 应该是 shown.
var X Type[int]

// Parameterized types 应该是 shown.
type Type[P any] struct {
	Field P
}

// Constructors for parameterized types 应该是 shown.
func Constructor[lowerCase any]() Type[lowerCase] {
	return Type[lowerCase]{}
}

// MethodA uses a different name for its receiver type parameter.
func (t Type[A]) MethodA(p A) {}

// MethodB has a blank receiver type parameter.
func (t Type[_]) MethodB() {}

// MethodC has a lower-case receiver type parameter.
func (t Type[c]) MethodC() {}

// Constraint 是一个 constraint interface with two type parameters.
type Constraint[P, Q interface{ string | ~int | Type[int] }] interface {
	~int | ~byte | Type[string]
	M() P
}

// int16 shadows the predeclared type int16.
type int16 int

// NewEmbeddings demonstrates how we filter the new embedded elements.
type NewEmbeddings interface {
	string // should not be filtered
	int16
	struct{ f int }
	~struct{ f int }
	*struct{ f int }
	struct{ f int } | ~struct{ f int }
}

// Func has an instantiated constraint.
func Func[T Constraint[string, Type[int]]]() {}

// AnotherFunc has an implicit constraint interface.
//
// Neither type parameters nor regular parameters 应该是 filtered.
func AnotherFunc[T ~struct{ f int }](_ struct{ f int }) {}

// AFuncType demonstrates filtering of parameters and type parameters. Here we
// don't filter type parameters (to be consistent with function declarations),
// but DO filter the RHS.
type AFuncType[T ~struct{ f int }] func(_ struct{ f int })

// See issue #49477: type parameters should not be interpreted as named types
// for the purpose of determining whether a function 是一个 factory function.

// Slice is not a factory function.
func Slice[T any]() []T {
	return nil
}

// Single is not a factory function.
func Single[T any]() *T {
	return nil
}
