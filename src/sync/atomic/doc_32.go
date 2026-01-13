// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build 386 || arm || mips || mipsle

package atomic

// SwapInt64 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Int64.Swap]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func SwapInt64(addr *int64, new int64) (old int64)

// SwapUint64 原子地将 new 存储到 *addr 中并返回之前的 *addr 值。
// 改用更符合人体工学且不易出错的 [Uint64.Swap]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func SwapUint64(addr *uint64, new uint64) (old uint64)

// CompareAndSwapInt64 对 int64 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Int64.CompareAndSwap]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func CompareAndSwapInt64(addr *int64, old, new int64) (swapped bool)

// CompareAndSwapUint64 对 uint64 值执行比较并交换操作。
// 改用更符合人体工学且不易出错的 [Uint64.CompareAndSwap]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func CompareAndSwapUint64(addr *uint64, old, new uint64) (swapped bool)

// AddInt64 原子地将 delta 加到 *addr 中并返回新值。
// 改用更符合人体工学且不易出错的 [Int64.Add]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func AddInt64(addr *int64, delta int64) (new int64)

// AddUint64 原子地将 delta 加到 *addr 中并返回新值。
// 要从 x 减去有符号正常量值 c，请执行 AddUint64(&x, ^uint64(c-1))。
// 特别是，要递减 x，请执行 AddUint64(&x, ^uint64(0))。
// 改用更符合人体工学且不易出错的 [Uint64.Add]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func AddUint64(addr *uint64, delta uint64) (new uint64)

// AndInt64 使用提供的位掩码对 *addr 执行原子按位 AND 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Int64.And]。
func AndInt64(addr *int64, mask int64) (old int64)

// AndUint64 使用提供的位掩码对 *addr 执行原子按位 AND 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uint64.And]。
func AndUint64(addr *uint64, mask uint64) (old uint64)

// OrInt64 使用提供的位掩码对 *addr 执行原子按位 OR 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Int64.Or]。
func OrInt64(addr *int64, mask int64) (old int64)

// OrUint64 使用提供的位掩码对 *addr 执行原子按位 OR 操作
// 并返回旧值。
// 改用更符合人体工学且不易出错的 [Uint64.Or]。
func OrUint64(addr *uint64, mask uint64) (old uint64)

// LoadInt64 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Int64.Load]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func LoadInt64(addr *int64) (val int64)

// LoadUint64 原子地加载 *addr。
// 改用更符合人体工学且不易出错的 [Uint64.Load]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func LoadUint64(addr *uint64) (val uint64)

// StoreInt64 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Int64.Store]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func StoreInt64(addr *int64, val int64)

// StoreUint64 原子地将 val 存储到 *addr 中。
// 改用更符合人体工学且不易出错的 [Uint64.Store]
// （特别是如果你针对 32 位平台；请参见 bugs 部分）。
func StoreUint64(addr *uint64, val uint64)
