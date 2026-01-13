// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package tabwriter 实现了一个写入过滤器（tabwriter.Writer），
// 将输入中的制表符分隔列转换为正确对齐的文本。
//
// 该包使用了在 http://nickgravgaard.com/elastictabstops/index.html
// 描述的弹性制表位算法。
//
// text/tabwriter 包已冻结，不再接受新功能。
package tabwriter

import (
	"fmt"
	"io"
	"unicode/utf8"
)

// ----------------------------------------------------------------------------
// 过滤器实现

// cell 表示由制表符或换行符终止的文本段。
// 文本本身存储在单独的缓冲区中；cell 仅描述
// 该段的字节大小、rune 宽度，以及它是否由 htab（'\t'）终止。
type cell struct {
	size  int  // 单元格字节大小
	width int  // 单元格 rune 宽度
	htab  bool // 如果单元格由 htab（'\t'）终止则为 true
}

// Writer 是一个过滤器，它在输入的制表符分隔列周围插入填充，
// 以在输出中对齐它们。
//
// Writer 将传入的字节视为 UTF-8 编码的文本，由水平制表符（'\t'）
// 或垂直制表符（'\v'）终止的单元格，以及换行符（'\n'）或
// 换页符（'\f'）组成；换行符和换页符都充当换行符。
//
// 连续行中由制表符终止的单元格构成一列。Writer 根据需要
// 插入填充，使列中的所有单元格具有相同的宽度，从而有效地
// 对齐列。它假设所有字符具有相同的宽度，但制表符除外，
// 必须为其指定 tabwidth。列单元格必须由制表符终止，而不是
// 由制表符分隔：行末尾未由制表符终止的尾随文本形成一个
// 单元格，但该单元格不是对齐列的一部分。
// 例如，在这个例子中（其中 | 代表水平制表符）：
//
//	aaaa|bbb|d
//	aa  |b  |dd
//	a   |
//	aa  |cccc|eee
//
// b 和 c 在不同的列中（b 列不是完全连续的）。d 和 e 根本
// 不在列中（没有终止制表符，列也不会连续）。
//
// Writer 假设所有 Unicode 码点具有相同的宽度；
// 这在某些字体中可能不正确，或者如果字符串包含组合字符。
//
// 如果设置了 [DiscardEmptyColumns]，则完全由垂直（或"软"）
// 制表符终止的空列将被丢弃。由水平（或"硬"）制表符终止的
// 列不受此标志影响。
//
// 如果 Writer 配置为过滤 HTML，则 HTML 标签和实体将被
// 传递。出于格式化目的，标签和实体的宽度分别假定为
// 零（标签）和一（实体）。
//
// 文本段可以通过用 [Escape] 字符括起来来转义。tabwriter
// 将转义的文本段原样传递。特别是，它不解释段内的任何
// 制表符或换行符。如果设置了 [StripEscape] 标志，则 Escape
// 字符将从输出中剥离；否则它们也会被传递。出于格式化目的，
// 转义文本的宽度始终在排除 Escape 字符后计算。
//
// 换页符的作用类似于换行符，但它还终止当前行中的所有列
// （实际上调用 [Writer.Flush]）。下一行中由制表符终止的单元格
// 开始新列。除非在 HTML 标签内或转义文本段内找到，否则
// 换页符在输出中显示为换行符。
//
// Writer 必须在内部缓冲输入，因为一行的正确间距可能
// 取决于未来行中的单元格。客户端必须在完成调用
// [Writer.Write] 后调用 Flush。
type Writer struct {
	// 配置
	output   io.Writer
	minwidth int
	tabwidth int
	padding  int
	padbytes [8]byte
	flags    uint

	// 当前状态
	buf     []byte   // 收集的文本，不包括制表符或换行符
	pos     int      // 缓冲区位置，到此为止已计算不完整单元格的 cell.width
	cell    cell     // 当前不完整的单元格；cell.width 到 buf[pos] 为止，不包括忽略的部分
	endChar byte     // 转义序列的终止字符（转义用 Escape，HTML 标签/实体用 '>'、';'，或 0）
	lines   [][]cell // 行列表；每行是单元格列表
	widths  []int    // 列宽度列表（以 rune 为单位）- 在格式化期间重用
}

