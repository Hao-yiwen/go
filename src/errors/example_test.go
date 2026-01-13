// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package errors_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"
)

// MyError 是一个包含时间和消息的错误实现。
type MyError struct {
	When time.Time
	What string
}

func (e MyError) Error() string {
	return fmt.Sprintf("%v: %v", e.When, e.What)
}

func oops() error {
	return MyError{
		time.Date(1989, 3, 15, 22, 30, 0, 0, time.UTC),
		"the file system has gone away",
	}
}

func Example() {
	if err := oops(); err != nil {
		fmt.Println(err)
	}
	// Output: 1989-03-15 22:30:00 +0000 UTC: the file system has gone away
}

func ExampleNew() {
	err := errors.New("emit macho dwarf: elf header corrupted")
	if err != nil {
		fmt.Print(err)
	}
	// Output: emit macho dwarf: elf header corrupted
}

func OopsNew() error {
	return errors.New("an error")
}

var ErrSentinel = errors.New("an error")

func OopsSentinel() error {
	return ErrSentinel
}

// 每次调用 [errors.New] 都会返回一个唯一的错误实例，
// 即使参数相同也是如此。要匹配由 [errors.New] 创建的错误，
// 请声明一个哨兵错误并重新使用它。
func ExampleNew_unique() {
	err1 := OopsNew()
	err2 := OopsNew()
	fmt.Println("Errors using distinct errors.New calls:")
	fmt.Printf("Is(%q, %q) = %v\n", err1, err2, errors.Is(err1, err2))

	err3 := OopsSentinel()
	err4 := OopsSentinel()
	fmt.Println()
	fmt.Println("Errors using a sentinel error:")
	fmt.Printf("Is(%q, %q) = %v\n", err3, err4, errors.Is(err3, err4))

	// Output:
	// Errors using distinct errors.New calls:
	// Is("an error", "an error") = false
	//
	// Errors using a sentinel error:
	// Is("an error", "an error") = true
}

// fmt 包的 Errorf 函数允许我们使用该包的格式化功能来创建描述性的错误消息。
func ExampleNew_errorf() {
	const name, id = "bimmler", 17
	err := fmt.Errorf("user %q (id %d) not found", name, id)
	if err != nil {
		fmt.Print(err)
	}
	// Output: user "bimmler" (id 17) not found
}

func ExampleJoin() {
	err1 := errors.New("err1")
	err2 := errors.New("err2")
	err := errors.Join(err1, err2)
	fmt.Println(err)
	if errors.Is(err, err1) {
		fmt.Println("err is err1")
	}
	if errors.Is(err, err2) {
		fmt.Println("err is err2")
	}
	fmt.Println(err.(interface{ Unwrap() []error }).Unwrap())
	// Output:
	// err1
	// err2
	// err is err1
	// err is err2
	// [err1 err2]
}

func ExampleIs() {
	if _, err := os.Open("non-existing"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			fmt.Println("file does not exist")
		} else {
			fmt.Println(err)
		}
	}

	// Output:
	// file does not exist
}

type MyIsError struct {
	err string
}

func (e MyIsError) Error() string {
	return e.err
}
func (e MyIsError) Is(err error) bool {
	return err == fs.ErrPermission
}

// 自定义错误可以实现 "Is(error) bool" 方法来匹配其他错误值，
// 这会覆盖 [errors.Is] 的默认匹配行为。
func ExampleIs_custom_match() {
	var err error = MyIsError{"an error"}
	fmt.Println("Error equals fs.ErrPermission:", err == fs.ErrPermission)
	fmt.Println("Error is fs.ErrPermission:", errors.Is(err, fs.ErrPermission))

	// Output:
	// Error equals fs.ErrPermission: false
	// Error is fs.ErrPermission: true
}

func ExampleAs() {
	if _, err := os.Open("non-existing"); err != nil {
		var pathError *fs.PathError
		if errors.As(err, &pathError) {
			fmt.Println("Failed at path:", pathError.Path)
		} else {
			fmt.Println(err)
		}
	}

	// Output:
	// Failed at path: non-existing
}

func ExampleAsType() {
	if _, err := os.Open("non-existing"); err != nil {
		if pathError, ok := errors.AsType[*fs.PathError](err); ok {
			fmt.Println("Failed at path:", pathError.Path)
		} else {
			fmt.Println(err)
		}
	}
	// Output:
	// Failed at path: non-existing
}

type MyAsError struct {
	err string
}

func (e MyAsError) Error() string {
	return e.err
}
func (e MyAsError) As(target any) bool {
	pe, ok := target.(**fs.PathError)
	if !ok {
		return false
	}
	*pe = &fs.PathError{
		Op:   "custom",
		Path: "/",
		Err:  errors.New(e.err),
	}
	return true
}

// 自定义错误可以实现 "As(any) bool" 方法来匹配其他错误类型，
// 这会覆盖 [errors.As] 的默认匹配行为。
func ExampleAs_custom_match() {
	var err error = MyAsError{"an error"}
	fmt.Println("Error:", err)
	fmt.Printf("TypeOf err: %T\n", err)

	var pathError *fs.PathError
	ok := errors.As(err, &pathError)
	fmt.Println("Error as fs.PathError:", ok)
	fmt.Println("fs.PathError:", pathError)

	// Output:
	// Error: an error
	// TypeOf err: errors_test.MyAsError
	// Error as fs.PathError: true
	// fs.PathError: custom /: an error
}

// 自定义错误可以实现 "As(any) bool" 方法来匹配其他错误类型，
// 这会覆盖 [errors.AsType] 的默认匹配行为。
func ExampleAsType_custom_match() {
	var err error = MyAsError{"an error"}
	fmt.Println("Error:", err)
	fmt.Printf("TypeOf err: %T\n", err)

	pathError, ok := errors.AsType[*fs.PathError](err)
	fmt.Println("Error as fs.PathError:", ok)
	fmt.Println("fs.PathError:", pathError)

	// Output:
	// Error: an error
	// TypeOf err: errors_test.MyAsError
	// Error as fs.PathError: true
	// fs.PathError: custom /: an error
}

func ExampleUnwrap() {
	err1 := errors.New("error1")
	err2 := fmt.Errorf("error2: [%w]", err1)
	fmt.Println(err2)
	fmt.Println(errors.Unwrap(err2))
	// Output:
	// error2: [error1]
	// error1
}
