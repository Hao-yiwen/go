// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package token

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
)

// 如果设置了 debug，无效的偏移和位置值会导致恐慌
// (go.dev/issue/57490)。
const debug = false

// 位置
//
// Position 描述一个任意的源代码位置，包括文件、行和列位置。
// 如果行号 > 0，则 Position 是有效的。
type Position struct {
	Filename string // 文件名（如果有）
	Offset   int    // 偏移量，从 0 开始
	Line     int    // 行号，从 1 开始
	Column   int    // 列号，从 1 开始（字节计数）
}

// IsValid 报告位置是否有效。
func (pos *Position) IsValid() bool { return pos.Line > 0 }

// String 返回以下几种形式之一的字符串：
//
//	file:line:column    带文件名的有效位置
//	file:line           带文件名但无列号的有效位置 (column == 0)
//	line:column         不带文件名的有效位置
//	line                不带文件名且无列号的有效位置 (column == 0)
//	file                带文件名的无效位置
//	-                   无文件名的无效位置
func (pos Position) String() string {
	s := pos.Filename
	if pos.IsValid() {
		if s != "" {
			s += ":"
		}
		s += strconv.Itoa(pos.Line)
		if pos.Column != 0 {
			s += fmt.Sprintf(":%d", pos.Column)
		}
	}
	if s == "" {
		s = "-"
	}
	return s
}

// Pos 是一个源代码位置在文件集中的紧凑编码。
// 它可以转换为 [Position] 以获得更方便但更大的表示。
//
// 给定文件的 Pos 值是范围 [base, base+size] 中的一个数字，
// 其中 base 和 size 在将文件添加到文件集时指定。
// Pos 值与对应文件基址之间的差异对应于该位置（由 Pos 值表示）
// 相对于文件开始的字节偏移。因此，文件基址偏移是表示
// 文件中第一个字节的 Pos 值。
//
// 要为特定的源代码偏移（以字节为单位）创建 Pos 值，
// 首先使用 [FileSet.AddFile] 将相应文件添加到当前文件集，
// 然后为该文件调用 [File.Pos](offset)。给定特定文件集 fset 的 Pos 值 p，
// 对应的 [Position] 值通过调用 fset.Position(p) 获得。
//
// Pos 值可以直接用常见的比较操作符进行比较：
// 如果两个 Pos 值 p 和 q 在同一文件中，比较 p 和 q 等价于
// 比较相应的源文件偏移。如果 p 和 q 在不同文件中，
// 如果 p 隐含的文件在 q 隐含的文件之前添加到相应的文件集，则 p < q 为真。
type Pos int

// [Pos] 的零值是 NoPos；没有与之相关的文件和行信息，
// NoPos.IsValid() 为假。NoPos 总是小于任何其他 [Pos] 值。
// NoPos 对应的 [Position] 值是 [Position] 的零值。
const NoPos Pos = 0

// IsValid 报告位置是否有效。
func (p Pos) IsValid() bool {
	return p != NoPos
}

// 文件

// File 是属于 [FileSet] 的文件的句柄。
// File 有一个名称、大小和行偏移表。
//
// 使用 [FileSet.AddFile] 创建一个 File。
// 一个 File 可能属于多个 FileSet；请参见 [FileSet.AddExistingFiles]。
type File struct {
	name string // 提供给 AddFile 的文件名
	base int    // 此文件的 Pos 值范围是 [base...base+size]
	size int    // 提供给 AddFile 的文件大小

	// lines 和 infos 由互斥锁保护
	mutex sync.Mutex
	lines []int // lines 包含每行第一个字符的偏移（第一个条目总是 0）
	infos []lineInfo
}

// Name 返回与 AddFile 注册的文件 f 的文件名。
func (f *File) Name() string {
	return f.name
}

// Base 返回与 AddFile 注册的文件 f 的基址偏移。
func (f *File) Base() int {
	return f.base
}

// Size 返回与 AddFile 注册的文件 f 的大小。
func (f *File) Size() int {
	return f.size
}

// End 返回与 AddFile 注册的文件 f 的结束位置。
func (f *File) End() Pos {
	return Pos(f.base + f.size)
}

// LineCount 返回文件 f 中的行数。
func (f *File) LineCount() int {
	f.mutex.Lock()
	n := len(f.lines)
	f.mutex.Unlock()
	return n
}

