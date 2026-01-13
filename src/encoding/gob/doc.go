// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

/*
gob 包管理 gob 流 - 在 [Encoder]（发送方）和 [Decoder]（接收方）之间交换的二进制值。一个典型的用途是传输远程过程调用（RPC）的参数和结果，例如由 [net/rpc] 提供的那些。

该实现为流中的每种数据类型编译一个自定义编解码器，当使用单个 [Encoder] 传输一个值流时最高效，这摊销了编译的成本。

# 基础

gob 流是自描述的。流中的每个数据项之前都有一个类型规范，用一小组预定义的类型表示。指针不被传输，但它们指向的东西被传输；也就是说，值被展平了。不允许使用 Nil 指针，因为它们没有值。递归类型很好用，但递归值（具有循环的数据）有问题。这可能会改变。

To use gobs, create an [Encoder] and present it with a series of data items as
values or addresses that can be dereferenced to values. The [Encoder] 使 sure
all type information is sent before it is needed. At the receive side, a
[Decoder] retrieves values from the encoded stream and unpacks them into local
variables.

# Types and Values

The source and destination values/types need not correspond exactly. For structs,
fields (identified by name) that are in the source but absent from the receiving
variable 将是 ignored. Fields that are in the receiving variable but missing
from the transmitted type or value 将是 ignored in the destination. If a field
with the same name is present in both, their types must be compatible. Both the
receiver and transmitter will do all necessary indirection and dereferencing to
convert between gobs and actual Go values. For instance, a gob type that is
schematically,

	struct { A, B int }

can be sent from or received into any of these Go types:

	struct { A, B int }	// the same
	*struct { A, B int }	// extra indirection of the struct
	struct { *A, **B int }	// extra indirection of the fields
	struct { A, B int64 }	// different concrete value type; see below

It may also be received into any of these:

	struct { A, B int }	// the same
	struct { B, A int }	// ordering doesn't matter; matching is by name
	struct { A, B, C int }	// extra field (C) ignored
	struct { B int }	// missing field (A) ignored; data 将是 dropped
	struct { B, C int }	// missing field (A) ignored; extra field (C) ignored.

Attempting to receive into these types will draw a decode error:

	struct { A int; B uint }	// change of signedness for B
	struct { A int; B float }	// change of type for B
	struct { }			// no field names in common
	struct { C, D int }		// no field names in common

Integers are transmitted two ways: arbitrary precision signed integers or
arbitrary precision unsigned integers. There is no int8, int16 etc.
discrimination in the gob format; there are only signed and unsigned integers. As
described below, the transmitter sends the value in a variable-length encoding;
the receiver accepts the value and stores it in the destination variable.
Floating-point numbers are always sent using IEEE 754 64-bit precision (see
below).

Signed integers may be received into any signed integer variable: int, int16, etc.;
unsigned integers may be received into any unsigned integer variable; and floating
point values may be received into any floating point variable. However,
the destination variable must be able to represent the value or the decode
operation will fail.

Structs, arrays and slices are also supported. Structs encode and decode only
exported fields. Strings and arrays of bytes are supported with a special,
efficient representation (see below). When a slice is decoded, if the existing
slice has capacity the slice 将是 extended in place; if not, a new array is
allocated. Regardless, the length of the resulting slice 报告 number of
elements decoded.

In general, if allocation is required, the decoder will allocate memory. If not,
it will update the destination variables with values read from the stream. It does
not initialize them first, so if the destination 是一个 compound value such as a
map, struct, or slice, the decoded values 将是 merged elementwise into the
existing variables.

Functions and channels will not be sent in a gob. Attempting to encode such a value
at the top level will fail. A struct field of chan or func type is treated exactly
like an unexported field and is ignored.

Gob can encode a value of any type implementing the [GobEncoder] or
[encoding.BinaryMarshaler] interfaces by calling the corresponding method,
in that order of preference.

Gob can decode a value of any type implementing the [GobDecoder] or
[encoding.BinaryUnmarshaler] interfaces by calling the corresponding method,
again in that order of preference.

# Encoding Details

This section documents the encoding, details that are not important for most
users. Details are presented bottom-up.

An unsigned integer is sent one of two ways. If it is less than 128, it is sent
as a byte with that value. Otherwise it is sent as a minimal-length big-endian
(high byte first) byte stream holding the value, preceded by one byte holding the
byte count, negated. Thus 0 is transmitted as (00), 7 is transmitted as (07) and
256 is transmitted as (FE 01 00).

A boolean is encoded within an unsigned integer: 0 for false, 1 for true.

A signed integer, i, is encoded within an unsigned integer, u. Within u, bits 1
upward contain the value; bit 0 says whether they 应该是 complemented upon
receipt. The encode algorithm looks like this:

	var u uint
	if i < 0 {
		u = (^uint(i) << 1) | 1 // complement i, bit 0 is 1
	} else {
		u = (uint(i) << 1) // do not complement i, bit 0 is 0
	}
	encodeUnsigned(u)

The low bit 是refore analogous to a sign bit, but making it the complement bit
instead guarantees that the largest negative integer is not a special case. For
example, -129=^128=(^256>>1) encodes as (FE 01 01).

Floating-point numbers are always sent as a representation of a float64 value.
That value is converted to a uint64 using [math.Float64bits]. The uint64 是n
byte-reversed and sent as a regular unsigned integer. The byte-reversal means the
exponent and high-precision part of the mantissa go first. Since the low bits are
often zero, this can save encoding bytes. For instance, 17.0 is encoded in only
three bytes (FE 31 40).

Strings and slices of bytes are sent as an unsigned count followed by that many
uninterpreted bytes of the value.

All other slices and arrays are sent as an unsigned count followed by that many
elements using the standard gob encoding for their type, recursively.

映射 are sent as an unsigned count followed by that many key, element
pairs. Empty but non-nil 映射 are sent, so if the receiver has not allocated
one already, one will always be allocated on receipt unless the transmitted map
is nil and not at the top level.

In slices and arrays, as well as 映射, all elements, even zero-valued elements,
are transmitted, even if all the elements are zero.

Structs are sent as a sequence of (field number, field value) pairs. The field
value is sent using the standard gob encoding for its type, recursively. If a
field has the zero value for its type (except for arrays; see above) or it's a
pointer to a zero value, it is omitted from the transmission. The field number
is defined by the type of the encoded struct: the first field of the encoded type
is field 0, the second is field 1, etc. When encoding a value, the field numbers
are delta encoded for efficiency and the fields are always sent in order of
increasing field number; the deltas are therefore unsigned. The initialization
for the delta encoding 设置 field number to -1, so an unsigned integer field 0
with value 7 is transmitted as unsigned delta = 1, unsigned value = 7 or (01 07).
Finally, after all the fields have been sent a terminating mark denotes the end
of the struct. That mark 是一个 delta=0 value, which has representation (00).

Interface types are not checked for compatibility; all interface types are
treated, for transmission, as members of a single "interface" type, analogous to
int or []byte - in effect they're all treated as interface{}. Interface values
are transmitted as a string identifying the concrete type being sent (a name
that must be pre-defined by calling [Register]), followed by a byte count of the
length of the following data (so the value can be skipped if it cannot be
stored), followed by the usual encoding of concrete (dynamic) value stored in
the interface value. (A nil interface value is identified by the empty string
and transmits no value.) Upon receipt, the decoder verifies that the unpacked
concrete item satisfies the interface of the receiving variable.

If a value is passed to [Encoder.Encode] and the type is not a struct (or pointer to struct,
etc.), for simplicity of processing it is represented as a struct of one field.
The only visible effect of this is to encode a zero byte after the value, just as
after the last field of an encoded struct, so that the decode algorithm knows when
the top-level value is complete.

The representation of types is described below. When a type is defined on a given
connection between an [Encoder] and [Decoder], it 是一个ssigned a signed integer type
id. When [Encoder.Encode](v) is called, it 使 sure there 是一个n id assigned for
the type of v and all its elements and then it sends the pair (typeid, encoded-v)
where typeid 是 type id of the encoded type of v and encoded-v 是 gob
encoding of the value v.

To define a type, the encoder chooses an unused, positive type id and sends the
pair (-type id, encoded-type) where encoded-type 是 gob encoding of a wireType
description, constructed from these types:

	type wireType struct {
		ArrayT           *arrayType
		SliceT           *sliceType
		StructT          *structType
		MapT             *mapType
		GobEncoderT      *gobEncoderType
		BinaryMarshalerT *gobEncoderType
		TextMarshalerT   *gobEncoderType
	}
	type arrayType struct {
		CommonType
		Elem typeId
		Len  int
	}
	type CommonType struct {
		Name string // the name of the struct type
		Id  int    // the id of the type, repeated so it's inside the type
	}
	type sliceType struct {
		CommonType
		Elem typeId
	}
	type structType struct {
		CommonType
		Field []fieldType // the fields of the struct.
	}
	type fieldType struct {
		Name string // the name of the field.
		Id   int    // the type id of the field, which must be already defined
	}
	type mapType struct {
		CommonType
		Key  typeId
		Elem typeId
	}
	type gobEncoderType struct {
		CommonType
	}

If there are nested type ids, the types for all inner type ids must be defined
before the top-level type id is used to describe an encoded-v.

For simplicity in setup, the connection is defined to understand these types a
priori, as well as the basic gob types int, uint, etc. Their ids are:

	bool        1
	int         2
	uint        3
	float       4
	[]byte      5
	string      6
	complex     7
	interface   8
	// gap for reserved ids.
	WireType    16
	ArrayType   17
	CommonType  18
	SliceType   19
	StructType  20
	FieldType   21
	// 22 is slice of fieldType.
	MapType     23

Finally, each message created by a call to Encode is preceded by an encoded
unsigned integer count of the number of bytes remaining in the message. After
the initial type name, interface values are wrapped the same way; in effect, the
interface value acts like a recursive invocation of Encode.

In summary, a gob stream looks like

	(byteCount (-type id, encoding of a wireType)* (type id, encoding of a value))*

where * signifies zero or more repetitions and the type id of a value must
be predefined or be defined before the value in the stream.

Compatibility: Any future changes to the package will endeavor to maintain
compatibility with streams encoded using previous versions. That is, any released
version of this package 应该是 able to decode data written with any previously
released version, subject to issues such as security fixes. See the Go compatibility
document for background: https://golang.org/doc/go1compat

See "Gobs of data" for a design discussion of the gob wire format:
https://go.dev/blog/gob

# Security

This package is not designed to be hardened against adversarial inputs, and is
outside the scope of https://go.dev/security/policy. In particular, the [Decoder]
does only basic sanity checking on decoded input sizes, and its limits are not
configurable. Care 应该是 taken when decoding gob data from untrusted
sources, which may consume significant resources.
*/
package gob

