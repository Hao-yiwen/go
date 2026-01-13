// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package tabwriter_test

import (
	"fmt"
	"os"
	"text/tabwriter"
)

func ExampleWriter_Init() {
	w := new(tabwriter.Writer)

	// 以制表符分隔的列格式化，制表位为 8。
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)
	fmt.Fprintln(w, "a\tb\tc\td\t.")
	fmt.Fprintln(w, "123\t12345\t1234567\t123456789\t.")
	fmt.Fprintln(w)
	w.Flush()

	// 在最小宽度为 5 的空格分隔列中右对齐格式化，
	// 并且至少有一个空格的填充（这样较宽的列条目
	// 就不会相互接触）。
	w.Init(os.Stdout, 5, 0, 1, ' ', tabwriter.AlignRight)
	fmt.Fprintln(w, "a\tb\tc\td\t.")
	fmt.Fprintln(w, "123\t12345\t1234567\t123456789\t.")
	fmt.Fprintln(w)
	w.Flush()

	// 输出：
	// a	b	c	d		.
	// 123	12345	1234567	123456789	.
	//
	//     a     b       c         d.
	//   123 12345 1234567 123456789.
}

func Example_elastic() {
	// 观察 b 和 d 尽管出现在每行的第二个单元格中，
	// 但它们属于不同的列。
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 1, '.', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(w, "a\tb\tc")
	fmt.Fprintln(w, "aa\tbb\tcc")
	fmt.Fprintln(w, "aaa\t") // 尾部制表符
	fmt.Fprintln(w, "aaaa\tdddd\teeee")
	w.Flush()

	// 输出：
	// ....a|..b|c
	// ...aa|.bb|cc
	// ..aaa|
	// .aaaa|.dddd|eeee
}

func Example_trailingTab() {
	// 观察第三行没有尾部制表符，
	// 所以它的最后一个单元格不是对齐列的一部分。
	const padding = 3
	w := tabwriter.NewWriter(os.Stdout, 0, 0, padding, '-', tabwriter.AlignRight|tabwriter.Debug)
	fmt.Fprintln(w, "a\tb\taligned\t")
	fmt.Fprintln(w, "aa\tbb\taligned\t")
	fmt.Fprintln(w, "aaa\tbbb\tunaligned") // 没有尾部制表符
	fmt.Fprintln(w, "aaaa\tbbbb\taligned\t")
	w.Flush()

	// 输出：
	// ------a|------b|---aligned|
	// -----aa|-----bb|---aligned|
	// ----aaa|----bbb|unaligned
	// ---aaaa|---bbbb|---aligned|
}