// AddLine 为新行添加行偏移。
// 行偏移必须大于前一行的偏移，且小于文件大小；
// 否则行偏移被忽略。
func (f *File) AddLine(offset int) {
	f.mutex.Lock()
	if i := len(f.lines); (i == 0 || f.lines[i-1] < offset) && offset < f.size {
		f.lines = append(f.lines, offset)
	}
	f.mutex.Unlock()
}

// MergeLine 将一行与下一行合并。这类似于将行尾的换行符替换为空格
// （以不改变其余偏移）。要获取行号，请参见例如 [Position.Line]。
// 如果给定无效的行号，MergeLine 将会 panic。
func (f *File) MergeLine(line int) {
	if line < 1 {
		panic(fmt.Sprintf("invalid line number %d (应该是 >= 1)", line))
	}
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if line >= len(f.lines) {
		panic(fmt.Sprintf("invalid line number %d (应该是 < %d)", line, len(f.lines)))
	}
	// 要将编号为 <line> 的行与编号为 <line+1> 的行合并，
	// 我们需要移除 lines 中对应于编号为 <line+1> 的行的条目。
	// lines 中对应于编号为 <line+1> 的行的条目位于索引 <line> 处，
	// 因为 lines 中的索引是基于 0 的，而行号是基于 1 的。
	copy(f.lines[line:], f.lines[line+1:])
	f.lines = f.lines[:len(f.lines)-1]
}

// Lines 返回由 [File.SetLines] 描述的形式的有效行偏移表。
// 调用者不得改变结果。
func (f *File) Lines() []int {
	f.mutex.Lock()
	lines := f.lines
	f.mutex.Unlock()
	return lines
}

// SetLines 为文件设置行偏移，并报告是否成功。
// 行偏移是每行第一个字符的偏移；
// 例如，对于内容 "ab\nc\n"，行偏移是 {0, 3}。
// 空文件有一个空的行偏移表。
// 每个行偏移必须大于前一行的偏移，且小于文件大小；
// 否则 SetLines 失败并返回 false。
// SetLines 返回后，调用者不得改变提供的切片。
func (f *File) SetLines(lines []int) bool {
	// 验证 lines 表的有效性
	size := f.size
	for i, offset := range lines {
		if i > 0 && offset <= lines[i-1] || size <= offset {
			return false
		}
	}

	// 设置 lines 表
	f.mutex.Lock()
	f.lines = lines
	f.mutex.Unlock()
	return true
}

// SetLinesForContent 为给定的文件内容设置行偏移。
// 它忽略改变位置的 //line 注释。
func (f *File) SetLinesForContent(content []byte) {
	var lines []int
	line := 0
	for offset, b := range content {
		if line >= 0 {
			lines = append(lines, line)
		}
		line = -1
		if b == '\n' {
			line = offset + 1
		}
	}

	// set lines table
	f.mutex.Lock()
	f.lines = lines
	f.mutex.Unlock()
}

// LineStart 返回指定行开始的 [Pos] 值。
// 它忽略使用 [File.AddLineColumnInfo] 设置的任何替代位置。
// 如果基于 1 的行号无效，LineStart 会 panic。
func (f *File) LineStart(line int) Pos {
	if line < 1 {
		panic(fmt.Sprintf("invalid line number %d (应该是 >= 1)", line))
	}
	f.mutex.Lock()
	defer f.mutex.Unlock()
	if line > len(f.lines) {
		panic(fmt.Sprintf("invalid line number %d (应该是 < %d)", line, len(f.lines)))
	}
	return Pos(f.base + f.lines[line-1])
}

// lineInfo 对象描述替代文件、行和列号信息（例如通过 //line 指令提供的信息），
// 用于给定的文件偏移。
type lineInfo struct {
	// 字段被导出以使其可被 gob 访问
	Offset       int
	Filename     string
	Line, Column int
}

// AddLineInfo 类似于 [File.AddLineColumnInfo]，参数 column = 1。
// 它在此是为了与 Go 1.11 之前的代码向后兼容。
func (f *File) AddLineInfo(offset int, filename string, line int) {
	f.AddLineColumnInfo(offset, filename, line, 1)
}

