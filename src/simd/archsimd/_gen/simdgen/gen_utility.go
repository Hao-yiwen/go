// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"text/template"
	"unicode"
)

func templateOf(temp, name string) *template.Template {
	t, err := template.New(name).Parse(temp)
	if err != nil {
		panic(fmt.Errorf("failed to parse template %s: %w", name, err))
	}
	return t
}

func createPath(goroot string, file string) (*os.File, error) {
	fp := filepath.Join(goroot, file)
	dir := filepath.Dir(fp)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
	}
	f, err := os.Create(fp)
	if err != nil {
		return nil, fmt.Errorf("failed to create file %s: %w", fp, err)
	}
	return f, nil
}

func formatWriteAndClose(out *bytes.Buffer, goroot string, file string) {
	b, err := format.Source(out.Bytes())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		fmt.Fprintf(os.Stderr, "%s\n", numberLines(out.Bytes()))
		fmt.Fprintf(os.Stderr, "%v\n", err)
		panic(err)
	} else {
		writeAndClose(b, goroot, file)
	}
}

func writeAndClose(b []byte, goroot string, file string) {
	ofile, err := createPath(goroot, file)
	if err != nil {
		panic(err)
	}
	ofile.Write(b)
	ofile.Close()
}

// numberLines 接收一个字节切片，返回一个每行都带有行号的字符串，
// 行号从 1 开始。
func numberLines(data []byte) string {
	var buf bytes.Buffer
	r := bytes.NewReader(data)
	s := bufio.NewScanner(r)
	for i := 1; s.Scan(); i++ {
		fmt.Fprintf(&buf, "%d: %s\n", i, s.Text())
	}
	return buf.String()
}

type inShape uint8
type outShape uint8
type maskShape uint8
type immShape uint8
type memShape uint8

const (
	InvalidIn     inShape = iota
	PureVregIn            // 仅向量寄存器输入
	OneKmaskIn            // 向量和 kmask 输入
	OneImmIn              // 向量和立即数输入
	OneKmaskImmIn         // 向量、kmask 和立即数输入
	PureKmaskIn           // 仅掩码输入
)

const (
	InvalidOut     outShape = iota
	NoOut                   // 无输出
	OneVregOut              // (一个) 向量寄存器输出
	OneGregOut              // (一个) 通用寄存器输出
	OneKmaskOut             // 掩码输出
	OneVregOutAtIn          // 第一个输入也是输出
)

const (
	InvalidMask maskShape = iota
	NoMask                // 无掩码
	OneMask               // 带掩码 (K1 到 K7)
	AllMasks              // K 掩码指令 (K0-K7)
)

const (
	InvalidImm  immShape = iota
	NoImm                // 无立即数
	ConstImm             // 仅常量立即数
	VarImm               // 用户提供的纯立即数参数
	ConstVarImm          // 用户参数和常量的组合
)

const (
	InvalidMem memShape = iota
	NoMem
	VregMemIn // 该指令包含一个正在加载向量寄存器的内存输入。
)

