// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package iter_test

import (
	"fmt"
	. "iter"
	"runtime"
	"testing"
)

func count(n int) Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				break
			}
		}
	}
}

func squares(n int) Seq2[int, int64] {
	return func(yield func(int, int64) bool) {
		for i := range n {
			if !yield(i, int64(i)*int64(i)) {
				break
			}
		}
	}
}

func TestPull(t *testing.T) {
	for end := 0; end <= 3; end++ {
		t.Run(fmt.Sprint(end), func(t *testing.T) {
			ng := stableNumGoroutine()
			wantNG := func(want int) {
				if xg := runtime.NumGoroutine() - ng; xg != want {
					t.Helper()
					t.Errorf("have %d extra goroutines, want %d", xg, want)
				}
			}
			wantNG(0)
			next, stop := Pull(count(3))
			wantNG(1)
			for i := range end {
				v, ok := next()
				if v != i || ok != true {
					t.Fatalf("next() = %d, %v, want %d, %v", v, ok, i, true)
				}
				wantNG(1)
			}
			wantNG(1)
			if end < 3 {
				stop()
				wantNG(0)
			}
			for range 2 {
				v, ok := next()
				if v != 0 || ok != false {
					t.Fatalf("next() = %d, %v, want %d, %v", v, ok, 0, false)
				}
				wantNG(0)
			}
			wantNG(0)

			stop()
			stop()
			stop()
			wantNG(0)
		})
	}
}

func TestPull2(t *testing.T) {
	for end := 0; end <= 3; end++ {
		t.Run(fmt.Sprint(end), func(t *testing.T) {
			ng := stableNumGoroutine()
			wantNG := func(want int) {
				if xg := runtime.NumGoroutine() - ng; xg != want {
					t.Helper()
					t.Errorf("have %d extra goroutines, want %d", xg, want)
				}
			}
			wantNG(0)
			next, stop := Pull2(squares(3))
			wantNG(1)
			for i := range end {
				k, v, ok := next()
				if k != i || v != int64(i*i) || ok != true {
					t.Fatalf("next() = %d, %d, %v, want %d, %d, %v", k, v, ok, i, i*i, true)
				}
				wantNG(1)
			}
			wantNG(1)
			if end < 3 {
				stop()
				wantNG(0)
			}
			for range 2 {
				k, v, ok := next()
				if v != 0 || ok != false {
					t.Fatalf("next() = %d, %d, %v, want %d, %d, %v", k, v, ok, 0, 0, false)
				}
				wantNG(0)
			}
			wantNG(0)

			stop()
			stop()
			stop()
			wantNG(0)
		})
	}
}

// stableNumGoroutine 类似于 NumGoroutine，但通过让任何退出的 goroutine 完成退出来尝试确保值的稳定性。
func stableNumGoroutine() int {
	// 稳定化 NumGoroutine 值的想法是在调用 runtime.Gosched 之间
	// 连续多次看到相同的值。使用 GOMAXPROCS=1，我们试图确保
	// 其他 goroutine 运行，以便它们达到稳定点。
	// 这不能保证，因为 goroutine 仍然可能 Gosched 回到自己，
	// 所以我们要求 NumGoroutine 连续 100 次相同。
	// 这应该足以确保所有 goroutine 有机会运行到完成（或到达
	// 某个阻塞点），对于一小组测试 goroutine。
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	c := 0
	ng := runtime.NumGoroutine()
	for i := 0; i < 1000; i++ {
		nng := runtime.NumGoroutine()
		if nng == ng {
			c++
		} else {
			c = 0
			ng = nng
		}
		if c >= 100 {
			// 连续 100 次相同的值就足够好了。
			return ng
		}
		runtime.Gosched()
	}
	panic("failed to stabilize NumGoroutine after 1000 iterations")
}

func TestPullDoubleNext(t *testing.T) {
	next, _ := Pull(doDoubleNext())
	nextSlot = next
	next()
	if nextSlot != nil {
		t.Fatal("double next did not fail")
	}
}

var nextSlot func() (int, bool)

