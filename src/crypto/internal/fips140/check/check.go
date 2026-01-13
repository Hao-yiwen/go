// 版权所有 2024 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package check 实现了 FIPS 140 加载时代码+数据验证。
// 除了 hmac 和 sha256 外，提供密码功能的每个 FIPS 包
// 必须导入 crypto/internal/fips140/check，以便验证发生
// 在包全局变量初始化之前。
// hmac 和 sha256 包由此包使用，因此它们无法导入它。
// 相反，这些包必须小心不要在 init 期间更改全局变量。
// （如有必要，我们可以让 check 在检查完成后
// 在这些包中调用 PostCheck 函数。）
package check

import (
	"crypto/internal/fips140"
	"crypto/internal/fips140/hmac"
	"crypto/internal/fips140/sha256"
	"crypto/internal/fips140deps/byteorder"
	"crypto/internal/fips140deps/godebug"
	"io"
	"unsafe"
)

// Verified 在验证成功时设置。当 [fips140.Enabled] 为真时，
// 总是可以期望为真，否则 init 会 panic。
var Verified bool

// Linkinfo 存储由链接器准备的 go:fipsinfo 符号。
// 详见 cmd/link/internal/ld/fips.go。
//
//go:linkname Linkinfo go:fipsinfo
var Linkinfo struct {
	Magic [16]byte
	Sum   [32]byte
	Self  uintptr
	Sects [4]struct {
		// 注意：这些必须是 unsafe.Pointer，而不是 uintptr，
		// 否则 checkptr 会在 go test -race 期间关于
		// 将 uintptr 转换为指向数据段的指针时 panic。
		Start unsafe.Pointer
		End   unsafe.Pointer
	}
}

// "\xff"+fipsMagic 是预期的 linkinfo.Magic。
// 我们避免明确写出来，以便字符串不会
// 在正常二进制文件中的其他地方出现，只是作为预防措施。
const fipsMagic = " Go fipsinfo \xff\x00"

var zeroSum [32]byte

func init() {
	if !fips140.Enabled {
		return
	}

	if err := fips140.Supported(); err != nil {
		panic("fips140: " + err.Error())
	}

	if Linkinfo.Magic[0] != 0xff || string(Linkinfo.Magic[1:]) != fipsMagic || Linkinfo.Sum == zeroSum {
		panic("fips140: no verification checksum found")
	}

	h := hmac.New(sha256.New, make([]byte, 32))
	w := io.Writer(h)

	/*
		// 取消注释以调试。
		// 注释（相对于 const bool 标志）
		// 以避免在默认构建中导入"os"。
		f, err := os.Create("fipscheck.o")
		if err != nil {
			panic(err)
		}
		w = io.MultiWriter(h, f)
	*/

	w.Write([]byte("go fips object v1\n"))

	var nbuf [8]byte
	for _, sect := range Linkinfo.Sects {
		n := uintptr(sect.End) - uintptr(sect.Start)
		byteorder.BEPutUint64(nbuf[:], uint64(n))
		w.Write(nbuf[:])
		w.Write(unsafe.Slice((*byte)(sect.Start), n))
	}
	sum := h.Sum(nil)

	if [32]byte(sum) != Linkinfo.Sum {
		panic("fips140: verification mismatch")
	}

	// "在模块软件或固件完整性测试期间生成的临时值应 [05.10]
	// 从模块中清零，完整性测试完成后"
	clear(sum)
	clear(nbuf[:])
	h.Reset()

	if godebug.Value("fips140") == "debug" {
		println("fips140: verified code+data")
	}

	Verified = true
}
