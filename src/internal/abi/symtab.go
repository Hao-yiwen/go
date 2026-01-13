// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

// PCLnTabMagic 是 PC/行表开头的版本号。
// 这是 .pclntab 节的开始，也是 runtime.pcHeader。
// 魔数的选择使得用不同的字节序读取该值不会产生相同的值。
// 这使我们可以通过魔数来确定字节序。
type PCLnTabMagic uint32

const (
	// Go 1.2 到 Go 1.15 使用的初始 PCLnTabMagic 值。
	Go12PCLnTabMagic PCLnTabMagic = 0xfffffffb
	// Go 1.16 到 Go 1.17 使用的 PCLnTabMagic 值。
	// 头部添加了多个字段（CL 241598）。
	Go116PCLnTabMagic PCLnTabMagic = 0xfffffffa
	// Go 1.18 到 Go 1.19 使用的 PCLnTabMagic 值。
	// 函数数据的入口 PC 从地址改为偏移量（CL 351463）。
	Go118PCLnTabMagic PCLnTabMagic = 0xfffffff0
	// Go 1.20 及更高版本使用的 PCLnTabMagic 值。
	// 生成的符号名称中添加了 ":"（#37762）。
	Go120PCLnTabMagic PCLnTabMagic = 0xfffffff1

	// CurrentPCLnTabMagic 是当前工具链发出的值。
	// 这由链接器写入 pcHeader，并由运行时和 debug/gosym
	// （以及 Delve 等外部工具）读取。
	//
	// 更新 pclntab 版本时请更改此值。
	// 更改此导出值是可以的，因为这是一个 internal 包。
	CurrentPCLnTabMagic = Go120PCLnTabMagic
)

// FuncFlag 记录关于函数的位信息，传递给运行时。
type FuncFlag uint8

const (
	// FuncFlagTopFrame 表示出现在其栈顶部的函数。
	// 回溯例程在这样的函数处停止，并认为这是成功、完整的栈遍历。
	// TopFrame 函数的例子包括 goexit（出现在用户 goroutine 栈顶部）
	// 和 mstart（出现在系统 goroutine 栈顶部）。
	FuncFlagTopFrame FuncFlag = 1 << iota

	// FuncFlagSPWrite 表示向 SP 写入任意值的函数
	// （除了加或减常量之外的任何写入）。
	// 回溯例程无法将这种变化编码到 pcsp 表中，
	// 因此函数回溯无法安全地展开经过 SPWrite 函数。
	// 在 SPWrite 函数处停止被认为是不完整的栈展开。
	// 在某些上下文中（特别是垃圾回收器栈扫描），这是致命错误。
	FuncFlagSPWrite

	// FuncFlagAsm 表示函数是用汇编实现的。
	FuncFlagAsm
)

// FuncID 标识需要被运行时特殊处理的特定函数。
// 注意，在某些涉及插件的情况下，可能存在特定特殊运行时函数的多个副本。
type FuncID uint8

const (
	// 如果添加 FuncID，可能还需要在 ../../cmd/internal/objabi/funcid.go 的映射中添加条目

	FuncIDNormal FuncID = iota // 不是特殊函数
	FuncID_abort
	FuncID_asmcgocall
	FuncID_asyncPreempt
	FuncID_cgocallback
	FuncID_corostart
	FuncID_debugCallV2
	FuncID_gcBgMarkWorker
	FuncID_goexit
	FuncID_gogo
	FuncID_gopanic
	FuncID_handleAsyncEvent
	FuncID_mcall
	FuncID_morestack
	FuncID_mstart
	FuncID_panicwrap
	FuncID_rt0_go
	FuncID_runtime_main
	FuncID_runFinalizers
	FuncID_runCleanups
	FuncID_sigpanic
	FuncID_systemstack
	FuncID_systemstack_switch
	FuncIDWrapper // 任何自动生成的代码（hash/eq 算法、方法包装器等）
)

// ArgsSizeUnknown 设置在 Func.argsize 中，用于标记所有参数大小未知的函数
// （C 可变参数函数，以及没有显式规范的汇编代码）。
// 此值由编译器、汇编器或链接器生成。
const ArgsSizeUnknown = -0x80000000

// Go 二进制文件中 PCDATA 和 FUNCDATA 表的 ID。
//
// 这些必须与 ../../../runtime/funcdata.h 一致。
const (
	PCDATA_UnsafePoint   = 0
	PCDATA_StackMapIndex = 1
	PCDATA_InlTreeIndex  = 2
	PCDATA_ArgLiveIndex  = 3
	PCDATA_PanicBounds   = 4

	FUNCDATA_ArgsPointerMaps    = 0
	FUNCDATA_LocalsPointerMaps  = 1
	FUNCDATA_StackObjects       = 2
	FUNCDATA_InlTree            = 3
	FUNCDATA_OpenCodedDeferInfo = 4
	FUNCDATA_ArgInfo            = 5
	FUNCDATA_ArgLiveInfo        = 6
	FUNCDATA_WrapInfo           = 7
)

// PCDATA_UnsafePoint 表的特殊值。
const (
	UnsafePointSafe   = -1 // 对异步抢占安全
	UnsafePointUnsafe = -2 // 对异步抢占不安全

	// UnsafePointRestart1(2) 应用于一系列指令，
	// 如果在其中发生异步抢占，恢复时我们应该将 PC 回退到序列的开始。
	// 我们需要两个值，以便在两个序列相邻时区分序列的开始/结束。
	UnsafePointRestart1 = -3
	UnsafePointRestart2 = -4

	// 类似 UnsafePointRestart1，但如果被异步抢占则回到函数入口。
	UnsafePointRestartAtEntry = -5
)

const MINFUNC = 16 // 函数的最小大小

const FuncTabBucketSize = 256 * MINFUNC // pc->func 查找表中桶的大小
