// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// errors 包实现操作错误的函数。
//
// [New] 函数创建仅包含文本消息的错误。
//
// 如果错误 e 的类型具有以下方法之一，则 e 包装另一个错误：
//
//	Unwrap() error
//	Unwrap() []error
//
// 如果 e.Unwrap() 返回非 nil 错误 w 或包含 w 的切片，
// 则我们说 e 包装了 w。从 e.Unwrap() 返回的 nil 错误
// 表示 e 没有包装任何错误。Unwrap 方法返回包含 nil 错误值的
// []error 是无效的。
//
// 创建包装错误的简单方法是调用 [fmt.Errorf] 并将 %w 动词应用于错误参数：
//
//	wrapsErr := fmt.Errorf("... %w ...", ..., err, ...)
//
// 连续解包错误会创建一棵树。[Is] 和 [As] 函数通过首先检查错误本身，
// 然后依次检查其每个子项的树来检查错误树
// （前序、深度优先遍历）。
//
// 有关包装的理念以及何时包装的更深入讨论，
// 请参阅 https://go.dev/blog/go1.13-errors。
//
// [Is] 检查其第一个参数的树，寻找与第二个参数匹配的错误。
// 它报告是否找到匹配项。应优先使用它而不是简单的相等检查：
//
//	if errors.Is(err, fs.ErrExist)
//
// 优于
//
//	if err == fs.ErrExist
//
// 因为如果 err 包装了 [io/fs.ErrExist]，前者将成功。
//
// [AsType] 检查其参数的树，寻找类型与其类型参数匹配的错误。
// 如果成功，它返回该类型的相应值和 true。
// 否则，它返回该类型的零值和 false。形式
//
//	if perr, ok := errors.AsType[*fs.PathError](err); ok {
//		fmt.Println(perr.Path)
//	}
//
// 优于
//
//	if perr, ok := err.(*fs.PathError); ok {
//		fmt.Println(perr.Path)
//	}
//
// 因为如果 err 包装了 [*io/fs.PathError]，前者将成功。
package errors

// New 返回一个格式化为给定文本的错误。
// 即使文本相同，每次调用 New 也会返回一个不同的错误值。
func New(text string) error {
	return &errorString{text}
}

// errorString 是 error 的简单实现。
type errorString struct {
	s string
}

func (e *errorString) Error() string {
	return e.s
}

// ErrUnsupported 表示请求的操作无法执行，因为它不受支持。
// 例如，在使用不支持硬链接的文件系统时调用 [os.Link]。
//
// 函数和方法不应返回此错误，而应返回包含适当上下文的错误，
// 该错误满足
//
//	errors.Is(err, errors.ErrUnsupported)
//
// 通过直接包装 ErrUnsupported 或实现 [Is] 方法。
//
// 函数和方法应记录将返回包装此错误的情况。
var ErrUnsupported = New("unsupported operation")
