// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// pem 包实现了 PEM 数据编码，PEM 起源于隐私增强邮件（Privacy Enhanced Mail）。
// 如今 PEM 编码最常见的用途是在 TLS 密钥和证书中。见 RFC 1421。
package pem

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"slices"
	"strings"
)

// Block 表示一个 PEM 编码的结构。
//
// 编码形式为：
//
//	-----BEGIN Type-----
//	Headers
//	base64-encoded Bytes
//	-----END Type-----
//
// 其中 [Block.Headers] 是一个可能为空的 Key: Value 行序列。
type Block struct {
	Type    string            // 类型，取自前导部分（例如 "RSA PRIVATE KEY"）。
	Headers map[string]string // 可选的头部。
	Bytes   []byte            // 内容的解码字节。通常是 DER 编码的 ASN.1 结构。
}

// getLine 从给定的字节数组中返回由 \r\n 或 \n 分隔的第一行。
// 该行不包括尾部空白或尾部换行符字节。
// 还返回字节数组的余下部分（也不包括换行符字节），
// 该部分将始终小于原始参数。
func getLine(data []byte) (line, rest []byte, consumed int) {
	i := bytes.IndexByte(data, '\n')
	var j int
	if i < 0 {
		i = len(data)
		j = i
	} else {
		j = i + 1
		if i > 0 && data[i-1] == '\r' {
			i--
		}
	}
	return bytes.TrimRight(data[0:i], " \t"), data[j:], j
}

// removeSpacesAndTabs 返回其输入的副本，删除了所有空格和制表符（如果有）。
// 否则返回未修改的输入。
//
// base64 解码器已经跳过换行字符，所以我们不需要在这里过滤它们。
func removeSpacesAndTabs(data []byte) []byte {
	if !bytes.ContainsAny(data, " \t") {
		// 快速路径；PEM 中的大多数 base64 数据包含换行符，但没有空格或制表符。
		// 跳过额外的分配和工作。
		return data
	}
	result := make([]byte, len(data))
	n := 0

	for _, b := range data {
		if b == ' ' || b == '\t' {
			continue
		}
		result[n] = b
		n++
	}

	return result[0:n]
}

var pemStart = []byte("\n-----BEGIN ")
var pemEnd = []byte("\n-----END ")
var pemEndOfLine = []byte("-----")
var colon = []byte(":")

// Decode 将在输入中查找下一个 PEM 格式的块（证书、私钥等）。
// 它返回该块和输入的余下部分。如果未找到 PEM 数据，p 为 nil，
// 整个输入在 rest 中返回。块必须从一行的开始处开始，到一行的结尾处结束。
func Decode(data []byte) (p *Block, rest []byte) {
	// pemStart 以换行符开头。但在字节数组的最开始，
	// 我们将接受不带换行符的启动字符串。
	rest = data

	endTrailerIndex := 0
	for {
		// 如果我们已经尝试解析一个块，跳过我们已经看到的 END。
		if endTrailerIndex < 0 || endTrailerIndex > len(rest) {
			return nil, data
		}
		rest = rest[endTrailerIndex:]

		// 查找第一个 END 行，然后查找末尾行前的最后一个 BEGIN 行。
		// 这让我们可以跳过任何没有匹配的 END 的重复的 BEGIN 行。
		endIndex := bytes.Index(rest, pemEnd)
		if endIndex < 0 {
			return nil, data
		}
		endTrailerIndex = endIndex + len(pemEnd)
		beginIndex := bytes.LastIndex(rest[:endIndex], pemStart[1:])
		if beginIndex < 0 || (beginIndex > 0 && rest[beginIndex-1] != '\n') {
			continue
		}
		rest = rest[beginIndex+len(pemStart)-1:]
		endIndex -= beginIndex + len(pemStart) - 1
		endTrailerIndex -= beginIndex + len(pemStart) - 1

		var typeLine []byte
		var consumed int
		typeLine, rest, consumed = getLine(rest)
		endIndex -= consumed
		endTrailerIndex -= consumed
		if !bytes.HasSuffix(typeLine, pemEndOfLine) {
			continue
		}
		typeLine = typeLine[0 : len(typeLine)-len(pemEndOfLine)]

		p = &Block{
			Headers: make(map[string]string),
			Type:    string(typeLine),
		}

		for {
			// This loop terminates because getLine's second result is
			// always smaller than its argument.
			if len(rest) == 0 {
				return nil, data
			}
			line, next, consumed := getLine(rest)

			key, val, ok := bytes.Cut(line, colon)
			if !ok {
				break
			}

			// TODO(agl): 需要处理跨越多行的值。
			key = bytes.TrimSpace(key)
			val = bytes.TrimSpace(val)
			p.Headers[string(key)] = string(val)
			rest = next
			endIndex -= consumed
			endTrailerIndex -= consumed
		}

		// If there were headers, there 必须是 a newline between the headers
		// and the END line, so endIndex 应该是 >= 0.
		if len(p.Headers) > 0 && endIndex < 0 {
			continue
		}

		// After the "-----" of the ending line, there 应该是 the same type
		// and then a final five dashes.
		endTrailer := rest[endTrailerIndex:]
		endTrailerLen := len(typeLine) + len(pemEndOfLine)
		if len(endTrailer) < endTrailerLen {
			continue
		}

		restOfEndLine := endTrailer[endTrailerLen:]
		endTrailer = endTrailer[:endTrailerLen]
		if !bytes.HasPrefix(endTrailer, typeLine) ||
			!bytes.HasSuffix(endTrailer, pemEndOfLine) {
			continue
		}

		// 该行必须只以空白符结尾。
		if s, _, _ := getLine(restOfEndLine); len(s) != 0 {
			continue
		}

		p.Bytes = []byte{}
		if endIndex > 0 {
			base64Data := removeSpacesAndTabs(rest[:endIndex])
			p.Bytes = make([]byte, base64.StdEncoding.DecodedLen(len(base64Data)))
			n, err := base64.StdEncoding.Decode(p.Bytes, base64Data)
			if err != nil {
				continue
			}
			p.Bytes = p.Bytes[:n]
		}

		// the -1 is because we might have only matched pemEnd without the
		// leading newline 如果 PEM block was empty.
		_, rest, _ = getLine(rest[endIndex+len(pemEnd)-1:])
		return p, rest
	}
}