// AddLineColumnInfo 为给定的文件偏移添加替代文件、行和列号信息。
// 偏移必须大于先前添加的替代行信息的偏移，且小于文件大小；
// 否则该信息被忽略。
//
// AddLineColumnInfo 通常用于为行指令（例如 //line filename:line:column）
// 注册替代位置信息。
func (f *File) AddLineColumnInfo(offset int, filename string, line, column int) {
	f.mutex.Lock()
	if i := len(f.infos); (i == 0 || f.infos[i-1].Offset < offset) && offset < f.size {
		f.infos = append(f.infos, lineInfo{offset, filename, line, column})
	}
	f.mutex.Unlock()
}

// fixOffset 修复越界偏移，使得 0 <= offset <= f.size。
func (f *File) fixOffset(offset int) int {
	switch {
	case offset < 0:
		if !debug {
			return 0
		}
	case offset > f.size:
		if !debug {
			return f.size
		}
	default:
		return offset
	}

	// 仅在需要时生成此代码
	if debug {
		panic(fmt.Sprintf("offset %d out of bounds [%d, %d] (position %d out of bounds [%d, %d])",
			0 /* for symmetry */, offset, f.size,
			f.base+offset, f.base, f.base+f.size))
	}
	return 0
}

// Pos 返回给定文件偏移的 Pos 值。
//
// 如果偏移为负，结果是文件的开始位置；
// 如果偏移太大，结果是文件的结束位置（另见 go.dev/issue/57490）。
//
// 以下不变式虽然对一般的 Pos 值不成立，但对结果 p 成立：
// f.Pos(f.Offset(p)) == p.
func (f *File) Pos(offset int) Pos {
	return Pos(f.base + f.fixOffset(offset))
}

// Offset 返回给定文件位置 p 的偏移。
//
// 如果 p 在文件的开始位置之前（或如果 p 是 NoPos），
// 结果是 0；如果 p 超过文件的结束位置，
// 结果是文件大小（另见 go.dev/issue/57490）。
//
// 以下不变式虽然对一般的偏移值不成立，但对结果偏移成立：
// f.Offset(f.Pos(offset)) == offset
func (f *File) Offset(p Pos) int {
	return f.fixOffset(int(p) - f.base)
}

// Line 返回给定文件位置 p 的行号；
// p 必须是该文件中的 [Pos] 值或 [NoPos]。
func (f *File) Line(p Pos) int {
	return f.Position(p).Line
}

func searchLineInfos(a []lineInfo, x int) int {
	i, found := slices.BinarySearchFunc(a, x, func(a lineInfo, x int) int {
		return cmp.Compare(a.Offset, x)
	})
	if !found {
		// 我们想要包含 x 的 lineInfo，但如果我们没有找到 x，
		// 则 i 是下一个。
		i--
	}
	return i
}

// unpack 返回给定文件偏移的文件名、行号和列号。
// 如果设置了 adjusted，unpack 将返回可能由 //line 注释调整的文件名和行信息；
// 否则这些注释会被忽略。
func (f *File) unpack(offset int, adjusted bool) (filename string, line, column int) {
	f.mutex.Lock()
	filename = f.name
	if i := searchInts(f.lines, offset); i >= 0 {
		line, column = i+1, offset-f.lines[i]+1
	}
	if adjusted && len(f.infos) > 0 {
		// 几个文件有额外的行信息
		if i := searchLineInfos(f.infos, offset); i >= 0 {
			alt := &f.infos[i]
			filename = alt.Filename
			if i := searchInts(f.lines, alt.Offset); i >= 0 {
				// i+1 是记录替代位置的行号
				d := line - (i + 1) // 距替代位置基的行距
				line = alt.Line + d
				if alt.Column == 0 {
					// 替代列是未知的 => 相对列是未知的
					// （line 指令的当前规范要求这一点适用于下一个
					// PosBase/line 指令，而不仅仅是新行）
					column = 0
				} else if d == 0 {
					// 替代位置基在当前行
					// => 列相对于替代列
					column = alt.Column + (offset - alt.Offset)
				}
			}
		}
	}
	// TODO(mvdan): 一旦 https://go.dev/issue/38471 被修复以消除性能损失，
	// 将 Unlock 移回 Lock 下的 defer 语句。
	f.mutex.Unlock()
	return
}

func (f *File) position(p Pos, adjusted bool) (pos Position) {
	offset := f.fixOffset(int(p) - f.base)
	pos.Offset = offset
	pos.Filename, pos.Line, pos.Column = f.unpack(offset, adjusted)
	return
}

