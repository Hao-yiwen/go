// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"unicode"
	"unicode/utf8"
)

// TextHandler 是一个 [Handler]，它将 Record 写入 [io.Writer]，作为由空格分隔且后跟换行符的 key=value 对序列。
type TextHandler struct {
	*commonHandler
}

// NewTextHandler 创建一个 [TextHandler]，它写入 w，使用给定的选项。
// 如果 opts 为 nil，则使用默认选项。
func NewTextHandler(w io.Writer, opts *HandlerOptions) *TextHandler {
	if opts == nil {
		opts = &HandlerOptions{}
	}
	return &TextHandler{
		&commonHandler{
			json: false,
			w:    w,
			opts: *opts,
			mu:   &sync.Mutex{},
		},
	}
}

// Enabled 报告处理程序是否处理给定级别的记录。
// 处理程序忽略级别较低的记录。
func (h *TextHandler) Enabled(_ context.Context, level Level) bool {
	return h.commonHandler.enabled(level)
}

// WithAttrs 返回一个新的 [TextHandler]，其属性由 h 的属性后跟 attrs 组成。
func (h *TextHandler) WithAttrs(attrs []Attr) Handler {
	return &TextHandler{commonHandler: h.commonHandler.withAttrs(attrs)}
}

func (h *TextHandler) WithGroup(name string) Handler {
	return &TextHandler{commonHandler: h.commonHandler.withGroup(name)}
}

// Handle 将其参数 [Record] 格式化为由空格分隔的 key=value 项的单行。
//
// 如果 Record 的时间为零，则省略时间。否则，键是 "time"，值以 RFC3339 格式（毫秒精度）输出。
//
// 级别的键是 "level"，其值是调用 [Level.String] 的结果。
//
// 如果设置了 AddSource 选项且可用源信息，键是 "source"，值作为 FILE:LINE 输出。
//
// 消息的键是 "msg"。
//
// 要修改这些或其他属性，或从输出中删除它们，请使用 [HandlerOptions.ReplaceAttr]。
//
// 如果值实现了 [encoding.TextMarshaler]，则写入 MarshalText 的结果。
// 否则，写入 [fmt.Sprint] 的结果。
//
// 如果键和值包含 Unicode 空格字符、非打印字符、'"' 或 '='，则使用 [strconv.Quote] 引用它们。
//
// 组内的键由由点分隔的组件 (键或组名) 组成。不执行进一步的转义。
// 因此，无法从键 "a.b.c" 确定是否有两个组 "a" 和 "b" 及一个键 "c"，
// 或单个组 "a.b" 和键 "c"，或单个组 "a" 和键 "b.c"。
// 如果需要重构键的组结构，即使在组件内的点存在的情况下，
// 使用 [HandlerOptions.ReplaceAttr] 在键中编码该信息。
//
// 对 Handle 的每次调用都会导致对 io.Writer.Write 的单一序列化调用。
func (h *TextHandler) Handle(_ context.Context, r Record) error {
	return h.commonHandler.handle(r)
}

func appendTextValue(s *handleState, v Value) error {
	switch v.Kind() {
	case KindString:
		s.appendString(v.str())
	case KindTime:
		s.appendTime(v.time())
	case KindAny:
		if tm, ok := v.any.(encoding.TextMarshaler); ok {
			data, err := tm.MarshalText()
			if err != nil {
				return err
			}
			// TODO: avoid the conversion to string.
			s.appendString(string(data))
			return nil
		}
		if bs, ok := byteSlice(v.any); ok {
			// As of Go 1.19, this only allocates for strings longer than 32 bytes.
			s.buf.WriteString(strconv.Quote(string(bs)))
			return nil
		}
		s.appendString(fmt.Sprintf("%+v", v.Any()))
	default:
		*s.buf = v.append(*s.buf)
	}
	return nil
}

// byteSlice returns its argument as a []byte if the argument's
// underlying type is []byte, along with a second return value of true.
// Otherwise it returns nil, false.
func byteSlice(a any) ([]byte, bool) {
	if bs, ok := a.([]byte); ok {
		return bs, true
	}
	// Like Printf's %s, we allow both the slice type and the byte element type to be named.
	t := reflect.TypeOf(a)
	if t != nil && t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
		return reflect.ValueOf(a).Bytes(), true
	}
	return nil, false
}

func needsQuoting(s string) bool {
	if len(s) == 0 {
		return true
	}
	for i := 0; i < len(s); {
		b := s[i]
		if b < utf8.RuneSelf {
			// Quote anything except a backslash that would need quoting in a
			// JSON string, as well as space and '='
			if b != '\\' && (b == ' ' || b == '=' || !safeSet[b]) {
				return true
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError || unicode.IsSpace(r) || !unicode.IsPrint(r) {
			return true
		}
		i += size
	}
	return false
}
