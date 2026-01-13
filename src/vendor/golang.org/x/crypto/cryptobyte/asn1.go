// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package cryptobyte

import (
	encoding_asn1 "encoding/asn1"
	"fmt"
	"math/big"
	"reflect"
	"time"

	"golang.org/x/crypto/cryptobyte/asn1"
)

// 此文件包含 String 和 Builder 的 ASN.1 相关方法。

// Builder

// AddASN1Int64 追加一个 DER 编码的 ASN.1 INTEGER。
func (b *Builder) AddASN1Int64(v int64) {
	b.addASN1Signed(asn1.INTEGER, v)
}

// AddASN1Int64WithTag 追加一个带有给定标签的 DER 编码的 ASN.1 INTEGER。
func (b *Builder) AddASN1Int64WithTag(v int64, tag asn1.Tag) {
	b.addASN1Signed(tag, v)
}

// AddASN1Enum 追加一个 DER 编码的 ASN.1 ENUMERATION。
func (b *Builder) AddASN1Enum(v int64) {
	b.addASN1Signed(asn1.ENUM, v)
}

func (b *Builder) addASN1Signed(tag asn1.Tag, v int64) {
	b.AddASN1(tag, func(c *Builder) {
		length := 1
		for i := v; i >= 0x80 || i < -0x80; i >>= 8 {
			length++
		}

		for ; length > 0; length-- {
			i := v >> uint((length-1)*8) & 0xff
			c.AddUint8(uint8(i))
		}
	})
}

// AddASN1Uint64 追加一个 DER 编码的 ASN.1 INTEGER。
func (b *Builder) AddASN1Uint64(v uint64) {
	b.AddASN1(asn1.INTEGER, func(c *Builder) {
		length := 1
		for i := v; i >= 0x80; i >>= 8 {
			length++
		}

		for ; length > 0; length-- {
			i := v >> uint((length-1)*8) & 0xff
			c.AddUint8(uint8(i))
		}
	})
}

// AddASN1BigInt 追加一个 DER 编码的 ASN.1 INTEGER。
func (b *Builder) AddASN1BigInt(n *big.Int) {
	if b.err != nil {
		return
	}

	b.AddASN1(asn1.INTEGER, func(c *Builder) {
		if n.Sign() < 0 {
			// 负数必须转换为二进制补码形式。所以我们取反并减 1。
			// 如果最高有效位未设置，则需要在开头填充 0xff 以保持数字为负。
			nMinus1 := new(big.Int).Neg(n)
			nMinus1.Sub(nMinus1, bigOne)
			bytes := nMinus1.Bytes()
			for i := range bytes {
				bytes[i] ^= 0xff
			}
			if len(bytes) == 0 || bytes[0]&0x80 == 0 {
				c.add(0xff)
			}
			c.add(bytes...)
		} else if n.Sign() == 0 {
			c.add(0)
		} else {
			bytes := n.Bytes()
			if bytes[0]&0x80 != 0 {
				c.add(0)
			}
			c.add(bytes...)
		}
	})
}

// AddASN1OctetString 追加一个 DER 编码的 ASN.1 OCTET STRING。
func (b *Builder) AddASN1OctetString(bytes []byte) {
	b.AddASN1(asn1.OCTET_STRING, func(c *Builder) {
		c.AddBytes(bytes)
	})
}

const generalizedTimeFormatStr = "20060102150405Z0700"

// AddASN1GeneralizedTime 追加一个 DER 编码的 ASN.1 GENERALIZEDTIME。
func (b *Builder) AddASN1GeneralizedTime(t time.Time) {
	if t.Year() < 0 || t.Year() > 9999 {
		b.err = fmt.Errorf("cryptobyte: cannot represent %v as a GeneralizedTime", t)
		return
	}
	b.AddASN1(asn1.GeneralizedTime, func(c *Builder) {
		c.AddBytes([]byte(t.Format(generalizedTimeFormatStr)))
	})
}

// AddASN1UTCTime 追加一个 DER 编码的 ASN.1 UTCTime。
func (b *Builder) AddASN1UTCTime(t time.Time) {
	b.AddASN1(asn1.UTCTime, func(c *Builder) {
		// 按照 X.509 配置文件的使用方式，UTCTime 只能表示 1950 到 2049 年。
		if t.Year() < 1950 || t.Year() >= 2050 {
			b.err = fmt.Errorf("cryptobyte: cannot represent %v as a UTCTime", t)
			return
		}
		c.AddBytes([]byte(t.Format(defaultUTCTimeFormatStr)))
	})
}

