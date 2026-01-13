// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package slogtest 实现了对 log/slog.Handler 实现进行测试的支持。
package slogtest

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"runtime"
	"testing"
	"time"
)

type testCase struct {
	// 子测试名称。
	name string
	// 如果非空，explanation 解释违反的约束。
	explanation string
	// f 使用其参数 logger 执行单个日志事件。
	// 为了使 mkdescs.sh 能够生成正确的描述，
	// f 的主体必须出现在单行上，其第一个
	// 非空白字符是 "l."。
	f func(*slog.Logger)
	// 如果 mod 不为 nil，则在将生成的 Record
	// 传递给 Handler 之前调用它来修改该 Record。
	mod func(*slog.Record)
	// checks 是要在结果上运行的检查列表。
	checks []check
}

var cases = []testCase{
	{
		name:        "built-ins",
		explanation: withSource("this test expects slog.TimeKey, slog.LevelKey and slog.MessageKey"),
		f: func(l *slog.Logger) {
			l.Info("message")
		},
		checks: []check{
			hasKey(slog.TimeKey),
			hasKey(slog.LevelKey),
			hasAttr(slog.MessageKey, "message"),
		},
	},
	{
		name:        "attrs",
		explanation: withSource("a Handler should output attributes passed to the logging function"),
		f: func(l *slog.Logger) {
			l.Info("message", "k", "v")
		},
		checks: []check{
			hasAttr("k", "v"),
		},
	},
	{
		name:        "empty-attr",
		explanation: withSource("a Handler should ignore an empty Attr"),
		f: func(l *slog.Logger) {
			l.Info("msg", "a", "b", "", nil, "c", "d")
		},
		checks: []check{
			hasAttr("a", "b"),
			missingKey(""),
			hasAttr("c", "d"),
		},
	},
	{
		name:        "zero-time",
		explanation: withSource("a Handler should ignore a zero Record.Time"),
		f: func(l *slog.Logger) {
			l.Info("msg", "k", "v")
		},
		mod: func(r *slog.Record) { r.Time = time.Time{} },
		checks: []check{
			missingKey(slog.TimeKey),
		},
	},
	{
		name:        "WithAttrs",
		explanation: withSource("a Handler should include the attributes from the WithAttrs method"),
		f: func(l *slog.Logger) {
			l.With("a", "b").Info("msg", "k", "v")
		},
		checks: []check{
			hasAttr("a", "b"),
			hasAttr("k", "v"),
		},
	},
	{
		name:        "groups",
		explanation: withSource("a Handler should handle Group attributes"),
		f: func(l *slog.Logger) {
			l.Info("msg", "a", "b", slog.Group("G", slog.String("c", "d")), "e", "f")
		},
		checks: []check{
			hasAttr("a", "b"),
			inGroup("G", hasAttr("c", "d")),
			hasAttr("e", "f"),
		},
	},
	{
		name:        "empty-group",
		explanation: withSource("a Handler should ignore an empty group"),
		f: func(l *slog.Logger) {
			l.Info("msg", "a", "b", slog.Group("G"), "e", "f")
		},
		checks: []check{
			hasAttr("a", "b"),
			missingKey("G"),
			hasAttr("e", "f"),
		},
	},
	{
		name:        "inline-group",
		explanation: withSource("a Handler should inline the Attrs of a group with an empty key"),
		f: func(l *slog.Logger) {
			l.Info("msg", "a", "b", slog.Group("", slog.String("c", "d")), "e", "f")

		},
		checks: []check{
			hasAttr("a", "b"),
			hasAttr("c", "d"),
			hasAttr("e", "f"),
		},
	},
	{
		name:        "WithGroup",
		explanation: withSource("a Handler should handle the WithGroup method"),
		f: func(l *slog.Logger) {
			l.WithGroup("G").Info("msg", "a", "b")
		},
		checks: []check{
			hasKey(slog.TimeKey),
			hasKey(slog.LevelKey),
			hasAttr(slog.MessageKey, "msg"),
			missingKey("a"),
			inGroup("G", hasAttr("a", "b")),
		},
	},
	{
		name:        "multi-With",
		explanation: withSource("a Handler should handle multiple WithGroup and WithAttr calls"),
		f: func(l *slog.Logger) {
			l.With("a", "b").WithGroup("G").With("c", "d").WithGroup("H").Info("msg", "e", "f")
		},
		checks: []check{
			hasKey(slog.TimeKey),
			hasKey(slog.LevelKey),
			hasAttr(slog.MessageKey, "msg"),
			hasAttr("a", "b"),
			inGroup("G", hasAttr("c", "d")),
			inGroup("G", inGroup("H", hasAttr("e", "f"))),
		},
	},
	{
		name:        "empty-group-record",
		explanation: withSource("a Handler should not output groups if there are no attributes"),
		f: func(l *slog.Logger) {
			l.With("a", "b").WithGroup("G").With("c", "d").WithGroup("H").Info("msg")
		},
		checks: []check{
			hasKey(slog.TimeKey),
			hasKey(slog.LevelKey),
			hasAttr(slog.MessageKey, "msg"),
			hasAttr("a", "b"),
			inGroup("G", hasAttr("c", "d")),
			inGroup("G", missingKey("H")),
		},
	},
	{
		name:        "nested-empty-group-record",
		explanation: withSource("a Handler should not output nested groups if there are no attributes"),
		f: func(l *slog.Logger) {
			l.With("a", "b").WithGroup("G").With("c", "d").WithGroup("H").WithGroup("I").Info("msg")
		},
		checks: []check{
			hasKey(slog.TimeKey),
			hasKey(slog.LevelKey),
			hasAttr(slog.MessageKey, "msg"),
			hasAttr("a", "b"),
			inGroup("G", hasAttr("c", "d")),
			inGroup("G", missingKey("H")),
			inGroup("G", missingKey("I")),
		},
	},
	{
		name:        "resolve",
		explanation: withSource("a Handler should call Resolve on attribute values"),
		f: func(l *slog.Logger) {
			l.Info("msg", "k", &replace{"replaced"})
		},
		checks: []check{hasAttr("k", "replaced")},
	},
	{
		name:        "resolve-groups",
		explanation: withSource("a Handler should call Resolve on attribute values in groups"),
		f: func(l *slog.Logger) {
			l.Info("msg",
				slog.Group("G",
					slog.String("a", "v1"),
					slog.Any("b", &replace{"v2"})))
		},
		checks: []check{
			inGroup("G", hasAttr("a", "v1")),
			inGroup("G", hasAttr("b", "v2")),
		},
	},
	{
		name:        "resolve-WithAttrs",
		explanation: withSource("a Handler should call Resolve on attribute values from WithAttrs"),
		f: func(l *slog.Logger) {
			l = l.With("k", &replace{"replaced"})
			l.Info("msg")
		},
		checks: []check{hasAttr("k", "replaced")},
	},
	{
		name:        "resolve-WithAttrs-groups",
		explanation: withSource("a Handler should call Resolve on attribute values in groups from WithAttrs"),
		f: func(l *slog.Logger) {
			l = l.With(slog.Group("G",
				slog.String("a", "v1"),
				slog.Any("b", &replace{"v2"})))
			l.Info("msg")
		},
		checks: []check{
			inGroup("G", hasAttr("a", "v1")),
			inGroup("G", hasAttr("b", "v2")),
		},
	},
	{
		name:        "empty-PC",
		explanation: withSource("a Handler should not output SourceKey if the PC is zero"),
		f: func(l *slog.Logger) {
			l.Info("message")
		},
		mod: func(r *slog.Record) { r.PC = 0 },
		checks: []check{
			missingKey(slog.SourceKey),
		},
	},
}