// addLine 添加新行。
// flushed 是一个提示，指示底层 writer 是否刚刚刷新。
// 如果是，则前一行可能不是新行单元格的好指标。
func (b *Writer) addLine(flushed bool) {
	// 增长切片而不是追加，
	// 因为这给了我们重用现有 []cell 的机会。
	if n := len(b.lines) + 1; n <= cap(b.lines) {
		b.lines = b.lines[:n]
		b.lines[n-1] = b.lines[n-1][:0]
	} else {
		b.lines = append(b.lines, nil)
	}

	if !flushed {
		// 前一行可能是当前行将有多少单元格的好指标。
		// 如果当前行的容量小于此值，
		// 则放弃它并创建一个新的。
		if n := len(b.lines); n >= 2 {
			if prev := len(b.lines[n-2]); prev > cap(b.lines[n-1]) {
				b.lines[n-1] = make([]cell, 0, prev)
			}
		}
	}
}

// 重置当前状态。
func (b *Writer) reset() {
	b.buf = b.buf[:0]
	b.pos = 0
	b.cell = cell{}
	b.endChar = 0
	b.lines = b.lines[0:0]
	b.widths = b.widths[0:0]
	b.addLine(true)
}

// 内部表示（当前状态）：
//
// - 所有写入的文本都附加到 buf；制表符和换行符被剥离
// - 在任何给定时间，末尾都有一个（可能为空的）不完整单元格
//   （单元格在制表符或换行符之后开始）
// - cell.size 是到目前为止属于单元格的字节数
// - cell.width 是该单元格从单元格开始到位置 pos 的文本 rune 宽度；
//   如果启用了 html 过滤，则 html 标签和实体不包括在此宽度中
// - 已处理文本的大小和宽度保存在 lines 列表中，
//   该列表包含每行的单元格列表
// - widths 列表是一个临时列表，包含格式化期间使用的当前宽度；
//   它保存在 Writer 中是因为它被重用
//
//                    |<---------- size ---------->|
//                    |                            |
//                    |<- width ->|<- ignored ->|  |
//                    |           |             |  |
// [---processed---tab------------<tag>...</tag>...]
// ^                  ^                         ^
// |                  |                         |
// buf                不完整单元格的开始         pos

// 格式化可以通过这些标志控制。
const (
	// 忽略 html 标签，并将实体（以 '&' 开头，以 ';' 结尾）
	// 视为单个字符（宽度 = 1）。
	FilterHTML uint = 1 << iota

	// 剥离括起转义文本段的 Escape 字符，
	// 而不是将它们与文本一起原样传递。
	StripEscape

	// 强制单元格内容右对齐。
	// 默认是左对齐。
	AlignRight

	// 将空列视为一开始就不存在于输入中。
	DiscardEmptyColumns

	// 总是使用制表符作为缩进列（即左侧前导空单元格的填充），
	// 与 padchar 无关。
	TabIndent

	// 在列之间打印竖线（'|'）（格式化之后）。
	// 被丢弃的列显示为零宽度列（"||"）。
	Debug
)

