// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package c

import "a"

// ----------------------------------------------------------------------------
// Test that empty declarations don't cause problems

const ()

type ()

var ()

// ----------------------------------------------------------------------------
// Test that types with documentation on both, the Decl and the Spec node
// are handled correctly.

// 一个(should see this)
type A struct{}

// B (should see this)
type (
	B struct{}
)

type (
	// C (should see this)
	C struct{}
)

// D (should not see this)
type (
	// D (should see this)
	D struct{}
)

// E (should see this for E2 and E3)
type (
	// E1 (should see this)
	E1 struct{}
	E2 struct{}
	E3 struct{}
	// E4 (should see this)
	E4 struct{}
)

// ----------------------------------------------------------------------------
// Test that local and imported types are different when
// handling anonymous fields.

type T1 struct{}

func (t1 *T1) M() {}

// T2 must not show methods of local T1
type T2 struct {
	a.T1 // not the same as locally declared T1
}