// PositionFor 返回给定文件位置 p 的 Position 值。
// 如果 p 越界，它会被调整以匹配 File.Offset 行为。
// 如果设置了 adjusted，位置可能被位置改变的 //line 注释调整；
// 否则这些注释会被忽略。
// p 必须是 f 中的 Pos 值或 NoPos。
func (f *File) PositionFor(p Pos, adjusted bool) (pos Position) {
	if p != NoPos {
		pos = f.position(p, adjusted)
	}
	return
}

// Position 返回给定文件位置 p 的 Position 值。
// 如果 p 越界，它会被调整以匹配 File.Offset 行为。
// 调用 f.Position(p) 等价于调用 f.PositionFor(p, true)。
func (f *File) Position(p Pos) (pos Position) {
	return f.PositionFor(p, true)
}

// 文件集

// FileSet 表示一组源文件。文件集的方法是同步的；
// 多个 goroutine 可以并发调用它们。
//
// 文件集中每个文件的字节偏移被映射到不同的（整数）区间，
// 每个文件一个区间 [base, base+size]。[FileSet.Base] 表示文件中的第一个字节，
// size 是对应的文件大小。[Pos] 值是这样的区间中的一个值。
// 通过确定 [Pos] 值所属的区间，可以计算该文件、其文件基址，
// 从而计算 [Pos] 值表示的字节偏移（位置）。
//
// 添加新文件时，必须提供文件基址。这可以是任何整数值，
// 超过文件集中任何文件的任何区间的结尾。为了方便，[FileSet.Base]
// 提供了这样的值，它就是最近添加的文件的 Pos 区间的结尾加一。
// 除非以后需要扩展区间，否则应该使用 [FileSet.Base] 作为
// [FileSet.AddFile] 的参数。
//
// 当不再需要文件时，可以从 FileSet 中删除 [File]。
// 这可能会减少长时间运行的应用程序的内存使用。
type FileSet struct {
	mutex sync.RWMutex         // 保护文件集
	base  int                  // 下一个文件的基址偏移
	tree  tree                 // 按升序基序排列的文件树
	last  atomic.Pointer[File] // 最后查找的文件的缓存
}

// NewFileSet 创建一个新的文件集。
func NewFileSet() *FileSet {
	return &FileSet{
		base: 1, // 0 == NoPos
	}
}

// Base 返回在添加下一个文件时必须提供给 [FileSet.AddFile] 的最小基址偏移。
func (s *FileSet) Base() int {
	s.mutex.RLock()
	b := s.base
	s.mutex.RUnlock()
	return b
}

// AddFile 使用给定的文件名、基址偏移和文件大小向文件集 s 添加新文件，
// 并返回该文件。多个文件可能有相同的名称。基址偏移不得小于 [FileSet.Base]，
// 且大小不得为负。作为特殊情况，如果提供负基址，
// 则使用 [FileSet.Base] 的当前值。
//
// 添加文件会将文件集的 [FileSet.Base] 值设置为 base + size + 1，
// 作为下一个文件的最小基值。给定文件偏移 offs 的 [Pos] 值 p 与以下关系存在：
//
//	int(p) = base + offs
//
// 其中 offs 在范围 [0, size] 内，因此 p 在范围 [base, base+size] 内。
// 为了方便，可以使用 [File.Pos] 从文件偏移创建文件特定的位置值。
func (s *FileSet) AddFile(filename string, base, size int) *File {
	// Allocate f outside the critical section.
	f := &File{name: filename, size: size, lines: []int{0}}

	s.mutex.Lock()
	defer s.mutex.Unlock()
	if base < 0 {
		base = s.base
	}
	if base < s.base {
		panic(fmt.Sprintf("invalid base %d (应该是 >= %d)", base, s.base))
	}
	f.base = base
	if size < 0 {
		panic(fmt.Sprintf("invalid size %d (应该是 >= 0)", size))
	}
	// base >= s.base && size >= 0
	base += size + 1 // +1 因为 EOF 也有一个位置
	if base < 0 {
		panic("token.Pos offset overflow (> 2G of source code in file set)")
	}
	// 将文件添加到文件集
	s.base = base
	s.tree.add(f)
	s.last.Store(f)
	return f
}

