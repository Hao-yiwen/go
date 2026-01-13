// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// path 包实现了用于操作以斜杠分隔的路径的实用工具函数。
//
// path 包只应用于以正斜杠分隔的路径，例如 URL 中的路径。
// 本包不处理带有驱动器号或反斜杠的 Windows 路径；
// 如需操作操作系统路径，请使用 [path/filepath] 包。
package path

import "internal/bytealg"

// lazybuf 是一个延迟构造的路径缓冲区。
// 它支持追加、读取之前追加的字节以及获取最终字符串。
// 在输出与 s 产生差异之前，它不会分配缓冲区来保存输出。
type lazybuf struct {
	s   string
	buf []byte
	w   int
}

func (b *lazybuf) index(i int) byte {
	if b.buf != nil {
		return b.buf[i]
	}
	return b.s[i]
}

func (b *lazybuf) append(c byte) {
	if b.buf == nil {
		if b.w < len(b.s) && b.s[b.w] == c {
			b.w++
			return
		}
		b.buf = make([]byte, len(b.s))
		copy(b.buf, b.s[:b.w])
	}
	b.buf[b.w] = c
	b.w++
}

func (b *lazybuf) string() string {
	if b.buf == nil {
		return b.s[:b.w]
	}
	return string(b.buf[:b.w])
}

// Clean 通过纯词法处理返回与 path 等价的最短路径名。
// 它反复应用以下规则，直到无法再进行处理：
//
//  1. 将多个斜杠替换为单个斜杠。
//  2. 消除每个 . 路径名元素（当前目录）。
//  3. 消除每个内部的 .. 路径名元素（父目录）
//     以及它前面的非 .. 元素。
//  4. 消除以根路径开头的 .. 元素：
//     即在路径开头将 "/.." 替换为 "/"。
//
// 返回的路径仅在它是根目录 "/" 时才以斜杠结尾。
//
// 如果此处理过程的结果是空字符串，Clean 返回字符串 "."。
//
// 另请参阅 Rob Pike 的文章 "Lexical File Names in Plan 9 or
// Getting Dot-Dot Right"，
// https://9p.io/sys/doc/lexnames.html
func Clean(path string) string {
	if path == "" {
		return "."
	}

	rooted := path[0] == '/'
	n := len(path)

	// 不变量：
	//	从 path 读取；r 是下一个要处理的字节的索引。
	//	写入 buf；w 是下一个要写入的字节的索引。
	//	dotdot 是 buf 中 .. 必须停止的索引，因为
	//		它是开头的斜杠或者是开头的 ../../.. 前缀。
	out := lazybuf{s: path}
	r, dotdot := 0, 0
	if rooted {
		out.append('/')
		r, dotdot = 1, 1
	}

	for r < n {
		switch {
		case path[r] == '/':
			// 空路径元素
			r++
		case path[r] == '.' && (r+1 == n || path[r+1] == '/'):
			// . 元素
			r++
		case path[r] == '.' && path[r+1] == '.' && (r+2 == n || path[r+2] == '/'):
			// .. 元素：删除到最后一个 /
			r += 2
			switch {
			case out.w > dotdot:
				// 可以回溯
				out.w--
				for out.w > dotdot && out.index(out.w) != '/' {
					out.w--
				}
			case !rooted:
				// 无法回溯，但不是根路径，所以追加 .. 元素。
				if out.w > 0 {
					out.append('/')
				}
				out.append('.')
				out.append('.')
				dotdot = out.w
			}
		default:
			// 真实路径元素。
			// 如果需要，添加斜杠
			if rooted && out.w != 1 || !rooted && out.w != 0 {
				out.append('/')
			}
			// 复制元素
			for ; r < n && path[r] != '/'; r++ {
				out.append(path[r])
			}
		}
	}

	// 将空字符串转换为 "."
	if out.w == 0 {
		return "."
	}

	return out.string()
}

// Split 在最后一个斜杠之后立即分割路径，
// 将其分为目录和文件名组件。
// 如果路径中没有斜杠，Split 返回空的 dir，
// file 设置为 path。
// 返回的值具有 path = dir+file 的属性。
func Split(path string) (dir, file string) {
	i := bytealg.LastIndexByteString(path, '/')
	return path[:i+1], path[i+1:]
}

// Join 将任意数量的路径元素连接成单个路径，
// 用斜杠分隔它们。空元素会被忽略。
// 结果会经过 Clean 处理。但是，如果参数列表
// 为空或其所有元素都为空，Join 返回空字符串。
func Join(elem ...string) string {
	size := 0
	for _, e := range elem {
		size += len(e)
	}
	if size == 0 {
		return ""
	}
	buf := make([]byte, 0, size+len(elem)-1)
	for _, e := range elem {
		if len(buf) > 0 || e != "" {
			if len(buf) > 0 {
				buf = append(buf, '/')
			}
			buf = append(buf, e...)
		}
	}
	return Clean(string(buf))
}

// Ext 返回路径使用的文件扩展名。
// 扩展名是路径中最后一个斜杠分隔元素中
// 从最后一个点开始的后缀；
// 如果没有点，则为空。
func Ext(path string) string {
	for i := len(path) - 1; i >= 0 && path[i] != '/'; i-- {
		if path[i] == '.' {
			return path[i:]
		}
	}
	return ""
}

// Base 返回路径的最后一个元素。
// 在提取最后一个元素之前会删除尾部斜杠。
// 如果路径为空，Base 返回 "."。
// 如果路径完全由斜杠组成，Base 返回 "/"。
func Base(path string) string {
	if path == "" {
		return "."
	}
	// 去除尾部斜杠。
	for len(path) > 0 && path[len(path)-1] == '/' {
		path = path[0 : len(path)-1]
	}
	// 找到最后一个元素
	if i := bytealg.LastIndexByteString(path, '/'); i >= 0 {
		path = path[i+1:]
	}
	// 如果现在为空，说明它只有斜杠。
	if path == "" {
		return "/"
	}
	return path
}

// IsAbs 报告路径是否为绝对路径。
func IsAbs(path string) bool {
	return len(path) > 0 && path[0] == '/'
}

// Dir 返回路径中除最后一个元素之外的所有内容，通常是路径的目录。
// 使用 [Split] 删除最后一个元素后，路径会经过 Clean 处理，
// 并删除尾部斜杠。
// 如果路径为空，Dir 返回 "."。
// 如果路径完全由斜杠后跟非斜杠字节组成，Dir 返回单个斜杠。
// 在任何其他情况下，返回的路径不以斜杠结尾。
func Dir(path string) string {
	dir, _ := Split(path)
	return Clean(dir)
}
