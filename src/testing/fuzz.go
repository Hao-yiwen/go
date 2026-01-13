// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package testing

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"
)

func initFuzzFlags() {
	matchFuzz = flag.String("test.fuzz", "", "run the fuzz test matching `regexp`")
	flag.Var(&fuzzDuration, "test.fuzztime", "time to spend fuzzing; default is to run indefinitely")
	flag.Var(&minimizeDuration, "test.fuzzminimizetime", "time to spend minimizing a value after finding a failing input")

	fuzzCacheDir = flag.String("test.fuzzcachedir", "", "directory where interesting fuzzing inputs are stored (for use only by cmd/go)")
	isFuzzWorker = flag.Bool("test.fuzzworker", false, "coordinate with the parent process to fuzz random values (for use only by cmd/go)")
}

var (
	matchFuzz        *string
	fuzzDuration     durationOrCountFlag
	minimizeDuration = durationOrCountFlag{d: 60 * time.Second, allowZero: true}
	fuzzCacheDir     *string
	isFuzzWorker     *bool

	// corpusDir 是包内模糊测试种子语料库的父目录。
	corpusDir = "testdata/fuzz"
)

// fuzzWorkerExitCode 用作模糊测试工作进程在内部错误后的退出代码。
// 这将内部错误与不受控制的 panic 和其他失败区分开来。
// 与 internal/fuzz.workerExitCode 保持同步。
const fuzzWorkerExitCode = 70

// InternalFuzzTarget 是一个内部类型，但由于跨包使用而被导出；
// 它是 "go test" 命令实现的一部分。
type InternalFuzzTarget struct {
	Name string
	Fn   func(f *F)
}

// F 是传递给模糊测试的类型。
//
// 模糊测试针对提供的模糊目标运行生成的输入，可以
// 发现并报告被测代码中的潜在错误。
//
// 模糊测试默认运行种子语料库，其中包括由 [F.Add] 提供的条目
// 和 testdata/fuzz/<FuzzTestName> 目录中的条目。在进行任何必要的设置
// 和调用 [F.Add] 之后，模糊测试必须调用 [F.Fuzz] 来提供模糊目标。
// 有关示例，请参阅 testing 包文档，有关详细信息，请参阅 [F.Fuzz] 和 [F.Add]
// 方法文档。
//
// *F 方法只能在 [F.Fuzz] 之前调用。一旦测试正在执行模糊目标，
// 只能使用 [*T] 方法。在 [F.Fuzz] 函数中允许的唯一 *F 方法
// 是 [F.Failed] 和 [F.Name]。
type F struct {
	common
	fstate *fuzzState
	tstate *testState

	// inFuzzFn 在模糊函数运行时为 true。当 inFuzzFn 为 true 时，
	// 大多数 F 方法不能被调用。
	inFuzzFn bool

	// corpus 是一组种子语料库条目，使用 F.Add 添加并从 testdata 加载。
	corpus []corpusEntry

	result     fuzzResult
	fuzzCalled bool
}

var _ TB = (*F)(nil)

// corpusEntry 是与 internal/fuzz.CorpusEntry 相同类型的别名。
// 我们使用类型别名是因为我们不想导出这个类型，
// 而且我们不能从 testing 导入 internal/fuzz。
type corpusEntry = struct {
	Parent     string
	Path       string
	Data       []byte
	Values     []any
	Generation int
	IsSeed     bool
}

