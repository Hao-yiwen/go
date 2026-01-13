// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 测试覆盖率支持。

package testing

// CoverBlock 记录单个基本块的覆盖率数据。
// 字段是 1 索引的，就像在编辑器中一样：例如，文件的第一行是编号 1。
// 列以字节为单位测量。
// 注意：此结构是测试基础设施的内部结构，可能会更改。
// 它（尚未）被 Go 1 兼容性指南覆盖。
type CoverBlock struct {
	Line0 uint32 // 块开始的行号。
	Col0  uint16 // 块开始的列号。
	Line1 uint32 // 块结束的行号。
	Col1  uint16 // 块结束的列号。
	Stmts uint16 // 该块中包含的语句数。
}

// Cover 记录有关测试覆盖率检查的信息。
// 注意：此结构是测试基础设施的内部结构，可能会更改。
// 它（尚未）被 Go 1 兼容性指南覆盖。
type Cover struct {
	Mode            string
	Counters        map[string][]uint32
	Blocks          map[string][]CoverBlock
	CoveredPackages string
}

// RegisterCover 记录测试的覆盖率数据累加器。
// 注意：此函数是测试基础设施的内部函数，可能会更改。
// 它（尚未）被 Go 1 兼容性指南覆盖。
func RegisterCover(c Cover) {
}