// AddASN1BitString 追加一个 DER 编码的 ASN.1 BIT STRING。
// 这不支持不是完整字节数的 BIT STRING。
func (b *Builder) AddASN1BitString(data []byte) {
	b.AddASN1(asn1.BIT_STRING, func(b *Builder) {
		b.AddUint8(0)
		b.AddBytes(data)
	})
}

func (b *Builder) addBase128Int(n int64) {
	var length int
	if n == 0 {
		length = 1
	} else {
		for i := n; i > 0; i >>= 7 {
			length++
		}
	}

	for i := length - 1; i >= 0; i-- {
		o := byte(n >> uint(i*7))
		o &= 0x7f
		if i != 0 {
			o |= 0x80
		}

		b.add(o)
	}
}

func isValidOID(oid encoding_asn1.ObjectIdentifier) bool {
	if len(oid) < 2 {
		return false
	}

	if oid[0] > 2 || (oid[0] <= 1 && oid[1] >= 40) {
		return false
	}

	for _, v := range oid {
		if v < 0 {
			return false
		}
	}

	return true
}

func (b *Builder) AddASN1ObjectIdentifier(oid encoding_asn1.ObjectIdentifier) {
	b.AddASN1(asn1.OBJECT_IDENTIFIER, func(b *Builder) {
		if !isValidOID(oid) {
			b.err = fmt.Errorf("cryptobyte: invalid OID: %v", oid)
			return
		}

		b.addBase128Int(int64(oid[0])*40 + int64(oid[1]))
		for _, v := range oid[2:] {
			b.addBase128Int(int64(v))
		}
	})
}

func (b *Builder) AddASN1Boolean(v bool) {
	b.AddASN1(asn1.BOOLEAN, func(b *Builder) {
		if v {
			b.AddUint8(0xff)
		} else {
			b.AddUint8(0)
		}
	})
}

func (b *Builder) AddASN1NULL() {
	b.add(uint8(asn1.NULL), 0)
}

// MarshalASN1 对其输入调用 encoding_asn1.Marshal，如果成功则追加结果，
// 如果发生错误则记录错误。
func (b *Builder) MarshalASN1(v interface{}) {
	// 注意(martinkr): 这有点像是一个技巧，用于将 encoding_asn1.Marshal 错误
	// 传播到 Builder.err。注意：如果你用嵌入到结构体中的值调用 MarshalASN1，
	// 其标签信息会丢失。
	if b.err != nil {
		return
	}
	bytes, err := encoding_asn1.Marshal(v)
	if err != nil {
		b.err = err
		return
	}
	b.AddBytes(bytes)
}

// AddASN1 追加一个 ASN.1 对象。对象以给定的标签为前缀。
// 不支持大于 30 的标签，会导致错误（即仅支持低标签号形式）。
// 传递给 BuilderContinuation 的子构建器可用于构建 ASN.1 对象的内容。
func (b *Builder) AddASN1(tag asn1.Tag, f BuilderContinuation) {
	if b.err != nil {
		return
	}
	// 低五位全部设置的标识符表示高标签号格式（两个或更多八位字节），我们不支持。
	if tag&0x1f == 0x1f {
		b.err = fmt.Errorf("cryptobyte: high-tag number identifier octets not supported: 0x%x", tag)
		return
	}
	b.AddUint8(uint8(tag))
	b.addLengthPrefixed(1, true, f)
}

// String

// ReadASN1Boolean 解码一个 ASN.1 BOOLEAN 并将其转换为布尔表示形式存入 out，
// 然后前进。它报告读取是否成功。
func (s *String) ReadASN1Boolean(out *bool) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.BOOLEAN) || len(bytes) != 1 {
		return false
	}

	switch bytes[0] {
	case 0:
		*out = false
	case 0xff:
		*out = true
	default:
		return false
	}

	return true
}

