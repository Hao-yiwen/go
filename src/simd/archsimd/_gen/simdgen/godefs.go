// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"fmt"
	"log"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"simd/archsimd/_gen/unify"
)

type Operation struct {
	rawOperation

	// Go 是此操作的 Go 方法名。
	//
	// 它是通过在原始 Go 方法名上添加可选后缀派生的。
	// 目前，"Masked" 是唯一的后缀。
	Go string

	// Documentation 是此 API 的文档字符串。
	//
	// 它是从原始文档计算而来的：
	//
	// - "NAME" 被替换为 Go 方法名。
	//
	// - 对于掩码操作，会添加关于掩码的说明。
	Documentation string

	// In 是 Go 方法的参数序列。
	//
	// 对于掩码操作，这将附加掩码操作数。
	In []Operand
}

// rawOperation 是 [Operation] 的统一器表示。它在统一器解码后
// 被翻译成更解析的形式。
type rawOperation struct {
	Go string // 基本 Go 方法名

	GoArch       string  // 此定义的 GOARCH
	Asm          string  // 汇编助记符
	OperandOrder *string // 可选的操作数顺序，用于更好的 Go 声明
	// 可选标签，指示此操作与特殊的通用->机器 SSA 降低规则配对。
	// 应该与 gen_simdrules.go 中的特殊模板配对使用。
	SpecialLower *string

	In              []Operand // 参数
	InVariant       []Operand // 可选参数
	Out             []Operand // 结果
	MemFeatures     *string   // 此操作支持的内存操作数特性
	MemFeaturesData *string   // 与 MemFeatures 相关的附加数据
	Commutative     bool      // 交换性
	CPUFeature      string    // CPUID/Has* 特性名称
	Zeroing         *bool     // nil => 使用汇编后缀 ".Z"；false => 不使用汇编后缀 ".Z"
	Documentation   *string   // 文档将附加到存根注释中。
	AddDoc          *string   // 要附加的附加文档。
	// ConstMask 是一个技巧，用于减少用户为常量立即数编写的定义大小。
	// 如果存在，它将被复制到 [In[0].Const]。
	ConstImm *string
	// NameAndSizeCheck 用于检查 [BWDQ] 是否映射到 (8|16|32|64) elemBits。
	NameAndSizeCheck *bool
	// 如果非空，gen_simdTypes.go 和 gen_intrinsics 中的所有生成将被跳过。
	NoTypes *string
	// 如果非空，gen_simdGenericOps 和 gen_simdrules 中的所有生成将被跳过。
	NoGenericOps *string
	// 如果非空，此字符串将附加到机器 SSA 操作名称。例如 "const"
	SSAVariant *string
	// 如果为 true，则不为掩码变体发出方法声明、通用操作或内置函数。
	// 但会发出特定于架构的操作码和优化。
	HideMaskMethods *bool
}

func (o *Operation) IsMasked() bool {
	if len(o.InVariant) == 0 {
		return false
	}
	if len(o.InVariant) == 1 && o.InVariant[0].Class == "mask" {
		return true
	}
	panic(fmt.Errorf("unknown inVariant"))
}

func (o *Operation) SkipMaskedMethod() bool {
	if o.HideMaskMethods == nil {
		return false
	}
	if *o.HideMaskMethods && o.IsMasked() {
		return true
	}
	return false
}

var reForName = regexp.MustCompile(`\bNAME\b`)

