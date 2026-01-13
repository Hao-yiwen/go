// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package abi

import "internal/goarch"

type InterfaceSwitch struct {
	Cache  *InterfaceSwitchCache
	NCases int

	// NCases 个元素的数组。
	// 每个 case 必须是非空接口类型。
	Cases [1]*InterfaceType
}

type InterfaceSwitchCache struct {
	Mask    uintptr                      // 索引的掩码。必须是 2 的幂减 1
	Entries [1]InterfaceSwitchCacheEntry // 共 Mask+1 个条目
}

type InterfaceSwitchCacheEntry struct {
	// 源值的类型（一个 *Type）
	Typ uintptr
	// 要分派到的 case 编号
	Case int
	// 用于结果 case 变量的 itab（一个 *runtime.itab）
	Itab uintptr
}

func UseInterfaceSwitchCache(arch goarch.ArchFamilyType) bool {
	// 我们需要原子加载指令来使缓存具有多线程安全性。
	// （AtomicLoadPtr 需要在 cmd/compile/internal/ssa/_gen/ARCH.rules 中实现。）
	switch arch {
	case goarch.AMD64, goarch.ARM64, goarch.LOONG64, goarch.MIPS, goarch.MIPS64, goarch.PPC64, goarch.RISCV64, goarch.S390X:
		return true
	default:
		return false
	}
}

type TypeAssert struct {
	Cache   *TypeAssertCache
	Inter   *InterfaceType
	CanFail bool
}
type TypeAssertCache struct {
	Mask    uintptr
	Entries [1]TypeAssertCacheEntry
}
type TypeAssertCacheEntry struct {
	// 源值的类型（一个 *runtime._type）
	Typ uintptr
	// 用于结果的 itab（一个 *runtime.itab）
	// 如果设置了 CanFail 且转换会失败，则为 nil。
	Itab uintptr
}
