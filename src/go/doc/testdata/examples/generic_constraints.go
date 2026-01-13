// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package p_test

import (
	"fmt"
	"time"
)

type C1 interface {
	string | int
}

type C2 interface {
	M(time.Time)
}

type G[T C1] int

func g[T C2](x T) {}

type Tm int

func (Tm) M(time.Time) {}

type Foo int

func Example() {
	fmt.Println("hello")
}

func ExampleGeneric() {
	var x G[string]
	g(Tm(3))
	fmt.Println(x)
}