// TestHandler tests a [slog.Handler].
// If TestHandler finds any misbehaviors, it returns an error for each,
// combined into a single error with [errors.Join].
//
// TestHandler installs the given Handler in a [slog.Logger] and
// makes several calls to the Logger's output methods.
// The Handler should be enabled for levels Info and above.
//
// The results function is invoked after all such calls.
// It should return a slice of map[string]any, one for each call to a Logger output method.
// The keys and values of the map should correspond to the keys and values of the Handler's
// output. Each group in the output should be represented as its own nested map[string]any.
// The standard keys [slog.TimeKey], [slog.LevelKey] and [slog.MessageKey] should be used.
//
// If the Handler outputs JSON, then calling [encoding/json.Unmarshal] with a `map[string]any`
// will create the right data structure.
//
// If a Handler intentionally drops an attribute that is checked by a test,
// then the results function should check for its absence and add it to the map it returns.
func TestHandler(h slog.Handler, results func() []map[string]any) error {
	// Run the handler on the test cases.
	for _, c := range cases {
		ht := h
		if c.mod != nil {
			ht = &wrapper{h, c.mod}
		}
		l := slog.New(ht)
		c.f(l)
	}

	// Collect and check the results.
	var errs []error
	res := results()
	if g, w := len(res), len(cases); g != w {
		return fmt.Errorf("got %d results, want %d", g, w)
	}
	for i, got := range res {
		c := cases[i]
		for _, check := range c.checks {
			if problem := check(got); problem != "" {
				errs = append(errs, fmt.Errorf("%s: %s", problem, c.explanation))
			}
		}
	}
	return errors.Join(errs...)
}