func doDoubleNext() Seq[int] {
	return func(_ func(int) bool) {
		defer func() {
			if recover() != nil {
				nextSlot = nil
			}
		}()
		nextSlot()
	}
}

func TestPullDoubleNext2(t *testing.T) {
	next, _ := Pull2(doDoubleNext2())
	nextSlot2 = next
	next()
	if nextSlot2 != nil {
		t.Fatal("double next did not fail")
	}
}

var nextSlot2 func() (int, int, bool)

func doDoubleNext2() Seq2[int, int] {
	return func(_ func(int, int) bool) {
		defer func() {
			if recover() != nil {
				nextSlot2 = nil
			}
		}()
		nextSlot2()
	}
}

func TestPullDoubleYield(t *testing.T) {
	next, stop := Pull(storeYield())
	next()
	if yieldSlot == nil {
		t.Fatal("yield failed")
	}
	defer func() {
		if recover() != nil {
			yieldSlot = nil
		}
		stop()
	}()
	yieldSlot(5)
	if yieldSlot != nil {
		t.Fatal("double yield did not fail")
	}
}

func storeYield() Seq[int] {
	return func(yield func(int) bool) {
		yieldSlot = yield
		if !yield(5) {
			return
		}
	}
}

var yieldSlot func(int) bool

func TestPullDoubleYield2(t *testing.T) {
	next, stop := Pull2(storeYield2())
	next()
	if yieldSlot2 == nil {
		t.Fatal("yield failed")
	}
	defer func() {
		if recover() != nil {
			yieldSlot2 = nil
		}
		stop()
	}()
	yieldSlot2(23, 77)
	if yieldSlot2 != nil {
		t.Fatal("double yield did not fail")
	}
}

func storeYield2() Seq2[int, int] {
	return func(yield func(int, int) bool) {
		yieldSlot2 = yield
		if !yield(23, 77) {
			return
		}
	}
}

var yieldSlot2 func(int, int) bool

