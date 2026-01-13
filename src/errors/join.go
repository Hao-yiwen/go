// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package errors

import (
	"unsafe"
)

// Join 返回包装给定错误的错误。
// 任何 nil 错误值都会被丢弃。
// 如果 errs 中的每个值都是 nil，Join 返回 nil。
// 错误格式化为通过调用 errs 每个元素的 Error 方法获得的字符串的连接，
// 每个字符串之间有换行符。
//
// Join 返回的非 nil 错误实现 Unwrap() []error 方法。
// 可以使用 [Is] 和 [As] 检查这些错误。
func Join(errs ...error) error {
	n := 0
	for _, err := range errs {
		if err != nil {
			n++
		}
	}
	if n == 0 {
		return nil
	}
	e := &joinError{
		errs: make([]error, 0, n),
	}
	for _, err := range errs {
		if err != nil {
			e.errs = append(e.errs, err)
		}
	}
	return e
}

type joinError struct {
	errs []error
}

func (e *joinError) Error() string {
	// 由于如果 errs 中的每个值都是 nil，Join 返回 nil，
	// 所以 e.errs 不能为空。
	if len(e.errs) == 1 {
		return e.errs[0].Error()
	}

	b := []byte(e.errs[0].Error())
	for _, err := range e.errs[1:] {
		b = append(b, '\n')
		b = append(b, err.Error()...)
	}
	// 此时，b 至少有一个字节 '\n'。
	return unsafe.String(&b[0], len(b))
}

func (e *joinError) Unwrap() []error {
	return e.errs
}