// ReadASN1Integer 将 ASN.1 INTEGER 解码到 out 并前进。如果 out 不指向整数、
// big.Int 或 []byte，则会 panic。只有正值和零值可以解码到 []byte，
// 它们作为与 s 共享内存的大端二进制值返回。正值不会有前导零，
// 零将作为单个零字节返回。ReadASN1Integer 报告读取是否成功。
func (s *String) ReadASN1Integer(out interface{}) bool {
	switch out := out.(type) {
	case *int, *int8, *int16, *int32, *int64:
		var i int64
		if !s.readASN1Int64(&i) || reflect.ValueOf(out).Elem().OverflowInt(i) {
			return false
		}
		reflect.ValueOf(out).Elem().SetInt(i)
		return true
	case *uint, *uint8, *uint16, *uint32, *uint64:
		var u uint64
		if !s.readASN1Uint64(&u) || reflect.ValueOf(out).Elem().OverflowUint(u) {
			return false
		}
		reflect.ValueOf(out).Elem().SetUint(u)
		return true
	case *big.Int:
		return s.readASN1BigInt(out)
	case *[]byte:
		return s.readASN1Bytes(out)
	default:
		panic("out does not point to an integer type")
	}
}

func checkASN1Integer(bytes []byte) bool {
	if len(bytes) == 0 {
		// INTEGER 至少用一个八位字节编码。
		return false
	}
	if len(bytes) == 1 {
		return true
	}
	if bytes[0] == 0 && bytes[1]&0x80 == 0 || bytes[0] == 0xff && bytes[1]&0x80 == 0x80 {
		// 值不是最小编码。
		return false
	}
	return true
}

var bigOne = big.NewInt(1)

func (s *String) readASN1BigInt(out *big.Int) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.INTEGER) || !checkASN1Integer(bytes) {
		return false
	}
	if bytes[0]&0x80 == 0x80 {
		// 负数。
		neg := make([]byte, len(bytes))
		for i, b := range bytes {
			neg[i] = ^b
		}
		out.SetBytes(neg)
		out.Add(out, bigOne)
		out.Neg(out)
	} else {
		out.SetBytes(bytes)
	}
	return true
}

func (s *String) readASN1Bytes(out *[]byte) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.INTEGER) || !checkASN1Integer(bytes) {
		return false
	}
	if bytes[0]&0x80 == 0x80 {
		return false
	}
	for len(bytes) > 1 && bytes[0] == 0 {
		bytes = bytes[1:]
	}
	*out = bytes
	return true
}

func (s *String) readASN1Int64(out *int64) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.INTEGER) || !checkASN1Integer(bytes) || !asn1Signed(out, bytes) {
		return false
	}
	return true
}

func asn1Signed(out *int64, n []byte) bool {
	length := len(n)
	if length > 8 {
		return false
	}
	for i := 0; i < length; i++ {
		*out <<= 8
		*out |= int64(n[i])
	}
	// 向上和向下移位以对结果进行符号扩展。
	*out <<= 64 - uint8(length)*8
	*out >>= 64 - uint8(length)*8
	return true
}

func (s *String) readASN1Uint64(out *uint64) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.INTEGER) || !checkASN1Integer(bytes) || !asn1Unsigned(out, bytes) {
		return false
	}
	return true
}

func asn1Unsigned(out *uint64, n []byte) bool {
	length := len(n)
	if length > 9 || length == 9 && n[0] != 0 {
		// 对于 uint64 来说太大。
		return false
	}
	if n[0]&0x80 != 0 {
		// 负数。
		return false
	}
	for i := 0; i < length; i++ {
		*out <<= 8
		*out |= uint64(n[i])
	}
	return true
}

// ReadASN1Int64WithTag 将带有给定标签的 ASN.1 INTEGER 解码到 out 并前进。
// 它报告读取是否成功并产生了可以用 int64 表示的值。
func (s *String) ReadASN1Int64WithTag(out *int64, tag asn1.Tag) bool {
	var bytes String
	return s.ReadASN1(&bytes, tag) && checkASN1Integer(bytes) && asn1Signed(out, bytes)
}

// ReadASN1Enum 将 ASN.1 ENUMERATION 解码到 out 并前进。
// 它报告读取是否成功。
func (s *String) ReadASN1Enum(out *int) bool {
	var bytes String
	var i int64
	if !s.ReadASN1(&bytes, asn1.ENUM) || !checkASN1Integer(bytes) || !asn1Signed(&i, bytes) {
		return false
	}
	if int64(int(i)) != i {
		return false
	}
	*out = int(i)
	return true
}