// opShape 返回描述操作形状的几个整数，以及 op 的修改版本：
//
// opNoImm 是 op，但其输入不包含常量立即数。
//
// 此函数不修改 op。
func (op *Operation) shape() (shapeIn inShape, shapeOut outShape, maskType maskShape, immType immShape,
	opNoImm Operation) {
	if len(op.Out) > 1 {
		panic(fmt.Errorf("simdgen only supports 1 output: %s", op))
	}
	var outputReg int
	if len(op.Out) == 1 {
		outputReg = op.Out[0].AsmPos
		if op.Out[0].Class == "vreg" {
			shapeOut = OneVregOut
		} else if op.Out[0].Class == "greg" {
			shapeOut = OneGregOut
		} else if op.Out[0].Class == "mask" {
			shapeOut = OneKmaskOut
		} else {
			panic(fmt.Errorf("simdgen only supports output of class vreg or mask: %s", op))
		}
	} else {
		shapeOut = NoOut
		// TODO: 这些只是加载/存储吗？
		// 我们手动支持了两个 Load 和 Store，这些够吗？
		panic(fmt.Errorf("simdgen only supports 1 output: %s", op))
	}
	hasImm := false
	maskCount := 0
	hasVreg := false
	for _, in := range op.In {
		if in.AsmPos == outputReg {
			if shapeOut != OneVregOutAtIn && in.AsmPos == 0 && in.Class == "vreg" {
				shapeOut = OneVregOutAtIn
			} else {
				panic(fmt.Errorf("simdgen only support output and input sharing the same position case of \"the first input is vreg and the only output\": %s", op))
			}
		}
		if in.Class == "immediate" {
			// 对 XED 数据的手动检查发现 AMD64 SIMD 指令最多
			// 有 1 个立即数。所以我们不需要在这里检查这个。
			if *in.Bits != 8 {
				panic(fmt.Errorf("simdgen only supports immediates of 8 bits: %s", op))
			}
			hasImm = true
		} else if in.Class == "mask" {
			maskCount++
		} else {
			hasVreg = true
		}
	}
	opNoImm = *op

	removeImm := func(o *Operation) {
		o.In = o.In[1:]
	}
	if hasImm {
		removeImm(&opNoImm)
		if op.In[0].Const != nil {
			if op.In[0].ImmOffset != nil {
				immType = ConstVarImm
			} else {
				immType = ConstImm
			}
		} else if op.In[0].ImmOffset != nil {
			immType = VarImm
		} else {
			panic(fmt.Errorf("simdgen requires imm to have at least one of ImmOffset or Const set: %s", op))
		}
	} else {
		immType = NoImm
	}
	if maskCount == 0 {
		maskType = NoMask
	} else {
		maskType = OneMask
	}
	checkPureMask := func() bool {
		if hasImm {
			panic(fmt.Errorf("simdgen does not support immediates in pure mask operations: %s", op))
		}
		if hasVreg {
			panic(fmt.Errorf("simdgen does not support more than 1 masks in non-pure mask operations: %s", op))
		}
		return false
	}
	if !hasImm && maskCount == 0 {
		shapeIn = PureVregIn
	} else if !hasImm && maskCount > 0 {
		if maskCount == 1 {
			shapeIn = OneKmaskIn
		} else {
			if checkPureMask() {
				return
			}
			shapeIn = PureKmaskIn
			maskType = AllMasks
		}
	} else if hasImm && maskCount == 0 {
		shapeIn = OneImmIn
	} else {
		if maskCount == 1 {
			shapeIn = OneKmaskImmIn
		} else {
			checkPureMask()
			return
		}
	}
	return
}

// regShape 返回寄存器形状的字符串表示。
func (op *Operation) regShape(mem memShape) (string, error) {
	_, _, _, _, gOp := op.shape()
	var regInfo, fixedName string
	var vRegInCnt, gRegInCnt, kMaskInCnt, vRegOutCnt, gRegOutCnt, kMaskOutCnt, memInCnt, memOutCnt int
	for i, in := range gOp.In {
		switch in.Class {
		case "vreg":
			vRegInCnt++
		case "greg":
			gRegInCnt++
		case "mask":
			kMaskInCnt++
		case "memory":
			if mem != VregMemIn {
				panic("simdgen only knows VregMemIn in regShape")
			}
			memInCnt++
			vRegInCnt++
		}
		if in.FixedReg != nil {
			fixedName = fmt.Sprintf("%sAtIn%d", *in.FixedReg, i)
		}
	}
	for i, out := range gOp.Out {
		// 如果发生类覆盖，那实际上不是掩码而是向量寄存器。
		if out.Class == "vreg" || out.OverwriteClass != nil {
			vRegOutCnt++
		} else if out.Class == "greg" {
			gRegOutCnt++
		} else if out.Class == "mask" {
			kMaskOutCnt++
		} else if out.Class == "memory" {
			if mem != VregMemIn {
				panic("simdgen only knows VregMemIn in regShape")
			}
			vRegOutCnt++
			memOutCnt++
		}
		if out.FixedReg != nil {
			fixedName = fmt.Sprintf("%sAtIn%d", *out.FixedReg, i)
		}
	}
	var inRegs, inMasks, outRegs, outMasks string

	rmAbbrev := func(s string, i int) string {
		if i == 0 {
			return ""
		}
		if i == 1 {
			return s
		}
		return fmt.Sprintf("%s%d", s, i)

	}

	inRegs = rmAbbrev("v", vRegInCnt)
	inRegs += rmAbbrev("gp", gRegInCnt)
	inMasks = rmAbbrev("k", kMaskInCnt)

	outRegs = rmAbbrev("v", vRegOutCnt)
	outRegs += rmAbbrev("gp", gRegOutCnt)
	outMasks = rmAbbrev("k", kMaskOutCnt)

	if kMaskInCnt == 0 && kMaskOutCnt == 0 && gRegInCnt == 0 && gRegOutCnt == 0 {
		// 对于纯 v，我们可以将其缩写为 v%d%d。
		regInfo = fmt.Sprintf("v%d%d", vRegInCnt, vRegOutCnt)
	} else if kMaskInCnt == 0 && kMaskOutCnt == 0 {
		regInfo = fmt.Sprintf("%s%s", inRegs, outRegs)
	} else {
		regInfo = fmt.Sprintf("%s%s%s%s", inRegs, inMasks, outRegs, outMasks)
	}
	if memInCnt > 0 {
		if memInCnt == 1 {
			regInfo += "load"
		} else {
			panic("simdgen does not understand more than 1 mem op as of now")
		}
	}
	if memOutCnt > 0 {
		panic("simdgen does not understand memory as output as of now")
	}
	regInfo += fixedName
	return regInfo, nil
}

