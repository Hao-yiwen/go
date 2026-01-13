// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

package internal

import "errors"

// NotForPublicUse 是一个 marker type that an API is for internal use only.
// It 执行not perfectly prevent usage of that API, but helps to restrict usage.
// Anything with this marker is not covered by the Go compatibility agreement.
type NotForPublicUse struct{}

// AllowInternalUse is passed from "json" to "jsontext" to authenticate
// that the caller can have access to internal functionality.
var AllowInternalUse NotForPublicUse

// Sentinel error values internally shared between jsonv1 and jsonv2.
var (
	ErrCycle           = errors.New("encountered a cycle")
	ErrNonNilReference = errors.New("value must be passed as a non-nil pointer reference")
	ErrNilInterface    = errors.New("cannot derive concrete type for nil interface with finite type set")
)

var (
	// TransformMarshalError converts a v2 error into a v1 error.
	// It is called only at the top-level of a Marshal function.
	TransformMarshalError func(any, error) error
	// NewMarshalerError constructs a jsonv1.MarshalerError.
	// It is called after a user-defined Marshal method/function fails.
	NewMarshalerError func(any, error, string) error
	// TransformUnmarshalError converts a v2 error into a v1 error.
	// It is called only at the top-level of a Unmarshal function.
	TransformUnmarshalError func(any, error) error

	// NewRawNumber returns new(jsonv1.Number).
	NewRawNumber func() any
	// RawNumberOf returns jsonv1.Number(b).
	RawNumberOf func(b []byte) any
)