// Helper 将调用函数标记为测试辅助函数。
// 在打印文件和行信息时，该函数将被跳过。
// Helper 可以从多个 goroutine 同时调用。
func (f *F) Helper() {
	if f.inFuzzFn {
		panic("testing: f.Helper was called inside the fuzz target, use t.Helper instead")
	}

	// common.Helper 在此处内联。
	// 如果我们调用它，它会将 F.Helper 标记为辅助函数，
	// 而不是调用者。
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.helperPCs == nil {
		f.helperPCs = make(map[uintptr]struct{})
	}
	// 在此处重复 callerName 的代码以节省遍历栈帧
	var pc [1]uintptr
	n := runtime.Callers(2, pc[:]) // skip runtime.Callers + Helper
	if n == 0 {
		panic("testing: zero callers found")
	}
	if _, found := f.helperPCs[pc[0]]; !found {
		f.helperPCs[pc[0]] = struct{}{}
		f.helperNames = nil // map will be recreated next time it is needed
	}
}

// Fail 将函数标记为已失败但继续执行。
func (f *F) Fail() {
	// (*F).Fail 可能由 (*T).Fail 调用，这是我们应该允许的。但是，
	// 我们不应该允许从 (*F).Fuzz 函数内部直接调用 (*F).Fail。
	if f.inFuzzFn {
		panic("testing: f.Fail was called inside the fuzz target, use t.Fail instead")
	}
	f.common.Helper()
	f.common.Fail()
}

// Skipped 报告测试是否被跳过。
func (f *F) Skipped() bool {
	// (*F).Skipped 可能由 tRunner 调用，这是我们应该允许的。但是，
	// 我们不应该允许从 (*F).Fuzz 函数内部直接调用 (*F).Skipped。
	if f.inFuzzFn {
		panic("testing: f.Skipped was called inside the fuzz target, use t.Skipped instead")
	}
	f.common.Helper()
	return f.common.Skipped()
}

// Add 将参数添加到模糊测试的种子语料库中。如果在模糊目标之后或内部调用，
// 这将是一个空操作，并且 args 必须与模糊目标的参数匹配。
func (f *F) Add(args ...any) {
	var values []any
	for i := range args {
		if t := reflect.TypeOf(args[i]); !supportedTypes[t] {
			panic(fmt.Sprintf("testing: unsupported type to Add %v", t))
		}
		values = append(values, args[i])
	}
	f.corpus = append(f.corpus, corpusEntry{Values: values, IsSeed: true, Path: fmt.Sprintf("seed#%d", len(f.corpus))})
}

// supportedTypes 表示所有可以进行模糊测试的支持类型。
var supportedTypes = map[reflect.Type]bool{
	reflect.TypeFor[[]byte]():  true,
	reflect.TypeFor[string]():  true,
	reflect.TypeFor[bool]():    true,
	reflect.TypeFor[byte]():    true,
	reflect.TypeFor[rune]():    true,
	reflect.TypeFor[float32](): true,
	reflect.TypeFor[float64](): true,
	reflect.TypeFor[int]():     true,
	reflect.TypeFor[int8]():    true,
	reflect.TypeFor[int16]():   true,
	reflect.TypeFor[int32]():   true,
	reflect.TypeFor[int64]():   true,
	reflect.TypeFor[uint]():    true,
	reflect.TypeFor[uint8]():   true,
	reflect.TypeFor[uint16]():  true,
	reflect.TypeFor[uint32]():  true,
	reflect.TypeFor[uint64]():  true,
}

