// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"context"
	"fmt"
	"io"
	"log/slog/internal/buffer"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"time"
)

// Handler 处理由 Logger 生成的日志记录。
//
// 一个典型的处理程序可能将日志记录打印到标准错误，
// 或将它们写入文件或数据库，或者可能用额外的属性增强它们
// 并将它们传递给另一个处理程序。
//
// Handler 的任何方法都可能与自身或其他方法并发调用。
// 由 Handler 负责管理这种并发。
//
// slog 包的用户不应直接调用 Handler 方法。
// 他们应该使用 [Logger] 的方法。
//
// 在实现自己的处理程序之前，请查看 https://go.dev/s/slog-handler-guide。
type Handler interface {
	// Enabled 报告处理程序是否处理给定级别的记录。
	// 处理程序忽略级别较低的记录。
	// 它在早期调用，在任何参数处理之前，
	// 以在应该丢弃日志事件时节省工作。
	// 如果从 Logger 方法调用，第一个参数是传递给该方法的上下文，
	// 或者如果传递了 nil 或该方法不接受上下文，则为 context.Background()。
	// 传递上下文以便 Enabled 可以使用其值来做出决定。
	Enabled(context.Context, Level) bool

	// Handle 处理 Record。
	// 它仅在 Enabled 返回 true 时才会被调用。
	// Context 参数与 Enabled 相同。
	// 它仅用于为 Handler 提供对上下文值的访问。
	// 取消上下文不应该影响记录处理。
	// (除其他外，日志消息可能需要调试与取消相关的问题。)
	//
	// 生成输出的 Handle 方法应遵守以下规则：
	//   - 如果 r.Time 是零时间，忽略该时间。
	//   - 如果 r.PC 为零，忽略它。
	//   - Attr 的值应该被解析。
	//   - 如果 Attr 的键和值都是零值，忽略该 Attr。
	//     这可以通过 attr.Equal(Attr{}) 来测试。
	//   - 如果一个组的键为空，内联该组的 Attr。
	//   - 如果一个组没有 Attr (即使它有一个非空键)，忽略它。
	//
	// [Logger] 丢弃来自 Handle 的任何错误。包装 Handle 方法以处理来自 Handler 的任何错误。
	Handle(context.Context, Record) error

	// WithAttrs 返回一个新的 Handler，其属性由接收者的属性和参数组成。
	// Handler 拥有该切片：它可以保留、修改或丢弃它。
	WithAttrs(attrs []Attr) Handler

	// WithGroup 返回一个新的 Handler，其给定组已附加到接收者的现有组。
	// 所有后续属性的键，无论是通过 With 添加还是在 Record 中，
	// 都应由组名的序列限定。
	//
	// 此限定如何发生取决于 Handler，只要此 Handler 的属性键
	// 与具有不同组名序列的另一个 Handler 的属性键不同。
	//
	// Handler 应将 WithGroup 视为开始一个 Attr 组，该组在日志事件的末尾结束。也就是说，
	//
	//     logger.WithGroup("s").LogAttrs(ctx, level, msg, slog.Int("a", 1), slog.Int("b", 2))
	//
	// 应该表现得像
	//
	//     logger.LogAttrs(ctx, level, msg, slog.Group("s", slog.Int("a", 1), slog.Int("b", 2)))
	//
	// 如果名称为空，WithGroup 返回接收者。
	WithGroup(name string) Handler
}

type defaultHandler struct {
	ch *commonHandler
	// internal.DefaultOutput，除了测试外
	output func(pc uintptr, data []byte) error
}

func newDefaultHandler(output func(uintptr, []byte) error) *defaultHandler {
	return &defaultHandler{
		ch:     &commonHandler{json: false},
		output: output,
	}
}

func (*defaultHandler) Enabled(_ context.Context, l Level) bool {
	return l >= logLoggerLevel.Level()
}

// 在字符串中收集级别、属性和消息，
// 并使用默认的 log.Logger 写入。
// 让 log.Logger 处理时间和文件/行。
func (h *defaultHandler) Handle(ctx context.Context, r Record) error {
	buf := buffer.New()
	buf.WriteString(r.Level.String())
	buf.WriteByte(' ')
	buf.WriteString(r.Message)
	state := h.ch.newHandleState(buf, true, " ")
	defer state.free()
	state.appendNonBuiltIns(r)
	return h.output(r.PC, *buf)
}

