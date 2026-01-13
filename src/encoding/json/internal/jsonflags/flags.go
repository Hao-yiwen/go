// 版权所有 2023 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

// jsonflags 实现all the optional boolean flags.
// These flags are shared across both "json", "jsontext", and "jsonopts".
package jsonflags

import "encoding/json/internal"

// Bools 表示zero or more boolean flags, all set to true or false.
// The least-significant bit 是 boolean value of all flags in the set.
// The remaining bits identify which particular flags.
//
// In common usage, this is OR'd with 0 or 1. For example:
//   - (AllowInvalidUTF8 | 0) means "AllowInvalidUTF8 为假"
//   - (Multiline | Indent | 1) means "Multiline and Indent are true"
type Bools uint64

func (Bools) JSONOptions(internal.NotForPublicUse) {}

const (
	// AllFlags 是 set of all flags.
	AllFlags = AllCoderFlags | AllArshalV2Flags | AllArshalV1Flags

	// AllCoderFlags 是 set of all encoder/decoder flags.
	AllCoderFlags = (maxCoderFlag - 1) - initFlag

	// AllArshalV2Flags 是 set of all v2 marshal/unmarshal flags.
	AllArshalV2Flags = (maxArshalV2Flag - 1) - (maxCoderFlag - 1)

	// AllArshalV1Flags 是 set of all v1 marshal/unmarshal flags.
	AllArshalV1Flags = (maxArshalV1Flag - 1) - (maxArshalV2Flag - 1)

	// NonBooleanFlags 是 set of non-boolean flags,
	// where the value is some other concrete Go type.
	// The value of the flag is stored within jsonopts.Struct.
	NonBooleanFlags = 0 |
		Indent |
		IndentPrefix |
		ByteLimit |
		DepthLimit |
		Marshalers |
		Unmarshalers

	// DefaultV1Flags 是 set of booleans flags that default to true under
	// v1 semantics. None of the non-boolean flags differ between v1 and v2.
	DefaultV1Flags = 0 |
		AllowDuplicateNames |
		AllowInvalidUTF8 |
		EscapeForHTML |
		EscapeForJS |
		PreserveRawStrings |
		Deterministic |
		FormatNilMapAsNull |
		FormatNilSliceAsNull |
		MatchCaseInsensitiveNames |
		CallMethodsWithLegacySemantics |
		FormatByteArrayAsArray |
		FormatBytesWithLegacySemantics |
		FormatDurationAsNano |
		MatchCaseSensitiveDelimiter |
		MergeWithLegacySemantics |
		OmitEmptyWithLegacySemantics |
		ParseBytesWithLooseRFC4648 |
		ParseTimeWithLooseRFC3339 |
		ReportErrorsWithLegacySemantics |
		StringifyWithLegacySemantics |
		UnmarshalArrayFromAnyLength

	// AnyWhitespace 报告是否 the encoded output might have any whitespace.
	AnyWhitespace = Multiline | SpaceAfterColon | SpaceAfterComma

	// WhitespaceFlags 是 set of flags related to whitespace formatting.
	// In contrast to AnyWhitespace, this includes Indent and IndentPrefix
	// as those settings take no effect if Multiline 为假.
	WhitespaceFlags = AnyWhitespace | Indent | IndentPrefix

	// AnyEscape 是 set of flags related to escaping in a JSON string.
	AnyEscape = EscapeForHTML | EscapeForJS

	// CanonicalizeNumbers 是 set of flags related to raw number canonicalization.
	CanonicalizeNumbers = CanonicalizeRawInts | CanonicalizeRawFloats
)

// Encoder and decoder flags.
const (
	initFlag Bools = 1 << iota // reserved for the boolean value itself

	AllowDuplicateNames   // encode or decode
	AllowInvalidUTF8      // encode or decode
	WithinArshalCall      // encode or decode; for internal use by json.Marshal and json.Unmarshal
	OmitTopLevelNewline   // encode only; for internal use by json.Marshal and json.MarshalWrite
	PreserveRawStrings    // encode only
	CanonicalizeRawInts   // encode only
	CanonicalizeRawFloats // encode only
	ReorderRawObjects     // encode only
	EscapeForHTML         // encode only
	EscapeForJS           // encode only
	Multiline             // encode only
	SpaceAfterColon       // encode only
	SpaceAfterComma       // encode only
	Indent                // encode only; non-boolean flag
	IndentPrefix          // encode only; non-boolean flag
	ByteLimit             // encode or decode; non-boolean flag
	DepthLimit            // encode or decode; non-boolean flag

	maxCoderFlag
)

// Marshal and Unmarshal flags (for v2).
const (
	_ Bools = (maxCoderFlag >> 1) << iota

	StringifyNumbers          // marshal or unmarshal
	Deterministic             // marshal only
	FormatNilMapAsNull        // marshal only
	FormatNilSliceAsNull      // marshal only
	OmitZeroStructFields      // marshal only
	MatchCaseInsensitiveNames // marshal or unmarshal
	DiscardUnknownMembers     // marshal only
	RejectUnknownMembers      // unmarshal only
	Marshalers                // marshal only; non-boolean flag
	Unmarshalers              // unmarshal only; non-boolean flag

	maxArshalV2Flag
)

