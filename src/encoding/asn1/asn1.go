// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// asn1 包实现了 DER 编码的 ASN.1 数据结构的解析，
// 如 ITU-T Rec X.690 中所定义。
//
// 另请参阅 "A Layman's Guide to a Subset of ASN.1, BER, and DER,"
// http://luca.ntop.org/Teaching/Appunti/asn1.html。
package asn1

// ASN.1 是一种用于指定抽象对象的语法，而 BER、DER、PER、XER 等
// 是这些对象的不同编码格式。在这里，我们将处理
// DER，即可辨别编码规则。DER 用于 X.509，因为
// 它解析速度快，而且与 BER 不同，每个对象都有唯一的编码。
// 在计算对象的哈希值时，重要的是结果字节在两端必须相同，
// 而 DER 消除了这种误差余地。
//
// ASN.1 非常复杂，本包并不打算实现所有功能。

import (
	"errors"
	"fmt"
	"internal/saferio"
	"math"
	"math/big"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// StructuralError 表示 ASN.1 数据是有效的，但接收它的 Go 类型不匹配。
type StructuralError struct {
	Msg string
}

func (e StructuralError) Error() string { return "asn1: structure error: " + e.Msg }

// SyntaxError 表示 ASN.1 数据是无效的。
type SyntaxError struct {
	Msg string
}

func (e SyntaxError) Error() string { return "asn1: syntax error: " + e.Msg }

// 我们首先依次处理每种原始类型。

// 布尔值

func parseBool(bytes []byte) (ret bool, err error) {
	if len(bytes) != 1 {
		err = SyntaxError{"invalid boolean"}
		return
	}

	// DER 要求"如果编码表示布尔值 TRUE，
	// 其单个内容八位字节的所有八位都应设置为 1。"
	// 因此只有 0 和 255 是有效的编码值。
	switch bytes[0] {
	case 0:
		ret = false
	case 0xff:
		ret = true
	default:
		err = SyntaxError{"invalid boolean"}
	}

	return
}

// 整数

// checkInteger 如果给定的字节是有效的 DER 编码整数则返回 nil，否则返回错误。
func checkInteger(bytes []byte) error {
	if len(bytes) == 0 {
		return StructuralError{"empty integer"}
	}
	if len(bytes) == 1 {
		return nil
	}
	if (bytes[0] == 0 && bytes[1]&0x80 == 0) || (bytes[0] == 0xff && bytes[1]&0x80 == 0x80) {
		return StructuralError{"integer not minimally-encoded"}
	}
	return nil
}

// parseInt64 将给定的字节视为大端序有符号整数并返回结果。
func parseInt64(bytes []byte) (ret int64, err error) {
	err = checkInteger(bytes)
	if err != nil {
		return
	}
	if len(bytes) > 8 {
		// 在这种情况下会溢出 int64。
		err = StructuralError{"integer too large"}
		return
	}
	for bytesRead := 0; bytesRead < len(bytes); bytesRead++ {
		ret <<= 8
		ret |= int64(bytes[bytesRead])
	}

	// 向上和向下移位以对结果进行符号扩展。
	ret <<= 64 - uint8(len(bytes))*8
	ret >>= 64 - uint8(len(bytes))*8
	return
}

// parseInt32 将给定的字节视为大端序有符号整数并返回结果。
func parseInt32(bytes []byte) (int32, error) {
	if err := checkInteger(bytes); err != nil {
		return 0, err
	}
	ret64, err := parseInt64(bytes)
	if err != nil {
		return 0, err
	}
	if ret64 != int64(int32(ret64)) {
		return 0, StructuralError{"integer too large"}
	}
	return int32(ret64), nil
}

var bigOne = big.NewInt(1)

// parseBigInt 将给定的字节视为大端序有符号整数并返回结果。
func parseBigInt(bytes []byte) (*big.Int, error) {
	if err := checkInteger(bytes); err != nil {
		return nil, err
	}
	ret := new(big.Int)
	if len(bytes) > 0 && bytes[0]&0x80 == 0x80 {
		// 这是一个负数。
		notBytes := make([]byte, len(bytes))
		for i := range notBytes {
			notBytes[i] = ^bytes[i]
		}
		ret.SetBytes(notBytes)
		ret.Add(ret, bigOne)
		ret.Neg(ret)
		return ret, nil
	}
	ret.SetBytes(bytes)
	return ret, nil
}

// 位串

// BitString 是当你需要 ASN.1 BIT STRING 类型时使用的结构。
// 位串在内存中填充到最近的字节边界，并记录有效位的数量。填充位将为零。
type BitString struct {
	Bytes     []byte // 打包到字节中的位。
	BitLength int    // 以位为单位的长度。
}

// At 返回给定索引处的位。如果索引超出范围则返回 0。
func (b BitString) At(i int) int {
	if i < 0 || i >= b.BitLength {
		return 0
	}
	x := i / 8
	y := 7 - uint(i%8)
	return int(b.Bytes[x]>>y) & 1
}

// RightAlign 返回一个填充位在开头的切片。该切片可能与 BitString 共享内存。
func (b BitString) RightAlign() []byte {
	shift := uint(8 - (b.BitLength % 8))
	if shift == 8 || len(b.Bytes) == 0 {
		return b.Bytes
	}

	a := make([]byte, len(b.Bytes))
	a[0] = b.Bytes[0] >> shift
	for i := 1; i < len(b.Bytes); i++ {
		a[i] = b.Bytes[i-1] << (8 - shift)
		a[i] |= b.Bytes[i] >> shift
	}

	return a
}

// parseBitString 从给定的字节切片解析 ASN.1 位串并返回它。
func parseBitString(bytes []byte) (ret BitString, err error) {
	if len(bytes) == 0 {
		err = SyntaxError{"zero length BIT STRING"}
		return
	}
	paddingBits := int(bytes[0])
	if paddingBits > 7 ||
		len(bytes) == 1 && paddingBits > 0 ||
		bytes[len(bytes)-1]&((1<<bytes[0])-1) != 0 {
		err = SyntaxError{"invalid padding bits in BIT STRING"}
		return
	}
	ret.BitLength = (len(bytes)-1)*8 - paddingBits
	ret.Bytes = bytes[1:]
	return
}

// 空值

// NullRawValue 是一个 [RawValue]，其 Tag 设置为 ASN.1 NULL 类型标签（5）。
var NullRawValue = RawValue{Tag: TagNull}

// NullBytes 包含表示 DER 编码的 ASN.1 NULL 类型的字节。
var NullBytes = []byte{TagNull, 0}

// 对象标识符

// ObjectIdentifier 表示一个 ASN.1 对象标识符。
type ObjectIdentifier []int

// Equal 报告 oi 和 other 是否表示相同的标识符。
func (oi ObjectIdentifier) Equal(other ObjectIdentifier) bool {
	return slices.Equal(oi, other)
}

func (oi ObjectIdentifier) String() string {
	var s strings.Builder
	s.Grow(32)

	buf := make([]byte, 0, 19)
	for i, v := range oi {
		if i > 0 {
			s.WriteByte('.')
		}
		s.Write(strconv.AppendInt(buf, int64(v), 10))
	}

	return s.String()
}

// parseObjectIdentifier 从给定的字节解析对象标识符并返回它。
// 对象标识符是在层次结构中分配的可变长度整数序列。
func parseObjectIdentifier(bytes []byte) (s ObjectIdentifier, err error) {
	if len(bytes) == 0 {
		err = SyntaxError{"zero length OBJECT IDENTIFIER"}
		return
	}

	// 在最坏的情况下，我们从第一个字节（其编码方式不同）获得两个元素，
	// 然后每个可变长度整数都是单字节长。
	s = make([]int, len(bytes)+1)

	// 第一个可变长度整数是 40*value1 + value2：
	// 根据这种打包方式，value1 只能取 0、1 和 2。
	// 当 value1 = 0 或 value1 = 1 时，value2 <= 39。当 value1 = 2 时，
	// 对 value2 没有限制。
	v, offset, err := parseBase128Int(bytes, 0)
	if err != nil {
		return
	}
	if v < 80 {
		s[0] = v / 40
		s[1] = v % 40
	} else {
		s[0] = 2
		s[1] = v - 80
	}

	i := 2
	for ; offset < len(bytes); i++ {
		v, offset, err = parseBase128Int(bytes, offset)
		if err != nil {
			return
		}
		s[i] = v
	}
	s = s[0:i]
	return
}

// 枚举

// Enumerated 表示为普通的 int。
type Enumerated int

// 标志

// Flag 接受任何数据，如果存在则设置为 true。
type Flag bool

// parseBase128Int 从给定字节切片的给定偏移量解析 base-128 编码的整数。
// 它返回值和新的偏移量。
func parseBase128Int(bytes []byte, initOffset int) (ret, offset int, err error) {
	offset = initOffset
	var ret64 int64
	for shifted := 0; offset < len(bytes); shifted++ {
		// 每字节 5 * 7 位 == 35 位数据
		// 因此表示要么是非最小的，要么对于 int32 来说太大
		if shifted == 5 {
			err = StructuralError{"base 128 integer too large"}
			return
		}
		ret64 <<= 7
		b := bytes[offset]
		// 整数应该是最小编码的，所以前导八位字节永远不应该是 0x80
		if shifted == 0 && b == 0x80 {
			err = SyntaxError{"integer is not minimally encoded"}
			return
		}
		ret64 |= int64(b & 0x7f)
		offset++
		if b&0x80 == 0 {
			ret = int(ret64)
			// 确保返回的值在所有平台上都适合 int
			if ret64 > math.MaxInt32 {
				err = StructuralError{"base 128 integer too large"}
			}
			return
		}
	}
	err = SyntaxError{"truncated base 128 integer"}
	return
}

// UTC 时间

func parseUTCTime(bytes []byte) (ret time.Time, err error) {
	s := string(bytes)

	formatStr := "0601021504Z0700"
	ret, err = time.Parse(formatStr, s)
	if err != nil {
		formatStr = "060102150405Z0700"
		ret, err = time.Parse(formatStr, s)
	}
	if err != nil {
		return
	}

	if serialized := ret.Format(formatStr); serialized != s {
		err = fmt.Errorf("asn1: time did not serialize back to the original value and may be invalid: given %q, but serialized as %q", s, serialized)
		return
	}

	if ret.Year() >= 2050 {
		// UTCTime 只编码 2050 年之前的时间。参见 https://tools.ietf.org/html/rfc5280#section-4.1.2.5.1
		ret = ret.AddDate(-100, 0, 0)
	}

	return
}

// parseGeneralizedTime 从给定的字节切片解析 GeneralizedTime 并返回结果时间。
func parseGeneralizedTime(bytes []byte) (ret time.Time, err error) {
	const formatStr = "20060102150405.999999999Z0700"
	s := string(bytes)

	if ret, err = time.Parse(formatStr, s); err != nil {
		return
	}

	if serialized := ret.Format(formatStr); serialized != s {
		err = fmt.Errorf("asn1: time did not serialize back to the original value and may be invalid: given %q, but serialized as %q", s, serialized)
	}

	return
}

// 数字字符串

// parseNumericString 从给定的字节数组解析 ASN.1 NumericString 并返回它。
func parseNumericString(bytes []byte) (ret string, err error) {
	for _, b := range bytes {
		if !isNumeric(b) {
			return "", SyntaxError{"NumericString 包含 invalid character"}
		}
	}
	return string(bytes), nil
}

// isNumeric 报告给定的 b 是否在 ASN.1 NumericString 集合中。
func isNumeric(b byte) bool {
	return '0' <= b && b <= '9' ||
		b == ' '
}

// 可打印字符串

// parsePrintableString 从给定的字节数组解析 ASN.1 PrintableString 并返回它。
func parsePrintableString(bytes []byte) (ret string, err error) {
	for _, b := range bytes {
		if !isPrintable(b, allowAsterisk, allowAmpersand) {
			err = SyntaxError{"PrintableString 包含 invalid character"}
			return
		}
	}
	ret = string(bytes)
	return
}

type asteriskFlag bool
type ampersandFlag bool

const (
	allowAsterisk  asteriskFlag = true
	rejectAsterisk asteriskFlag = false

	allowAmpersand  ampersandFlag = true
	rejectAmpersand ampersandFlag = false
)

// isPrintable 报告给定的 b 是否在 ASN.1 PrintableString 集合中。
// 如果 asterisk 是 allowAsterisk，则也允许 '*'，这反映了现有实践。
// 如果 ampersand 是 allowAmpersand，则也允许 '&'。
func isPrintable(b byte, asterisk asteriskFlag, ampersand ampersandFlag) bool {
	return 'a' <= b && b <= 'z' ||
		'A' <= b && b <= 'Z' ||
		'0' <= b && b <= '9' ||
		'\'' <= b && b <= ')' ||
		'+' <= b && b <= '/' ||
		b == ' ' ||
		b == ':' ||
		b == '=' ||
		b == '?' ||
		// 这在技术上不允许出现在 PrintableString 中。
		// 但是，带有通配符字符串的 x509 证书并不总是使用正确的字符串类型，
		// 所以我们允许它。
		(bool(asterisk) && b == '*') ||
		// 这在技术上也不允许。但是，它不仅相对常见，
		// 而且还有一些 CA 证书包含它。
		// 其中至少有一个要到 2027 年才会过期。
		(bool(ampersand) && b == '&')
}

// IA5 字符串

// parseIA5String 从给定的字节切片解析 ASN.1 IA5String（ASCII 字符串）并返回它。
func parseIA5String(bytes []byte) (ret string, err error) {
	for _, b := range bytes {
		if b >= utf8.RuneSelf {
			err = SyntaxError{"IA5String 包含 invalid character"}
			return
		}
	}
	ret = string(bytes)
	return
}

// T61 字符串

// parseT61String 从给定的字节切片解析 ASN.1 T61String（8 位干净字符串）并返回它。
func parseT61String(bytes []byte) (ret string, err error) {
	// T.61 是一种已废弃的 ITU 8 位字符编码，先于 Unicode 出现。
	// T.61 使用的代码页布局几乎完全映射到 ISO 8859-1（Latin-1）
	// 字符编码的代码页布局，但 Latin-1 中的一些字符在 T.61 中不存在。
	//
	// 我们不去映射哪些字符存在于 Latin-1 但不在 T.61 中，
	// 而是直接将这些字符串视为使用 Latin-1 编码。这与世界上大多数
	// 实现的做法一致，包括 BoringSSL。
	buf := make([]byte, 0, len(bytes))
	for _, v := range bytes {
		// 所有 1 字节的 UTF-8 符文与 Latin-1 一一对应。
		buf = utf8.AppendRune(buf, rune(v))
	}
	return string(buf), nil
}

// UTF8 字符串

// parseUTF8String 从给定的字节数组解析 ASN.1 UTF8String（原始 UTF-8）并返回它。
func parseUTF8String(bytes []byte) (ret string, err error) {
	if !utf8.Valid(bytes) {
		return "", errors.New("asn1: invalid UTF-8 string")
	}
	return string(bytes), nil
}

// BMP 字符串

// parseBMPString 从给定的字节切片解析 ASN.1 BMPString
//（ISO/IEC/ITU 10646-1 的基本多语言平面）并返回它。
func parseBMPString(bmpString []byte) (string, error) {
	// BMPString 使用已废弃的 UCS-2 16 位字符编码，
	// 它覆盖了基本多语言平面（BMP）。UTF-16 是 UCS-2 的扩展，
	// 包含所有相同的码点，但也包括多码点字符（通过使用代理码点）。
	// 我们可以将 UCS-2 编码的字符串视为 UTF-16 编码的字符串，
	// 只要我们拒绝 UTF-16 特定的码点。这与 BoringSSL 的行为一致。

	if len(bmpString)%2 != 0 {
		return "", errors.New("invalid BMPString")
	}

	// 如果存在终止符则去除。
	if l := len(bmpString); l >= 2 && bmpString[l-1] == 0 && bmpString[l-2] == 0 {
		bmpString = bmpString[:l-2]
	}

	s := make([]uint16, 0, len(bmpString)/2)
	for len(bmpString) > 0 {
		point := uint16(bmpString[0])<<8 + uint16(bmpString[1])
		// 拒绝永久保留的 UTF-16 码点：
		// 非字符（0xfffe、0xffff 和 0xfdd0-0xfdef）和代理项（0xd800-0xdfff）。
		if point == 0xfffe || point == 0xffff ||
			(point >= 0xfdd0 && point <= 0xfdef) ||
			(point >= 0xd800 && point <= 0xdfff) {
			return "", errors.New("invalid BMPString")
		}
		s = append(s, point)
		bmpString = bmpString[2:]
	}

	return string(utf16.Decode(s)), nil
}

// RawValue 表示一个未解码的 ASN.1 对象。
type RawValue struct {
	Class, Tag int
	IsCompound bool
	Bytes      []byte
	FullBytes  []byte // 包含标签和长度
}

// RawContent 用于表示需要为结构体保留未解码的 DER 数据。
// 要使用它，结构体的第一个字段必须是这种类型。
// 其他任何字段使用这种类型都是错误的。
type RawContent []byte

// 标签

// parseTagAndLength 从给定偏移量的字节切片中解析 ASN.1 标签和长度对。
// 它返回解析后的数据和新的偏移量。SET 和 SET OF（标签 17）被映射到
// SEQUENCE 和 SEQUENCE OF（标签 16），因为在本代码中我们不区分有序和无序对象。
func parseTagAndLength(bytes []byte, initOffset int) (ret tagAndLength, offset int, err error) {
	offset = initOffset
	// parseTagAndLength 不应该在没有至少一个字节可读的情况下被调用。
	// 因此这个检查是为了健壮性：
	if offset >= len(bytes) {
		err = errors.New("asn1: internal error in parseTagAndLength")
		return
	}
	b := bytes[offset]
	offset++
	ret.class = int(b >> 6)
	ret.isCompound = b&0x20 == 0x20
	ret.tag = int(b & 0x1f)

	// 如果低五位被设置，那么标签号实际上是之后的 base 128 编码
	if ret.tag == 0x1f {
		ret.tag, offset, err = parseBase128Int(bytes, offset)
		if err != nil {
			return
		}
		// 标签应该以最小形式编码。
		if ret.tag < 0x1f {
			err = SyntaxError{"non-minimal tag"}
			return
		}
	}
	if offset >= len(bytes) {
		err = SyntaxError{"truncated tag or length"}
		return
	}
	b = bytes[offset]
	offset++
	if b&0x80 == 0 {
		// 长度编码在低 7 位中。
		ret.length = int(b & 0x7f)
	} else {
		// 低 7 位给出后续长度字节的数量。
		numBytes := int(b & 0x7f)
		if numBytes == 0 {
			err = SyntaxError{"indefinite length found (not DER)"}
			return
		}
		ret.length = 0
		for i := 0; i < numBytes; i++ {
			if offset >= len(bytes) {
				err = SyntaxError{"truncated tag or length"}
				return
			}
			b = bytes[offset]
			offset++
			if ret.length >= 1<<23 {
				// 我们无法在不溢出的情况下左移 ret.length。
				err = StructuralError{"length too large"}
				return
			}
			ret.length <<= 8
			ret.length |= int(b)
			if ret.length == 0 {
				// DER 要求长度是最小的。
				err = StructuralError{"superfluous leading zeros in length"}
				return
			}
		}
		// 短长度必须以短形式编码。
		if ret.length < 0x80 {
			err = StructuralError{"non-minimal length"}
			return
		}
	}

	return
}

// parseSequenceOf 用于 SEQUENCE OF 和 SET OF 值。它尝试从给定的字节切片
// 解析多个 ASN.1 值，并将它们作为给定类型的 Go 值切片返回。
func parseSequenceOf(bytes []byte, sliceType reflect.Type, elemType reflect.Type) (ret reflect.Value, err error) {
	matchAny, expectedTag, compoundType, ok := getUniversalType(elemType)
	if !ok {
		err = StructuralError{"unknown Go type for slice"}
		return
	}

	// 首先我们遍历输入并计算元素数量，
	// 检查每种情况下类型是否正确。
	numElements := 0
	for offset := 0; offset < len(bytes); {
		var t tagAndLength
		t, offset, err = parseTagAndLength(bytes, offset)
		if err != nil {
			return
		}
		switch t.tag {
		case TagIA5String, TagGeneralString, TagT61String, TagUTF8String, TagNumericString, TagBMPString:
			// 我们假装各种其他字符串类型是 PRINTABLE STRING，
			// 这样它们的序列就可以被解析为 []string。
			t.tag = TagPrintableString
		case TagGeneralizedTime, TagUTCTime:
			// 同样，两种时间类型被同等对待。
			t.tag = TagUTCTime
		}

		if !matchAny && (t.class != ClassUniversal || t.isCompound != compoundType || t.tag != expectedTag) {
			err = StructuralError{"sequence tag mismatch"}
			return
		}
		if invalidLength(offset, t.length, len(bytes)) {
			err = SyntaxError{"truncated sequence"}
			return
		}
		offset += t.length
		numElements++
	}
	elemSize := uint64(elemType.Size())
	safeCap := saferio.SliceCapWithSize(elemSize, uint64(numElements))
	if safeCap < 0 {
		err = SyntaxError{fmt.Sprintf("%s slice too big: %d elements of %d bytes", elemType.Kind(), numElements, elemSize)}
		return
	}
	ret = reflect.MakeSlice(sliceType, 0, safeCap)
	params := fieldParameters{}
	offset := 0
	for i := 0; i < numElements; i++ {
		ret = reflect.Append(ret, reflect.Zero(elemType))
		offset, err = parseField(ret.Index(i), bytes, offset, params)
		if err != nil {
			return
		}
	}
	return
}

var (
	bitStringType        = reflect.TypeFor[BitString]()
	objectIdentifierType = reflect.TypeFor[ObjectIdentifier]()
	enumeratedType       = reflect.TypeFor[Enumerated]()
	flagType             = reflect.TypeFor[Flag]()
	timeType             = reflect.TypeFor[time.Time]()
	rawValueType         = reflect.TypeFor[RawValue]()
	rawContentsType      = reflect.TypeFor[RawContent]()
	bigIntType           = reflect.TypeFor[*big.Int]()
)

// invalidLength 报告 offset + length > sliceLength，或者加法是否会溢出。
func invalidLength(offset, length, sliceLength int) bool {
	return offset+length < offset || offset+length > sliceLength
}

// parseField 是主要的解析函数。给定一个字节切片和数组中的偏移量，
// 它将尝试解析出一个合适的 ASN.1 值并将其存储在给定的 Value 中。
func parseField(v reflect.Value, bytes []byte, initOffset int, params fieldParameters) (offset int, err error) {
	offset = initOffset
	fieldType := v.Type()

	// 如果我们的数据用完了，可能是末尾有可选元素。
	if offset == len(bytes) {
		if !setDefaultValue(v, params) {
			err = SyntaxError{"sequence truncated"}
		}
		return
	}

	// 处理 ANY 类型。
	if ifaceType := fieldType; ifaceType.Kind() == reflect.Interface && ifaceType.NumMethod() == 0 {
		var t tagAndLength
		t, offset, err = parseTagAndLength(bytes, offset)
		if err != nil {
			return
		}
		if invalidLength(offset, t.length, len(bytes)) {
			err = SyntaxError{"data truncated"}
			return
		}
		var result any
		if !t.isCompound && t.class == ClassUniversal {
			innerBytes := bytes[offset : offset+t.length]
			switch t.tag {
			case TagBoolean:
				result, err = parseBool(innerBytes)
			case TagPrintableString:
				result, err = parsePrintableString(innerBytes)
			case TagNumericString:
				result, err = parseNumericString(innerBytes)
			case TagIA5String:
				result, err = parseIA5String(innerBytes)
			case TagT61String:
				result, err = parseT61String(innerBytes)
			case TagUTF8String:
				result, err = parseUTF8String(innerBytes)
			case TagInteger:
				result, err = parseInt64(innerBytes)
			case TagBitString:
				result, err = parseBitString(innerBytes)
			case TagOID:
				result, err = parseObjectIdentifier(innerBytes)
			case TagUTCTime:
				result, err = parseUTCTime(innerBytes)
			case TagGeneralizedTime:
				result, err = parseGeneralizedTime(innerBytes)
			case TagOctetString:
				result = innerBytes
			case TagBMPString:
				result, err = parseBMPString(innerBytes)
			default:
				// 如果我们不知道如何处理该类型，我们就将 Value 保留为 nil。
			}
		}
		offset += t.length
		if err != nil {
			return
		}
		if result != nil {
			v.Set(reflect.ValueOf(result))
		}
		return
	}

	t, offset, err := parseTagAndLength(bytes, offset)
	if err != nil {
		return
	}
	if params.explicit {
		expectedClass := ClassContextSpecific
		if params.application {
			expectedClass = ClassApplication
		}
		if offset == len(bytes) {
			err = StructuralError{"explicit tag has no child"}
			return
		}
		if t.class == expectedClass && t.tag == *params.tag && (t.length == 0 || t.isCompound) {
			if fieldType == rawValueType {
				// RawValues 不应解析内部元素。
			} else if t.length > 0 {
				t, offset, err = parseTagAndLength(bytes, offset)
				if err != nil {
					return
				}
			} else {
				if fieldType != flagType {
					err = StructuralError{"zero length explicit tag was not an asn1.Flag"}
					return
				}
				v.SetBool(true)
				return
			}
		} else {
			// 标签不匹配，它可能是一个可选元素。
			ok := setDefaultValue(v, params)
			if ok {
				offset = initOffset
			} else {
				err = StructuralError{"explicitly tagged member didn't match"}
			}
			return
		}
	}

	matchAny, universalTag, compoundType, ok1 := getUniversalType(fieldType)
	if !ok1 {
		err = StructuralError{fmt.Sprintf("unknown Go type: %v", fieldType)}
		return
	}

	// 字符串的特殊情况：所有 ASN.1 字符串类型都映射到 Go 类型 string。
	// 当 getUniversalType 看到 string 时返回 PrintableString 的标签，
	// 所以如果我们在线上看到不同的字符串类型，我们就更改通用类型以匹配。
	if universalTag == TagPrintableString {
		if t.class == ClassUniversal {
			switch t.tag {
			case TagIA5String, TagGeneralString, TagT61String, TagUTF8String, TagNumericString, TagBMPString:
				universalTag = t.tag
			}
		} else if params.stringType != 0 {
			universalTag = params.stringType
		}
	}

	// 时间的特殊情况：UTCTime 和 GeneralizedTime 都映射到 Go 类型 time.Time。
	// 当 getUniversalType 看到 time.Time 时返回 UTCTime 的标签，
	// 所以如果我们在线上看到不同的时间类型，或者字段被标记为不同的类型，
	// 我们就更改通用类型以匹配。
	if universalTag == TagUTCTime {
		if t.class == ClassUniversal {
			if t.tag == TagGeneralizedTime {
				universalTag = t.tag
			}
		} else if params.timeType != 0 {
			universalTag = params.timeType
		}
	}

	if params.set {
		universalTag = TagSet
	}

	matchAnyClassAndTag := matchAny
	expectedClass := ClassUniversal
	expectedTag := universalTag

	if !params.explicit && params.tag != nil {
		expectedClass = ClassContextSpecific
		expectedTag = *params.tag
		matchAnyClassAndTag = false
	}

	if !params.explicit && params.application && params.tag != nil {
		expectedClass = ClassApplication
		expectedTag = *params.tag
		matchAnyClassAndTag = false
	}

	if !params.explicit && params.private && params.tag != nil {
		expectedClass = ClassPrivate
		expectedTag = *params.tag
		matchAnyClassAndTag = false
	}

	// 此时我们已经解开了任何显式标签。
	if !matchAnyClassAndTag && (t.class != expectedClass || t.tag != expectedTag) ||
		(!matchAny && t.isCompound != compoundType) {
		// 标签不匹配。同样，它可能是一个可选元素。
		ok := setDefaultValue(v, params)
		if ok {
			offset = initOffset
		} else {
			err = StructuralError{fmt.Sprintf("tags don't match (%d vs %+v) %+v %s @%d", expectedTag, t, params, fieldType.Name(), offset)}
		}
		return
	}
	if invalidLength(offset, t.length, len(bytes)) {
		err = SyntaxError{"data truncated"}
		return
	}
	innerBytes := bytes[offset : offset+t.length]
	offset += t.length

	// 我们首先处理本包中定义的结构。
	switch v := v.Addr().Interface().(type) {
	case *RawValue:
		*v = RawValue{t.class, t.tag, t.isCompound, innerBytes, bytes[initOffset:offset]}
		return
	case *ObjectIdentifier:
		*v, err = parseObjectIdentifier(innerBytes)
		return
	case *BitString:
		*v, err = parseBitString(innerBytes)
		return
	case *time.Time:
		if universalTag == TagUTCTime {
			*v, err = parseUTCTime(innerBytes)
			return
		}
		*v, err = parseGeneralizedTime(innerBytes)
		return
	case *Enumerated:
		parsedInt, err1 := parseInt32(innerBytes)
		if err1 == nil {
			*v = Enumerated(parsedInt)
		}
		err = err1
		return
	case *Flag:
		*v = true
		return
	case **big.Int:
		parsedInt, err1 := parseBigInt(innerBytes)
		if err1 == nil {
			*v = parsedInt
		}
		err = err1
		return
	}
	switch val := v; val.Kind() {
	case reflect.Bool:
		parsedBool, err1 := parseBool(innerBytes)
		if err1 == nil {
			val.SetBool(parsedBool)
		}
		err = err1
		return
	case reflect.Int, reflect.Int32, reflect.Int64:
		if val.Type().Size() == 4 {
			parsedInt, err1 := parseInt32(innerBytes)
			if err1 == nil {
				val.SetInt(int64(parsedInt))
			}
			err = err1
		} else {
			parsedInt, err1 := parseInt64(innerBytes)
			if err1 == nil {
				val.SetInt(parsedInt)
			}
			err = err1
		}
		return
	// TODO(dfc) 添加对剩余整数类型的支持
	case reflect.Struct:
		structType := fieldType

		for i := 0; i < structType.NumField(); i++ {
			if !structType.Field(i).IsExported() {
				err = StructuralError{"struct 包含 unexported fields"}
				return
			}
		}

		if structType.NumField() > 0 &&
			structType.Field(0).Type == rawContentsType {
			bytes := bytes[initOffset:offset]
			val.Field(0).Set(reflect.ValueOf(RawContent(bytes)))
		}

		innerOffset := 0
		for i := 0; i < structType.NumField(); i++ {
			field := structType.Field(i)
			if i == 0 && field.Type == rawContentsType {
				continue
			}
			innerOffset, err = parseField(val.Field(i), innerBytes, innerOffset, parseFieldParameters(field.Tag.Get("asn1")))
			if err != nil {
				return
			}
		}
		// 我们允许 SEQUENCE 末尾有额外的字节，因为随着版本号的增加，
		// 在 X.509 中一直使用在末尾添加元素的方式。
		return
	case reflect.Slice:
		sliceType := fieldType
		if sliceType.Elem().Kind() == reflect.Uint8 {
			val.Set(reflect.MakeSlice(sliceType, len(innerBytes), len(innerBytes)))
			reflect.Copy(val, reflect.ValueOf(innerBytes))
			return
		}
		newSlice, err1 := parseSequenceOf(innerBytes, sliceType, sliceType.Elem())
		if err1 == nil {
			val.Set(newSlice)
		}
		err = err1
		return
	case reflect.String:
		var v string
		switch universalTag {
		case TagPrintableString:
			v, err = parsePrintableString(innerBytes)
		case TagNumericString:
			v, err = parseNumericString(innerBytes)
		case TagIA5String:
			v, err = parseIA5String(innerBytes)
		case TagT61String:
			v, err = parseT61String(innerBytes)
		case TagUTF8String:
			v, err = parseUTF8String(innerBytes)
		case TagGeneralString:
			// GeneralString 在 ISO-2022/ECMA-35 中指定，
			// 简要审查表明它包含允许在字符串中间更改编码等的结构。
			// 我们放弃并将其作为 8 位字符串传递。
			v, err = parseT61String(innerBytes)
		case TagBMPString:
			v, err = parseBMPString(innerBytes)

		default:
			err = SyntaxError{fmt.Sprintf("internal error: unknown string type %d", universalTag)}
		}
		if err == nil {
			val.SetString(v)
		}
		return
	}
	err = StructuralError{"unsupported: " + v.Type().String()}
	return
}

// canHaveDefaultValue 报告 k 是否是我们会为其设置默认值的 Kind。
//（本质上是有符号整数。）
func canHaveDefaultValue(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	}

	return false
}

