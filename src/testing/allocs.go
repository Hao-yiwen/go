// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package testing

import (
	"runtime"
)

// AllocsPerRun 返回调用 f 期间的平均内存分配次数。
// 虽然返回值类型为 float64，但它始终是一个整数值。
//
// 为了计算分配次数，该函数将首先运行一次作为预热。
// 然后测量并返回指定运行次数内的平均分配次数。
//
// AllocsPerRun 在测量期间将 [runtime.GOMAXPROCS] 设置为 1，
// 并在返回前恢复原值。
func AllocsPerRun(runs int, f func()) (avg float64) {
	if parallelStart.Load() != parallelStop.Load() {
		panic("testing: AllocsPerRun called during parallel test")
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	// 预热函数
	f()

	// 测量起始统计数据
	var memstats runtime.MemStats
	runtime.ReadMemStats(&memstats)
	mallocs := 0 - memstats.Mallocs

	// 运行函数指定的次数
	for i := 0; i < runs; i++ {
		f()
	}

	// 读取最终统计数据
	runtime.ReadMemStats(&memstats)
	mallocs += memstats.Mallocs

	// 计算运行次数内的平均分配次数（不包括预热）。
	// 由于 API 设计的原因，我们被迫返回 float64，但我们使用整数除法，
	// 这样我们可以判断 AllocsPerRun()==1 而不是 AllocsPerRun()<2。
	return float64(mallocs / uint64(runs))
}