// sortOperand 对 op.In 排序，将立即数放在最前面，然后是 vreg，掩码放在最后。
// TODO: 验证这是否是 prog 结构的安全假设。
// 根据我的观察，在汇编中立即数总是第一个，
// 掩码总是最后一个，vreg 在中间。
func (op *Operation) sortOperand() {
	priority := map[string]int{"immediate": 0, "vreg": 1, "greg": 1, "mask": 2}
	sort.SliceStable(op.In, func(i, j int) bool {
		pi := priority[op.In[i].Class]
		pj := priority[op.In[j].Class]
		if pi != pj {
			return pi < pj
		}
		return op.In[i].AsmPos < op.In[j].AsmPos
	})
}

// adjustAsm 调整汇编以使其与 Go 的汇编器对齐。
func (op *Operation) adjustAsm() {
	if op.Asm == "VCVTTPD2DQ" || op.Asm == "VCVTTPD2UDQ" ||
		op.Asm == "VCVTQQ2PS" || op.Asm == "VCVTUQQ2PS" ||
		op.Asm == "VCVTPD2PS" {
		switch *op.In[0].Bits {
		case 128:
			op.Asm += "X"
		case 256:
			op.Asm += "Y"
		}
	}
}

// goNormalType 返回不返回向量的操作结果的 Go 类型名，
// 即返回通用寄存器中结果的操作。目前 Go 的 simd 库中只有一类操作
// 这样做 (GetElem)，因此这是专门为此设计的，
// 但如果有其他情况，这个问题（硬件寄存器宽度与 Go 类型
// 宽度不匹配）似乎可能会再次出现。
func (op Operation) goNormalType() string {
	if op.Go == "GetElem" {
		// GetElem 将向量的一个元素返回到通用寄存器中，
		// 但就硬件而言，该结果要么是 32 位要么是 64 位宽，
		// 无论向量元素宽度是多少。
		// 这并非"错误"，但对于 Go 源代码来说不是正确的答案。
		// 要获得正确的 Go 类型，需要将基本类型（"int"、"uint"、"float"）
		// 与输入向量元素宽度（8、16、32、64 位）组合。

		at := 0 // at 的正确值取决于立即数是否被剥离
		if op.In[at].Class == "immediate" {
			at++
		}
		return fmt.Sprintf("%s%d", *op.Out[0].Base, *op.In[at].ElemBits)
	}
	panic(fmt.Errorf("Implement goNormalType for %v", op))
}

// SSAType 返回 SSA 生成中类型引用的字符串，
// 例如在内置函数生成模板中。
func (op Operation) SSAType() string {
	if op.Out[0].Class == "greg" {
		return fmt.Sprintf("types.Types[types.T%s]", strings.ToUpper(op.goNormalType()))
	}
	return fmt.Sprintf("types.TypeVec%d", *op.Out[0].Bits)
}