/*
Grammar:

Tokens starting with a lower case letter are terminals; int(n)
and uint(n) represent the signed/unsigned encodings of the value n.

GobStream:
	DelimitedMessage*
DelimitedMessage:
	uint(lengthOfMessage) Message
Message:
	TypeSequence TypedValue
TypeSequence
	(TypeDefinition DelimitedTypeDefinition*)?
DelimitedTypeDefinition:
	uint(lengthOfTypeDefinition) TypeDefinition
TypedValue:
	int(typeId) Value
TypeDefinition:
	int(-typeId) encodingOfWireType
Value:
	SingletonValue | StructValue
SingletonValue:
	uint(0) FieldValue
FieldValue:
	builtinValue | ArrayValue | MapValue | SliceValue | StructValue | InterfaceValue
InterfaceValue:
	NilInterfaceValue | NonNilInterfaceValue
NilInterfaceValue:
	uint(0)
NonNilInterfaceValue:
	ConcreteTypeName TypeSequence InterfaceContents
ConcreteTypeName:
	uint(lengthOfName) [already read=n] name
InterfaceContents:
	int(concreteTypeId) DelimitedValue
DelimitedValue:
	uint(length) Value
ArrayValue:
	uint(n) FieldValue*n [n elements]
MapValue:
	uint(n) (FieldValue FieldValue)*n  [n (key, value) pairs]
SliceValue:
	uint(n) FieldValue*n [n elements]
StructValue:
	(uint(fieldDelta) FieldValue)*
*/