func (o *Operation) DecodeUnified(v *unify.Value) error {
	if err := v.Decode(&o.rawOperation); err != nil {
		return err
	}

	isMasked := o.IsMasked()

	// 计算完整的 Go 方法名。
	o.Go = o.rawOperation.Go
	if isMasked {
		o.Go += "Masked"
	}

	// 计算文档字符串。
	if o.rawOperation.Documentation != nil {
		o.Documentation = *o.rawOperation.Documentation
	} else {
		o.Documentation = "// UNDOCUMENTED"
	}
	o.Documentation = reForName.ReplaceAllString(o.Documentation, o.Go)
	if isMasked {
		o.Documentation += "\n//\n// This operation is applied selectively under a write mask."
		// 如果存在掩码，则抑制导出方法的通用操作和方法声明。
		if unicode.IsUpper([]rune(o.Go)[0]) {
			trueVal := "true"
			o.NoGenericOps = &trueVal
			o.NoTypes = &trueVal
		}
	}
	if o.rawOperation.AddDoc != nil {
		o.Documentation += "\n" + reForName.ReplaceAllString(*o.rawOperation.AddDoc, o.Go)
	}

	o.In = append(o.rawOperation.In, o.rawOperation.InVariant...)

	// 对于向下转换，如果结果有更多元素，则高位元素被置零。
	// TODO: 我们应该在 YAML 文件中编码这个逻辑，而不是在这里硬编码。
	if len(o.In) > 0 && len(o.Out) > 0 {
		inLanes := o.In[0].Lanes
		outLanes := o.Out[0].Lanes
		if inLanes != nil && outLanes != nil && *inLanes < *outLanes {
			if (strings.Contains(o.Go, "Saturate") || strings.Contains(o.Go, "Truncate")) &&
				!strings.Contains(o.Go, "Concat") {
				o.Documentation += "\n// Results are packed to low elements in the returned vector, its upper elements are zeroed."
			}
		}
	}

	return nil
}

func (o *Operation) VectorWidth() int {
	out := o.Out[0]
	if out.Class == "vreg" {
		return *out.Bits
	} else if out.Class == "greg" || out.Class == "mask" {
		for i := range o.In {
			if o.In[i].Class == "vreg" {
				return *o.In[i].Bits
			}
		}
	}
	panic(fmt.Errorf("Figure out what the vector width is for %v and implement it", *o))
}

// 目前 simdgen 将大多数指令的机器操作名称计算为 $Name$OutputSize，
// 按照这种表示法，这些指令是"重载的"。
// 例如：
// (Uint16x8) ConvertToInt8
// (Uint16x16) ConvertToInt8
// 都是 VPMOVWB128。
// 为了使它们可区分，我们还需要将输入大小附加到它们上。
// TODO: 在生成的代码中对它们进行良好的文档记录。
var demotingConvertOps = map[string]bool{
	"VPMOVQD128": true, "VPMOVSQD128": true, "VPMOVUSQD128": true, "VPMOVQW128": true, "VPMOVSQW128": true,
	"VPMOVUSQW128": true, "VPMOVDW128": true, "VPMOVSDW128": true, "VPMOVUSDW128": true, "VPMOVQB128": true,
	"VPMOVSQB128": true, "VPMOVUSQB128": true, "VPMOVDB128": true, "VPMOVSDB128": true, "VPMOVUSDB128": true,
	"VPMOVWB128": true, "VPMOVSWB128": true, "VPMOVUSWB128": true,
	"VPMOVQDMasked128": true, "VPMOVSQDMasked128": true, "VPMOVUSQDMasked128": true, "VPMOVQWMasked128": true, "VPMOVSQWMasked128": true,
	"VPMOVUSQWMasked128": true, "VPMOVDWMasked128": true, "VPMOVSDWMasked128": true, "VPMOVUSDWMasked128": true, "VPMOVQBMasked128": true,
	"VPMOVSQBMasked128": true, "VPMOVUSQBMasked128": true, "VPMOVDBMasked128": true, "VPMOVSDBMasked128": true, "VPMOVUSDBMasked128": true,
	"VPMOVWBMasked128": true, "VPMOVSWBMasked128": true, "VPMOVUSWBMasked128": true,
}

func machineOpName(maskType maskShape, gOp Operation) string {
	asm := gOp.Asm
	if maskType == OneMask {
		asm += "Masked"
	}
	asm = fmt.Sprintf("%s%d", asm, gOp.VectorWidth())
	if gOp.SSAVariant != nil {
		asm += *gOp.SSAVariant
	}
	if demotingConvertOps[asm] {
		// 还需要附加源的大小。
		// TODO: 应该是 "%sto%d"。
		asm = fmt.Sprintf("%s_%d", asm, *gOp.In[0].Bits)
	}
	return asm
}

func compareStringPointers(x, y *string) int {
	if x != nil && y != nil {
		return compareNatural(*x, *y)
	}
	if x == nil && y == nil {
		return 0
	}
	if x == nil {
		return -1
	}
	return 1
}

