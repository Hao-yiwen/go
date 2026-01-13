// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"context"
	"errors"
)

// NewMultiHandler 使用给定的 Handler 创建一个 [MultiHandler]。
func NewMultiHandler(handlers ...Handler) *MultiHandler {
	h := make([]Handler, len(handlers))
	copy(h, handlers)
	return &MultiHandler{multi: h}
}

// MultiHandler 是一个 [Handler]，它调用所有给定的 Handler。
// 其 Enable 方法报告任何处理程序的 Enabled 方法是否返回 true。
// 其 Handle、WithAttr 和 WithGroup 方法在每个启用的处理程序上调用相应的方法。
type MultiHandler struct {
	multi []Handler
}

func (h *MultiHandler) Enabled(ctx context.Context, l Level) bool {
	for i := range h.multi {
		if h.multi[i].Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (h *MultiHandler) Handle(ctx context.Context, r Record) error {
	var errs []error
	for i := range h.multi {
		if h.multi[i].Enabled(ctx, r.Level) {
			if err := h.multi[i].Handle(ctx, r.Clone()); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

func (h *MultiHandler) WithAttrs(attrs []Attr) Handler {
	handlers := make([]Handler, 0, len(h.multi))
	for i := range h.multi {
		handlers = append(handlers, h.multi[i].WithAttrs(attrs))
	}
	return &MultiHandler{multi: handlers}
}

func (h *MultiHandler) WithGroup(name string) Handler {
	handlers := make([]Handler, 0, len(h.multi))
	for i := range h.multi {
		handlers = append(handlers, h.multi[i].WithGroup(name))
	}
	return &MultiHandler{multi: handlers}
}