// Run exercises a [slog.Handler] on the same test cases as [TestHandler], but
// runs each case in a subtest. For each test case, it first calls newHandler to
// get an instance of the handler under test, then runs the test case, then
// calls result to get the result. If the test case fails, it calls t.Error.
func Run(t *testing.T, newHandler func(*testing.T) slog.Handler, result func(*testing.T) map[string]any) {
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHandler(t)
			if c.mod != nil {
				h = &wrapper{h, c.mod}
			}
			l := slog.New(h)
			c.f(l)
			got := result(t)
			for _, check := range c.checks {
				if p := check(got); p != "" {
					t.Errorf("%s: %s", p, c.explanation)
				}
			}
		})
	}
}

type check func(map[string]any) string

func hasKey(key string) check {
	return func(m map[string]any) string {
		if _, ok := m[key]; !ok {
			return fmt.Sprintf("missing key %q", key)
		}
		return ""
	}
}

func missingKey(key string) check {
	return func(m map[string]any) string {
		if _, ok := m[key]; ok {
			return fmt.Sprintf("unexpected key %q", key)
		}
		return ""
	}
}

func hasAttr(key string, wantVal any) check {
	return func(m map[string]any) string {
		if s := hasKey(key)(m); s != "" {
			return s
		}
		gotVal := m[key]
		if !reflect.DeepEqual(gotVal, wantVal) {
			return fmt.Sprintf("%q: got %#v, want %#v", key, gotVal, wantVal)
		}
		return ""
	}
}

func inGroup(name string, c check) check {
	return func(m map[string]any) string {
		v, ok := m[name]
		if !ok {
			return fmt.Sprintf("missing group %q", name)
		}
		g, ok := v.(map[string]any)
		if !ok {
			return fmt.Sprintf("value for group %q is not map[string]any", name)
		}
		return c(g)
	}
}

type wrapper struct {
	slog.Handler
	mod func(*slog.Record)
}

func (h *wrapper) Handle(ctx context.Context, r slog.Record) error {
	h.mod(&r)
	return h.Handler.Handle(ctx, r)
}

func withSource(s string) string {
	_, file, line, ok := runtime.Caller(1)
	if !ok {
		panic("runtime.Caller failed")
	}
	return fmt.Sprintf("%s (%s:%d)", s, file, line)
}

type replace struct {
	v any
}

func (r *replace) LogValue() slog.Value { return slog.AnyValue(r.v) }

func (r *replace) String() string {
	return fmt.Sprintf("<replace(%v)>", r.v)
}