func compareIntPointers(x, y *int) int {
	if x != nil && y != nil {
		return *x - *y
	}
	if x == nil && y == nil {
		return 0
	}
	if x == nil {
		return -1
	}
	return 1
}

func compareOperations(x, y Operation) int {
	if c := compareNatural(x.Go, y.Go); c != 0 {
		return c
	}
	xIn, yIn := x.In, y.In

	if len(xIn) > len(yIn) && xIn[len(xIn)-1].Class == "mask" {
		xIn = xIn[:len(xIn)-1]
	} else if len(xIn) < len(yIn) && yIn[len(yIn)-1].Class == "mask" {
		yIn = yIn[:len(yIn)-1]
	}

	if len(xIn) < len(yIn) {
		return -1
	}
	if len(xIn) > len(yIn) {
		return 1
	}
	if len(x.Out) < len(y.Out) {
		return -1
	}
	if len(x.Out) > len(y.Out) {
		return 1
	}
	for i := range xIn {
		ox, oy := &xIn[i], &yIn[i]
		if c := compareOperands(ox, oy); c != 0 {
			return c
		}
	}
	return 0
}

func compareOperands(x, y *Operand) int {
	if c := compareNatural(x.Class, y.Class); c != 0 {
		return c
	}
	if x.Class == "immediate" {
		return compareStringPointers(x.ImmOffset, y.ImmOffset)
	} else {
		if c := compareStringPointers(x.Base, y.Base); c != 0 {
			return c
		}
		if c := compareIntPointers(x.ElemBits, y.ElemBits); c != 0 {
			return c
		}
		if c := compareIntPointers(x.Bits, y.Bits); c != 0 {
			return c
		}
		return 0
	}
}

type Operand struct {
	Class string // "mask"、"immediate"、"vreg"、"greg" 和 "mem" 之一

	Go     *string // 此操作数的 Go 类型
	AsmPos int     // 此操作数在汇编指令中的位置

	Base     *string // 基本 Go 类型（"int"、"uint"、"float"）
	ElemBits *int    // 元素位宽
	Bits     *int    // 总向量位宽

	Const *string // 立即数的可选常量值。
	// 可选的立即数参数偏移量。如果此字段非空，
	// 此操作数将是立即数操作数：
	// 编译器将用户传递的值右移 ImmOffset，并将其设置为操作的 AuxInt 字段。
	ImmOffset *string
	Name      *string // Go 内置函数声明中的可选名称
	Lanes     *int    // *Lanes 等于 Bits/ElemBits，除了标量，此时 *Lanes == 1
	// TreatLikeAScalarOfSize 意味着只使用向量的低 $TreatLikeAScalarOfSize 位，
	// 所以在 API 级别我们可以将其仅作为此大小的标量值；然后我们
	// 可以在内置函数阶段将其覆盖为正确大小的向量。
	TreatLikeAScalarOfSize *int
	// 如果非空，表示 [Class] 字段在此处被覆盖，目前用于
	// 将 AVX2 比较的结果覆盖为掩码。
	OverwriteClass *string
	// 如果非空，表示 [Base] 字段在此处被覆盖。此字段纯粹是因为
	// Intel 的 XED 数据不一致而存在。例如 VANDNP[SD] 将其操作数标记为 int。
	OverwriteBase *string
	// 如果非空，表示 [ElementBits] 字段被覆盖。此字段纯粹是因为
	// Intel 的 XED 数据不一致而存在。例如 AVX512 VPMADDUBSW 将其操作数
	// elemBits 标记为 16，实际应该是 8。
	OverwriteElementBits *int
	// FixedReg 是固定寄存器的名称
	FixedReg *string
}

// isDigit 如果字节是 ASCII 数字则返回 true。
func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}