func (h *defaultHandler) WithAttrs(as []Attr) Handler {
	return &defaultHandler{h.ch.withAttrs(as), h.output}
}

func (h *defaultHandler) WithGroup(name string) Handler {
	return &defaultHandler{h.ch.withGroup(name), h.output}
}

// HandlerOptions 是 [TextHandler] 或 [JSONHandler] 的选项。
// 零 HandlerOptions 完全由默认值组成。
type HandlerOptions struct {
	// AddSource 导致处理程序计算日志语句的源代码位置，
	// 并向输出添加 SourceKey 属性。
	AddSource bool

	// Level 报告将被记录的最小记录级别。
	// 处理程序丢弃较低级别的记录。
	// 如果 Level 为 nil，处理程序假定 LevelInfo。
	// 处理程序为处理的每条记录调用 Level.Level；
	// 要动态调整最小级别，请使用 LevelVar。
	Level Leveler

	// ReplaceAttr 被调用以在记录之前重写每个非组属性。
	// 属性的值已被解析 (参见 [Value.Resolve])。
	// 如果 ReplaceAttr 返回零 Attr，该属性将被丢弃。
	//
	// 键为 "time"、"level"、"source" 和 "msg" 的内置属性
	// 被传递给此函数，除了如果时间为零则省略时间，
	// 如果 AddSource 为 false 则省略源。
	//
	// 第一个参数是当前包含该 Attr 的打开组的列表。
	// 它不得被保留或修改。ReplaceAttr 从不为组属性调用，
	// 仅为它们的内容调用。例如，属性列表
	//
	//     Int("a", 1), Group("g", Int("b", 2)), Int("c", 3)
	//
	// 导致连续调用 ReplaceAttr，其参数如下：
	//
	//     nil, Int("a", 1)
	//     []string{"g"}, Int("b", 2)
	//     nil, Int("c", 3)
	//
	// ReplaceAttr 可用于更改内置属性的默认键、转换类型
	// (例如，将 `time.Time` 替换为自 Unix 纪元以来的整数秒)、
	// 清理个人信息或从输出中移除属性。
	ReplaceAttr func(groups []string, a Attr) Attr
}

// "内置"属性的键。
const (
	// TimeKey 是内置处理程序用于调用日志方法的时间的键。
	// 关联的 Value 是 [time.Time]。
	TimeKey = "time"
	// LevelKey 是内置处理程序用于日志调用的级别的键。
	// 关联的值是 [Level]。
	LevelKey = "level"
	// MessageKey 是内置处理程序用于日志调用的消息的键。
	// 关联的值是字符串。
	MessageKey = "msg"
	// SourceKey 是内置处理程序用于日志调用的源文件和行的键。
	// 关联的值是 *[Source]。
	SourceKey = "source"
)

type commonHandler struct {
	json              bool // true => 输出 JSON；false => 输出文本
	opts              HandlerOptions
	preformattedAttrs []byte
	// groupPrefix 仅用于文本处理程序。
	// 它保存已预格式化的组的前缀。
	// 当 WithGroup 的调用后跟 WithAttrs 的调用时，组将出现在这里。
	groupPrefix string
	groups      []string // 从 WithGroup 开始的所有组
	nOpenGroups int      // preformattedAttrs 中打开的组的数量
	mu          *sync.Mutex
	w           io.Writer
}

func (h *commonHandler) clone() *commonHandler {
	// 我们不能使用赋值，因为我们不能复制互斥锁。
	return &commonHandler{
		json:              h.json,
		opts:              h.opts,
		preformattedAttrs: slices.Clip(h.preformattedAttrs),
		groupPrefix:       h.groupPrefix,
		groups:            slices.Clip(h.groups),
		nOpenGroups:       h.nOpenGroups,
		w:                 h.w,
		mu:                h.mu, // 在此处理程序的所有克隆之间共享的互斥锁
	}
}

// enabled 报告 l 是否大于或等于最小级别。
func (h *commonHandler) enabled(l Level) bool {
	minLevel := LevelInfo
	if h.opts.Level != nil {
		minLevel = h.opts.Level.Level()
	}
	return l >= minLevel
}

