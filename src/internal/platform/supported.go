// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:generate go test . -run=^TestGenerated$ -fix

package platform

// An OSArch 是一对 GOOS 和 GOARCH 值，表示一个平台。
type OSArch struct {
	GOOS, GOARCH string
}

func (p OSArch) String() string {
	return p.GOOS + "/" + p.GOARCH
}

// RaceDetectorSupported 报告 goos/goarch 是否支持竞态
// 检测器。cmd/dist/test.go 中有此函数的副本。
// 竞态检测器仅在 arm64 上支持 48 位 VMA。但它会始终
// 为 arm64 返回 true，因为我们在
// 编译时没有 VMA 大小信息。
func RaceDetectorSupported(goos, goarch string) bool {
	switch goos {
	case "linux":
		return goarch == "amd64" || goarch == "arm64" || goarch == "loong64" || goarch == "ppc64le" || goarch == "riscv64" || goarch == "s390x"
	case "darwin":
		return goarch == "amd64" || goarch == "arm64"
	case "freebsd", "netbsd", "windows":
		return goarch == "amd64"
	default:
		return false
	}
}

// MSanSupported 报告 goos/goarch 是否支持内存
// 检测器选项。
func MSanSupported(goos, goarch string) bool {
	switch goos {
	case "linux":
		return goarch == "amd64" || goarch == "arm64" || goarch == "loong64"
	case "freebsd":
		return goarch == "amd64"
	default:
		return false
	}
}

// ASanSupported 报告 goos/goarch 是否支持地址
// 检测器选项。
func ASanSupported(goos, goarch string) bool {
	switch goos {
	case "linux":
		return goarch == "arm64" || goarch == "amd64" || goarch == "loong64" || goarch == "riscv64" || goarch == "ppc64le"
	default:
		return false
	}
}

// FuzzSupported 报告 goos/goarch 是否支持模糊测试
// ('go test -fuzz=.')。
func FuzzSupported(goos, goarch string) bool {
	switch goos {
	case "darwin", "freebsd", "linux", "openbsd", "windows":
		return true
	default:
		return false
	}
}

// FuzzInstrumented 报告 goos/goarch 上的模糊测试是否使用覆盖
// 检测。（FuzzInstrumented 意味着 FuzzSupported。）
func FuzzInstrumented(goos, goarch string) bool {
	switch goarch {
	case "amd64", "arm64", "loong64":
		// TODO(#14565): support more architectures.
		return FuzzSupported(goos, goarch)
	default:
		return false
	}
}

// MustLinkExternal 报告 goos/goarch 是否需要外部链接
// 有或没有 cgo 依赖关系。
func MustLinkExternal(goos, goarch string, withCgo bool) bool {
	if withCgo {
		switch goarch {
		case "mips", "mipsle", "mips64", "mips64le":
			// 内部链接 cgo 在某些体系结构上不完整。
			// https://go.dev/issue/14449
			return true
		case "ppc64":
			// Big Endian PPC64 cgo 内部链接未针对 aix 或 linux 实现。
			// https://go.dev/issue/8912
			if goos == "aix" || goos == "linux" {
				return true
			}
		}

		switch goos {
		case "android":
			return true
		case "dragonfly":
			// 似乎在 Dragonfly 上，线程本地存储是
			// 由动态链接器设置，所以内部 cgo 链接
			// 不起作用。测试用例是"go test runtime/cgo"。
			return true
		}
	}

	switch goos {
	case "android":
		if goarch != "arm64" {
			return true
		}
	case "ios":
		if goarch == "arm64" {
			return true
		}
	}
	return false
}