// [Writer] 必须通过调用 Init 进行初始化。第一个参数（output）
// 指定过滤器输出。其余参数控制格式化：
//
//	minwidth	最小单元格宽度，包括任何填充
//	tabwidth	制表符的宽度（等效的空格数）
//	padding		在计算单元格宽度之前添加到单元格的填充
//	padchar		用于填充的 ASCII 字符
//			如果 padchar == '\t'，Writer 将假设
//			格式化输出中 '\t' 的宽度是 tabwidth，
//			并且单元格左对齐，与 align_left 无关
//			（为了正确的显示效果，tabwidth 必须与
//			显示结果的查看器中的制表符宽度相对应）
//	flags		格式化控制
func (b *Writer) Init(output io.Writer, minwidth, tabwidth, padding int, padchar byte, flags uint) *Writer {
	if minwidth < 0 || tabwidth < 0 || padding < 0 {
		panic("negative minwidth, tabwidth, or padding")
	}
	b.output = output
	b.minwidth = minwidth
	b.tabwidth = tabwidth
	b.padding = padding
	for i := range b.padbytes {
		b.padbytes[i] = padchar
	}
	if padchar == '\t' {
		// 制表符填充强制左对齐
		flags &^= AlignRight
	}
	b.flags = flags

	b.reset()

	return b
}

// 调试支持（保留代码）
func (b *Writer) dump() {
	pos := 0
	for i, line := range b.lines {
		print("(", i, ") ")
		for _, c := range line {
			print("[", string(b.buf[pos:pos+c.size]), "]")
			pos += c.size
		}
		print("\n")
	}
	print("\n")
}

// 本地错误包装器，以便我们可以将想要作为错误返回的错误
// 与真正的 panic（我们不想作为错误返回）区分开来
type osError struct {
	err error
}

func (b *Writer) write0(buf []byte) {
	n, err := b.output.Write(buf)
	if n != len(buf) && err == nil {
		err = io.ErrShortWrite
	}
	if err != nil {
		panic(osError{err})
	}
}

func (b *Writer) writeN(src []byte, n int) {
	for n > len(src) {
		b.write0(src)
		n -= len(src)
	}
	b.write0(src[0:n])
}

var (
	newline = []byte{'\n'}
	tabs    = []byte("\t\t\t\t\t\t\t\t")
)

func (b *Writer) writePadding(textw, cellw int, useTabs bool) {
	if b.padbytes[0] == '\t' || useTabs {
		// 使用制表符进行填充
		if b.tabwidth == 0 {
			return // 制表符没有宽度 - 无法进行任何填充
		}
		// 使 cellw 成为 b.tabwidth 的最小倍数
		cellw = (cellw + b.tabwidth - 1) / b.tabwidth * b.tabwidth
		n := cellw - textw // 填充量
		if n < 0 {
			panic("internal error")
		}
		b.writeN(tabs, (n+b.tabwidth-1)/b.tabwidth)
		return
	}

	// 使用非制表符进行填充
	b.writeN(b.padbytes[0:], cellw-textw)
}

var vbar = []byte{'|'}

func (b *Writer) writeLines(pos0 int, line0, line1 int) (pos int) {
	pos = pos0
	for i := line0; i < line1; i++ {
		line := b.lines[i]

		// 如果设置了 TabIndent，使用制表符填充前导空单元格
		useTabs := b.flags&TabIndent != 0

		for j, c := range line {
			if j > 0 && b.flags&Debug != 0 {
				// 指示列分隔
				b.write0(vbar)
			}

			if c.size == 0 {
				// 空单元格
				if j < len(b.widths) {
					b.writePadding(c.width, b.widths[j], useTabs)
				}
			} else {
				// 非空单元格
				useTabs = false
				if b.flags&AlignRight == 0 { // 左对齐
					b.write0(b.buf[pos : pos+c.size])
					pos += c.size
					if j < len(b.widths) {
						b.writePadding(c.width, b.widths[j], false)
					}
				} else { // 右对齐
					if j < len(b.widths) {
						b.writePadding(c.width, b.widths[j], false)
					}
					b.write0(b.buf[pos : pos+c.size])
					pos += c.size
				}
			}
		}

		if i+1 == len(b.lines) {
			// 最后一个缓冲行 - 我们没有换行符，所以只写入
			// 任何未完成的缓冲数据
			b.write0(b.buf[pos : pos+b.cell.size])
			pos += b.cell.size
		} else {
			// 不是最后一行 - 写入换行符
			b.write0(newline)
		}
	}
	return
}