// setDefaultValue 用于从标签字符串将默认值安装到 Value 中。
// 如果字段是可选的，即使没有提供默认值或未能将其安装到 Value 中，它也是成功的。
func setDefaultValue(v reflect.Value, params fieldParameters) (ok bool) {
	if !params.optional {
		return
	}
	ok = true
	if params.defaultValue == nil {
		return
	}
	if canHaveDefaultValue(v.Kind()) {
		v.SetInt(*params.defaultValue)
	}
	return
}

// Unmarshal 解析 DER 编码的 ASN.1 数据结构 b，
// 并使用 reflect 包填充 val 指向的任意值。
// 因为 Unmarshal 使用 reflect 包，所以被写入的结构体必须使用大写字段名。
// 如果 val 为 nil 或不是指针，Unmarshal 返回错误。
//
// 解析 b 后，任何剩余且未用于填充 val 的字节将在 rest 中返回。
// 当将 SEQUENCE 解析为结构体时，SEQUENCE 中没有在 val 中匹配字段的
// 任何尾部元素将不会包含在 rest 中，因为这些被视为 SEQUENCE 的有效元素
// 而不是尾部数据。
//
//   - ASN.1 INTEGER 可以写入 int、int32、int64 或 *[big.Int]。
//     如果编码值不适合 Go 类型，Unmarshal 返回解析错误。
//
//   - ASN.1 BIT STRING 可以写入 [BitString]。
//
//   - ASN.1 OCTET STRING 可以写入 []byte。
//
//   - ASN.1 OBJECT IDENTIFIER 可以写入 [ObjectIdentifier]。
//
//   - ASN.1 ENUMERATED 可以写入 [Enumerated]。
//
//   - ASN.1 UTCTIME 或 GENERALIZEDTIME 可以写入 [time.Time]。
//
//   - ASN.1 PrintableString、IA5String 或 NumericString 可以写入 string。
//
//   - 以上任何 ASN.1 值都可以写入 interface{}。
//     存储在接口中的值具有相应的 Go 类型。对于整数，该类型是 int64。
//
//   - ASN.1 SEQUENCE OF x 或 SET OF x 可以写入切片，
//     如果 x 可以写入切片的元素类型。
//
//   - ASN.1 SEQUENCE 或 SET 可以写入结构体，
//     如果序列中的每个元素都可以写入结构体中的相应元素。
//
// 结构体字段上的以下标签对 Unmarshal 有特殊含义：
//
//	application 指定使用 APPLICATION 标签
//	private     指定使用 PRIVATE 标签
//	default:x   设置可选整数字段的默认值（仅在 optional 也存在时使用）
//	explicit    指定额外的显式标签包装隐式标签
//	optional    将字段标记为 ASN.1 OPTIONAL
//	set         期望 SET 类型而不是 SEQUENCE 类型
//	tag:x       指定 ASN.1 标签号；暗示 ASN.1 CONTEXT SPECIFIC
//
// 当将带有 IMPLICIT 标签的 ASN.1 值解码为字符串字段时，
// Unmarshal 将默认为 PrintableString，它不支持 '@' 和 '&' 等字符。
// 要强制使用其他编码，请使用以下标签：
//
//	ia5     使字符串作为 ASN.1 IA5String 值解组
//	numeric 使字符串作为 ASN.1 NumericString 值解组
//	utf8    使字符串作为 ASN.1 UTF8String 值解组
//
// 当将带有 IMPLICIT 标签的 ASN.1 值解码为 time.Time 字段时，
// Unmarshal 将默认为 UTCTime，它不支持时区或小数秒。
// 要强制使用 GeneralizedTime，请使用以下标签：
//
//	generalized 使 time.Time 作为 ASN.1 GeneralizedTime 值解组
//
// 如果结构体第一个字段的类型是 RawContent，则结构体的原始 ASN1 内容
// 将存储在其中。
//
// 如果切片类型的名称以 "SET" 结尾，则将其视为设置了 "set" 标签。
// 这导致将类型解释为 SET OF x 而不是 SEQUENCE OF x。
// 这可用于无法给出结构体标签的嵌套切片。
//
// 不支持其他 ASN.1 类型；如果遇到它们，Unmarshal 返回解析错误。
func Unmarshal(b []byte, val any) (rest []byte, err error) {
	return UnmarshalWithParams(b, val, "")
}

// invalidUnmarshalError 描述传递给 Unmarshal 的无效参数。
//（Unmarshal 的参数必须是非 nil 指针。）
type invalidUnmarshalError struct {
	Type reflect.Type
}

func (e *invalidUnmarshalError) Error() string {
	if e.Type == nil {
		return "asn1: Unmarshal recipient value is nil"
	}

	if e.Type.Kind() != reflect.Pointer {
		return "asn1: Unmarshal recipient value is non-pointer " + e.Type.String()
	}
	return "asn1: Unmarshal recipient value is nil " + e.Type.String()
}

// UnmarshalWithParams 允许为顶级元素指定字段参数。
// params 的形式与字段标签相同。
func UnmarshalWithParams(b []byte, val any, params string) (rest []byte, err error) {
	v := reflect.ValueOf(val)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return nil, &invalidUnmarshalError{reflect.TypeOf(val)}
	}
	offset, err := parseField(v.Elem(), b, 0, parseFieldParameters(params))
	if err != nil {
		return nil, err
	}
	return b[offset:], nil
}