func (s *String) readBase128Int(out *int) bool {
	ret := 0
	for i := 0; len(*s) > 0; i++ {
		if i == 5 {
			return false
		}
		// 避免在 32 位平台上溢出 int。
		// 我们不希望基于架构有不同的行为。
		if ret >= 1<<(31-7) {
			return false
		}
		ret <<= 7
		b := s.read(1)[0]

		// ITU-T X.690，第 8.19.2 节：
		// 子标识符应以尽可能少的八位字节编码，
		// 即子标识符的前导八位字节不应具有值 0x80。
		if i == 0 && b == 0x80 {
			return false
		}

		ret |= int(b & 0x7f)
		if b&0x80 == 0 {
			*out = ret
			return true
		}
	}
	return false // 被截断
}

// ReadASN1ObjectIdentifier 将 ASN.1 OBJECT IDENTIFIER 解码到 out 并前进。
// 它报告读取是否成功。
func (s *String) ReadASN1ObjectIdentifier(out *encoding_asn1.ObjectIdentifier) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.OBJECT_IDENTIFIER) || len(bytes) == 0 {
		return false
	}

	// 在最坏的情况下，我们从第一个字节（编码方式不同）获得两个元素，
	// 然后每个 varint 都是单字节长。
	components := make([]int, len(bytes)+1)

	// 第一个 varint 是 40*value1 + value2：
	// 根据这种打包方式，value1 只能取值 0、1 和 2。
	// 当 value1 = 0 或 value1 = 1 时，value2 <= 39。当 value1 = 2 时，
	// 对 value2 没有限制。
	var v int
	if !bytes.readBase128Int(&v) {
		return false
	}
	if v < 80 {
		components[0] = v / 40
		components[1] = v % 40
	} else {
		components[0] = 2
		components[1] = v - 80
	}

	i := 2
	for ; len(bytes) > 0; i++ {
		if !bytes.readBase128Int(&v) {
			return false
		}
		components[i] = v
	}
	*out = components[:i]
	return true
}

// ReadASN1GeneralizedTime 将 ASN.1 GENERALIZEDTIME 解码到 out 并前进。
// 它报告读取是否成功。
func (s *String) ReadASN1GeneralizedTime(out *time.Time) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.GeneralizedTime) {
		return false
	}
	t := string(bytes)
	res, err := time.Parse(generalizedTimeFormatStr, t)
	if err != nil {
		return false
	}
	if serialized := res.Format(generalizedTimeFormatStr); serialized != t {
		return false
	}
	*out = res
	return true
}

const defaultUTCTimeFormatStr = "060102150405Z0700"

// ReadASN1UTCTime 将 ASN.1 UTCTime 解码到 out 并前进。
// 它报告读取是否成功。
func (s *String) ReadASN1UTCTime(out *time.Time) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.UTCTime) {
		return false
	}
	t := string(bytes)

	formatStr := defaultUTCTimeFormatStr
	var err error
	res, err := time.Parse(formatStr, t)
	if err != nil {
		// 如果无法解析秒精度，则回退到分钟精度。
		// 如果我们遵循 X.509 或 X.690，我们不应该支持这个，但我们确实支持。
		formatStr = "0601021504Z0700"
		res, err = time.Parse(formatStr, t)
	}
	if err != nil {
		return false
	}

	if serialized := res.Format(formatStr); serialized != t {
		return false
	}

	if res.Year() >= 2050 {
		// UTCTime 将低位数字 50-99 解释为 1950-99。
		// 这仅适用于其在 X.509 配置文件中的使用。
		// 参见 https://tools.ietf.org/html/rfc5280#section-4.1.2.5.1
		res = res.AddDate(-100, 0, 0)
	}
	*out = res
	return true
}

// ReadASN1BitString 将 ASN.1 BIT STRING 解码到 out 并前进。
// 它报告读取是否成功。
func (s *String) ReadASN1BitString(out *encoding_asn1.BitString) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.BIT_STRING) || len(bytes) == 0 ||
		len(bytes)*8/8 != len(bytes) {
		return false
	}

	paddingBits := bytes[0]
	bytes = bytes[1:]
	if paddingBits > 7 ||
		len(bytes) == 0 && paddingBits != 0 ||
		len(bytes) > 0 && bytes[len(bytes)-1]&(1<<paddingBits-1) != 0 {
		return false
	}

	out.BitLength = len(bytes)*8 - int(paddingBits)
	out.Bytes = bytes
	return true
}

