// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// encoding 包定义了供其他包共享的接口，
// 这些包用于在字节级和文本表示之间转换数据。
// 检查这些接口的包包括 encoding/gob、encoding/json 和 encoding/xml。
// 因此，只需实现一次接口，就可以使一个类型在多种编码中可用。
// 实现这些接口的标准类型包括 time.Time 和 net.IP。
// 这些接口成对出现，分别用于生成和消费编码数据。
//
// 向现有类型添加编码/解码方法可能构成破坏性变更，
// 因为它们可用于与使用不同库版本编写的程序进行通信时的序列化。
// Go 项目维护的包的策略是：仅当不存在现有的、合理的序列化方法时，
// 才允许添加序列化函数。
package encoding

// BinaryMarshaler 是由能够将自身序列化为二进制形式的对象实现的接口。
//
// MarshalBinary 将接收者编码为二进制形式并返回结果。
type BinaryMarshaler interface {
	MarshalBinary() (data []byte, err error)
}

// BinaryUnmarshaler 是由能够反序列化其二进制表示的对象实现的接口。
//
// UnmarshalBinary 必须能够解码由 MarshalBinary 生成的形式。
// 如果 UnmarshalBinary 希望在返回后保留数据，则必须复制该数据。
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) error
}

// BinaryAppender 是由能够追加其二进制表示的对象实现的接口。
// 如果一个类型同时实现了 [BinaryAppender] 和 [BinaryMarshaler]，
// 那么 v.MarshalBinary() 必须在语义上等同于 v.AppendBinary(nil)。
type BinaryAppender interface {
	// AppendBinary 将自身的二进制表示追加到 b 的末尾
	// （如有必要会分配更大的切片）并返回更新后的切片。
	//
	// 实现不得保留 b，也不得修改 b[:len(b)] 范围内的任何字节。
	AppendBinary(b []byte) ([]byte, error)
}

// TextMarshaler 是由能够将自身序列化为文本形式的对象实现的接口。
//
// MarshalText 将接收者编码为 UTF-8 编码的文本并返回结果。
type TextMarshaler interface {
	MarshalText() (text []byte, err error)
}

// TextUnmarshaler 是由能够反序列化其文本表示的对象实现的接口。
//
// UnmarshalText 必须能够解码由 MarshalText 生成的形式。
// 如果 UnmarshalText 希望在返回后保留文本，则必须复制该文本。
type TextUnmarshaler interface {
	UnmarshalText(text []byte) error
}

// TextAppender 是由能够追加其文本表示的对象实现的接口。
// 如果一个类型同时实现了 [TextAppender] 和 [TextMarshaler]，
// 那么 v.MarshalText() 必须在语义上等同于 v.AppendText(nil)。
type TextAppender interface {
	// AppendText 将自身的文本表示追加到 b 的末尾
	// （如有必要会分配更大的切片）并返回更新后的切片。
	//
	// 实现不得保留 b，也不得修改 b[:len(b)] 范围内的任何字节。
	AppendText(b []byte) ([]byte, error)
}
