// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package slog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog/internal/buffer"
	"strconv"
	"sync"
	"time"
	"unicode/utf8"
)

// JSONHandler 是一个 [Handler]，它将 Record 写入 [io.Writer] 作为行分隔的 JSON 对象。
type JSONHandler struct {
	*commonHandler
}

// NewJSONHandler 创建一个 [JSONHandler]，它写入 w，使用给定的选项。
// 如果 opts 为 nil，则使用默认选项。
func NewJSONHandler(w io.Writer, opts *HandlerOptions) *JSONHandler {
	if opts == nil {
		opts = &HandlerOptions{}
	}
	return &JSONHandler{
		&commonHandler{
			json: true,
			w:    w,
			opts: *opts,
			mu:   &sync.Mutex{},
		},
	}
}

// Enabled 报告处理程序是否处理给定级别的记录。
// 处理程序忽略级别较低的记录。
func (h *JSONHandler) Enabled(_ context.Context, level Level) bool {
	return h.commonHandler.enabled(level)
}

// WithAttrs 返回一个新的 [JSONHandler]，其属性由 h 的属性后跟 attrs 组成。
func (h *JSONHandler) WithAttrs(attrs []Attr) Handler {
	return &JSONHandler{commonHandler: h.commonHandler.withAttrs(attrs)}
}

func (h *JSONHandler) WithGroup(name string) Handler {
	return &JSONHandler{commonHandler: h.commonHandler.withGroup(name)}
}

// Handle 将其参数 [Record] 格式化为单行上的 JSON 对象。
//
// 如果 Record 的时间为零，则省略时间。
// 否则，键是 "time"，值按 json.Marshal 的方式输出。
//
// 级别的键是 "level"，其值是调用 [Level.String] 的结果。
//
// 如果设置了 AddSource 选项且可用源信息，键是 "source"，
// 值是 [Source] 类型的记录。
//
// 消息的键是 "msg"。
//
// 要修改这些或其他属性，或从输出中删除它们，请使用 [HandlerOptions.ReplaceAttr]。
//
// 值的格式与 [encoding/json.Encoder]（SetEscapeHTML(false) 除外）一样，但有两个例外。
//
// 首先，其值为 error 类型的 Attr 被格式化为字符串，通过调用其 Error 方法。
// 只有 Attr 中的错误获得此特殊处理，
// 不是嵌入在结构体、切片、映射或其他由 [encoding/json] 包处理的数据结构中的错误。
//
// 其次，编码失败不会导致 Handle 返回错误。相反，错误消息被格式化为字符串。
//
// 对 Handle 的每次调用都会导致对 io.Writer.Write 的单一序列化调用。
func (h *JSONHandler) Handle(_ context.Context, r Record) error {
	return h.commonHandler.handle(r)
}

// 根据 time.Time.MarshalJSON 改编以避免分配。
func appendJSONTime(s *handleState, t time.Time) {
	if y := t.Year(); y < 0 || y >= 10000 {
		// RFC 3339 明确说明年份恰好是 4 位数字。
		// 参见 golang.org/issue/4556#c15 了解更多讨论。
		s.appendError(errors.New("time.Time year outside of range [0,9999]"))
	}
	s.buf.WriteByte('"')
	*s.buf = t.AppendFormat(*s.buf, time.RFC3339Nano)
	s.buf.WriteByte('"')
}

func appendJSONValue(s *handleState, v Value) error {
	switch v.Kind() {
	case KindString:
		s.appendString(v.str())
	case KindInt64:
		*s.buf = strconv.AppendInt(*s.buf, v.Int64(), 10)
	case KindUint64:
		*s.buf = strconv.AppendUint(*s.buf, v.Uint64(), 10)
	case KindFloat64:
		// json.Marshal 对浮点数的处理很有趣；它不总是匹配
		// strconv.AppendFloat。所以直接调用它。
		// 这很昂贵，但浮点数很少见。
		if err := appendJSONMarshal(s.buf, v.Float64()); err != nil {
			return err
		}
	case KindBool:
		*s.buf = strconv.AppendBool(*s.buf, v.Bool())
	case KindDuration:
		// 执行 json.Marshal 所做的操作。
		*s.buf = strconv.AppendInt(*s.buf, int64(v.Duration()), 10)
	case KindTime:
		s.appendTime(v.Time())
	case KindAny:
		a := v.Any()
		_, jm := a.(json.Marshaler)
		if err, ok := a.(error); ok && !jm {
			s.appendString(err.Error())
		} else {
			return appendJSONMarshal(s.buf, a)
		}
	default:
		panic(fmt.Sprintf("bad kind: %s", v.Kind()))
	}
	return nil
}

type jsonEncoder struct {
	buf *bytes.Buffer
	// 使用 json.Encoder 以避免转义 HTML。
	json *json.Encoder
}

var jsonEncoderPool = &sync.Pool{
	New: func() any {
		enc := &jsonEncoder{
			buf: new(bytes.Buffer),
		}
		enc.json = json.NewEncoder(enc.buf)
		enc.json.SetEscapeHTML(false)
		return enc
	},
}