// BuildModeSupported 报告 goos/goarch 是否支持给定的构建模式
// 使用给定的编译器。
// cmd/dist/test.go 中有此函数的副本。
func BuildModeSupported(compiler, buildmode, goos, goarch string) bool {
	if compiler == "gccgo" {
		return true
	}

	if _, ok := distInfo[OSArch{goos, goarch}]; !ok {
		return false // platform unrecognized
	}

	platform := goos + "/" + goarch
	switch buildmode {
	case "archive":
		return true

	case "c-archive":
		switch goos {
		case "aix", "darwin", "ios", "windows":
			return true
		case "linux":
			switch goarch {
			case "386", "amd64", "arm", "armbe", "arm64", "arm64be", "loong64", "ppc64le", "riscv64", "s390x":
				// linux/ppc64 不受支持，因为它
				// 还不支持外部链接模式。
				return true
			default:
				// 其他目标不支持 -shared，
				// 根据 cmd/compile/internal/base/flag.go 中的
				// ParseFlags。
				// 对于 c-archive，Go 工具传递 -shared，
				// 以便结果适合包含
				// 在 PIE 或共享库中。
				return false
			}
		case "freebsd":
			return goarch == "amd64"
		}
		return false

	case "c-shared":
		switch platform {
		case "linux/amd64", "linux/arm", "linux/arm64", "linux/loong64", "linux/386", "linux/ppc64le", "linux/riscv64", "linux/s390x",
			"android/amd64", "android/arm", "android/arm64", "android/386",
			"freebsd/amd64",
			"darwin/amd64", "darwin/arm64",
			"windows/amd64", "windows/386", "windows/arm64",
			"wasip1/wasm":
			return true
		}
		return false

	case "default":
		return true

	case "exe":
		return true

	case "pie":
		switch platform {
		case "linux/386", "linux/amd64", "linux/arm", "linux/arm64", "linux/loong64", "linux/ppc64le", "linux/riscv64", "linux/s390x",
			"android/amd64", "android/arm", "android/arm64", "android/386",
			"freebsd/amd64",
			"darwin/amd64", "darwin/arm64",
			"ios/amd64", "ios/arm64",
			"aix/ppc64",
			"openbsd/arm64",
			"windows/386", "windows/amd64", "windows/arm64":
			return true
		}
		return false

	case "shared":
		switch platform {
		case "linux/386", "linux/amd64", "linux/arm", "linux/arm64", "linux/ppc64le", "linux/s390x":
			return true
		}
		return false

	case "plugin":
		switch platform {
		case "linux/amd64", "linux/arm", "linux/arm64", "linux/386", "linux/loong64", "linux/riscv64", "linux/s390x", "linux/ppc64le",
			"android/amd64", "android/386",
			"darwin/amd64", "darwin/arm64",
			"freebsd/amd64":
			return true
		}
		return false

	default:
		return false
	}
}

func InternalLinkPIESupported(goos, goarch string) bool {
	switch goos + "/" + goarch {
	case "android/arm64",
		"darwin/amd64", "darwin/arm64",
		"linux/amd64", "linux/arm64", "linux/loong64", "linux/ppc64le",
		"windows/386", "windows/amd64", "windows/arm64":
		return true
	}
	return false
}

// DefaultPIE 报告使用"default"构建模式时 goos/goarch 是否生成 PIE 二进制文件。
// 在 Windows 上，这受 -race 的影响，
// 所以强制调用者传入该值以集中该选择。
func DefaultPIE(goos, goarch string, isRace bool) bool {
	switch goos {
	case "android", "ios":
		return true
	case "windows":
		if isRace {
			// Windows 上不支持带 -race 的 PIE；
			// 请参阅 https://go.dev/cl/416174。
			return false
		}
		return true
	case "darwin":
		return true
	}
	return false
}

// ExecutableHasDWARF 报告链接的可执行文件是否包含 DWARF
// 符号在 goos/goarch 上。
func ExecutableHasDWARF(goos, goarch string) bool {
	switch goos {
	case "plan9", "ios":
		return false
	}
	return true
}

// osArchInfo 描述从 cmd/dist 提取的 OSArch 信息
// 并存储在生成的 distInfo 映射中。
type osArchInfo struct {
	CgoSupported bool
	FirstClass   bool
	Broken       bool
}

// CgoSupported 报告 goos/goarch 是否支持 cgo。
func CgoSupported(goos, goarch string) bool {
	return distInfo[OSArch{goos, goarch}].CgoSupported
}

// FirstClass 报告 goos/goarch 是否被认为是"第一类"端口。
// （请参阅 https://go.dev/wiki/PortingPolicy#first-class-ports。）
func FirstClass(goos, goarch string) bool {
	return distInfo[OSArch{goos, goarch}].FirstClass
}

// Broken 报告 goos/goarch 是否被认为是一个损坏的端口。
// （请参阅 https://go.dev/wiki/PortingPolicy#broken-ports。）
func Broken(goos, goarch string) bool {
	return distInfo[OSArch{goos, goarch}].Broken
}