// ReadASN1BitStringAsBytes 将 ASN.1 BIT STRING 解码到 out 并前进。
// 如果 BIT STRING 不是完整的字节数，则会出错。它报告读取是否成功。
func (s *String) ReadASN1BitStringAsBytes(out *[]byte) bool {
	var bytes String
	if !s.ReadASN1(&bytes, asn1.BIT_STRING) || len(bytes) == 0 {
		return false
	}

	paddingBits := bytes[0]
	if paddingBits != 0 {
		return false
	}
	*out = bytes[1:]
	return true
}

// ReadASN1Bytes 读取 DER 编码的 ASN.1 元素的内容（不包括标签和长度字节）
// 到 out，并前进。元素必须匹配给定的标签。它报告读取是否成功。
func (s *String) ReadASN1Bytes(out *[]byte, tag asn1.Tag) bool {
	return s.ReadASN1((*String)(out), tag)
}

// ReadASN1 读取 DER 编码的 ASN.1 元素的内容（不包括标签和长度字节）
// 到 out，并前进。元素必须匹配给定的标签。它报告读取是否成功。
//
// 不支持大于 30 的标签（即仅支持低标签号格式）。
func (s *String) ReadASN1(out *String, tag asn1.Tag) bool {
	var t asn1.Tag
	if !s.ReadAnyASN1(out, &t) || t != tag {
		return false
	}
	return true
}

// ReadASN1Element 读取 DER 编码的 ASN.1 元素的内容（包括标签和长度字节）
// 到 out，并前进。元素必须匹配给定的标签。它报告读取是否成功。
//
// 不支持大于 30 的标签（即仅支持低标签号格式）。
func (s *String) ReadASN1Element(out *String, tag asn1.Tag) bool {
	var t asn1.Tag
	if !s.ReadAnyASN1Element(out, &t) || t != tag {
		return false
	}
	return true
}

// ReadAnyASN1 读取 DER 编码的 ASN.1 元素的内容（不包括标签和长度字节）
// 到 out，将 outTag 设置为其标签，并前进。它报告读取是否成功。
//
// 不支持大于 30 的标签（即仅支持低标签号格式）。
func (s *String) ReadAnyASN1(out *String, outTag *asn1.Tag) bool {
	return s.readASN1(out, outTag, true /* skip header */)
}

// ReadAnyASN1Element 读取 DER 编码的 ASN.1 元素的内容
//（包括标签和长度字节）到 out，将 outTag 设置为其标签，并前进。
// 它报告读取是否成功。
//
// 不支持大于 30 的标签（即仅支持低标签号格式）。
func (s *String) ReadAnyASN1Element(out *String, outTag *asn1.Tag) bool {
	return s.readASN1(out, outTag, false /* include header */)
}

// PeekASN1Tag 报告字符串上的下一个 ASN.1 值是否以给定的标签开头。
func (s String) PeekASN1Tag(tag asn1.Tag) bool {
	if len(s) == 0 {
		return false
	}
	return asn1.Tag(s[0]) == tag
}

// SkipASN1 读取并丢弃具有给定标签的 ASN.1 元素。它报告操作是否成功。
func (s *String) SkipASN1(tag asn1.Tag) bool {
	var unused String
	return s.ReadASN1(&unused, tag)
}

// ReadOptionalASN1 尝试将带有给定标签的 DER 编码的 ASN.1 元素的内容
//（不包括标签和长度字节）读取到 out。它将是否找到具有该标签的元素存储在
// outPresent 中，除非 outPresent 为 nil。它报告读取是否成功。
func (s *String) ReadOptionalASN1(out *String, outPresent *bool, tag asn1.Tag) bool {
	present := s.PeekASN1Tag(tag)
	if outPresent != nil {
		*outPresent = present
	}
	if present && !s.ReadASN1(out, tag) {
		return false
	}
	return true
}

// SkipOptionalASN1 使 s 跳过具有给定标签的 ASN.1 元素，
// 否则保持 s 不变。它报告操作是否成功。
func (s *String) SkipOptionalASN1(tag asn1.Tag) bool {
	if !s.PeekASN1Tag(tag) {
		return true
	}
	var unused String
	return s.ReadASN1(&unused, tag)
}