// 格式化 line0 和 line1 之间的文本（不包括 line1）；pos
// 是对应于 line0 开头的缓冲区位置。
// 返回对应于 line1 开头的缓冲区位置，以及错误（如果有）。
func (b *Writer) format(pos0 int, line0, line1 int) (pos int) {
	pos = pos0
	column := len(b.widths)
	for this := line0; this < line1; this++ {
		line := b.lines[this]

		if column >= len(line)-1 {
			continue
		}
		// 此列中存在单元格 => 此行
		// 比前一行有更多的单元格
		// （每行的最后一个单元格被忽略，因为单元格是
		// 制表符终止的；每行的最后一个单元格描述
		// 换行符/换页符之前的文本，不属于列）

		// 打印未打印的行，直到块的开始
		pos = b.writeLines(pos, line0, this)
		line0 = this

		// 列块开始
		width := b.minwidth // 最小列宽度
		discardable := true // 如果此列中的所有单元格都是空的且"软"，则为 true
		for ; this < line1; this++ {
			line = b.lines[this]
			if column >= len(line)-1 {
				break
			}
			// 此列中存在单元格
			c := line[column]
			// 更新宽度
			if w := c.width + b.padding; w > width {
				width = w
			}
			// 更新 discardable
			if c.width > 0 || c.htab {
				discardable = false
			}
		}
		// 列块结束

		// 如有必要，丢弃空列
		if discardable && b.flags&DiscardEmptyColumns != 0 {
			width = 0
		}

		// 格式化并打印此列右侧的所有列
		// （我们知道此列和左侧所有列的宽度）
		b.widths = append(b.widths, width) // 推入宽度
		pos = b.format(pos, line0, this)
		b.widths = b.widths[0 : len(b.widths)-1] // 弹出宽度
		line0 = this
	}

	// 打印未打印的行，直到结束
	return b.writeLines(pos, line0, line1)
}

// 将文本附加到当前单元格。
func (b *Writer) append(text []byte) {
	b.buf = append(b.buf, text...)
	b.cell.size += len(text)
}

// 更新单元格宽度。
func (b *Writer) updateWidth() {
	b.cell.width += utf8.RuneCount(b.buf[b.pos:])
	b.pos = len(b.buf)
}

// 要转义文本段，请用 Escape 字符将其括起来。
// 例如，此字符串 "Ignore this tab: \xff\t\xff" 中的制表符
// 不终止单元格，并且出于格式化目的构成宽度为一的单个字符。
//
// 选择值 0xff 是因为它不能出现在有效的 UTF-8 序列中。
const Escape = '\xff'

// 开始转义模式。
func (b *Writer) startEscape(ch byte) {
	switch ch {
	case Escape:
		b.endChar = Escape
	case '<':
		b.endChar = '>'
	case '&':
		b.endChar = ';'
	}
}

// 终止转义模式。如果转义的文本是 HTML 标签，出于格式化目的
// 其宽度假定为零；如果是 HTML 实体，其宽度假定为一。
// 在所有其他情况下，宽度是文本的 unicode 宽度。
func (b *Writer) endEscape() {
	switch b.endChar {
	case Escape:
		b.updateWidth()
		if b.flags&StripEscape == 0 {
			b.cell.width -= 2 // 不计算 Escape 字符
		}
	case '>': // 零宽度标签
	case ';':
		b.cell.width++ // 实体，计为一个 rune
	}
	b.pos = len(b.buf)
	b.endChar = 0
}

// 通过将当前单元格添加到当前行的单元格列表来终止它。
// 返回该行中的单元格数。
func (b *Writer) terminateCell(htab bool) int {
	b.cell.htab = htab
	line := &b.lines[len(b.lines)-1]
	*line = append(*line, b.cell)
	b.cell = cell{}
	return len(*line)
}