// GoType 返回此操作返回的 Go 类型（相对于 simd 包），
// 例如 "int32" 或 "Int8x16"。这用于模板中。
func (op Operation) GoType() string {
	if op.Out[0].Class == "greg" {
		return op.goNormalType()
	}
	return *op.Out[0].Go
}

// ImmName 返回操作的立即数操作数使用的名称。
// 这可以在 yaml 中用操作数上的 "name" 覆盖，
// 否则，目前默认为 "constant"
func (op Operation) ImmName() string {
	return op.Op0Name("constant")
}

func (o Operand) OpName(s string) string {
	if n := o.Name; n != nil {
		return *n
	}
	if o.Class == "mask" {
		return "mask"
	}
	return s
}

func (o Operand) OpNameAndType(s string) string {
	return o.OpName(s) + " " + *o.Go
}

// GoExported 返回首字符大写的 [Go]。
func (op Operation) GoExported() string {
	return capitalizeFirst(op.Go)
}

// DocumentationExported 返回方法名大写的 [Documentation]。
func (op Operation) DocumentationExported() string {
	return strings.ReplaceAll(op.Documentation, op.Go, op.GoExported())
}

// Op0Name 返回第 0 个操作数使用的名称，
// 如果存在的话，否则使用参数。
func (op Operation) Op0Name(s string) string {
	return op.In[0].OpName(s)
}

// Op1Name 返回第 1 个操作数使用的名称，
// 如果存在的话，否则使用参数。
func (op Operation) Op1Name(s string) string {
	return op.In[1].OpName(s)
}

// Op2Name 返回第 2 个操作数使用的名称，
// 如果存在的话，否则使用参数。
func (op Operation) Op2Name(s string) string {
	return op.In[2].OpName(s)
}

// Op3Name 返回第 3 个操作数使用的名称，
// 如果存在的话，否则使用参数。
func (op Operation) Op3Name(s string) string {
	return op.In[3].OpName(s)
}

// Op0NameAndType 返回第 0 个操作数使用的名称和类型，
// 如果提供了名称，否则使用参数值作为默认值。
func (op Operation) Op0NameAndType(s string) string {
	return op.In[0].OpNameAndType(s)
}

// Op1NameAndType 返回第 1 个操作数使用的名称和类型，
// 如果提供了名称，否则使用参数值作为默认值。
func (op Operation) Op1NameAndType(s string) string {
	return op.In[1].OpNameAndType(s)
}

// Op2NameAndType 返回第 2 个操作数使用的名称和类型，
// 如果提供了名称，否则使用参数值作为默认值。
func (op Operation) Op2NameAndType(s string) string {
	return op.In[2].OpNameAndType(s)
}

// Op3NameAndType 返回第 3 个操作数使用的名称和类型，
// 如果提供了名称，否则使用参数值作为默认值。
func (op Operation) Op3NameAndType(s string) string {
	return op.In[3].OpNameAndType(s)
}

// Op4NameAndType 返回第 4 个操作数使用的名称和类型，
// 如果提供了名称，否则使用参数值作为默认值。
func (op Operation) Op4NameAndType(s string) string {
	return op.In[4].OpNameAndType(s)
}

var immClasses []string = []string{"BAD0Imm", "BAD1Imm", "op1Imm8", "op2Imm8", "op3Imm8", "op4Imm8"}
var classes []string = []string{"BAD0", "op1", "op2", "op3", "op4"}

