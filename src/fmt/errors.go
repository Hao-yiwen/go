// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fmt

import (
	"errors"
	"internal/stringslite"
	"slices"
)

// Errorf 根据格式说明符进行格式化并将字符串作为满足 error 的值返回。
//
// 如果格式说明符包含带错误操作数的 %w 动词，
// 返回的错误将实现一个返回该操作数的 Unwrap 方法。
// 如果有多个 %w 动词，返回的错误将实现一个
// Unwrap 方法，返回一个 []error，包含所有 %w 操作数，
// 按它们在参数中出现的顺序。
// 使用 %w 动词提供不实现 error 接口的操作数是无效的。
// %w 动词在其他方面是 %v 的同义词。
func Errorf(format string, a ...any) (err error) {
	// 这个函数以某种不自然的方式被拆分
	// 以便它和 errors.New 调用都可以被内联。
	if err = errorf(format, a...); err != nil {
		return err
	}
	// 不需要格式化。我们可以避免一些分配和其他工作。
	// 详见 https://go.dev/cl/708836。
	return errors.New(format)
}

// errorf 格式化并返回一个错误值，如果不需要格式化则返回 nil。
func errorf(format string, a ...any) error {
	if len(a) == 0 && stringslite.IndexByte(format, '%') == -1 {
		return nil
	}
	p := newPrinter()
	p.wrapErrs = true
	p.doPrintf(format, a)
	s := string(p.buf)
	var err error
	switch len(p.wrappedErrs) {
	case 0:
		err = errors.New(s)
	case 1:
		w := &wrapError{msg: s}
		w.err, _ = a[p.wrappedErrs[0]].(error)
		err = w
	default:
		if p.reordered {
			slices.Sort(p.wrappedErrs)
		}
		var errs []error
		for i, argNum := range p.wrappedErrs {
			if i > 0 && p.wrappedErrs[i-1] == argNum {
				continue
			}
			if e, ok := a[argNum].(error); ok {
				errs = append(errs, e)
			}
		}
		err = &wrapErrors{s, errs}
	}
	p.free()
	return err
}

type wrapError struct {
	msg string
	err error
}

func (e *wrapError) Error() string {
	return e.msg
}

func (e *wrapError) Unwrap() error {
	return e.err
}

type wrapErrors struct {
	msg  string
	errs []error
}

func (e *wrapErrors) Error() string {
	return e.msg
}

func (e *wrapErrors) Unwrap() []error {
	return e.errs
}