func appendJSONMarshal(buf *buffer.Buffer, v any) error {
	j := jsonEncoderPool.Get().(*jsonEncoder)
	defer func() {
		// 为了减少峰值分配，仅将较小的缓冲区返回到池中。
		const maxBufferSize = 16 << 10
		if j.buf.Cap() > maxBufferSize {
			return
		}
		j.buf.Reset()
		jsonEncoderPool.Put(j)
	}()

	if err := j.json.Encode(v); err != nil {
		return err
	}

	bs := j.buf.Bytes()
	buf.Write(bs[:len(bs)-1]) // 删除最后的换行符
	return nil
}

// appendEscapedJSONString 为 JSON 转义 s 并将其附加到 buf。
// 它不用引号将字符串括起来。
//
// 根据 encoding/json/encode.go:encodeState.string 修改，
// escapeHTML 设置为 false。
func appendEscapedJSONString(buf []byte, s string) []byte {
	char := func(b byte) { buf = append(buf, b) }
	str := func(s string) { buf = append(buf, s...) }

	start := 0
	for i := 0; i < len(s); {
		if b := s[i]; b < utf8.RuneSelf {
			if safeSet[b] {
				i++
				continue
			}
			if start < i {
				str(s[start:i])
			}
			char('\\')
			switch b {
			case '\\', '"':
				char(b)
			case '\n':
				char('n')
			case '\r':
				char('r')
			case '\t':
				char('t')
			default:
				// 这对 < 0x20 的字节进行编码，除了 \t、\n 和 \r。
				str(`u00`)
				char(hex[b>>4])
				char(hex[b&0xF])
			}
			i++
			start = i
			continue
		}
		c, size := utf8.DecodeRuneInString(s[i:])
		if c == utf8.RuneError && size == 1 {
			if start < i {
				str(s[start:i])
			}
			str(`\ufffd`)
			i += size
			start = i
			continue
		}
		// U+2028 是行分隔符。
		// U+2029 是段落分隔符。
		// 它们在技术上都是 JSON 字符串中的有效字符，
		// 但在 JSONP 中不起作用，JSONP 必须作为 JavaScript 进行评估，
		// 并且可能在那里导致安全漏洞。转义它们在 JSON 中是有效的，所以我们无条件地这样做。
		// 参见 http://timelessrepo.com/json-isnt-a-javascript-subset 了解讨论。
		if c == '\u2028' || c == '\u2029' {
			if start < i {
				str(s[start:i])
			}
			str(`\u202`)
			char(hex[c&0xF])
			i += size
			start = i
			continue
		}
		i += size
	}
	if start < len(s) {
		str(s[start:])
	}
	return buf
}

const hex = "0123456789abcdef"

// 从 encoding/json/tables.go 复制。
//
// safeSet 保存值 true，如果具有给定数组位置的 ASCII 字符
// 可以在 JSON 字符串内部表示而无需任何进一步的转义。
//
// 除了 ASCII 控制字符 (0-31)、双引号 (") 和反斜杠字符 ("\") 之外，所有值都为 true。
var safeSet = [utf8.RuneSelf]bool{
	' ':      true,
	'!':      true,
	'"':      false,
	'#':      true,
	'$':      true,
	'%':      true,
	'&':      true,
	'\'':     true,
	'(':      true,
	')':      true,
	'*':      true,
	'+':      true,
	',':      true,
	'-':      true,
	'.':      true,
	'/':      true,
	'0':      true,
	'1':      true,
	'2':      true,
	'3':      true,
	'4':      true,
	'5':      true,
	'6':      true,
	'7':      true,
	'8':      true,
	'9':      true,
	':':      true,
	';':      true,
	'<':      true,
	'=':      true,
	'>':      true,
	'?':      true,
	'@':      true,
	'A':      true,
	'B':      true,
	'C':      true,
	'D':      true,
	'E':      true,
	'F':      true,
	'G':      true,
	'H':      true,
	'I':      true,
	'J':      true,
	'K':      true,
	'L':      true,
	'M':      true,
	'N':      true,
	'O':      true,
	'P':      true,
	'Q':      true,
	'R':      true,
	'S':      true,
	'T':      true,
	'U':      true,
	'V':      true,
	'W':      true,
	'X':      true,
	'Y':      true,
	'Z':      true,
	'[':      true,
	'\\':     false,
	']':      true,
	'^':      true,
	'_':      true,
	'`':      true,
	'a':      true,
	'b':      true,
	'c':      true,
	'd':      true,
	'e':      true,
	'f':      true,
	'g':      true,
	'h':      true,
	'i':      true,
	'j':      true,
	'k':      true,
	'l':      true,
	'm':      true,
	'n':      true,
	'o':      true,
	'p':      true,
	'q':      true,
	'r':      true,
	's':      true,
	't':      true,
	'u':      true,
	'v':      true,
	'w':      true,
	'x':      true,
	'y':      true,
	'z':      true,
	'{':      true,
	'|':      true,
	'}':      true,
	'~':      true,
	'\u007f': true,
}