// classifyOp 根据操作的存根和内置函数形状，返回分类字符串、修改后的操作，
// 以及可能的错误。
// 分类字符串在正则表达式集 "op[1234](Imm8)?(_<order>)?" 中，
// 其中 "<order>" 后缀可选地附加到其输入 yaml 中的 Operation。
// 分类字符串用于选择模板或模板的子句，
// 用于内置函数声明和编译器中的 ssagen 内置函数粘合代码。
func classifyOp(op Operation) (string, Operation, error) {
	_, _, _, immType, gOp := op.shape()

	var class string

	if immType == VarImm || immType == ConstVarImm {
		switch l := len(op.In); l {
		case 1:
			return "", op, fmt.Errorf("simdgen does not recognize this operation of only immediate input: %s", op)
		case 2, 3, 4, 5:
			class = immClasses[l]
		default:
			return "", op, fmt.Errorf("simdgen does not recognize this operation of input length %d: %s", len(op.In), op)
		}
		if order := op.OperandOrder; order != nil {
			class += "_" + *order
		}
		return class, op, nil
	} else {
		switch l := len(gOp.In); l {
		case 1, 2, 3, 4:
			class = classes[l]
		default:
			return "", op, fmt.Errorf("simdgen does not recognize this operation of input length %d: %s", len(op.In), op)
		}
		if order := op.OperandOrder; order != nil {
			class += "_" + *order
		}
		return class, gOp, nil
	}
}

func checkVecAsScalar(op Operation) (idx int, err error) {
	idx = -1
	sSize := 0
	for i, o := range op.In {
		if o.TreatLikeAScalarOfSize != nil {
			if idx == -1 {
				idx = i
				sSize = *o.TreatLikeAScalarOfSize
			} else {
				err = fmt.Errorf("simdgen only supports one TreatLikeAScalarOfSize in the arg list: %s", op)
				return
			}
		}
	}
	if idx >= 0 {
		if sSize != 8 && sSize != 16 && sSize != 32 && sSize != 64 {
			err = fmt.Errorf("simdgen does not recognize this uint size: %d, %s", sSize, op)
			return
		}
	}
	return
}

func rewriteVecAsScalarRegInfo(op Operation, regInfo string) (string, error) {
	idx, err := checkVecAsScalar(op)
	if err != nil {
		return "", err
	}
	if idx != -1 {
		if regInfo == "v21" {
			regInfo = "vfpv"
		} else if regInfo == "v2kv" {
			regInfo = "vfpkv"
		} else if regInfo == "v31" {
			regInfo = "v2fpv"
		} else if regInfo == "v3kv" {
			regInfo = "v2fpkv"
		} else {
			return "", fmt.Errorf("simdgen does not recognize uses of treatLikeAScalarOfSize with op regShape %s in op: %s", regInfo, op)
		}
	}
	return regInfo, nil
}

func rewriteLastVregToMem(op Operation) Operation {
	newIn := make([]Operand, len(op.In))
	lastVregIdx := -1
	for i := range len(op.In) {
		newIn[i] = op.In[i]
		if op.In[i].Class == "vreg" {
			lastVregIdx = i
		}
	}
	// vbcst 操作总是将其内存操作放在最后一个 vreg。
	if lastVregIdx == -1 {
		panic("simdgen cannot find one vreg in the mem op vreg original")
	}
	newIn[lastVregIdx].Class = "memory"
	op.In = newIn

	return op
}

// dedup 在完整结构级别上去重操作。
func dedup(ops []Operation) (deduped []Operation) {
	for _, op := range ops {
		seen := false
		for _, dop := range deduped {
			if reflect.DeepEqual(op, dop) {
				seen = true
				break
			}
		}
		if !seen {
			deduped = append(deduped, op)
		}
	}
	return
}

func (op Operation) GenericName() string {
	if op.OperandOrder != nil {
		switch *op.OperandOrder {
		case "21Type1", "231Type1":
			// Permute 使用 operand[1] 作为方法接收者。
			return op.Go + *op.In[1].Go
		}
	}
	if op.In[0].Class == "immediate" {
		return op.Go + *op.In[1].Go
	}
	return op.Go + *op.In[0].Go
}

