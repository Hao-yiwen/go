// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package error2

type I0 interface {
	// When embedded, the locally-declared error interface
	// is only visible if all declarations are shown.
	error
}

type T0 struct {
	ExportedField interface {
		// error should not be visible
		error
	}
}

type S0 struct {
	// In struct types, an embedded error must only be visible
	// if AllDecls is set.
	error
}

// This error declaration shadows the predeclared error type.
type error interface {
	Error() string
}