const pemLineLength = 64

type lineBreaker struct {
	line [pemLineLength]byte
	used int
	out  io.Writer
}

var nl = []byte{'\n'}

func (l *lineBreaker) Write(b []byte) (n int, err error) {
	if l.used+len(b) < pemLineLength {
		copy(l.line[l.used:], b)
		l.used += len(b)
		return len(b), nil
	}

	n, err = l.out.Write(l.line[0:l.used])
	if err != nil {
		return
	}
	excess := pemLineLength - l.used
	l.used = 0

	n, err = l.out.Write(b[0:excess])
	if err != nil {
		return
	}

	n, err = l.out.Write(nl)
	if err != nil {
		return
	}

	return l.Write(b[excess:])
}

func (l *lineBreaker) Close() (err error) {
	if l.used > 0 {
		_, err = l.out.Write(l.line[0:l.used])
		if err != nil {
			return
		}
		_, err = l.out.Write(nl)
	}

	return
}

func writeHeader(out io.Writer, k, v string) error {
	_, err := out.Write([]byte(k + ": " + v + "\n"))
	return err
}

// Encode 将 b 的 PEM 编码写入 out。
func Encode(out io.Writer, b *Block) error {
	// 在写入任何输出前检查无效块。
	for k := range b.Headers {
		if strings.包含(k, ":") {
			return errors.New("pem: cannot encode a header key that 包含 a colon")
		}
	}

	// 以下所有错误都来自于基础 io.Writer，
	// 所以现在可以安全地写入数据。

	if _, err := out.Write(pemStart[1:]); err != nil {
		return err
	}
	if _, err := out.Write([]byte(b.Type + "-----\n")); err != nil {
		return err
	}

	if len(b.Headers) > 0 {
		const procType = "Proc-Type"
		h := make([]string, 0, len(b.Headers))
		hasProcType := false
		for k := range b.Headers {
			if k == procType {
				hasProcType = true
				continue
			}
			h = append(h, k)
		}
		// Proc-Type 头部必须首先写入。
		// 参见 RFC 1421，section 4.6.1.1
		if hasProcType {
			if err := writeHeader(out, procType, b.Headers[procType]); err != nil {
				return err
			}
		}
		// 为了输出的一致性，按键排序写入其他头部。
		slices.Sort(h)
		for _, k := range h {
			if err := writeHeader(out, k, b.Headers[k]); err != nil {
				return err
			}
		}
		if _, err := out.Write(nl); err != nil {
			return err
		}
	}

	var breaker lineBreaker
	breaker.out = out

	b64 := base64.NewEncoder(base64.StdEncoding, &breaker)
	if _, err := b64.Write(b.Bytes); err != nil {
		return err
	}
	b64.Close()
	breaker.Close()

	if _, err := out.Write(pemEnd[1:]); err != nil {
		return err
	}
	_, err := out.Write([]byte(b.Type + "-----\n"))
	return err
}

// EncodeToMemory 返回 b 的 PEM 编码。
//
// 如果 b 有无效的头部且无法编码，
// EncodeToMemory 返回 nil。如果必须
// 报告此错误情况的详细信息，请使用 [Encode] 替代。
func EncodeToMemory(b *Block) []byte {
	var buf bytes.Buffer
	if err := Encode(&buf, b); err != nil {
		return nil
	}
	return buf.Bytes()
}
