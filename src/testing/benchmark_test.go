// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package testing_test

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"text/template"
	"time"
)

var prettyPrintTests = []struct {
	v        float64
	expected string
}{
	{0, "         0 x"},
	{1234.1, "      1234 x"},
	{-1234.1, "     -1234 x"},
	{999.950001, "      1000 x"},
	{999.949999, "       999.9 x"},
	{99.9950001, "       100.0 x"},
	{99.9949999, "        99.99 x"},
	{-99.9949999, "       -99.99 x"},
	{0.000999950001, "         0.001000 x"},
	{0.000999949999, "         0.0009999 x"}, // 最小情况
	{0.0000999949999, "         0.0001000 x"},
}

func TestPrettyPrint(t *testing.T) {
	for _, tt := range prettyPrintTests {
		buf := new(strings.Builder)
		testing.PrettyPrint(buf, tt.v, "x")
		if tt.expected != buf.String() {
			t.Errorf("prettyPrint(%v): expected %q, actual %q", tt.v, tt.expected, buf.String())
		}
	}
}

func TestResultString(t *testing.T) {
	// 测试小数 ns/op 处理
	r := testing.BenchmarkResult{
		N: 100,
		T: 240 * time.Nanosecond,
	}
	if r.NsPerOp() != 2 {
		t.Errorf("NsPerOp: expected 2, actual %v", r.NsPerOp())
	}
	if want, got := "     100\t         2.400 ns/op", r.String(); want != got {
		t.Errorf("String: expected %q, actual %q", want, got)
	}

	// 测试小于 1 ns/op 的情况（issue #31005）
	r.T = 40 * time.Nanosecond
	if want, got := "     100\t         0.4000 ns/op", r.String(); want != got {
		t.Errorf("String: expected %q, actual %q", want, got)
	}

	// 测试 0 ns/op
	r.T = 0
	if want, got := "     100", r.String(); want != got {
		t.Errorf("String: expected %q, actual %q", want, got)
	}
}

func TestRunParallel(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping in short mode")
	}
	testing.Benchmark(func(b *testing.B) {
		procs := uint32(0)
		iters := uint64(0)
		b.SetParallelism(3)
		b.RunParallel(func(pb *testing.PB) {
			atomic.AddUint32(&procs, 1)
			for pb.Next() {
				atomic.AddUint64(&iters, 1)
			}
		})
		if want := uint32(3 * runtime.GOMAXPROCS(0)); procs != want {
			t.Errorf("got %v procs, want %v", procs, want)
		}
		if iters != uint64(b.N) {
			t.Errorf("got %v iters, want %v", iters, b.N)
		}
	})
}

func TestRunParallelFail(t *testing.T) {
	testing.Benchmark(func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			// 该函数必须能够进行日志记录/中止
			// 而不会导致整个基准测试崩溃/死锁。
			b.Log("log")
			b.Error("error")
		})
	})
}

func TestRunParallelFatal(t *testing.T) {
	testing.Benchmark(func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if b.N > 1 {
					b.Fatal("error")
				}
			}
		})
	})
}

func TestRunParallelSkipNow(t *testing.T) {
	testing.Benchmark(func(b *testing.B) {
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if b.N > 1 {
					b.SkipNow()
				}
			}
		})
	})
}

func TestBenchmarkContext(t *testing.T) {
	testing.Benchmark(func(b *testing.B) {
		ctx := b.Context()
		if err := ctx.Err(); err != nil {
			b.Fatalf("expected non-canceled context, got %v", err)
		}

		var innerCtx context.Context
		b.Run("inner", func(b *testing.B) {
			innerCtx = b.Context()
			if err := innerCtx.Err(); err != nil {
				b.Fatalf("expected inner benchmark to not inherit canceled context, got %v", err)
			}
		})
		b.Run("inner2", func(b *testing.B) {
			if !errors.Is(innerCtx.Err(), context.Canceled) {
				t.Fatal("expected context of sibling benchmark to be canceled after its test function finished")
			}
		})

		t.Cleanup(func() {
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Fatal("expected context canceled before cleanup")
			}
		})
	})
}

func ExampleB_RunParallel() {
	// 对单个对象的 text/template.Template.Execute 进行并行基准测试。
	testing.Benchmark(func(b *testing.B) {
		templ := template.Must(template.New("test").Parse("Hello, {{.}}!"))
		// RunParallel 将创建 GOMAXPROCS 个 goroutine
		// 并在它们之间分配工作。
		b.RunParallel(func(pb *testing.PB) {
			// 每个 goroutine 都有自己的 bytes.Buffer。
			var buf bytes.Buffer
			for pb.Next() {
				// 循环体在所有 goroutine 中总共执行 b.N 次。
				buf.Reset()
				templ.Execute(&buf, "World")
			}
		})
	})
}

func TestReportMetric(t *testing.T) {
	res := testing.Benchmark(func(b *testing.B) {
		b.ReportMetric(12345, "ns/op")
		b.ReportMetric(0.2, "frobs/op")
	})
	// 测试内置覆盖。
	if res.NsPerOp() != 12345 {
		t.Errorf("NsPerOp: expected %v, actual %v", 12345, res.NsPerOp())
	}
	// 测试字符串化。
	res.N = 1 // 使输出稳定
	want := "       1\t     12345 ns/op\t         0.2000 frobs/op"
	if want != res.String() {
		t.Errorf("expected %q, actual %q", want, res.String())
	}
}

func ExampleB_ReportMetric() {
	// 这报告与特定算法相关的自定义基准测试指标（在本例中是排序）。
	testing.Benchmark(func(b *testing.B) {
		var compares int64
		for b.Loop() {
			s := []int{5, 4, 3, 2, 1}
			slices.SortFunc(s, func(a, b int) int {
				compares++
				return cmp.Compare(a, b)
			})
		}
		// 该指标是每次操作的，所以除以 b.N 并
		// 以 "/op" 单位报告。
		b.ReportMetric(float64(compares)/float64(b.N), "compares/op")
		// 该指标是每时间的，所以除以 b.Elapsed 并
		// 以 "/ns" 单位报告。
		b.ReportMetric(float64(compares)/float64(b.Elapsed().Nanoseconds()), "compares/ns")
	})
}

func ExampleB_ReportMetric_parallel() {
	// 这在并行情况下报告与特定算法相关的自定义基准测试指标（在本例中是排序）。
	testing.Benchmark(func(b *testing.B) {
		var compares atomic.Int64
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				s := []int{5, 4, 3, 2, 1}
				slices.SortFunc(s, func(a, b int) int {
					// 因为 RunParallel 会多次并行运行该函数，
					// 我们必须原子地增加计数器以避免竞争写入。
					compares.Add(1)
					return cmp.Compare(a, b)
				})
			}
		})

		// 注意：在所有并行调用完成后，只报告每个指标一次。

		// 该指标是每次操作的，所以除以 b.N 并
		// 以 "/op" 单位报告。
		b.ReportMetric(float64(compares.Load())/float64(b.N), "compares/op")
		// 该指标是每时间的，所以除以 b.Elapsed 并
		// 以 "/ns" 单位报告。
		b.ReportMetric(float64(compares.Load())/float64(b.Elapsed().Nanoseconds()), "compares/ns")
	})
}
