// 版权所有 2017 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// asn1 包包含用于使用 cryptobyte 包解析和构建 ASN.1 消息的支持类型。
package asn1

// Tag 表示一个 ASN.1 标识符八位字节，由标签号（指示类型）和类
//（如上下文特定或构造）组成。
//
// cryptobyte 包中的方法仅支持低标签号形式，即单个标识符八位字节，
// 其中第 7-8 位编码类，第 1-6 位编码标签号。
type Tag uint8

const (
	classConstructed     = 0x20
	classContextSpecific = 0x80
)

// Constructed 返回设置了构造类位的 t。
func (t Tag) Constructed() Tag { return t | classConstructed }

// ContextSpecific 返回设置了上下文特定类位的 t。
func (t Tag) ContextSpecific() Tag { return t | classContextSpecific }

// 以下是标准标签和类组合的列表。
const (
	BOOLEAN           = Tag(1)
	INTEGER           = Tag(2)
	BIT_STRING        = Tag(3)
	OCTET_STRING      = Tag(4)
	NULL              = Tag(5)
	OBJECT_IDENTIFIER = Tag(6)
	ENUM              = Tag(10)
	UTF8String        = Tag(12)
	SEQUENCE          = Tag(16 | classConstructed)
	SET               = Tag(17 | classConstructed)
	PrintableString   = Tag(19)
	T61String         = Tag(20)
	IA5String         = Tag(22)
	UTCTime           = Tag(23)
	GeneralizedTime   = Tag(24)
	GeneralString     = Tag(27)
)