// Fuzz 运行模糊函数 ff 进行模糊测试。如果 ff 对一组参数失败，
// 这些参数将被添加到种子语料库中。
//
// ff 必须是一个没有返回值的函数，其第一个参数是 [*T]，
// 其余参数是要进行模糊测试的类型。
// 例如：
//
//	f.Fuzz(func(t *testing.T, b []byte, i int) { ... })
//
// 允许以下类型：[]byte、string、bool、byte、rune、float32、
// float64、int、int8、int16、int32、int64、uint、uint8、uint16、uint32、uint64。
// 将来可能会支持更多类型。
//
// ff 不能调用任何 [*F] 方法，例如 [F.Log]、[F.Error]、[F.Skip]。
// 请改用相应的 [*T] 方法。在 F.Fuzz 函数中允许的唯一 [*F] 方法
// 是 [F.Failed] 和 [F.Name]。
//
// 此函数应该快速且确定性，其行为不应依赖于共享状态。
// 不应在模糊函数的执行之间保留可变输入参数或指向它们的指针，
// 因为支持它们的内存可能会在后续调用期间发生变化。
// ff 不能修改模糊引擎提供的参数的底层数据。
//
// 在模糊测试时，F.Fuzz 不会返回，直到发现问题、时间耗尽
// （使用 -fuzztime 设置）或测试进程被信号中断。
// F.Fuzz 应该只调用一次，除非事先调用了 [F.Skip] 或 [F.Fail]。
func (f *F) Fuzz(ff any) {
	if f.fuzzCalled {
		panic("testing: F.Fuzz called more than once")
	}
	f.fuzzCalled = true
	if f.failed {
		return
	}
	f.Helper()

	// ff 应该是 func(*testing.T, ...interface{}) 形式
	fn := reflect.ValueOf(ff)
	fnType := fn.Type()
	if fnType.Kind() != reflect.Func {
		panic("testing: F.Fuzz must receive a function")
	}
	if fnType.NumIn() < 2 || fnType.In(0) != reflect.TypeFor[*T]() {
		panic("testing: fuzz target must receive at least two arguments, where the first argument is a *T")
	}
	if fnType.NumOut() != 0 {
		panic("testing: fuzz target must not return a value")
	}

	// 保存函数的类型以与语料库进行比较。
	var types []reflect.Type
	for i := 1; i < fnType.NumIn(); i++ {
		t := fnType.In(i)
		if !supportedTypes[t] {
			panic(fmt.Sprintf("testing: unsupported type for fuzzing %v", t))
		}
		types = append(types, t)
	}

	// 加载 testdata 种子语料库。检查 testdata 语料库中条目的类型
	// 和使用 F.Add 声明的条目。
	//
	// 如果这是工作进程，不要加载种子语料库；我们不会使用它。
	if f.fstate.mode != fuzzWorker {
		for _, c := range f.corpus {
			if err := f.fstate.deps.CheckCorpus(c.Values, types); err != nil {
				// TODO(#48302): Report the source location of the F.Add call.
				f.Fatal(err)
			}
		}

		// 加载种子语料库
		c, err := f.fstate.deps.ReadCorpus(filepath.Join(corpusDir, f.name), types)
		if err != nil {
			f.Fatal(err)
		}
		for i := range c {
			c[i].IsSeed = true // 这些都是种子语料库值
			if f.fstate.mode == fuzzCoordinator {
				// 如果这是协调器进程，将值置零，因为我们不需要保留它们。
				c[i].Values = nil
			}
		}

		f.corpus = append(f.corpus, c...)
	}

	// run 在给定输入上调用 fn，作为具有自己 T 的子测试。
	// run 类似于 T.Run。测试过滤和清理工作方式类似。
	// fn 在其自己的 goroutine 中调用。
	run := func(captureOut io.Writer, e corpusEntry) (ok bool) {
		if e.Values == nil {
			// corpusEntry 必须有非空的 Values 才能运行测试。
			// 如果 Values 为 nil，这是我们代码中的错误。
			panic(fmt.Sprintf("corpus file %q was not unmarshaled", e.Path))
		}
		if shouldFailFast() {
			return true
		}
		testName := f.name
		if e.Path != "" {
			testName = fmt.Sprintf("%s/%s", testName, filepath.Base(e.Path))
		}
		if f.tstate.isFuzzing {
			// 模糊测试时不保留子测试名称。如果 fn 调用 T.Run，
			// 将有大量具有重复名称的子测试，这将使用大量内存。
			// 子测试名称没有用，因为没有办法确定性地重新运行它们。
			f.tstate.match.clearSubNames()
		}

		ctx, cancelCtx := context.WithCancel(f.ctx)

		// 记录此调用点的栈跟踪，以便如果子测试函数
		// （在单独的栈中运行）被标记为辅助函数，我们可以
		// 继续遍历栈到父测试中。
		var pc [maxStackLen]uintptr
		n := runtime.Callers(2, pc[:])
		t := &T{
			common: common{
				barrier:   make(chan bool),
				signal:    make(chan bool),
				name:      testName,
				parent:    &f.common,
				level:     f.level + 1,
				creator:   pc[:n],
				chatty:    f.chatty,
				ctx:       ctx,
				cancelCtx: cancelCtx,
			},
			tstate: f.tstate,
		}
		if captureOut != nil {
			// t.parent 是 f.common 的别名。
			t.parent.w = captureOut
		}
		t.w = indenter{&t.common}
		t.setOutputWriter()
		if t.chatty != nil {
			t.chatty.Updatef(t.name, "=== RUN   %s\n", t.name)
		}
		f.common.inFuzzFn, f.inFuzzFn = true, true
		go tRunner(t, func(t *T) {
			args := []reflect.Value{reflect.ValueOf(t)}
			for _, v := range e.Values {
				args = append(args, reflect.ValueOf(v))
			}
			// 在重置当前覆盖率之前，延迟快照，以确保它在 tRunner 函数
			// 退出之前被调用，无论是干净执行、发生 panic，
			// 还是 fuzzFn 调用了 t.Fatal。
			if f.tstate.isFuzzing {
				defer f.fstate.deps.SnapshotCoverage()
				f.fstate.deps.ResetCoverage()
			}
			fn.Call(args)
		})
		<-t.signal
		if t.chatty != nil && t.chatty.json {
			t.chatty.Updatef(t.parent.name, "=== NAME  %s\n", t.parent.name)
		}
		f.common.inFuzzFn, f.inFuzzFn = false, false
		return !t.Failed()
	}

	switch f.fstate.mode {
	case fuzzCoordinator:
		// 模糊测试已启用，这是由 'go test' 启动的测试进程。
		// 充当协调器进程，协调工作进程执行实际的模糊测试。
		corpusTargetDir := filepath.Join(corpusDir, f.name)
		cacheTargetDir := filepath.Join(*fuzzCacheDir, f.name)
		err := f.fstate.deps.CoordinateFuzzing(
			fuzzDuration.d,
			int64(fuzzDuration.n),
			minimizeDuration.d,
			int64(minimizeDuration.n),
			*parallel,
			f.corpus,
			types,
			corpusTargetDir,
			cacheTargetDir)
		if err != nil {
			f.result = fuzzResult{Error: err}
			f.Fail()
			fmt.Fprintf(f.w, "%v\n", err)
			if crashErr, ok := err.(fuzzCrashError); ok {
				crashPath := crashErr.CrashPath()
				fmt.Fprintf(f.w, "Failing input written to %s\n", crashPath)
				testName := filepath.Base(crashPath)
				fmt.Fprintf(f.w, "To re-run:\ngo test -run=%s/%s\n", f.name, testName)
			}
		}
		// TODO(jayconrod,katiehockman): 跨工作进程聚合统计信息
		// 并添加到 FuzzResult（即所花时间、迭代次数）

	case fuzzWorker:
		// 模糊测试已启用，这是一个工作进程。遵循协调器的指令。
		if err := f.fstate.deps.RunFuzzWorker(func(e corpusEntry) error {
			// 如果从模糊测试工作进程运行，不要写入 f.w（指向 Stdout）。
			// 这会变得非常冗长，特别是在最小化期间。
			// 改为返回错误，让调用者处理输出。
			var buf strings.Builder
			if ok := run(&buf, e); !ok {
				return errors.New(buf.String())
			}
			return nil
		}); err != nil {
			// 内部错误用 f.Fail 标记；用户代码也可能在 F.Fuzz 之前调用它。
			// 工作进程将以 fuzzWorkerExitCode 退出，表示这是一个失败
			// （'go test' 应该以非零退出）但不应记录失败的输入。
			f.Errorf("communicating with fuzzing coordinator: %v", err)
		}

	default:
		// 模糊测试未启用，或将稍后执行。现在只运行种子语料库。
		for _, e := range f.corpus {
			name := fmt.Sprintf("%s/%s", f.name, filepath.Base(e.Path))
			if _, ok, _ := f.tstate.match.fullName(nil, name); ok {
				run(f.w, e)
			}
		}
	}
}