func (h *commonHandler) withAttrs(as []Attr) *commonHandler {
	// 我们将忽略空组，因此如果整个切片由它们组成，则无需做任何事情。
	if countEmptyGroups(as) == len(as) {
		return h
	}
	h2 := h.clone()
	// 作为优化预格式化属性。
	state := h2.newHandleState((*buffer.Buffer)(&h2.preformattedAttrs), false, "")
	defer state.free()
	state.prefix.WriteString(h.groupPrefix)
	if pfa := h2.preformattedAttrs; len(pfa) > 0 {
		state.sep = h.attrSep()
		if h2.json && pfa[len(pfa)-1] == '{' {
			state.sep = ""
		}
	}
	// 记住缓冲区中的位置，以防所有属性都为空。
	pos := state.buf.Len()
	state.openGroups()
	if !state.appendAttrs(as) {
		state.buf.SetLen(pos)
	} else {
		// 为后续键记住新的前缀。
		h2.groupPrefix = state.prefix.String()
		// 记住 preformattedAttrs 中打开了多少个组，
		// 以便在处理 Record 时不再次打开它们。
		h2.nOpenGroups = len(h2.groups)
	}
	return h2
}

func (h *commonHandler) withGroup(name string) *commonHandler {
	h2 := h.clone()
	h2.groups = append(h2.groups, name)
	return h2
}

// handle 是 Handler.Handle 的内部实现，由 TextHandler 和 JSONHandler 使用。
func (h *commonHandler) handle(r Record) error {
	state := h.newHandleState(buffer.New(), true, "")
	defer state.free()
	if h.json {
		state.buf.WriteByte('{')
	}
	// Built-in attributes. They are not in a group.
	stateGroups := state.groups
	state.groups = nil // So ReplaceAttrs sees no groups instead of the pre groups.
	rep := h.opts.ReplaceAttr
	// time
	if !r.Time.IsZero() {
		key := TimeKey
		val := r.Time.Round(0) // strip monotonic to match Attr behavior
		if rep == nil {
			state.appendKey(key)
			state.appendTime(val)
		} else {
			state.appendAttr(Time(key, val))
		}
	}
	// level
	key := LevelKey
	val := r.Level
	if rep == nil {
		state.appendKey(key)
		state.appendString(val.String())
	} else {
		state.appendAttr(Any(key, val))
	}
	// source
	if h.opts.AddSource {
		src := r.Source()
		if src == nil {
			src = &Source{}
		}
		state.appendAttr(Any(SourceKey, src))
	}
	key = MessageKey
	msg := r.Message
	if rep == nil {
		state.appendKey(key)
		state.appendString(msg)
	} else {
		state.appendAttr(String(key, msg))
	}
	state.groups = stateGroups // Restore groups passed to ReplaceAttrs.
	state.appendNonBuiltIns(r)
	state.buf.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := h.w.Write(*state.buf)
	return err
}

func (s *handleState) appendNonBuiltIns(r Record) {
	// 预格式化的属性
	if pfa := s.h.preformattedAttrs; len(pfa) > 0 {
		s.buf.WriteString(s.sep)
		s.buf.Write(pfa)
		s.sep = s.h.attrSep()
		if s.h.json && pfa[len(pfa)-1] == '{' {
			s.sep = ""
		}
	}
	// Record 中的属性 -- 与内置属性不同，它们在从 WithGroup 开始的组中。
	// 如果记录没有属性，不输出任何组。
	nOpenGroups := s.h.nOpenGroups
	if r.NumAttrs() > 0 {
		s.prefix.WriteString(s.h.groupPrefix)
		// 该组可能会被证明是空的，即使它有属性
		// (例如，ReplaceAttr 可能会删除所有属性)。
		// 所以请记住我们在缓冲区中的位置，以便在必要时恢复该位置。
		pos := s.buf.Len()
		s.openGroups()
		nOpenGroups = len(s.h.groups)
		empty := true
		r.Attrs(func(a Attr) bool {
			if s.appendAttr(a) {
				empty = false
			}
			return true
		})
		if empty {
			s.buf.SetLen(pos)
			nOpenGroups = s.h.nOpenGroups
		}
	}
	if s.h.json {
		// 关闭所有打开的组。
		for range s.h.groups[:nOpenGroups] {
			s.buf.WriteByte('}')
		}
		// 关闭顶级对象。
		s.buf.WriteByte('}')
	}
}

