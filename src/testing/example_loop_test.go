// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package testing_test

import (
	"math/rand/v2"
	"testing"
)

// ExBenchmark 演示了如何在基准测试中使用 b.Loop。
//
// (如果这是一个真实的基准测试，而不是示例，这将被命名为
// BenchmarkSomething。)
func ExBenchmark(b *testing.B) {
	// 生成一个大的随机切片作为输入。
	// 由于这是在第一次调用 b.Loop() 之前完成的，
	// 它不计入基准测试时间。
	input := make([]int, 128<<10)
	for i := range input {
		input[i] = rand.Int()
	}

	// 执行基准测试。
	for b.Loop() {
		// 通常，编译器会被允许优化掉对 sum 的调用，
		// 因为它没有副作用并且结果未被使用。
		// 但是，在 b.Loop 循环内，编译器确保函数调用
		// 不会被优化掉。
		sum(input)
	}

	// 在循环外，计时器已停止，所以我们可以执行
	// 清理（如果必要）而不影响结果。
}

func sum(data []int) int {
	total := 0
	for _, value := range data {
		total += value
	}
	return total
}

func ExampleB_Loop() {
	testing.Benchmark(ExBenchmark)
}