// compareNatural 对两个字符串执行"自然排序"比较。
// 它按字典顺序比较非数字部分，按数值比较数字部分。
// 对于字符串不相等的"相等"字符串，如 "a01b" 和 "a1b"，
// strings.Compare 用于打破平局。
//
// 返回值：
//
//	如果 s1 < s2 返回 -1
//	如果 s1 == s2 返回 0
//	如果 s1 > s2 返回 +1
func compareNatural(s1, s2 string) int {
	i, j := 0, 0
	len1, len2 := len(s1), len(s2)

	for i < len1 && j < len2 {
		// 在两个字符串中查找非数字段或数字段。
		if isDigit(s1[i]) && isDigit(s2[j]) {
			// 数字段比较。
			numStart1 := i
			for i < len1 && isDigit(s1[i]) {
				i++
			}
			num1, _ := strconv.Atoi(s1[numStart1:i])

			numStart2 := j
			for j < len2 && isDigit(s2[j]) {
				j++
			}
			num2, _ := strconv.Atoi(s2[numStart2:j])

			if num1 < num2 {
				return -1
			}
			if num1 > num2 {
				return 1
			}
			// 如果数字相等，继续下一段。
		} else {
			// 非数字比较。
			if s1[i] < s2[j] {
				return -1
			}
			if s1[i] > s2[j] {
				return 1
			}
			i++
			j++
		}
	}

	// 处理 a01b 与 a1b 的情况；需要有一个顺序。
	return strings.Compare(s1, s2)
}

const generatedHeader = `// Code generated by 'simdgen -o godefs -goroot $GOROOT -xedPath $XED_PATH go.yaml types.yaml categories.yaml'; DO NOT EDIT.
`

func writeGoDefs(path string, cl unify.Closure) error {
	// TODO: 合并具有相同签名但有多个实现的操作（例如 SSE vs AVX）
	var ops []Operation
	for def := range cl.All() {
		var op Operation
		if !def.Exact() {
			continue
		}
		if err := def.Decode(&op); err != nil {
			log.Println(err.Error())
			log.Println(def)
			continue
		}
		// TODO: 验证这是否安全。
		op.sortOperand()
		op.adjustAsm()
		ops = append(ops, op)
	}
	slices.SortFunc(ops, compareOperations)
	// 解析的 XED 数据可能包含重复项，例如
	// 512 位 VPADDP。
	deduped := dedup(ops)
	slices.SortFunc(deduped, compareOperations)

	if *Verbose {
		log.Printf("dedup len: %d\n", len(ops))
	}
	var err error
	if err = overwrite(deduped); err != nil {
		return err
	}
	if *Verbose {
		log.Printf("dedup len: %d\n", len(deduped))
	}
	if !*FlagNoDedup {
		// TODO: 这可能会隐藏 API 定义中的错误，特别是当
		// 多个模式无意中导致相同的 API 时。使其更严格。
		if deduped, err = dedupGodef(deduped); err != nil {
			return err
		}
	}
	if *Verbose {
		log.Printf("dedup len: %d\n", len(deduped))
	}
	if !*FlagNoConstImmPorting {
		if err = copyConstImm(deduped); err != nil {
			return err
		}
	}
	if *Verbose {
		log.Printf("dedup len: %d\n", len(deduped))
	}
	reportXEDInconsistency(deduped)
	typeMap := parseSIMDTypes(deduped)

	formatWriteAndClose(writeSIMDTypes(typeMap), path, "src/"+simdPackage+"/types_amd64.go")
	formatWriteAndClose(writeSIMDFeatures(deduped), path, "src/"+simdPackage+"/cpu.go")
	f, fI := writeSIMDStubs(deduped, typeMap)
	formatWriteAndClose(f, path, "src/"+simdPackage+"/ops_amd64.go")
	formatWriteAndClose(fI, path, "src/"+simdPackage+"/ops_internal_amd64.go")
	formatWriteAndClose(writeSIMDIntrinsics(deduped, typeMap), path, "src/cmd/compile/internal/ssagen/simdintrinsics.go")
	formatWriteAndClose(writeSIMDGenericOps(deduped), path, "src/cmd/compile/internal/ssa/_gen/simdgenericOps.go")
	formatWriteAndClose(writeSIMDMachineOps(deduped), path, "src/cmd/compile/internal/ssa/_gen/simdAMD64ops.go")
	formatWriteAndClose(writeSIMDSSA(deduped), path, "src/cmd/compile/internal/amd64/simdssa.go")
	writeAndClose(writeSIMDRules(deduped).Bytes(), path, "src/cmd/compile/internal/ssa/_gen/simdAMD64.rules")

	return nil
}
