// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package foo_test

import (
	"fmt"

	"example.com/error"
)

func Print(s string) {
	fmt.Println(s)
}

func Example() {
	Print(error.Hello)
}
