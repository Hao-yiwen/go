// 版权所有 2018 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package errors

import (
	"internal/reflectlite"
)

// Unwrap 返回对 err 调用 Unwrap 方法的结果，如果 err 的类型
// 包含返回 error 的 Unwrap 方法。
// 否则，Unwrap 返回 nil。
//
// Unwrap 只调用形如 "Unwrap() error" 的方法。
// 特别是，Unwrap 不会解包 [Join] 返回的错误。
func Unwrap(err error) error {
	u, ok := err.(interface {
		Unwrap() error
	})
	if !ok {
		return nil
	}
	return u.Unwrap()
}

// Is 报告 err 树中是否有任何错误与 target 匹配。
// target 必须是可比较的。
//
// 树由 err 本身组成，后跟通过重复调用其 Unwrap() error 或
// Unwrap() []error 方法获得的错误。当 err 包装多个错误时，
// Is 检查 err 后跟其子项的深度优先遍历。
//
// 如果错误等于目标，或者它实现了 Is(error) bool 方法且
// Is(target) 返回 true，则认为该错误与目标匹配。
//
// 错误类型可能提供 Is 方法，以便可以将其视为等同于现有错误。
// 例如，如果 MyError 定义了
//
//	func (m MyError) Is(target error) bool { return target == fs.ErrExist }
//
// 则 Is(MyError{}, fs.ErrExist) 返回 true。标准库中的示例
// 请参见 [syscall.Errno.Is]。Is 方法应只浅层比较 err 和 target，
// 而不应对任何一个调用 [Unwrap]。
func Is(err, target error) bool {
	if err == nil || target == nil {
		return err == target
	}

	isComparable := reflectlite.TypeOf(target).Comparable()
	return is(err, target, isComparable)
}

func is(err, target error, targetComparable bool) bool {
	for {
		if targetComparable && err == target {
			return true
		}
		if x, ok := err.(interface{ Is(error) bool }); ok && x.Is(target) {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return false
			}
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if is(err, target, targetComparable) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

// As 在 err 的树中查找与 target 匹配的第一个错误，如果找到，
// 将 target 设置为该错误值并返回 true。否则，返回 false。
//
// 对于大多数用途，优先使用 [AsType]。As 等同于 [AsType]，
// 但设置其 target 参数而不是返回匹配的错误，
// 且不要求其 target 参数实现 error。
//
// 树由 err 本身组成，后跟通过重复调用其 Unwrap() error 或
// Unwrap() []error 方法获得的错误。当 err 包装多个错误时，
// As 检查 err 后跟其子项的深度优先遍历。
//
// 如果错误的具体值可分配给 target 指向的值，或者错误具有
// As(any) bool 方法且 As(target) 返回 true，则错误与 target 匹配。
// 在后一种情况下，As 方法负责设置 target。
//
// 错误类型可能提供 As 方法，以便可以将其视为不同的错误类型。
//
// 如果 target 不是指向实现 error 的类型或任何接口类型的非 nil 指针，
// As 会 panic。
func As(err error, target any) bool {
	if err == nil {
		return false
	}
	if target == nil {
		panic("errors: target cannot be nil")
	}
	val := reflectlite.ValueOf(target)
	typ := val.Type()
	if typ.Kind() != reflectlite.Ptr || val.IsNil() {
		panic("errors: target must be a non-nil pointer")
	}
	targetType := typ.Elem()
	if targetType.Kind() != reflectlite.Interface && !targetType.Implements(errorType) {
		panic("errors: *target must be interface or implement error")
	}
	return as(err, target, val, targetType)
}

func as(err error, target any, targetVal reflectlite.Value, targetType reflectlite.Type) bool {
	for {
		if reflectlite.TypeOf(err).AssignableTo(targetType) {
			targetVal.Elem().Set(reflectlite.ValueOf(err))
			return true
		}
		if x, ok := err.(interface{ As(any) bool }); ok && x.As(target) {
			return true
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return false
			}
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if err == nil {
					continue
				}
				if as(err, target, targetVal, targetType) {
					return true
				}
			}
			return false
		default:
			return false
		}
	}
}

var errorType = reflectlite.TypeOf((*error)(nil)).Elem()

// AsType 在 err 的树中查找与类型 E 匹配的第一个错误，
// 如果找到，返回该错误值和 true。否则，返回 E 的零值和 false。
//
// 树由 err 本身组成，后跟通过重复调用其 Unwrap() error 或
// Unwrap() []error 方法获得的错误。当 err 包装多个错误时，
// AsType 检查 err 后跟其子项的深度优先遍历。
//
// 如果类型断言 err.(E) 成立，或者错误具有 As(any) bool 方法
// 且当 target 是非 nil *E 时 err.As(target) 返回 true，
// 则错误 err 与类型 E 匹配。在后一种情况下，As 方法负责设置 target。
func AsType[E error](err error) (E, bool) {
	if err == nil {
		var zero E
		return zero, false
	}
	var pe *E // lazily initialized
	return asType(err, &pe)
}

func asType[E error](err error, ppe **E) (_ E, _ bool) {
	for {
		if e, ok := err.(E); ok {
			return e, true
		}
		if x, ok := err.(interface{ As(any) bool }); ok {
			if *ppe == nil {
				*ppe = new(E)
			}
			if x.As(*ppe) {
				return **ppe, true
			}
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return
			}
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if err == nil {
					continue
				}
				if x, ok := asType(err, ppe); ok {
					return x, true
				}
			}
			return
		default:
			return
		}
	}
}