func (b *Writer) handlePanic(err *error, op string) {
	if e := recover(); e != nil {
		if op == "Flush" {
			// 如果 Flush 遇到 panic，我们仍然需要重置。
			b.reset()
		}
		if nerr, ok := e.(osError); ok {
			*err = nerr.err
			return
		}
		panic(fmt.Sprintf("tabwriter: panic during %s (%v)", op, e))
	}
}

// Flush 应该在最后一次调用 [Writer.Write] 后调用，以确保
// [Writer] 中缓冲的任何数据都写入输出。出于格式化目的，
// 末尾任何不完整的转义序列都被视为完整。
func (b *Writer) Flush() error {
	return b.flush()
}

// flush 是 Flush 的内部版本，带有我们不想暴露的命名返回值。
func (b *Writer) flush() (err error) {
	defer b.handlePanic(&err, "Flush")
	b.flushNoDefers()
	return nil
}

// flushNoDefers 类似于 flush，但没有延迟的 handlePanic 调用。
// 这可以从其他已经有自己延迟 handlePanic 调用的方法调用，
// 例如 Write，以避免额外的 defer 工作。
func (b *Writer) flushNoDefers() {
	// 如果当前单元格不为空，则添加它
	if b.cell.size > 0 {
		if b.endChar != 0 {
			// 在转义内 - 即使不完整也终止它
			b.endEscape()
		}
		b.terminateCell(false)
	}

	// 格式化缓冲区内容
	b.format(0, 0, len(b.lines))
	b.reset()
}

var hbar = []byte("---\n")

// Write 将 buf 写入 writer b。
// 返回的唯一错误是写入底层输出流时遇到的错误。
func (b *Writer) Write(buf []byte) (n int, err error) {
	defer b.handlePanic(&err, "Write")

	// 将文本拆分为单元格
	n = 0
	for i, ch := range buf {
		if b.endChar == 0 {
			// 在转义外
			switch ch {
			case '\t', '\v', '\n', '\f':
				// 单元格结束
				b.append(buf[n:i])
				b.updateWidth()
				n = i + 1 // ch 已消费
				ncells := b.terminateCell(ch == '\t')
				if ch == '\n' || ch == '\f' {
					// 终止行
					b.addLine(ch == '\f')
					if ch == '\f' || ncells == 1 {
						// '\f' 总是强制刷新。否则，如果前一行
						// 只有一个单元格，它不会影响后续行的格式化
						// （每行的最后一个单元格被 format() 忽略），
						// 因此我们可以刷新 Writer 内容。
						b.flushNoDefers()
						if ch == '\f' && b.flags&Debug != 0 {
							// 指示节分隔
							b.write0(hbar)
						}
					}
				}

			case Escape:
				// 转义序列开始
				b.append(buf[n:i])
				b.updateWidth()
				n = i
				if b.flags&StripEscape != 0 {
					n++ // 剥离 Escape
				}
				b.startEscape(Escape)

			case '<', '&':
				// 可能是 html 标签/实体
				if b.flags&FilterHTML != 0 {
					// 标签/实体开始
					b.append(buf[n:i])
					b.updateWidth()
					n = i
					b.startEscape(ch)
				}
			}

		} else {
			// 在转义内
			if ch == b.endChar {
				// 标签/实体结束
				j := i + 1
				if ch == Escape && b.flags&StripEscape != 0 {
					j = i // 剥离 Escape
				}
				b.append(buf[n:j])
				n = i + 1 // ch 已消费
				b.endEscape()
			}
		}
	}

	// 附加剩余文本
	b.append(buf[n:])
	n = len(buf)
	return
}

// NewWriter 分配并初始化一个新的 [Writer]。
// 参数与 Init 函数相同。
func NewWriter(output io.Writer, minwidth, tabwidth, padding int, padchar byte, flags uint) *Writer {
	return new(Writer).Init(output, minwidth, tabwidth, padding, padchar, flags)
}