func TestPullPanic(t *testing.T) {
	t.Run("next", func(t *testing.T) {
		next, stop := Pull(panicSeq())
		if !panicsWith("boom", func() { next() }) {
			t.Fatal("failed to propagate panic on first next")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if _, ok := next(); ok {
			t.Fatal("next returned true after iterator panicked")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
	t.Run("stop", func(t *testing.T) {
		next, stop := Pull(panicCleanupSeq())
		x, ok := next()
		if !ok || x != 55 {
			t.Fatalf("expected (55, true) from next, got (%d, %t)", x, ok)
		}
		if !panicsWith("boom", func() { stop() }) {
			t.Fatal("failed to propagate panic on stop")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if _, ok := next(); ok {
			t.Fatal("next returned true after iterator panicked")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
}

func panicSeq() Seq[int] {
	return func(yield func(int) bool) {
		panic("boom")
	}
}

func panicCleanupSeq() Seq[int] {
	return func(yield func(int) bool) {
		for {
			if !yield(55) {
				panic("boom")
			}
		}
	}
}

func TestPull2Panic(t *testing.T) {
	t.Run("next", func(t *testing.T) {
		next, stop := Pull2(panicSeq2())
		if !panicsWith("boom", func() { next() }) {
			t.Fatal("failed to propagate panic on first next")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if _, _, ok := next(); ok {
			t.Fatal("next returned true after iterator panicked")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
	t.Run("stop", func(t *testing.T) {
		next, stop := Pull2(panicCleanupSeq2())
		x, y, ok := next()
		if !ok || x != 55 || y != 100 {
			t.Fatalf("expected (55, 100, true) from next, got (%d, %d, %t)", x, y, ok)
		}
		if !panicsWith("boom", func() { stop() }) {
			t.Fatal("failed to propagate panic on stop")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if _, _, ok := next(); ok {
			t.Fatal("next returned true after iterator panicked")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
}

func panicSeq2() Seq2[int, int] {
	return func(yield func(int, int) bool) {
		panic("boom")
	}
}

func panicCleanupSeq2() Seq2[int, int] {
	return func(yield func(int, int) bool) {
		for {
			if !yield(55, 100) {
				panic("boom")
			}
		}
	}
}

func panicsWith(v any, f func()) (panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			if r != v {
				panic(r)
			}
			panicked = true
		}
	}()
	f()
	return
}

func TestPullGoexit(t *testing.T) {
	t.Run("next", func(t *testing.T) {
		var next func() (int, bool)
		var stop func()
		if !goexits(t, func() {
			next, stop = Pull(goexitSeq())
			next()
		}) {
			t.Fatal("failed to Goexit from next")
		}
		if x, ok := next(); x != 0 || ok {
			t.Fatal("iterator returned valid value after iterator Goexited")
		}
		stop()
	})
	t.Run("stop", func(t *testing.T) {
		next, stop := Pull(goexitCleanupSeq())
		x, ok := next()
		if !ok || x != 55 {
			t.Fatalf("expected (55, true) from next, got (%d, %t)", x, ok)
		}
		if !goexits(t, func() {
			stop()
		}) {
			t.Fatal("failed to Goexit from stop")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if x, ok := next(); x != 0 || ok {
			t.Fatal("next returned true or non-zero value after iterator Goexited")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
}

func goexitSeq() Seq[int] {
	return func(yield func(int) bool) {
		runtime.Goexit()
	}
}

func goexitCleanupSeq() Seq[int] {
	return func(yield func(int) bool) {
		for {
			if !yield(55) {
				runtime.Goexit()
			}
		}
	}
}

func TestPull2Goexit(t *testing.T) {
	t.Run("next", func(t *testing.T) {
		var next func() (int, int, bool)
		var stop func()
		if !goexits(t, func() {
			next, stop = Pull2(goexitSeq2())
			next()
		}) {
			t.Fatal("failed to Goexit from next")
		}
		if x, y, ok := next(); x != 0 || y != 0 || ok {
			t.Fatal("iterator returned valid value after iterator Goexited")
		}
		stop()
	})
	t.Run("stop", func(t *testing.T) {
		next, stop := Pull2(goexitCleanupSeq2())
		x, y, ok := next()
		if !ok || x != 55 || y != 100 {
			t.Fatalf("expected (55, 100, true) from next, got (%d, %d, %t)", x, y, ok)
		}
		if !goexits(t, func() {
			stop()
		}) {
			t.Fatal("failed to Goexit from stop")
		}
		// 确保如果我们尝试调用 next 或 stop，我们不会再次 panic。
		if x, y, ok := next(); x != 0 || y != 0 || ok {
			t.Fatal("next returned true or non-zero after iterator Goexited")
		}
		// 再次调用 stop 应该是无操作。
		stop()
	})
}

func goexitSeq2() Seq2[int, int] {
	return func(yield func(int, int) bool) {
		runtime.Goexit()
	}
}

func goexitCleanupSeq2() Seq2[int, int] {
	return func(yield func(int, int) bool) {
		for {
			if !yield(55, 100) {
				runtime.Goexit()
			}
		}
	}
}

func goexits(t *testing.T, f func()) bool {
	t.Helper()

	exit := make(chan bool)
	go func() {
		cleanExit := false
		defer func() {
			exit <- recover() == nil && !cleanExit
		}()
		f()
		cleanExit = true
	}()
	return <-exit
}

func TestPullImmediateStop(t *testing.T) {
	next, stop := Pull(panicSeq())
	stop()
	// 确保如果我们尝试调用 next 或 stop，我们不会 panic。
	if _, ok := next(); ok {
		t.Fatal("next returned true after iterator was stopped")
	}
}

func TestPull2ImmediateStop(t *testing.T) {
	next, stop := Pull2(panicSeq2())
	stop()
	// 确保如果我们尝试调用 next 或 stop，我们不会 panic。
	if _, _, ok := next(); ok {
		t.Fatal("next returned true after iterator was stopped")
	}
}

func BenchmarkPull(b *testing.B) {
	seq := count(1)
	for range b.N {
		_, stop := Pull(seq)
		stop()
	}
}

func BenchmarkPull2(b *testing.B) {
	seq := squares(1)
	for range b.N {
		_, stop := Pull2(seq)
		stop()
	}
}
