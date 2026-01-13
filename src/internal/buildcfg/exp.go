// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package buildcfg

import (
	"fmt"
	"reflect"
	"strings"

	"internal/goexperiment"
)

// ExperimentFlags 表示相对于基线
// （平台默认）实验配置的一组 GOEXPERIMENT 标志。
type ExperimentFlags struct {
	goexperiment.Flags
	baseline goexperiment.Flags
}

// Experiment 包含为当前构建启用的工具链实验。
//
// （这不一定是编译器本身
// 构建时使用的实验集合。）
//
// Experiment.baseline 指定在当前工具链中默认启用的实验标志。
// 这实际上是"对照"
// 配置，任何与此的偏差都是一个实验。
var Experiment ExperimentFlags = func() ExperimentFlags {
	flags, err := ParseGOEXPERIMENT(GOOS, GOARCH, envOr("GOEXPERIMENT", defaultGOEXPERIMENT))
	if err != nil {
		Error = err
		return ExperimentFlags{}
	}
	return *flags
}()

// DefaultGOEXPERIMENT 是嵌入的默认 GOEXPERIMENT 字符串。
// 不保证它是规范的。
const DefaultGOEXPERIMENT = defaultGOEXPERIMENT

// FramePointerEnabled 启用平台约定的使用以
// 保存帧指针。
//
// 这曾经是一个实验，但现在它始终在
// 支持它的平台上启用。
//
// 注意：必须与 runtime.framepointer_enabled 一致。
var FramePointerEnabled = GOARCH == "amd64" || GOARCH == "arm64"

// ParseGOEXPERIMENT 解析 (GOOS, GOARCH, GOEXPERIMENT)
// 配置元组并返回启用的和基线实验
// 标志集。
//
// TODO(mdempsky): 移到 [internal/goexperiment]。
func ParseGOEXPERIMENT(goos, goarch, goexp string) (*ExperimentFlags, error) {
	// regabiSupported 在支持寄存器 ABI 的平台上设置为 true
	// 并默认启用。
	// regabiAlwaysOn 在寄存器 ABI 始终
	// 启用的平台上设置为 true。
	var regabiSupported, regabiAlwaysOn bool
	switch goarch {
	case "amd64", "arm64", "loong64", "ppc64le", "ppc64", "riscv64":
		regabiAlwaysOn = true
		regabiSupported = true
	case "s390x":
		regabiSupported = true
	}

	// dsymutil 的较旧版本（V16 之前的任何版本）不处理
	// DWARF5 中的 .debug_rnglists 部分。请参阅
	// https://github.com/golang/go/issues/26379#issuecomment-2677068742
	// 了解更多背景。这在 mac 上禁用了所有 DWARF5，这不是
	// 理想的（仅在我们知道的情况下禁用会更好
	// 构建将使用外部链接）。在 GOOS=aix 的情况下，
	// XCOFF 格式（据所知）似乎不
	// 支持 DWARF 特定的必要部分子类型
	// 如 .debug_addr（DWARF 5 所需）。
	dwarf5Supported := (goos != "darwin" && goos != "ios" && goos != "aix")

	baseline := goexperiment.Flags{
		RegabiWrappers:        regabiSupported,
		RegabiArgs:            regabiSupported,
		Dwarf5:                dwarf5Supported,
		RandomizedHeapBase64:  true,
		SizeSpecializedMalloc: true,
		GreenTeaGC:            true,
	}
	flags := &ExperimentFlags{
		Flags:    baseline,
		baseline: baseline,
	}

	// 从 GOEXPERIMENT 环境中获取对基线配置的任何更改。
	// 这可以在 make.bash 时设置
	// 并在构建时被覆盖。
	if goexp != "" {
		// 创建已知实验名称的映射。
		names := make(map[string]func(bool))
		rv := reflect.ValueOf(&flags.Flags).Elem()
		rt := rv.Type()
		for i := 0; i < rt.NumField(); i++ {
			field := rv.Field(i)
			names[strings.ToLower(rt.Field(i).Name)] = field.SetBool
		}

		// "regabi" 是所有工作 regabi 的别名
		// 子实验，而不是实验本身。这样做
		// 作为别名使 "regabi" 和 "noregabi"
		// 都做正确的事。
		names["regabi"] = func(v bool) {
			flags.RegabiWrappers = v
			flags.RegabiArgs = v
		}

		// 解析名称。
		for f := range strings.SplitSeq(goexp, ",") {
			if f == "" {
				continue
			}
			if f == "none" {
				// GOEXPERIMENT=none 禁用所有实验标志。
				// 这由 cmd/dist 使用，它不知道如何
				// 使用任何实验标志进行构建。
				flags.Flags = goexperiment.Flags{}
				continue
			}
			val := true
			if strings.HasPrefix(f, "no") {
				f, val = f[2:], false
			}
			set, ok := names[f]
			if !ok {
				return nil, fmt.Errorf("unknown GOEXPERIMENT %s", f)
			}
			set(val)
		}
	}

	if regabiAlwaysOn {
		flags.RegabiWrappers = true
		flags.RegabiArgs = true
	}
	// regabi 仅在 amd64、arm64、loong64、riscv64、s390x、ppc64 和 ppc64le 上受支持。
	if !regabiSupported {
		flags.RegabiWrappers = false
		flags.RegabiArgs = false
	}
	// 检查 regabi 依赖关系。
	if flags.RegabiArgs && !flags.RegabiWrappers {
		return nil, fmt.Errorf("GOEXPERIMENT regabiargs requires regabiwrappers")
	}
	return flags, nil
}

// String 返回规范的 GOEXPERIMENT 字符串以启用此实验
// 配置。（与基线中相同状态的实验被省略。）
func (exp *ExperimentFlags) String() string {
	return strings.Join(expList(&exp.Flags, &exp.baseline, false), ",")
}

// expList 返回与基线不同的实验的小写实验名称列表。
// base 可以为 nil 以表示没有
// 实验。如果 all 为 true，则包括所有实验标志，
// 无论基线如何。
func expList(exp, base *goexperiment.Flags, all bool) []string {
	var list []string
	rv := reflect.ValueOf(exp).Elem()
	var rBase reflect.Value
	if base != nil {
		rBase = reflect.ValueOf(base).Elem()
	}
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		name := strings.ToLower(rt.Field(i).Name)
		val := rv.Field(i).Bool()
		baseVal := false
		if base != nil {
			baseVal = rBase.Field(i).Bool()
		}
		if all || val != baseVal {
			if val {
				list = append(list, name)
			} else {
				list = append(list, "no"+name)
			}
		}
	}
	return list
}

// Enabled 返回启用的实验列表，作为
// 小写的实验名称。
func (exp *ExperimentFlags) Enabled() []string {
	return expList(&exp.Flags, nil, false)
}

// All 返回所有实验设置的列表。
// 禁用的实验在列表中以 "no" 为前缀。
func (exp *ExperimentFlags) All() []string {
	return expList(&exp.Flags, nil, true)
}
