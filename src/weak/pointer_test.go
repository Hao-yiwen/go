// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package weak_test

import (
	"context"
	"internal/goarch"
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"
	"weak"
)

type T struct {
	// 注：T 的定义方式是为了避免测试值与弱引用句柄进行微型分配，
	// 但由于对 go.dev/issue/76007 的修复，这应该不再可能发生。
	// TODO(mknyszek): 考虑对所有测试使用微型分配的值。
	t *T
	a int
	b int
}

func TestPointer(t *testing.T) {
	var zero weak.Pointer[T]
	if zero.Value() != nil {
		t.Error("Value of zero value of weak.Pointer is not nil")
	}
	zeroNil := weak.Make[T](nil)
	if zeroNil.Value() != nil {
		t.Error("Value of weak.Make[T](nil) is not nil")
	}

	bt := new(T)
	wt := weak.Make(bt)
	if st := wt.Value(); st != bt {
		t.Fatalf("weak pointer is not the same as strong pointer: %p vs. %p", st, bt)
	}
	// bt 仍然被引用。
	runtime.GC()

	if st := wt.Value(); st != bt {
		t.Fatalf("weak pointer is not the same as strong pointer after GC: %p vs. %p", st, bt)
	}
	// bt 不再被引用。
	runtime.GC()

	if st := wt.Value(); st != nil {
		t.Fatalf("expected weak pointer to be nil, got %p", st)
	}
}

func TestPointerEquality(t *testing.T) {
	var zero weak.Pointer[T]
	zeroNil := weak.Make[T](nil)
	if zero != zeroNil {
		t.Error("weak.Make[T](nil) != zero value of weak.Pointer[T]")
	}

	bt := make([]*T, 10)
	wt := make([]weak.Pointer[T], 10)
	wo := make([]weak.Pointer[int], 10)
	for i := range bt {
		bt[i] = new(T)
		wt[i] = weak.Make(bt[i])
		wo[i] = weak.Make(&bt[i].a)
	}
	for i := range bt {
		st := wt[i].Value()
		if st != bt[i] {
			t.Fatalf("weak pointer is not the same as strong pointer: %p vs. %p", st, bt[i])
		}
		if wp := weak.Make(st); wp != wt[i] {
			t.Fatalf("new weak pointer not equal to existing weak pointer: %v vs. %v", wp, wt[i])
		}
		if wp := weak.Make(&st.a); wp != wo[i] {
			t.Fatalf("new weak pointer not equal to existing weak pointer: %v vs. %v", wp, wo[i])
		}
		if i == 0 {
			continue
		}
		if wt[i] == wt[i-1] {
			t.Fatalf("expected weak pointers to not be equal to each other, but got %v", wt[i])
		}
	}
	// bt 仍然被引用。
	runtime.GC()
	for i := range bt {
		st := wt[i].Value()
		if st != bt[i] {
			t.Fatalf("weak pointer is not the same as strong pointer: %p vs. %p", st, bt[i])
		}
		if wp := weak.Make(st); wp != wt[i] {
			t.Fatalf("new weak pointer not equal to existing weak pointer: %v vs. %v", wp, wt[i])
		}
		if wp := weak.Make(&st.a); wp != wo[i] {
			t.Fatalf("new weak pointer not equal to existing weak pointer: %v vs. %v", wp, wo[i])
		}
		if i == 0 {
			continue
		}
		if wt[i] == wt[i-1] {
			t.Fatalf("expected weak pointers to not be equal to each other, but got %v", wt[i])
		}
	}
	bt = nil
	// bt 不再被引用。
	runtime.GC()
	for i := range wt {
		st := wt[i].Value()
		if st != nil {
			t.Fatalf("expected weak pointer to be nil, got %p", st)
		}
		if i == 0 {
			continue
		}
		if wt[i] == wt[i-1] {
			t.Fatalf("expected weak pointers to not be equal to each other, but got %v", wt[i])
		}
	}
}

func TestPointerFinalizer(t *testing.T) {
	bt := new(T)
	wt := weak.Make(bt)
	done := make(chan struct{}, 1)
	runtime.SetFinalizer(bt, func(bt *T) {
		if wt.Value() != nil {
			t.Errorf("weak pointer did not go nil before finalizer ran")
		}
		done <- struct{}{}
	})

	// 确保在 bt 存活期间弱指针仍然保持不变。
	runtime.GC()
	if wt.Value() == nil {
		t.Errorf("weak pointer went nil too soon")
	}
	runtime.KeepAlive(bt)

	// bt 不再被引用。
	//
	// 运行一个周期来排队终结器。
	runtime.GC()
	if wt.Value() != nil {
		t.Errorf("weak pointer did not go nil when finalizer was enqueued")
	}

	// 等待终结器运行。
	<-done

	// 终结器运行后，弱指针应该仍然为 nil。
	runtime.GC()
	if wt.Value() != nil {
		t.Errorf("weak pointer is non-nil even after finalization: %v", wt)
	}
}