// dedupGodef 在 [Op.Go]+[*Op.In[0].Go] 级别上去重操作。
// 去重意味着选择满足要求的最低高级架构：
// AVX512 将是最不优先的。
// 如果设置了 FlagNoDedup，它将向控制台报告重复项。
func dedupGodef(ops []Operation) ([]Operation, error) {
	seen := map[string][]Operation{}
	for _, op := range ops {
		_, _, _, _, gOp := op.shape()

		gN := gOp.GenericName()
		seen[gN] = append(seen[gN], op)
	}
	if *FlagReportDup {
		for gName, dup := range seen {
			if len(dup) > 1 {
				log.Printf("Duplicate for %s:\n", gName)
				for _, op := range dup {
					log.Printf("%s\n", op)
				}
			}
		}
		return ops, nil
	}
	isAVX512 := func(op Operation) bool {
		return strings.Contains(op.CPUFeature, "AVX512")
	}
	deduped := []Operation{}
	for _, dup := range seen {
		if len(dup) > 1 {
			slices.SortFunc(dup, func(i, j Operation) int {
				// 将非 AVX512 候选项放在开头
				if !isAVX512(i) && isAVX512(j) {
					return -1
				}
				if isAVX512(i) && !isAVX512(j) {
					return 1
				}
				if i.CPUFeature != j.CPUFeature {
					return strings.Compare(i.CPUFeature, j.CPUFeature)
				}
				// 奇怪的是，Intel 有时会为同一指令定义重复项，
				// 这会混淆 XED 内存操作合并逻辑：[MemFeature] 只会附加到一条指令
				// 一次，这意味着对于本质上重复的指令，只有一个会设置
				// 正确的 [MemFeature]。我们必须使这个排序对于 [MemFeature] 是确定性的。
				if i.MemFeatures != nil && j.MemFeatures == nil {
					return -1
				}
				if i.MemFeatures == nil && j.MemFeatures != nil {
					return 1
				}
				if i.Commutative != j.Commutative {
					if j.Commutative {
						return -1
					}
					return 1
				}
				// 它们的顺序不再重要了，至少目前是这样。
				return 0
			})
		}
		deduped = append(deduped, dup[0])
	}
	slices.SortFunc(deduped, compareOperations)
	return deduped, nil
}

// 将 op.ConstImm 复制到 op.In[0].Const
// 这是一个技巧，用于减少我们需要的常量立即数操作定义的大小。
func copyConstImm(ops []Operation) error {
	for _, op := range ops {
		if op.ConstImm == nil {
			continue
		}
		_, _, _, immType, _ := op.shape()

		if immType == ConstImm || immType == ConstVarImm {
			op.In[0].Const = op.ConstImm
		}
		// 否则，就不移植它 - 例如 {VPCMP[BWDQ] imm=0} 和 {VPCMPEQ[BWDQ]} 是
		// 相同的操作 "Equal"，[dedupgodef] 应该能够区分它们。
	}
	return nil
}

