// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package os

import (
	"internal/poll"
	"io/fs"
)

// 一些常见系统调用错误的可移植类似物。
//
// 从此包返回的错误可以使用 [errors.Is] 与这些错误进行比较。
var (
	// ErrInvalid 表示无效参数。
	// 当接收器为 nil 时，File 上的方法将返回此错误。
	ErrInvalid = fs.ErrInvalid // "invalid argument"

	ErrPermission = fs.ErrPermission // "permission denied" 权限被拒绝
	ErrExist      = fs.ErrExist      // "file already exists" 文件已存在
	ErrNotExist   = fs.ErrNotExist   // "file does not exist" 文件不存在
	ErrClosed     = fs.ErrClosed     // "file already closed" 文件已关闭

	ErrNoDeadline       = errNoDeadline()       // "file type does not support deadline" 文件类型不支持截止时间
	ErrDeadlineExceeded = errDeadlineExceeded() // "i/o timeout" I/O 超时
)

func errNoDeadline() error { return poll.ErrNoDeadline }

// errDeadlineExceeded 返回 os.ErrDeadlineExceeded 的值。
// 此错误来自 internal/poll 包，该包也被 net 包使用。
// 这样做可以确保 net 包在截止时间超过时返回 os.ErrDeadlineExceeded，
// 如 net.Conn.SetDeadline 所记录的那样，而不需要 net 包中的任何额外
// 工作，也不需要 internal/poll 包导入 os（它不能这样做，因为那会导致循环导入）。
func errDeadlineExceeded() error { return poll.ErrDeadlineExceeded }

type timeout interface {
	Timeout() bool
}

// PathError 记录错误以及导致错误的操作和文件路径。
type PathError = fs.PathError

// SyscallError 记录来自特定系统调用的错误。
type SyscallError struct {
	Syscall string
	Err     error
}

func (e *SyscallError) Error() string { return e.Syscall + ": " + e.Err.Error() }

func (e *SyscallError) Unwrap() error { return e.Err }

// Timeout 报告此错误是否表示超时。
func (e *SyscallError) Timeout() bool {
	t, ok := e.Err.(timeout)
	return ok && t.Timeout()
}

// NewSyscallError 返回一个新的 [SyscallError] 作为错误，
// 包含给定的系统调用名称和错误详情。
// 为方便起见，如果 err 为 nil，NewSyscallError 返回 nil。
func NewSyscallError(syscall string, err error) error {
	if err == nil {
		return nil
	}
	return &SyscallError{syscall, err}
}

// IsExist 返回一个布尔值，指示其参数是否已知报告
// 文件或目录已存在。[ErrExist] 以及一些 syscall 错误满足此条件。
//
// 此函数早于 [errors.Is]。它只支持 os 包返回的错误。
// 新代码应使用 errors.Is(err, fs.ErrExist)。
func IsExist(err error) bool {
	return underlyingErrorIs(err, ErrExist)
}

// IsNotExist 返回一个布尔值，指示其参数是否已知报告
// 文件或目录不存在。[ErrNotExist] 以及一些 syscall 错误满足此条件。
//
// 此函数早于 [errors.Is]。它只支持 os 包返回的错误。
// 新代码应使用 errors.Is(err, fs.ErrNotExist)。
func IsNotExist(err error) bool {
	return underlyingErrorIs(err, ErrNotExist)
}

// IsPermission 返回一个布尔值，指示其参数是否已知报告
// 权限被拒绝。[ErrPermission] 以及一些 syscall 错误满足此条件。
//
// 此函数早于 [errors.Is]。它只支持 os 包返回的错误。
// 新代码应使用 errors.Is(err, fs.ErrPermission)。
func IsPermission(err error) bool {
	return underlyingErrorIs(err, ErrPermission)
}

// IsTimeout 返回一个布尔值，指示其参数是否已知报告发生了超时。
//
// 此函数早于 [errors.Is]，并且错误是否表示超时的概念可能是模糊的。
// 例如，Unix 错误 EWOULDBLOCK 有时表示超时，有时不表示。
// 新代码应使用 errors.Is 与适合返回错误的调用的值，
// 例如 [os.ErrDeadlineExceeded]。
func IsTimeout(err error) bool {
	terr, ok := underlyingError(err).(timeout)
	return ok && terr.Timeout()
}

func underlyingErrorIs(err, target error) bool {
	// 注意此函数不是 errors.Is：
	// underlyingError 只解包它历史上处理的特定错误包装类型，
	// 而不是所有实现 Unwrap() 的错误。
	err = underlyingError(err)
	if err == target {
		return true
	}
	// 为了保持先前的行为，只检查 syscall 错误。
	e, ok := err.(syscallErrorType)
	return ok && e.Is(target)
}

// underlyingError 返回已知 os 错误类型的底层错误。
func underlyingError(err error) error {
	switch err := err.(type) {
	case *PathError:
		return err.Err
	case *LinkError:
		return err.Err
	case *SyscallError:
		return err.Err
	}
	return err
}
