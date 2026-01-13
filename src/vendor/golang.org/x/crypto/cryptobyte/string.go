// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// cryptobyte 包包含有助于解析和构造长度前缀二进制消息（包括 ASN.1 DER）的类型。
//（asn1 子包包含有用的 ASN.1 常量。）
//
// String 类型用于解析。它包装一个 []byte 切片，并提供用于逐值消费结构的辅助函数。
//
// Builder 类型用于构造消息。它提供用于追加值以及追加长度前缀子消息的辅助函数，
// 无需担心提前计算长度前缀。
//
// 有关入门信息，请参阅 Builder 和 String 类型的文档和示例。
package cryptobyte

// String 表示一个字节字符串。它提供从中解析固定长度和长度前缀值的方法。
type String []byte

// read 使 String 前进 n 个字节并返回它们。如果剩余字节少于 n，则返回 nil。
func (s *String) read(n int) []byte {
	if len(*s) < n || n < 0 {
		return nil
	}
	v := (*s)[:n]
	*s = (*s)[n:]
	return v
}

// Skip 使 String 前进 n 个字节并报告是否成功。
func (s *String) Skip(n int) bool {
	return s.read(n) != nil
}

// ReadUint8 将一个 8 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint8(out *uint8) bool {
	v := s.read(1)
	if v == nil {
		return false
	}
	*out = uint8(v[0])
	return true
}

// ReadUint16 将一个大端序 16 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint16(out *uint16) bool {
	v := s.read(2)
	if v == nil {
		return false
	}
	*out = uint16(v[0])<<8 | uint16(v[1])
	return true
}

// ReadUint24 将一个大端序 24 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint24(out *uint32) bool {
	v := s.read(3)
	if v == nil {
		return false
	}
	*out = uint32(v[0])<<16 | uint32(v[1])<<8 | uint32(v[2])
	return true
}

// ReadUint32 将一个大端序 32 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint32(out *uint32) bool {
	v := s.read(4)
	if v == nil {
		return false
	}
	*out = uint32(v[0])<<24 | uint32(v[1])<<16 | uint32(v[2])<<8 | uint32(v[3])
	return true
}

// ReadUint48 将一个大端序 48 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint48(out *uint64) bool {
	v := s.read(6)
	if v == nil {
		return false
	}
	*out = uint64(v[0])<<40 | uint64(v[1])<<32 | uint64(v[2])<<24 | uint64(v[3])<<16 | uint64(v[4])<<8 | uint64(v[5])
	return true
}

// ReadUint64 将一个大端序 64 位值解码到 out 并跳过它。它报告读取是否成功。
func (s *String) ReadUint64(out *uint64) bool {
	v := s.read(8)
	if v == nil {
		return false
	}
	*out = uint64(v[0])<<56 | uint64(v[1])<<48 | uint64(v[2])<<40 | uint64(v[3])<<32 | uint64(v[4])<<24 | uint64(v[5])<<16 | uint64(v[6])<<8 | uint64(v[7])
	return true
}

func (s *String) readUnsigned(out *uint32, length int) bool {
	v := s.read(length)
	if v == nil {
		return false
	}
	var result uint32
	for i := 0; i < length; i++ {
		result <<= 8
		result |= uint32(v[i])
	}
	*out = result
	return true
}

func (s *String) readLengthPrefixed(lenLen int, outChild *String) bool {
	lenBytes := s.read(lenLen)
	if lenBytes == nil {
		return false
	}
	var length uint32
	for _, b := range lenBytes {
		length = length << 8
		length = length | uint32(b)
	}
	v := s.read(int(length))
	if v == nil {
		return false
	}
	*outChild = v
	return true
}

// ReadUint8LengthPrefixed 读取 8 位长度前缀值的内容到 out 并跳过它。
// 它报告读取是否成功。
func (s *String) ReadUint8LengthPrefixed(out *String) bool {
	return s.readLengthPrefixed(1, out)
}

// ReadUint16LengthPrefixed 读取大端序 16 位长度前缀值的内容到 out 并跳过它。
// 它报告读取是否成功。
func (s *String) ReadUint16LengthPrefixed(out *String) bool {
	return s.readLengthPrefixed(2, out)
}

// ReadUint24LengthPrefixed 读取大端序 24 位长度前缀值的内容到 out 并跳过它。
// 它报告读取是否成功。
func (s *String) ReadUint24LengthPrefixed(out *String) bool {
	return s.readLengthPrefixed(3, out)
}

// ReadBytes 读取 n 个字节到 out 并跳过它们。它报告读取是否成功。
func (s *String) ReadBytes(out *[]byte, n int) bool {
	v := s.read(n)
	if v == nil {
		return false
	}
	*out = v
	return true
}

// CopyBytes 复制 len(out) 个字节到 out 并跳过它们。它报告复制操作是否成功。
func (s *String) CopyBytes(out []byte) bool {
	n := len(out)
	v := s.read(n)
	if v == nil {
		return false
	}
	return copy(out, v) == n
}

// Empty 报告字符串是否不包含任何字节。
func (s String) Empty() bool {
	return len(s) == 0
}