// attrSep 返回属性之间的分隔符。
func (h *commonHandler) attrSep() string {
	if h.json {
		return ","
	}
	return " "
}

// handleState 保存对 commonHandler.handle 的单个调用的状态。
// sep 的初始值决定是否在下一个键之前发出分隔符，之后它保持为 true。
type handleState struct {
	h       *commonHandler
	buf     *buffer.Buffer
	freeBuf bool           // 应该释放 buf 吗？
	sep     string         // 在下一个键之前写入的分隔符
	prefix  *buffer.Buffer // 对于文本：键前缀
	groups  *[]string      // 池分配的活跃组的切片，用于 ReplaceAttr
}

var groupPool = sync.Pool{New: func() any {
	s := make([]string, 0, 10)
	return &s
}}

func (h *commonHandler) newHandleState(buf *buffer.Buffer, freeBuf bool, sep string) handleState {
	s := handleState{
		h:       h,
		buf:     buf,
		freeBuf: freeBuf,
		sep:     sep,
		prefix:  buffer.New(),
	}
	if h.opts.ReplaceAttr != nil {
		s.groups = groupPool.Get().(*[]string)
		*s.groups = append(*s.groups, h.groups[:h.nOpenGroups]...)
	}
	return s
}

func (s *handleState) free() {
	if s.freeBuf {
		s.buf.Free()
	}
	if gs := s.groups; gs != nil {
		*gs = (*gs)[:0]
		groupPool.Put(gs)
	}
	s.prefix.Free()
}

func (s *handleState) openGroups() {
	for _, n := range s.h.groups[s.h.nOpenGroups:] {
		s.openGroup(n)
	}
}

// 组名和键的分隔符。
const keyComponentSep = '.'

// openGroup 使用给定的名称启动一个新的属性组。
func (s *handleState) openGroup(name string) {
	if s.h.json {
		s.appendKey(name)
		s.buf.WriteByte('{')
		s.sep = ""
	} else {
		s.prefix.WriteString(name)
		s.prefix.WriteByte(keyComponentSep)
	}
	// 为 ReplaceAttr 收集组名。
	if s.groups != nil {
		*s.groups = append(*s.groups, name)
	}
}

// closeGroup 结束具有给定名称的组。
func (s *handleState) closeGroup(name string) {
	if s.h.json {
		s.buf.WriteByte('}')
	} else {
		(*s.prefix) = (*s.prefix)[:len(*s.prefix)-len(name)-1 /* for keyComponentSep */]
	}
	s.sep = s.h.attrSep()
	if s.groups != nil {
		*s.groups = (*s.groups)[:len(*s.groups)-1]
	}
}

// appendAttrs 附加 Attr 的切片。
// 它报告是否附加了某些内容。
func (s *handleState) appendAttrs(as []Attr) bool {
	nonEmpty := false
	for _, a := range as {
		if s.appendAttr(a) {
			nonEmpty = true
		}
	}
	return nonEmpty
}

// appendAttr 附加 Attr 的键和值。
// 它处理替换和检查空键。
// 它报告是否附加了某些内容。
func (s *handleState) appendAttr(a Attr) bool {
	a.Value = a.Value.Resolve()
	if rep := s.h.opts.ReplaceAttr; rep != nil && a.Value.Kind() != KindGroup {
		var gs []string
		if s.groups != nil {
			gs = *s.groups
		}
		// a.Value 在调用 ReplaceAttr 之前被解析，所以用户不必这样做。
		a = rep(gs, a)
		// ReplaceAttr 函数可能返回一个未解析的 Attr。
		a.Value = a.Value.Resolve()
	}
	// 省略空 Attr。
	if a.isEmpty() {
		return false
	}
	// 特殊情况：Source。
	if v := a.Value; v.Kind() == KindAny {
		if src, ok := v.Any().(*Source); ok {
			if src.isEmpty() {
				return false
			}
			if s.h.json {
				a.Value = src.group()
			} else {
				a.Value = StringValue(fmt.Sprintf("%s:%d", src.File, src.Line))
			}
		}
	}
	if a.Value.Kind() == KindGroup {
		attrs := a.Value.Group()
		// 仅输出非空组。
		if len(attrs) > 0 {
			// 该组可能会被证明是空的，即使它有属性
			// (例如，ReplaceAttr 可能会删除所有属性)。
			// 所以请记住我们在缓冲区中的位置，以便在必要时恢复该位置。
			pos := s.buf.Len()
			// 内联具有空键的组。
			if a.Key != "" {
				s.openGroup(a.Key)
			}
			if !s.appendAttrs(attrs) {
				s.buf.SetLen(pos)
				return false
			}
			if a.Key != "" {
				s.closeGroup(a.Key)
			}
		}
	} else {
		s.appendKey(a.Key)
		s.appendValue(a.Value)
	}
	return true
}