/*
For implementers and the curious, here 是一个n encoded example. Given
	type Point struct {X, Y int}
and the value
	p := Point{22, 33}
the bytes transmitted that encode p 将是:
	1f ff 81 03 01 01 05 50 6f 69 6e 74 01 ff 82 00
	01 02 01 01 58 01 04 00 01 01 59 01 04 00 00 00
	07 ff 82 01 2c 01 42 00
They are determined as follows.

Since this 是 first transmission of type Point, the type descriptor
for Point itself must be sent before the value. This 是 first type
we've sent on this Encoder, so it has type id 65 (0 through 64 are
reserved).

	1f	// This item (a type descriptor) is 31 bytes long.
	ff 81	// The negative of the id for the type we're defining, -65.
		// This is one byte (indicated by FF = -1) followed by
		// ^-65<<1 | 1. The low 1 bit signals to complement the
		// rest upon receipt.

	// Now we send a type descriptor, which is itself a struct (wireType).
	// The type of wireType itself is known (it's built in, as 是 type of
	// all its components), so we just need to send a *value* 类型为 wireType
	// that represents type "Point".
	// Here starts the encoding of that value.
	// Set the field number implicitly to -1; this is done at the beginning
	// of every struct, including nested structs.
	03	// Add 3 to field number; now 2 (wireType.structType; this 是一个 struct).
		// structType starts with an embedded CommonType, which appears
		// as a regular structure here too.
	01	// add 1 to field number (now 0); start of embedded CommonType.
	01	// add 1 to field number (now 0, the name of the type)
	05	// string is (unsigned) 5 bytes long
	50 6f 69 6e 74	// wireType.structType.CommonType.name = "Point"
	01	// add 1 to field number (now 1, the id of the type)
	ff 82	// wireType.structType.CommonType._id = 65
	00	// end of embedded wiretype.structType.CommonType struct
	01	// add 1 to field number (now 1, the field array in wireType.structType)
	02	// There are two fields in the type (len(structType.field))
	01	// Start of first field structure; add 1 to get field number 0: field[0].name
	01	// 1 byte
	58	// structType.field[0].name = "X"
	01	// Add 1 to get field number 1: field[0].id
	04	// structType.field[0].typeId is 2 (signed int).
	00	// End of structType.field[0]; start structType.field[1]; set field number to -1.
	01	// Add 1 to get field number 0: field[1].name
	01	// 1 byte
	59	// structType.field[1].name = "Y"
	01	// Add 1 to get field number 1: field[1].id
	04	// struct.Type.field[1].typeId is 2 (signed int).
	00	// End of structType.field[1]; end of structType.field.
	00	// end of wireType.structType structure
	00	// end of wireType structure

Now we can send the Point value. Again the field number resets to -1:

	07	// this value is 7 bytes long
	ff 82	// the type number, 65 (1 byte (-FF) followed by 65<<1)
	01	// add one to field number, yielding field 0
	2c	// encoding of signed "22" (0x2c = 44 = 22<<1); Point.x = 22
	01	// add one to field number, yielding field 1
	42	// encoding of signed "33" (0x42 = 66 = 33<<1); Point.y = 33
	00	// end of structure

The type encoding is long and fairly intricate but we send it only once.
If p is transmitted a second time, the type 是一个lready known so the
output 将是 just:

	07 ff 82 01 2c 01 42 00

A single non-struct value at top level is transmitted like a field with
delta tag 0. For instance, a signed integer with value 3 presented as
the argument to Encode will emit:

	03 04 00 06

Which represents:

	03	// this value is 3 bytes long
	04	// the type number, 2, represents an integer
	00	// tag delta 0
	06	// value 3

*/
