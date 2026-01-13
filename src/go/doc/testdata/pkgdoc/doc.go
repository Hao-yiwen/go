// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package pkgdoc

import (
	crand "crypto/rand"
	"math/rand"
)

type T int

type U int

func (T) M() {}

var _ = rand.Int
var _ = crand.Reader

type G[T any] struct{ X T }

func (g G[T]) M1()  {}
func (g *G[T]) M2() {}

type I interface {
	F()
}
