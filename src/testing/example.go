// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package testing

import (
	"fmt"
	"runtime"
	"slices"
	"strings"
	"time"
)

type InternalExample struct {
	Name      string
	F         func()
	Output    string
	Unordered bool
}

// RunExamples 是一个内部函数，但由于跨包使用而被导出；
// 它是 "go test" 命令实现的一部分。
func RunExamples(matchString func(pat, str string) (bool, error), examples []InternalExample) (ok bool) {
	_, ok = runExamples(matchString, examples)
	return ok
}

func runExamples(matchString func(pat, str string) (bool, error), examples []InternalExample) (ran, ok bool) {
	ok = true

	m := newMatcher(matchString, *match, "-test.run", *skip)

	var eg InternalExample
	for _, eg = range examples {
		_, matched, _ := m.fullName(nil, eg.Name)
		if !matched {
			continue
		}
		ran = true
		if !runExample(eg) {
			ok = false
		}
	}

	return ran, ok
}

// processRunResult 计算运行示例测试结果的摘要和状态。
// stdout 是从测试的标准输出捕获的输出。
// recovered 是运行测试后调用 recover 的结果，以防它发生了 panic。
//
// 如果 stdout 与预期输出不匹配或 recovered 非空，它将把失败原因打印到标准输出。
// 如果测试是 chatty/verbose 的，它将把成功消息打印到标准输出。
// 如果 recovered 非空，它将使用该值 panic。
// 如果测试使用 nil panic，或调用了 runtime.Goexit，它将被
// 标记为失败并使用 errNilPanicOrGoexit panic
func (eg *InternalExample) processRunResult(stdout string, timeSpent time.Duration, finished bool, recovered any) (passed bool) {
	passed = true
	dstr := fmtDuration(timeSpent)
	var fail string
	got := strings.TrimSpace(stdout)
	want := strings.TrimSpace(eg.Output)
	if runtime.GOOS == "windows" {
		got = strings.ReplaceAll(got, "\r\n", "\n")
		want = strings.ReplaceAll(want, "\r\n", "\n")
	}
	if eg.Unordered {
		gotLines := slices.Sorted(strings.SplitSeq(got, "\n"))
		wantLines := slices.Sorted(strings.SplitSeq(want, "\n"))
		if !slices.Equal(gotLines, wantLines) && recovered == nil {
			fail = fmt.Sprintf("got:\n%s\nwant (unordered):\n%s\n", stdout, eg.Output)
		}
	} else {
		if got != want && recovered == nil {
			fail = fmt.Sprintf("got:\n%s\nwant:\n%s\n", got, want)
		}
	}
	if fail != "" || !finished || recovered != nil {
		fmt.Printf("%s--- FAIL: %s (%s)\n%s", chatty.prefix(), eg.Name, dstr, fail)
		passed = false
	} else if chatty.on {
		fmt.Printf("%s--- PASS: %s (%s)\n", chatty.prefix(), eg.Name, dstr)
	}

	if chatty.on && chatty.json {
		fmt.Printf("%s=== NAME   %s\n", chatty.prefix(), "")
	}

	if recovered != nil {
		// 通过 panic 传播之前恢复的结果。
		panic(recovered)
	} else if !finished {
		panic(errNilPanicOrGoexit)
	}

	return
}