func (f *F) report() {
	if *isFuzzWorker || f.parent == nil {
		return
	}
	dstr := fmtDuration(f.duration)
	format := "--- %s: %s (%s)\n"
	if f.Failed() {
		f.flushToParent(f.name, format, "FAIL", f.name, dstr)
	} else if f.chatty != nil {
		if f.Skipped() {
			f.flushToParent(f.name, format, "SKIP", f.name, dstr)
		} else {
			f.flushToParent(f.name, format, "PASS", f.name, dstr)
		}
	}
}

// fuzzResult 包含模糊测试运行的结果。
type fuzzResult struct {
	N     int           // 迭代次数。
	T     time.Duration // 总耗时。
	Error error         // Error 是来自失败输入的错误
}

func (r fuzzResult) String() string {
	if r.Error == nil {
		return ""
	}
	return r.Error.Error()
}

// fuzzCrashError 由模糊测试期间检测到的失败输入满足。
// 这些错误被写入种子语料库，可以使用 'go test' 重新运行。
// 模糊测试框架内的错误（如协调器和工作进程之间的 I/O 错误）
// 不满足此接口。
type fuzzCrashError interface {
	error
	Unwrap() error

	// CrashPath 返回与种子语料库中保存的崩溃输入文件对应的子测试路径。
	// 可以使用 go test -run=$test/$name 重新运行测试，
	// $test 是模糊测试名称，$name 是此处返回字符串的 filepath.Base。
	CrashPath() string
}