// ReadOptionalASN1Integer 尝试读取一个显式带有 tag 标签的可选 ASN.1 INTEGER
// 到 out 并前进。如果没有匹配标签的元素，则将 defaultValue 写入 out。
// 否则，其行为与 ReadASN1Integer 相同。
func (s *String) ReadOptionalASN1Integer(out interface{}, tag asn1.Tag, defaultValue interface{}) bool {
	var present bool
	var i String
	if !s.ReadOptionalASN1(&i, &present, tag) {
		return false
	}
	if !present {
		switch out.(type) {
		case *int, *int8, *int16, *int32, *int64,
			*uint, *uint8, *uint16, *uint32, *uint64, *[]byte:
			reflect.ValueOf(out).Elem().Set(reflect.ValueOf(defaultValue))
		case *big.Int:
			if defaultValue, ok := defaultValue.(*big.Int); ok {
				out.(*big.Int).Set(defaultValue)
			} else {
				panic("out points to big.Int, but defaultValue does not")
			}
		default:
			panic("invalid integer type")
		}
		return true
	}
	if !i.ReadASN1Integer(out) || !i.Empty() {
		return false
	}
	return true
}

// ReadOptionalASN1OctetString 尝试读取一个显式带有 tag 标签的可选
// ASN.1 OCTET STRING 到 out 并前进。如果没有匹配标签的元素，
// 则将 "out" 设置为 nil。它报告读取是否成功。
func (s *String) ReadOptionalASN1OctetString(out *[]byte, outPresent *bool, tag asn1.Tag) bool {
	var present bool
	var child String
	if !s.ReadOptionalASN1(&child, &present, tag) {
		return false
	}
	if outPresent != nil {
		*outPresent = present
	}
	if present {
		var oct String
		if !child.ReadASN1(&oct, asn1.OCTET_STRING) || !child.Empty() {
			return false
		}
		*out = oct
	} else {
		*out = nil
	}
	return true
}

// ReadOptionalASN1Boolean 尝试读取一个显式带有 tag 标签的可选
// ASN.1 BOOLEAN 到 out 并前进。如果没有匹配标签的元素，
// 则将 "out" 设置为 defaultValue。它报告读取是否成功。
func (s *String) ReadOptionalASN1Boolean(out *bool, tag asn1.Tag, defaultValue bool) bool {
	var present bool
	var child String
	if !s.ReadOptionalASN1(&child, &present, tag) {
		return false
	}

	if !present {
		*out = defaultValue
		return true
	}

	return child.ReadASN1Boolean(out)
}

func (s *String) readASN1(out *String, outTag *asn1.Tag, skipHeader bool) bool {
	if len(*s) < 2 {
		return false
	}
	tag, lenByte := (*s)[0], (*s)[1]

	if tag&0x1f == 0x1f {
		// ITU-T X.690 第 8.1.2 节
		//
		// 标签部分为 0x1f 的标识符八位字节表示具有两个或更多八位字节的
		// 高标签号形式标识符。我们只支持小于 31 的标签
		//（即低标签号形式，单八位字节标识符）。
		return false
	}

	if outTag != nil {
		*outTag = asn1.Tag(tag)
	}

	// ITU-T X.690 第 8.1.3 节
	//
	// 第一个长度字节的第 8 位指示长度是短形式还是长形式。
	var length, headerLen uint32 // length 包括 headerLen
	if lenByte&0x80 == 0 {
		// 短形式长度（第 8.1.3.4 节），编码在第 1-7 位。
		length = uint32(lenByte) + 2
		headerLen = 2
	} else {
		// 长形式长度（第 8.1.3.5 节）。第 1-7 位编码用于编码长度的八位字节数。
		lenLen := lenByte & 0x7f
		var len32 uint32

		if lenLen == 0 || lenLen > 4 || len(*s) < int(2+lenLen) {
			return false
		}

		lenBytes := String((*s)[2 : 2+lenLen])
		if !lenBytes.readUnsigned(&len32, int(lenLen)) {
			return false
		}

		// ITU-T X.690 第 10.1 节（DER 长度形式）要求使用最少数量的八位字节编码长度。
		if len32 < 128 {
			// 长度应该使用短形式编码。
			return false
		}
		if len32>>((lenLen-1)*8) == 0 {
			// 前导八位字节为 0。长度应该至少短一个字节。
			return false
		}

		headerLen = 2 + uint32(lenLen)
		if headerLen+len32 < len32 {
			// 溢出。
			return false
		}
		length = headerLen + len32
	}

	if int(length) < 0 || !s.ReadBytes((*[]byte)(out), int(length)) {
		return false
	}
	if skipHeader && !out.Skip(int(headerLen)) {
		panic("cryptobyte: internal error")
	}

	return true
}