// AddExistingFiles 将指定的文件添加到 FileSet（如果还不存在）。
// 调用者必须确保在生成的 FileSet 中显示的任何文件对都不重叠。
func (s *FileSet) AddExistingFiles(files ...*File) {
	// 此函数不能实现为：
	//
	//	for _, file := range files {
	//		if prev := fset.File(token.Pos(file.Base())); prev != nil {
	//			if prev != file {
	//				panic("FileSet 包含一个不同的文件，位置相同")
	//			}
	//			continue
	//		}
	//		file2 := fset.AddFile(file.Name(), file.Base(), file.Size())
	//		file2.SetLines(file.Lines())
	//	}
	//
	// 因为所有对 AddFile 的调用必须按递增顺序进行。
	// AddExistingFiles 让我们能够按顺序增强现有 FileSet，
	// 只要所有文件集都有不相交的范围。
	// 此方法也不保留行指令。

	s.mutex.Lock()
	defer s.mutex.Unlock()

	for _, f := range files {
		s.tree.add(f)
		s.base = max(s.base, f.Base()+f.Size()+1)
	}
}

// RemoveFile 从 [FileSet] 中删除一个文件，使后续查询其 [Pos] 区间产生负结果。
// 这减少了遇到无界文件流的长期存活 [FileSet] 的内存使用。
//
// 删除不属于该集合的文件没有任何效果。
func (s *FileSet) RemoveFile(file *File) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	s.last.CompareAndSwap(file, nil) // 清除最后文件缓存

	pn, _ := s.tree.locate(file.key())
	if *pn != nil && (*pn).file == file {
		s.tree.delete(pn)
	}
}

// Iterate 为文件集中的文件按升序 Base 顺序调用 yield，
// 直到 yield 返回 false。
func (s *FileSet) Iterate(yield func(*File) bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	// 在用户代码周围解锁。
	// 迭代器对 yield 的修改是鲁棒的。
	// 避免在此处使用 range，以便我们可以使用 defer。
	s.tree.all()(func(f *File) bool {
		s.mutex.RUnlock()
		defer s.mutex.RLock()
		return yield(f)
	})
}

func (s *FileSet) file(p Pos) *File {
	// 常见情况：p 在最后一个文件中。
	if f := s.last.Load(); f != nil && f.base <= int(p) && int(p) <= f.base+f.size {
		return f
	}

	s.mutex.RLock()
	defer s.mutex.RUnlock()

	pn, _ := s.tree.locate(key{int(p), int(p)})
	if n := *pn; n != nil {
		// 更新最后文件的缓存。竞态条件没关系，
		// 但独占锁会导致严重竞争。
		s.last.Store(n.file)
		return n.file
	}
	return nil
}

// File 返回包含位置 p 的文件。
// 如果未找到此类文件（例如对于 p == [NoPos]),
// the result is nil.
func (s *FileSet) File(p Pos) (f *File) {
	if p != NoPos {
		f = s.file(p)
	}
	return
}

// PositionFor converts a [Pos] p in the fileset into a [Position] value.
// If adjusted is set, the position 可能是 adjusted by position-altering
// //line comments; 否则 those comments are ignored.
// p 必须是 a [Pos] value in s or [NoPos].
func (s *FileSet) PositionFor(p Pos, adjusted bool) (pos Position) {
	if p != NoPos {
		if f := s.file(p); f != nil {
			return f.position(p, adjusted)
		}
	}
	return
}

// Position converts a [Pos] p in the fileset into a Position value.
// Calling s.Position(p) is equivalent to calling s.PositionFor(p, true).
func (s *FileSet) Position(p Pos) (pos Position) {
	return s.PositionFor(p, true)
}

// -----------------------------------------------------------------------------
// Helper functions

func searchInts(a []int, x int) int {
	// This function body 是一个 manually inlined version of:
	//
	//   return sort.Search(len(a), func(i int) bool { return a[i] > x }) - 1
	//
	// With better compiler optimizations, this may not be needed in the
	// future, but at the moment this change improves the go/printer
	// benchmark performance by ~30%. This has a direct impact on the
	// speed of gofmt and thus seems worthwhile (2011-04-29).
	// TODO(gri): Remove this when compilers have caught up.
	i, j := 0, len(a)
	for i < j {
		h := int(uint(i+j) >> 1) // avoid overflow when computing h
		// i ≤ h < j
		if a[h] <= x {
			i = h + 1
		} else {
			j = h
		}
	}
	return i - 1
}
