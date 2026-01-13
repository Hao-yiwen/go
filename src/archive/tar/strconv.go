// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package tar

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// hasNUL 报告 s 中是否存在 NUL 字符。
func hasNUL(s string) bool {
	return strings.Contains(s, "\x00")
}

// isASCII 报告输入是否是 ASCII C 风格字符串。
func isASCII(s string) bool {
	for _, c := range s {
		if c >= 0x80 || c == 0x00 {
			return false
		}
	}
	return true
}

// toASCII 将输入转换为 ASCII C 风格字符串。
// 这是尽力而为的转换，因此无效字符会被丢弃。
func toASCII(s string) string {
	if isASCII(s) {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, c := range s {
		if c < 0x80 && c != 0x00 {
			b = append(b, byte(c))
		}
	}
	return string(b)
}

type parser struct {
	err error // 最后看到的错误
}

type formatter struct {
	err error // 最后看到的错误
}

// parseString 将字节解析为 NUL 终止的 C 风格字符串。
// 如果未找到 NUL 字节，则整个切片作为字符串返回。
func (*parser) parseString(b []byte) string {
	if i := bytes.IndexByte(b, 0); i >= 0 {
		return string(b[:i])
	}
	return string(b)
}

// formatString 将 s 复制到 b 中，如果可能则添加 NUL 终止符。
func (f *formatter) formatString(b []byte, s string) {
	if len(s) > len(b) {
		f.err = ErrFieldTooLong
	}
	copy(b, s)
	if len(s) < len(b) {
		b[len(s)] = 0
	}

	// 一些有问题的读取器会将 V7 路径字段中带有尾部斜杠的普通文件
	// 视为目录，尽管记录在其他地方的完整路径（例如通过 PAX 记录）
	// 不包含尾部斜杠。
	if len(s) > len(b) && b[len(b)-1] == '/' {
		n := len(strings.TrimRight(s[:len(b)-1], "/"))
		b[n] = 0 // 用 NUL 终止符替换尾部斜杠
	}
}

// fitsInBase256 报告 x 是否可以使用 base-256 编码编码到 n 个字节中。
// 与八进制编码不同，base-256 编码不要求字符串以 NUL 字符结尾。
// 因此，所有 n 个字节都可用于输出。
//
// 如果在二进制模式下运行，这假设严格的 GNU 二进制模式；这意味着
// 第一个字节只能是 0x80 或 0xff。因此，第一个字节
// 等同于二进制补码形式的符号位。
func fitsInBase256(n int, x int64) bool {
	binBits := uint(n-1) * 8
	return n >= 9 || (x >= -1<<binBits && x < 1<<binBits)
}

// parseNumeric 将输入解析为 base-256 或八进制编码。
// 此函数可能返回负数。
// 如果解析失败或发生整数溢出，将设置 err。
func (p *parser) parseNumeric(b []byte) int64 {
	// 首先检查 base-256（二进制）格式。
	// 如果第一位被设置，则所有后续位构成一个
	// 大端字节序的二进制补码编码数。
	if len(b) > 0 && b[0]&0x80 != 0 {
		// 处理负数依赖于以下恒等式：
		//	-a-1 == ^a
		//
		// 如果数字为负，我们使用反转掩码来反转
		// 数据字节并将值视为无符号数。
		var inv byte // 如果为正或零则为 0x00，如果为负则为 0xff
		if b[0]&0x40 != 0 {
			inv = 0xff
		}

		var x uint64
		for i, c := range b {
			c ^= inv // 仅当 inv 为 0xff 时反转 c，否则不做任何操作
			if i == 0 {
				c &= 0x7f // 忽略第一个字节中的符号位
			}
			if (x >> 56) > 0 {
				p.err = ErrHeader // 整数溢出
				return 0
			}
			x = x<<8 | uint64(c)
		}
		if (x >> 63) > 0 {
			p.err = ErrHeader // 整数溢出
			return 0
		}
		if inv == 0xff {
			return ^int64(x)
		}
		return int64(x)
	}

	// 正常情况是 base-8（八进制）格式。
	return p.parseOctal(b)
}

// formatNumeric 如果可能，使用 base-8（八进制）编码将 x 编码到 b 中。
// 否则它将尝试使用 base-256（二进制）编码。
func (f *formatter) formatNumeric(b []byte, x int64) {
	if fitsInOctal(len(b), x) {
		f.formatOctal(b, x)
		return
	}

	if fitsInBase256(len(b), x) {
		for i := len(b) - 1; i >= 0; i-- {
			b[i] = byte(x)
			x >>= 8
		}
		b[0] |= 0x80 // 最高位表示二进制格式
		return
	}

	f.formatOctal(b, 0) // 最后手段，只写零
	f.err = ErrFieldTooLong
}

func (p *parser) parseOctal(b []byte) int64 {
	// 因为未使用的字段用 NUL 填充，我们需要
	// 跳过前导 NUL。字段也可能用空格或 NUL 填充。
	// 因此我们删除前导和尾随的 NUL 和空格以确保正确。
	b = bytes.Trim(b, " \x00")

	if len(b) == 0 {
		return 0
	}
	x, perr := strconv.ParseUint(p.parseString(b), 8, 64)
	if perr != nil {
		p.err = ErrHeader
	}
	return int64(x)
}

func (f *formatter) formatOctal(b []byte, x int64) {
	if !fitsInOctal(len(b), x) {
		x = 0 // 最后手段，只写零
		f.err = ErrFieldTooLong
	}

	s := strconv.FormatInt(x, 8)
	// 添加前导零，但为 NUL 留出空间。
	if n := len(b) - len(s) - 1; n > 0 {
		s = strings.Repeat("0", n) + s
	}
	f.formatString(b, s)
}

// fitsInOctal 报告整数 x 是否适合使用带有适当 NUL 终止符的
// 八进制编码的 n 字节长的字段。
func fitsInOctal(n int, x int64) bool {
	octBits := uint(n-1) * 3
	return x >= 0 && (n >= 22 || x < 1<<octBits)
}

// parsePAXTime 接受 PAX 规范中描述的 %d.%d 形式的字符串。
// 注意，此实现允许负时间戳，
// PAX 规范允许这样做，但并非总是可移植的。
func parsePAXTime(s string) (time.Time, error) {
	const maxNanoSecondDigits = 9

	// 将字符串分割为秒和亚秒部分。
	ss, sn, _ := strings.Cut(s, ".")

	// 解析秒。
	secs, err := strconv.ParseInt(ss, 10, 64)
	if err != nil {
		return time.Time{}, ErrHeader
	}
	if len(sn) == 0 {
		return time.Unix(secs, 0), nil // 没有亚秒值
	}

	// 解析纳秒。
	// 用 '0' 初始化数组以自动处理右填充。
	nanoDigits := [maxNanoSecondDigits]byte{'0', '0', '0', '0', '0', '0', '0', '0', '0'}
	for i := range len(sn) {
		switch c := sn[i]; {
		case c < '0' || c > '9':
			return time.Time{}, ErrHeader
		case i < len(nanoDigits):
			nanoDigits[i] = c
		}
	}
	nsecs, _ := strconv.ParseInt(string(nanoDigits[:]), 10, 64) // 验证后必定成功
	if len(ss) > 0 && ss[0] == '-' {
		return time.Unix(secs, -1*nsecs), nil // 负数修正
	}
	return time.Unix(secs, nsecs), nil
}

// formatPAXTime 将 ts 转换为 PAX 规范中描述的 %d.%d 形式的时间。
// 此函数能够处理负时间戳。
func formatPAXTime(ts time.Time) (s string) {
	secs, nsecs := ts.Unix(), ts.Nanosecond()
	if nsecs == 0 {
		return strconv.FormatInt(secs, 10)
	}

	// 如果秒为负，则执行修正。
	sign := ""
	if secs < 0 {
		sign = "-"             // 记住符号
		secs = -(secs + 1)     // 给 secs 加一秒
		nsecs = -(nsecs - 1e9) // 从 nsecs 中减去那一秒
	}
	return strings.TrimRight(fmt.Sprintf("%s%d.%09d", sign, secs, nsecs), "0")
}

// parsePAXRecord 将输入的 PAX 记录字符串解析为键值对。
// 如果解析成功，它将切掉当前读取的记录并返回剩余部分 r。
func parsePAXRecord(s string) (k, v, r string, err error) {
	// 大小字段在第一个空格处结束。
	nStr, rest, ok := strings.Cut(s, " ")
	if !ok {
		return "", "", s, ErrHeader
	}

	// 将第一个标记解析为十进制整数。
	n, perr := strconv.ParseInt(nStr, 10, 0) // 故意解析为原生 int
	if perr != nil || n < 5 || n > int64(len(s)) {
		return "", "", s, ErrHeader
	}
	n -= int64(len(nStr) + 1) // 从 s 中的索引转换为 rest 中的索引
	if n <= 0 {
		return "", "", s, ErrHeader
	}

	// 提取空格和最后换行符之间的所有内容。
	rec, nl, rem := rest[:n-1], rest[n-1:n], rest[n:]
	if nl != "\n" {
		return "", "", s, ErrHeader
	}

	// 第一个等号分隔键和值。
	k, v, ok = strings.Cut(rec, "=")
	if !ok {
		return "", "", s, ErrHeader
	}

	if !validPAXRecord(k, v) {
		return "", "", s, ErrHeader
	}
	return k, v, rem, nil
}

// formatPAXRecord 格式化单个 PAX 记录，并在前面加上适当的长度。
func formatPAXRecord(k, v string) (string, error) {
	if !validPAXRecord(k, v) {
		return "", ErrHeader
	}

	const padding = 3 // ' '、'=' 和 '\n' 的额外填充
	size := len(k) + len(v) + padding
	size += len(strconv.Itoa(size))
	record := strconv.Itoa(size) + " " + k + "=" + v + "\n"

	// 如果添加大小字段增加了记录大小，则进行最终调整。
	if len(record) != size {
		size = len(record)
		record = strconv.Itoa(size) + " " + k + "=" + v + "\n"
	}
	return record, nil
}

// validPAXRecord 报告键值对是否有效，其中每条记录的格式为：
//
//	"%d %s=%s\n" % (size, key, value)
//
// 键和值应该是 UTF-8，但由于存在大量有问题的写入器，
// 我们必须更加宽容。
// 因此，我们只拒绝所有带有 NUL 的键，并且只对 USTAR 字符串字段的
// PAX 版本拒绝值中的 NUL。
// 键不能包含 '=' 字符。
func validPAXRecord(k, v string) bool {
	if k == "" || strings.Contains(k, "=") {
		return false
	}
	switch k {
	case paxPath, paxLinkpath, paxUname, paxGname:
		return !hasNUL(v)
	default:
		return !hasNUL(k)
	}
}