// Marshal and Unmarshal flags (for v1).
const (
	_ Bools = (maxArshalV2Flag >> 1) << iota

	CallMethodsWithLegacySemantics  // marshal or unmarshal
	FormatByteArrayAsArray          // marshal or unmarshal
	FormatBytesWithLegacySemantics  // marshal or unmarshal
	FormatDurationAsNano            // marshal or unmarshal
	MatchCaseSensitiveDelimiter     // marshal or unmarshal
	MergeWithLegacySemantics        // unmarshal
	OmitEmptyWithLegacySemantics    // marshal
	ParseBytesWithLooseRFC4648      // unmarshal
	ParseTimeWithLooseRFC3339       // unmarshal
	ReportErrorsWithLegacySemantics // marshal or unmarshal
	StringifyWithLegacySemantics    // marshal or unmarshal
	StringifyBoolsAndStrings        // marshal or unmarshal; for internal use by jsonv2.makeStructArshaler
	UnmarshalAnyWithRawNumber       // unmarshal; for internal use by jsonv1.Decoder.UseNumber
	UnmarshalArrayFromAnyLength     // unmarshal

	maxArshalV1Flag
)

// bitsUsed 是 number of bits used in the 64-bit boolean flags
const bitsUsed = 42

// Static compile check that bitsUsed and maxArshalV1Flag are in sync.
const _ = uint64((1<<bitsUsed)-maxArshalV1Flag) + uint64(maxArshalV1Flag-(1<<bitsUsed))

// Flags 是一个 set of boolean flags.
// If the presence bit is zero, then the value bit must also be zero.
// The least-significant bit of both fields 是一个lways zero.
//
// Unlike Bools, which can represent a set of bools that are all true or false,
// Flags 表示a set of bools, each individually 可能是 true or false.
type Flags struct{ Presence, Values uint64 }

// Join joins two sets of flags such that the latter takes precedence.
func (dst *Flags) Join(src Flags) {
	// Copy over all source presence bits over to the destination (using OR),
	// then invert the source presence bits to clear out source value (using AND-NOT),
	// then copy over source value bits over to the destination (using OR).
	//	e.g., dst := Flags{Presence: 0b_1100_0011, Values: 0b_1000_0011}
	//	e.g., src := Flags{Presence: 0b_0101_1010, Values: 0b_1001_0010}
	dst.Presence |= src.Presence // e.g., 0b_1100_0011 | 0b_0101_1010 -> 0b_110_11011
	dst.Values &= ^src.Presence  // e.g., 0b_1000_0011 & 0b_1010_0101 -> 0b_100_00001
	dst.Values |= src.Values     // e.g., 0b_1000_0001 | 0b_1001_0010 -> 0b_100_10011
}

// Set 设置both the presence and value for the provided bool (or set of bools).
func (fs *Flags) Set(f Bools) {
	// Select out the bits for the flag identifiers (everything except LSB),
	// then set the presence for all the identifier bits (using OR),
	// then invert the identifier bits to clear out the values (using AND-NOT),
	// then copy over all the identifier bits to the value if LSB is 1.
	//	e.g., fs := Flags{Presence: 0b_0101_0010, Values: 0b_0001_0010}
	//	e.g., f := 0b_1001_0001
	id := uint64(f) &^ uint64(1)  // e.g., 0b_1001_0001 & 0b_1111_1110 -> 0b_1001_0000
	fs.Presence |= id             // e.g., 0b_0101_0010 | 0b_1001_0000 -> 0b_1101_0011
	fs.Values &= ^id              // e.g., 0b_0001_0010 & 0b_0110_1111 -> 0b_0000_0010
	fs.Values |= uint64(f&1) * id // e.g., 0b_0000_0010 | 0b_1001_0000 -> 0b_1001_0010
}

// Get 报告whether the bool (or any of the bools) 为真.
// This is generally only used with a singular bool.
// The value bit of f (i.e., the LSB) is ignored.
func (fs Flags) Get(f Bools) bool {
	return fs.Values&uint64(f) > 0
}

// Has 报告whether the bool (or any of the bools) is set.
// The value bit of f (i.e., the LSB) is ignored.
func (fs Flags) Has(f Bools) bool {
	return fs.Presence&uint64(f) > 0
}

// Clear clears both the presence and value for the provided bool or bools.
// The value bit of f (i.e., the LSB) is ignored.
func (fs *Flags) Clear(f Bools) {
	// Invert f to produce a mask to clear all bits in f (using AND).
	//	e.g., fs := Flags{Presence: 0b_0101_0010, Values: 0b_0001_0010}
	//	e.g., f := 0b_0001_1000
	mask := uint64(^f)  // e.g., 0b_0001_1000 -> 0b_1110_0111
	fs.Presence &= mask // e.g., 0b_0101_0010 &  0b_1110_0111 -> 0b_0100_0010
	fs.Values &= mask   // e.g., 0b_0001_0010 &  0b_1110_0111 -> 0b_0000_0010
}