func capitalizeFirst(s string) string {
	if s == "" {
		return ""
	}
	// 将字符串转换为符文切片以正确处理多字节字符。
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// overwrite 纠正一些由于以下原因导致的错误：
//   - XED 数据是错误的
//   - Go 的 SIMD API 要求，例如 AVX2 比较也应该产生掩码。
//     这个重写有严格的约束，请参阅错误消息。
//     这些约束也在 [writeSIMDRules]、[writeSIMDMachineOps]
//     和 [writeSIMDSSA] 中被利用，更新这些约束时请小心。
func overwrite(ops []Operation) error {
	hasClassOverwrite := false
	overwrite := func(op []Operand, idx int, o Operation) error {
		if op[idx].OverwriteElementBits != nil {
			if op[idx].ElemBits == nil {
				panic(fmt.Errorf("ElemBits is nil at operand %d of %v", idx, o))
			}
			*op[idx].ElemBits = *op[idx].OverwriteElementBits
			*op[idx].Lanes = *op[idx].Bits / *op[idx].ElemBits
			*op[idx].Go = fmt.Sprintf("%s%dx%d", capitalizeFirst(*op[idx].Base), *op[idx].ElemBits, *op[idx].Lanes)
		}
		if op[idx].OverwriteClass != nil {
			if op[idx].OverwriteBase == nil {
				panic(fmt.Errorf("simdgen: [OverwriteClass] must be set together with [OverwriteBase]: %s", op[idx]))
			}
			oBase := *op[idx].OverwriteBase
			oClass := *op[idx].OverwriteClass
			if oClass != "mask" {
				panic(fmt.Errorf("simdgen: [Class] overwrite only supports overwritting to mask: %s", op[idx]))
			}
			if oBase != "int" {
				panic(fmt.Errorf("simdgen: [Class] overwrite must set [OverwriteBase] to int: %s", op[idx]))
			}
			if op[idx].Class != "vreg" {
				panic(fmt.Errorf("simdgen: [Class] overwrite must be overwriting [Class] from vreg: %s", op[idx]))
			}
			hasClassOverwrite = true
			*op[idx].Base = oBase
			op[idx].Class = oClass
			*op[idx].Go = fmt.Sprintf("Mask%dx%d", *op[idx].ElemBits, *op[idx].Lanes)
		} else if op[idx].OverwriteBase != nil {
			oBase := *op[idx].OverwriteBase
			*op[idx].Go = strings.ReplaceAll(*op[idx].Go, capitalizeFirst(*op[idx].Base), capitalizeFirst(oBase))
			if op[idx].Class == "greg" {
				*op[idx].Go = strings.ReplaceAll(*op[idx].Go, *op[idx].Base, oBase)
			}
			*op[idx].Base = oBase
		}
		return nil
	}
	for i, o := range ops {
		hasClassOverwrite = false
		for j := range ops[i].In {
			if err := overwrite(ops[i].In, j, o); err != nil {
				return err
			}
			if hasClassOverwrite {
				return fmt.Errorf("simdgen does not support [OverwriteClass] in inputs: %s", ops[i])
			}
		}
		for j := range ops[i].Out {
			if err := overwrite(ops[i].Out, j, o); err != nil {
				return err
			}
		}
		if hasClassOverwrite {
			for _, in := range ops[i].In {
				if in.Class == "mask" {
					return fmt.Errorf("simdgen only supports [OverwriteClass] for operations without mask inputs")
				}
			}
		}
	}
	return nil
}

// reportXEDInconsistency 报告潜在的 XED 不一致性。
// 我们可以向 [Operation] 添加更多字段以启用更多检查并在此处实现。
// 支持的检查：
// [NameAndSizeCheck]：NAME[BWDQ] 应该相应地设置 elemBits。
// 此检查用于查找不一致性，然后我们可以向这些定义添加覆盖字段
// 以手动更正它们。
func reportXEDInconsistency(ops []Operation) error {
	for _, o := range ops {
		if o.NameAndSizeCheck != nil {
			suffixSizeMap := map[byte]int{'B': 8, 'W': 16, 'D': 32, 'Q': 64}
			checkOperand := func(opr Operand) error {
				if opr.ElemBits == nil {
					return fmt.Errorf("simdgen expects elemBits to be set when performing NameAndSizeCheck")
				}
				if v, ok := suffixSizeMap[o.Asm[len(o.Asm)-1]]; !ok {
					return fmt.Errorf("simdgen expects asm to end with [BWDQ] when performing NameAndSizeCheck")
				} else {
					if v != *opr.ElemBits {
						return fmt.Errorf("simdgen finds NameAndSizeCheck inconsistency in def: %s", o)
					}
				}
				return nil
			}
			for _, in := range o.In {
				if in.Class != "vreg" && in.Class != "mask" {
					continue
				}
				if in.TreatLikeAScalarOfSize != nil {
					// 这是一个不规则的操作数，不检查它。
					continue
				}
				if err := checkOperand(in); err != nil {
					return err
				}
			}
			for _, out := range o.Out {
				if err := checkOperand(out); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (o *Operation) hasMaskedMerging(maskType maskShape, outType outShape) bool {
	// BLEND 和 VMOVDQU 不是面向用户的操作，所以我们应该将它们过滤掉。
	return o.OperandOrder == nil && o.SpecialLower == nil && maskType == OneMask && outType == OneVregOut &&
		len(o.InVariant) == 1 && !strings.Contains(o.Asm, "BLEND") && !strings.Contains(o.Asm, "VMOVDQU")
}

func getVbcstData(s string) (feat1Match, feat2Match string) {
	_, err := fmt.Sscanf(s, "feat1=%[^;];feat2=%s", &feat1Match, &feat2Match)
	if err != nil {
		panic(err)
	}
	return
}

func (o Operation) String() string {
	return pprints(o)
}

func (op Operand) String() string {
	return pprints(op)
}
