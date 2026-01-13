// 版权所有 2020 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build goexperiment.jsonv2

// json 包实现了 semantic processing of JSON as specified in RFC 8259.
// JSON 是一个 simple data interchange format that can represent
// primitive data types such as booleans, strings, and numbers,
// in addition to structured data types such as objects and arrays.
//
// This package (encoding/json/v2) is experimental,
// and not subject to the Go 1 compatibility promise.
// It only exists when building with the GOEXPERIMENT=jsonv2 environment variable set.
// Most users should use [encoding/json].
//
// [Marshal] and [Unmarshal] encode and decode Go values
// to/from JSON text contained within a []byte.
// [MarshalWrite] and [UnmarshalRead] operate on JSON text
// by writing to or reading from an [io.Writer] or [io.Reader].
// [MarshalEncode] and [UnmarshalDecode] operate on JSON text
// by encoding to or decoding from a [jsontext.Encoder] or [jsontext.Decoder].
// [Options] 可能是 passed to each of the marshal or unmarshal functions
// to configure the semantic behavior of marshaling and unmarshaling
// (i.e., alter how JSON data is understood as Go data and vice versa).
// [jsontext.Options] may also be passed to the marshal or unmarshal functions
// to configure the syntactic behavior of encoding or decoding.
//
// The data types of JSON are mapped to/from the data types of Go based on
// the closest logical equivalent between the two type systems. For example,
// a JSON boolean corresponds with a Go bool,
// a JSON string corresponds with a Go string,
// a JSON number corresponds with a Go int, uint or float,
// a JSON array corresponds with a Go slice or array, and
// a JSON object corresponds with a Go struct or map.
// See the documentation on [Marshal] and [Unmarshal] for a comprehensive list
// of how the JSON and Go type systems correspond.
//
// Arbitrary Go types can customize their JSON representation by implementing
// [Marshaler], [MarshalerTo], [Unmarshaler], or [UnmarshalerFrom].
// This 提供authors of Go types with control over how their types are
// serialized as JSON. Alternatively, users can implement functions that match
// [MarshalFunc], [MarshalToFunc], [UnmarshalFunc], or [UnmarshalFromFunc]
// to specify the JSON representation for arbitrary types.
// This 提供callers of JSON functionality with control over
// how any arbitrary type is serialized as JSON.
//
// # JSON Representation of Go structs
//
// 一个Go struct is naturally represented as a JSON object,
// where each Go struct field corresponds with a JSON object member.
// When marshaling, all Go struct fields are recursively encoded in depth-first
// order as JSON object members except those that are ignored or omitted.
// When unmarshaling, JSON object members are recursively decoded
// into the corresponding Go struct fields.
// Object members that do not match any struct fields,
// also known as “unknown members”, are ignored by default or rejected
// if [RejectUnknownMembers] is specified.
//
// The representation of each struct field can be customized in the
// "json" struct field tag, where the tag 是一个 comma separated list of options.
// As a special case, 如果 entire tag is `json:"-"`,
// then the field is ignored with regard to its JSON representation.
// Some options also have equivalent behavior controlled by a caller-specified [Options].
// Field-specified options take precedence over caller-specified options.
//
// The first option 是 JSON object name override for the Go struct field.
// If the name is not specified, then the Go struct field name
// is used as the JSON object name. JSON names containing commas or quotes,
// or names identical to "" or "-", can be specified using
// a single-quoted string literal, where the syntax is identical to
// the Go grammar for a double-quoted string literal,
// but instead uses single quotes as the delimiters.
// By default, unmarshaling uses case-sensitive matching to identify
// the Go struct field associated with a JSON object name.
//
// After the name, the following tag options are supported:
//
//   - omitzero: When marshaling, the "omitzero" option 指定
//     the struct field 应该是 omitted 如果 field value is zero
//     as determined by the "IsZero() bool" method if present,
//     否则 based on whether the field 是 zero Go value.
//     This option has no effect when unmarshaling.
//
//   - omitempty: When marshaling, the "omitempty" option 指定
//     the struct field 应该是 omitted 如果 field value would have been
//     编码为 a JSON null, empty string, empty object, or empty array.
//     This option has no effect when unmarshaling.
//
//   - string: The "string" option 指定 [StringifyNumbers]
//     be set when marshaling or unmarshaling a struct field value.
//     This causes numeric types to be 编码为 a JSON number
//     within a JSON string, and to be 从 解码 a JSON string
//     containing the JSON number without any surrounding whitespace.
//     This extra level of encoding is often necessary since
//     many JSON parsers cannot precisely represent 64-bit integers.
//
//   - case: When unmarshaling, the "case" option 指定 how
//     JSON object names are matched with the JSON name for Go struct fields.
//     The option 是一个 key-value pair specified as "case:value" where
//     the value must either be 'ignore' or 'strict'.
//     The 'ignore' value 指定 matching is case-insensitive
//     where dashes and underscores are also ignored. If multiple fields match,
//     the first declared field in breadth-first order takes precedence.
//     The 'strict' value 指定 matching is case-sensitive.
//     This takes precedence over the [MatchCaseInsensitiveNames] option.
//
//   - inline: The "inline" option 指定
//     the JSON representable content of this field type is to be promoted
//     as if they were specified in the parent struct.
//     It 是 JSON equivalent of Go struct embedding.
//     A Go embedded field is implicitly inlined 除非 an explicit JSON name
//     is specified. The inlined field 必须是 a Go struct
//     (that does not implement any JSON methods), [jsontext.Value],
//     map[~string]T, or an unnamed pointer to such types. When marshaling,
//     inlined fields from a pointer type are omitted if it is nil.
//     Inlined fields 类型为 [jsontext.Value] and map[~string]T are called
//     “inlined fallbacks” as they can represent all possible
//     JSON object members not directly handled by the parent struct.
//     Only one inlined fallback field 可能是 specified in a struct,
//     while many non-fallback fields 可能是 specified. This option
//     must not be specified with any other option (including the JSON name).
//
//   - unknown: The "unknown" option 是一个 specialized variant
//     of the inlined fallback to indicate that this Go struct field
//     包含 any number of unknown JSON object members. The field type must
//     be a [jsontext.Value], map[~string]T, or an unnamed pointer to such types.
//     If [DiscardUnknownMembers] is specified when marshaling,
//     the contents of this field are ignored.
//     If [RejectUnknownMembers] is specified when unmarshaling,
//     any unknown object members are rejected regardless of whether
//     an inlined fallback with the "unknown" option exists. This option
//     must not be specified with any other option (including the JSON name).
//
//   - format: The "format" option 指定 a format flag
//     used to specialize the formatting of the field value.
//     The option 是一个 key-value pair specified as "format:value" where
//     the value 必须是 either a literal consisting of letters and numbers
//     (e.g., "format:RFC3339") or a single-quoted string literal
//     (e.g., "format:'2006-01-02'"). The interpretation of the format flag
//     is determined by the struct field type.
//
// The "omitzero" and "omitempty" options are mostly semantically identical.
// The former is defined in terms of the Go type system,
// while the latter in terms of the JSON type system.
// Consequently they behave differently in some circumstances.
// For example, only a nil slice or map is omitted under "omitzero", while
// an empty slice or map is omitted under "omitempty" regardless of nilness.
// The "omitzero" option is useful for types with a well-defined zero value
// (e.g., [net/netip.Addr]) or have an IsZero method (e.g., [time.Time.IsZero]).
//
// Every Go struct corresponds to a list of JSON representable fields
// which is constructed by performing a breadth-first search over
// all struct fields (excluding unexported or ignored fields),
// where the search recursively descends into inlined structs.
// The set of non-inlined fields in a struct must have unique JSON names.
// If multiple fields all have the same JSON name, then the one
// at shallowest depth takes precedence and the other fields at deeper depths
// are excluded from the list of JSON representable fields.
// If multiple fields at the shallowest depth have the same JSON name,
// but exactly one is explicitly tagged with a JSON name,
// then that field takes precedence and all others are excluded from the list.
// This 是一个nalogous to Go visibility rules for struct field selection
// with embedded struct types.
//
// Marshaling or unmarshaling a non-empty struct
// without any JSON representable fields results in a [SemanticError].
// Unexported fields must not have any `json` tags except for `json:"-"`.
//
// # Security Considerations
//
// JSON is frequently used as a data interchange format to communicate
// between different systems, possibly implemented in different languages.
// For interoperability and security reasons, it is important that
// all implementations agree upon the semantic meaning of the data.
//
// [For example, suppose we have two micro-services.]
// The first service is responsible for authenticating a JSON request,
// while the second service is responsible for executing the request
// (having assumed that the prior service authenticated the request).
// If an attacker were able to maliciously craft a JSON request such that
// both services believe that the same request is from different users,
// it could bypass the authenticator with valid credentials for one user,
// but maliciously perform an action on behalf of a different user.
//
// According to RFC 8259, there unfortunately exist many JSON texts
// that are syntactically valid but semantically ambiguous.
// For example, the standard does not define how to interpret duplicate
// names within an object.
//
// The v1 [encoding/json] and [encoding/json/v2] packages
// interpret some inputs in different ways. In particular:
//
//   - The standard 指定 JSON 必须是 encoded using UTF-8.
//     By default, v1 replaces invalid bytes of UTF-8 in JSON strings
//     with the Unicode replacement character,
//     while v2 rejects inputs with invalid UTF-8.
//     To change the default, specify the [jsontext.AllowInvalidUTF8] option.
//     The replacement of invalid UTF-8 是一个 form of data corruption
//     that alters the precise meaning of strings.
//
//   - The standard does not specify a particular behavior when
//     duplicate names are encountered within a JSON object,
//     which 意味着 different implementations may behave differently.
//     By default, v1 允许 for the presence of duplicate names,
//     while v2 rejects duplicate names.
//     To change the default, specify the [jsontext.AllowDuplicateNames] option.
//     If allowed, object members are processed in the order they are observed,
//     meaning that later values will replace or be merged into prior values,
//     depending on the Go value type.
//
//   - The standard 定义 a JSON object as an unordered collection of name/value pairs.
//     While ordering can be observed through the underlying [jsontext] API,
//     both v1 and v2 generally avoid exposing the ordering.
//     No application should semantically depend on the order of object members.
//     Allowing duplicate names 是一个 vector through which ordering of members
//     can accidentally be observed and depended upon.
//
//   - The standard suggests that JSON object names are typically compared
//     based on equality of the sequence of Unicode code points,
//     which implies that comparing names is often case-sensitive.
//     When unmarshaling a JSON object into a Go struct,
//     by default, v1 uses a (loose) case-insensitive match on the name,
//     while v2 uses a (strict) case-sensitive match on the name.
//     To change the default, specify the [MatchCaseInsensitiveNames] option.
//     The use of case-insensitive matching provides another vector through
//     which duplicate names can occur. Allowing case-insensitive matching
//     意味着 v1 or v2 might interpret JSON objects differently from most
//     other JSON implementations (which typically use a case-sensitive match).
//
//   - The standard does not specify a particular behavior when
//     an unknown name in a JSON object is encountered.
//     When unmarshaling a JSON object into a Go struct, by default
//     both v1 and v2 ignore unknown names and their corresponding values.
//     To change the default, specify the [RejectUnknownMembers] option.
//
//   - The standard suggests that implementations may use a float64
//     to represent a JSON number. Consequently, large JSON integers
//     may lose precision when stored as a floating-point type.
//     Both v1 and v2 correctly preserve precision when marshaling and
//     unmarshaling a concrete integer type. However, even if v1 and v2
//     preserve precision for concrete types, other JSON implementations
//     may not be able to preserve precision for outputs produced by v1 or v2.
//     The `string` tag option can be used to specify that an integer type
//     is to be quoted within a JSON string to avoid loss of precision.
//     Furthermore, v1 and v2 may still lose precision when unmarshaling
//     into an any interface value, where unmarshal uses a float64
//     by default to represent a JSON number.
//     To change the default, specify the [WithUnmarshalers] option
//     with a custom unmarshaler that pre-populates the interface value
//     with a concrete Go type that can preserve precision.
//
// RFC 8785 指定 a canonical form for any JSON text,
// which explicitly 定义 specific behaviors that RFC 8259 leaves undefined.
// In theory, if a text can successfully [jsontext.Value.Canonicalize]
// without changing the semantic meaning of the data, then it provides a
// greater degree of confidence that the data is more secure and interoperable.
//
// The v2 API generally chooses more secure defaults than v1,
// but care should still be taken with large integers or unknown members.
//
// [For example, suppose we have two micro-services.]: https://www.youtube.com/watch?v=avilmOcHKHE&t=1057s
package json

// requireKeyedLiterals can be embedded in a struct to require keyed literals.
type requireKeyedLiterals struct{}

// nonComparable can be embedded in a struct to prevent comparability.
type nonComparable [0]func()