// fuzzState 保存所有模糊测试通用的字段。
type fuzzState struct {
	deps testDeps
	mode fuzzMode
}

type fuzzMode uint8

const (
	seedCorpusOnly fuzzMode = iota
	fuzzCoordinator
	fuzzWorker
)

// runFuzzTests 运行与 -run 模式匹配的模糊测试。这将
// 仅为每个种子语料库运行 (*F).Fuzz 函数，而不使用
// 模糊引擎来生成或变异输入。
func runFuzzTests(deps testDeps, fuzzTests []InternalFuzzTarget, deadline time.Time) (ran, ok bool) {
	ok = true
	if len(fuzzTests) == 0 || *isFuzzWorker {
		return ran, ok
	}
	m := newMatcher(deps.MatchString, *match, "-test.run", *skip)
	var mFuzz *matcher
	if *matchFuzz != "" {
		mFuzz = newMatcher(deps.MatchString, *matchFuzz, "-test.fuzz", *skip)
	}

	for _, procs := range cpuList {
		runtime.GOMAXPROCS(procs)
		for i := uint(0); i < *count; i++ {
			if shouldFailFast() {
				break
			}

			tstate := newTestState(*parallel, m)
			tstate.deadline = deadline
			fstate := &fuzzState{deps: deps, mode: seedCorpusOnly}
			root := common{w: os.Stdout} // gather output in one place
			if Verbose() {
				root.chatty = newChattyPrinter(root.w)
			}
			for _, ft := range fuzzTests {
				if shouldFailFast() {
					break
				}
				testName, matched, _ := tstate.match.fullName(nil, ft.Name)
				if !matched {
					continue
				}
				if mFuzz != nil {
					if _, fuzzMatched, _ := mFuzz.fullName(nil, ft.Name); fuzzMatched {
						// 如果这将进行模糊测试，那么现在不要运行种子语料库。
						// 那将在稍后发生。
						continue
					}
				}
				ctx, cancelCtx := context.WithCancel(context.Background())
				f := &F{
					common: common{
						signal:    make(chan bool),
						barrier:   make(chan bool),
						name:      testName,
						parent:    &root,
						level:     root.level + 1,
						chatty:    root.chatty,
						ctx:       ctx,
						cancelCtx: cancelCtx,
					},
					tstate: tstate,
					fstate: fstate,
				}
				f.w = indenter{&f.common}
				f.setOutputWriter()
				if f.chatty != nil {
					f.chatty.Updatef(f.name, "=== RUN   %s\n", f.name)
				}
				go fRunner(f, ft.Fn)
				<-f.signal
				if f.chatty != nil && f.chatty.json {
					f.chatty.Updatef(f.parent.name, "=== NAME  %s\n", f.parent.name)
				}
				ok = ok && !f.Failed()
				ran = ran || f.ran
			}
			if !ran {
				// 此次迭代没有要运行的测试。
				// 这不会改变，所以没有理由继续尝试。
				break
			}
		}
	}

	return ran, ok
}