func TestPointerCleanup(t *testing.T) {
	bt := new(T)
	wt := weak.Make(bt)
	done := make(chan struct{}, 1)
	runtime.AddCleanup(bt, func(_ bool) {
		if wt.Value() != nil {
			t.Errorf("weak pointer did not go nil before cleanup was executed")
		}
		done <- struct{}{}
	}, true)

	// 确保在 bt 存活期间弱指针仍然保持不变。
	runtime.GC()
	if wt.Value() == nil {
		t.Errorf("weak pointer went nil too soon")
	}
	runtime.KeepAlive(bt)

	// bt 不再被引用。
	//
	// 运行一个周期来排队清理。
	runtime.GC()
	if wt.Value() != nil {
		t.Errorf("weak pointer did not go nil when cleanup was enqueued")
	}

	// 等待清理运行。
	<-done

	// 清理运行后，弱指针应该仍然为 nil。
	runtime.GC()
	if wt.Value() != nil {
		t.Errorf("weak pointer is non-nil even after cleanup: %v", wt)
	}
}

func TestPointerSize(t *testing.T) {
	var p weak.Pointer[T]
	size := unsafe.Sizeof(p)
	if size != goarch.PtrSize {
		t.Errorf("weak.Pointer[T] size = %d, want %d", size, goarch.PtrSize)
	}
}

// 问题 69210 的回归测试。
//
// 弱到强转换必须对新的强指针进行着色，否则
// 可能会创建唯一指向白色对象的强指针，
// 该对象隐藏在黑化堆栈中。
//
// 如果正确则永不失败，如果不正确则有较高概率失败。
func TestIssue69210(t *testing.T) {
	if testing.Short() {
		t.Skip("this is a stress test that takes seconds to run on its own")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 我们想要做的是制造这个 bug 发生的条件。具体来说，我们想要：
	//
	// 1. 创建一大堆仅被弱指针指向的对象，
	// 2. 在 GC 处于标记阶段时调用 Value，
	// 3. 新的强指针被 GC 遗漏，
	// 4. 接下来的 GC 周期标记一个空闲对象。
	//
	// 不幸的是，(2) 和 (3) 很难控制，但我们可以通过让多个 goroutine
	// 同时执行 (1)，同时另一个 goroutine 不断通过 runtime.GC 将我们保持在
	// GC 中来增加可能性。这就像向标靶投掷飞镖，直到它们落在恰当的位置。
	// 我们可以通过在创建强指针后添加一些延迟来增加 (4) 的可能性，但仅当
	// 它非零时。如果为零，这意味着它已经被收集，在这种情况下没有机会
	// 触发该 bug，所以我们希望尽快重试。我们的堆在这里很小，所以 GC 会很快进行。
	//
	// 截至 2024-09-03，删除弱到强转换期间对指针进行着色的那一行
	// 会导致此测试约 50% 的时间失败。

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			runtime.GC()

			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()
	for range max(runtime.GOMAXPROCS(-1)-1, 1) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				for range 5 {
					bt := new(T)
					wt := weak.Make(bt)
					bt = nil
					time.Sleep(1 * time.Millisecond)
					bt = wt.Value()
					if bt != nil {
						time.Sleep(4 * time.Millisecond)
						bt.t = bt
						bt.a = 12
					}
					runtime.KeepAlive(bt)
				}
				select {
				case <-ctx.Done():
					return
				default:
				}
			}
		}()
	}
	wg.Wait()
}

func TestIssue70739(t *testing.T) {
	x := make([]*int, 4<<16)
	wx1 := weak.Make(&x[1<<16])
	wx2 := weak.Make(&x[1<<16])
	if wx1 != wx2 {
		t.Fatal("failed to look up special and made duplicate weak handle; see issue #70739")
	}
}

var immortal T

func TestImmortalPointer(t *testing.T) {
	w0 := weak.Make(&immortal)
	if weak.Make(&immortal) != w0 {
		t.Error("immortal weak pointers to the same pointer not equal")
	}
	w0a := weak.Make(&immortal.a)
	w0b := weak.Make(&immortal.b)
	if w0a == w0b {
		t.Error("separate immortal pointers (same object) have the same pointer")
	}
	if got, want := w0.Value(), &immortal; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}
	if got, want := w0a.Value(), &immortal.a; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}
	if got, want := w0b.Value(), &immortal.b; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}

	// 运行几个周期。
	runtime.GC()
	runtime.GC()

	// 所有不朽的弱指针永远不应该被清除。
	if got, want := w0.Value(), &immortal; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}
	if got, want := w0a.Value(), &immortal.a; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}
	if got, want := w0b.Value(), &immortal.b; got != want {
		t.Errorf("immortal weak pointer to %p has unexpected Value %p", want, got)
	}
}

func TestPointerTiny(t *testing.T) {
	runtime.GC() // 清除微型分配缓存。

	const N = 1000
	wps := make([]weak.Pointer[int], N)
	for i := range N {
		// 注：*x 只是一个 int，所以该值很可能与弱句柄一起进行微型分配，
		// 假设 go.dev/issue/76007 中的 bug 存在。
		x := new(int)
		*x = i
		wps[i] = weak.Make(x)
	}

	// 获取清理开始运行。
	runtime.GC()

	// 期望至少 3/4 的弱指针已变为零。
	//
	// 请注意，我们提供了一些余地，因为我们的分配有可能与其他长期的微型分配
	// 分组，但对于绝大多数分配，这不应该发生。
	n := 0
	for _, wp := range wps {
		if wp.Value() == nil {
			n++
		}
	}
	const want = 3 * N / 4
	if n < want {
		t.Fatalf("not enough weak pointers are nil: expected at least %v, got %v", want, n)
	}
}