func (s *handleState) appendError(err error) {
	s.appendString(fmt.Sprintf("!ERROR:%v", err))
}

func (s *handleState) appendKey(key string) {
	s.buf.WriteString(s.sep)
	if s.prefix != nil && len(*s.prefix) > 0 {
		s.appendTwoStrings(string(*s.prefix), key)
	} else {
		s.appendString(key)
	}
	if s.h.json {
		s.buf.WriteByte(':')
	} else {
		s.buf.WriteByte('=')
	}
	s.sep = s.h.attrSep()
}

// appendTwoStrings 实现 appendString(prefix + key)，但速度更快。
func (s *handleState) appendTwoStrings(x, y string) {
	buf := *s.buf
	switch {
	case s.h.json:
		buf.WriteByte('"')
		buf = appendEscapedJSONString(buf, x)
		buf = appendEscapedJSONString(buf, y)
		buf.WriteByte('"')
	case !needsQuoting(x) && !needsQuoting(y):
		buf.WriteString(x)
		buf.WriteString(y)
	default:
		buf = strconv.AppendQuote(buf, x+y)
	}
	*s.buf = buf
}

func (s *handleState) appendString(str string) {
	if s.h.json {
		s.buf.WriteByte('"')
		*s.buf = appendEscapedJSONString(*s.buf, str)
		s.buf.WriteByte('"')
	} else {
		// text
		if needsQuoting(str) {
			*s.buf = strconv.AppendQuote(*s.buf, str)
		} else {
			s.buf.WriteString(str)
		}
	}
}

func (s *handleState) appendValue(v Value) {
	defer func() {
		if r := recover(); r != nil {
			// 如果它因 nil 指针而恐慌，最可能的情况是
			// 编码。TextMarshaler 或错误无法防止 nil，
			// 在这种情况下 "<nil>" 似乎是可行的选择。
			//
			// 根据 fmt/print.go 中的代码进行改编。
			if v := reflect.ValueOf(v.any); v.Kind() == reflect.Pointer && v.IsNil() {
				s.appendString("<nil>")
				return
			}

			// 否则只打印原始恐慌消息。
			s.appendString(fmt.Sprintf("!PANIC: %v", r))
		}
	}()

	var err error
	if s.h.json {
		err = appendJSONValue(s, v)
	} else {
		err = appendTextValue(s, v)
	}
	if err != nil {
		s.appendError(err)
	}
}

func (s *handleState) appendTime(t time.Time) {
	if s.h.json {
		appendJSONTime(s, t)
	} else {
		*s.buf = appendRFC3339Millis(*s.buf, t)
	}
}

func appendRFC3339Millis(b []byte, t time.Time) []byte {
	// 根据 time.RFC3339Nano 进行格式化，因为它得到了高度优化，
	// 但将其截断以使用毫秒分辨率。
	// 不幸的是，该格式会修剪尾部的 0，所以添加 1/10 毫秒
	// 以保证小数点后恰好有 4 位数字。
	const prefixLen = len("2006-01-02T15:04:05.000")
	n := len(b)
	t = t.Truncate(time.Millisecond).Add(time.Millisecond / 10)
	b = t.AppendFormat(b, time.RFC3339Nano)
	b = append(b[:n+prefixLen], b[n+prefixLen+1:]...) // 删除第 4 位数字
	return b
}

// DiscardHandler 丢弃所有日志输出。
// DiscardHandler.Enabled 对所有级别返回 false。
var DiscardHandler Handler = discardHandler{}

type discardHandler struct{}

func (dh discardHandler) Enabled(context.Context, Level) bool  { return false }
func (dh discardHandler) Handle(context.Context, Record) error { return nil }
func (dh discardHandler) WithAttrs(attrs []Attr) Handler       { return dh }
func (dh discardHandler) WithGroup(name string) Handler        { return dh }