// runFuzzing 运行与 -fuzz 模式匹配的模糊测试。只有一个这样的
// 模糊测试必须匹配。这将运行模糊引擎来生成和变异
// 针对模糊目标的新输入。
//
// 如果模糊测试被禁用（未设置 -test.fuzz），runFuzzing
// 立即返回。
func runFuzzing(deps testDeps, fuzzTests []InternalFuzzTarget) (ok bool) {
	if len(fuzzTests) == 0 || *matchFuzz == "" {
		return true
	}
	m := newMatcher(deps.MatchString, *matchFuzz, "-test.fuzz", *skip)
	tstate := newTestState(1, m)
	tstate.isFuzzing = true
	fstate := &fuzzState{
		deps: deps,
	}
	root := common{w: os.Stdout}
	if *isFuzzWorker {
		root.w = io.Discard
		fstate.mode = fuzzWorker
	} else {
		fstate.mode = fuzzCoordinator
	}
	if Verbose() && !*isFuzzWorker {
		root.chatty = newChattyPrinter(root.w)
	}
	var fuzzTest *InternalFuzzTarget
	var testName string
	var matched []string
	for i := range fuzzTests {
		name, ok, _ := tstate.match.fullName(nil, fuzzTests[i].Name)
		if !ok {
			continue
		}
		matched = append(matched, name)
		fuzzTest = &fuzzTests[i]
		testName = name
	}
	if len(matched) == 0 {
		fmt.Fprintln(os.Stderr, "testing: warning: no fuzz tests to fuzz")
		return true
	}
	if len(matched) > 1 {
		fmt.Fprintf(os.Stderr, "testing: will not fuzz, -fuzz matches more than one fuzz test: %v\n", matched)
		return false
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	f := &F{
		common: common{
			signal:    make(chan bool),
			barrier:   nil, // T.Parallel has no effect when fuzzing.
			name:      testName,
			parent:    &root,
			level:     root.level + 1,
			chatty:    root.chatty,
			ctx:       ctx,
			cancelCtx: cancelCtx,
		},
		fstate: fstate,
		tstate: tstate,
	}
	f.w = indenter{&f.common}
	f.setOutputWriter()
	if f.chatty != nil {
		f.chatty.Updatef(f.name, "=== RUN   %s\n", f.name)
	}
	go fRunner(f, fuzzTest.Fn)
	<-f.signal
	if f.chatty != nil {
		f.chatty.Updatef(f.parent.name, "=== NAME  %s\n", f.parent.name)
	}
	return !f.failed
}

// fRunner 包装对模糊测试的调用，并确保调用清理函数
// 和设置状态标志。fRunner 应该在其自己的 goroutine 中调用。
// 要等待其完成，请从 f.signal 接收。
//
// fRunner 类似于 tRunner，后者包装使用 T.Run 启动的子测试。
// 单元测试和模糊测试的工作方式略有不同，所以目前这些函数
// 没有合并。特别是，因为没有 F.Run 和 F.Parallel 方法，
// 即没有模糊子测试或并行模糊测试，所以做了一些简化。
// 我们还要求调用 F.Fuzz、F.Skip 或 F.Fail。
func fRunner(f *F, fn func(*F)) {
	// 当此 goroutine 完成时，无论是因为调用了 runtime.Goexit、
	// 开始了 panic，还是 fn 正常返回，都记录持续时间并发送
	// t.signal，表示模糊测试完成。
	defer func() {
		// 检测模糊测试是否在未调用 F.Fuzz、F.Fail 或 F.Skip 的情况下
		// 发生了 panic 或调用了 runtime.Goexit。如果是，则 panic
		// （可能替换 nil panic 值）。fRunner 展开后不应该有任何恢复，
		// 所以这应该会崩溃进程并打印栈。不幸的是，在这里恢复会添加
		// 栈帧，但原始 panic 的位置应该仍然清晰。
		f.checkRaces()
		if f.Failed() {
			numFailed.Add(1)
		}
		err := recover()
		if err == nil {
			f.mu.RLock()
			fuzzNotCalled := !f.fuzzCalled && !f.skipped && !f.failed
			if !f.finished && !f.skipped && !f.failed {
				err = errNilPanicOrGoexit
			}
			f.mu.RUnlock()
			if fuzzNotCalled && err == nil {
				f.Error("returned without calling F.Fuzz, F.Fail, or F.Skip")
			}
		}

		// Use a deferred call to ensure that we report that the test is
		// complete even if a cleanup function calls F.FailNow. See issue 41355.
		didPanic := false
		defer func() {
			if !didPanic {
				// Only report that the test is complete if it doesn't panic,
				// as otherwise the test binary can exit before the panic is
				// reported to the user. See issue 41479.
				f.signal <- true
			}
		}()

		// If we recovered a panic or inappropriate runtime.Goexit, fail the test,
		// flush the output log up to the root, then panic.
		doPanic := func(err any) {
			f.Fail()
			if r := f.runCleanup(recoverAndReturnPanic); r != nil {
				f.Logf("cleanup panicked with %v", r)
			}
			for root := &f.common; root.parent != nil; root = root.parent {
				root.mu.Lock()
				root.duration += highPrecisionTimeSince(root.start)
				d := root.duration
				root.mu.Unlock()
				root.flushToParent(root.name, "--- FAIL: %s (%s)\n", root.name, fmtDuration(d))
			}
			didPanic = true
			panic(err)
		}
		if err != nil {
			doPanic(err)
		}

		// No panic or inappropriate Goexit.
		f.duration += highPrecisionTimeSince(f.start)

		if len(f.sub) > 0 {
			// Unblock inputs that called T.Parallel while running the seed corpus.
			// This only affects fuzz tests run as normal tests.
			// While fuzzing, T.Parallel has no effect, so f.sub is empty, and this
			// branch is not taken. f.barrier is nil in that case.
			f.tstate.release()
			close(f.barrier)
			// Wait for the subtests to complete.
			for _, sub := range f.sub {
				<-sub.signal
			}
			cleanupStart := highPrecisionTimeNow()
			err := f.runCleanup(recoverAndReturnPanic)
			f.duration += highPrecisionTimeSince(cleanupStart)
			if err != nil {
				doPanic(err)
			}
		}

		// Report after all subtests have finished.
		f.report()
		f.done = true
		f.setRan()
	}()
	defer func() {
		if len(f.sub) == 0 {
			f.runCleanup(normalPanic)
		}
	}()

	f.start = highPrecisionTimeNow()
	f.resetRaces()
	fn(f)

	// Code beyond this point will not be executed when FailNow or SkipNow
	// is invoked.
	f.mu.Lock()
	f.finished = true
	f.mu.Unlock()
}
