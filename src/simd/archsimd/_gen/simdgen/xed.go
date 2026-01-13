// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"cmp"
	"fmt"
	"log"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"simd/archsimd/_gen/unify"

	"golang.org/x/arch/x86/xeddata"
	"gopkg.in/yaml.v3"
)

const (
	NOT_REG_CLASS = iota // 不是寄存器
	VREG_CLASS           // 分类为向量寄存器
	GREG_CLASS           // 分类为通用寄存器
)

// instVariant 是一个位图，表示具有可选参数的指令的变体。
type instVariant uint8

const (
	instVariantNone instVariant = 0

	// instVariantMasked 表示这是可选掩码指令的带掩码变体。
	instVariantMasked instVariant = 1 << iota
)

var operandRemarks int

// TODO: 文档。返回具有 Def 域的 Values。
func loadXED(xedPath string) []*unify.Value {
	// TODO: 显然还有很多工作要做。

	db, err := xeddata.NewDatabase(xedPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	var defs []*unify.Value
	type opData struct {
		inst *xeddata.Inst
		ops  []operand
		mem  string
	}
	// 从操作码到 opdata 的映射。
	memOps := make(map[string][]opData, 0)
	otherOps := make(map[string][]opData, 0)
	appendDefs := func(inst *xeddata.Inst, ops []operand, addFields map[string]string) {
		applyQuirks(inst, ops)

		defsPos := len(defs)
		defs = append(defs, instToUVal(inst, ops, addFields)...)

		if *flagDebugXED {
			for i := defsPos; i < len(defs); i++ {
				y, _ := yaml.Marshal(defs[i])
				fmt.Printf("==>\n%s\n", y)
			}
		}
	}
	err = xeddata.WalkInsts(xedPath, func(inst *xeddata.Inst) {
		inst.Pattern = xeddata.ExpandStates(db, inst.Pattern)

		switch {
		case inst.RealOpcode == "N":
			return // 跳过不稳定的指令
		case !(strings.HasPrefix(inst.Extension, "AVX") || strings.HasPrefix(inst.Extension, "SHA")):
			// 我们只对 AVX 和 SHA 指令感兴趣。
			return
		}

		if *flagDebugXED {
			fmt.Printf("%s:\n%+v\n", inst.Pos, inst)
		}

		ops, err := decodeOperands(db, strings.Fields(inst.Operands))
		if err != nil {
			operandRemarks++
			if *Verbose {
				log.Printf("%s: [%s] %s", inst.Pos, inst.Opcode(), err)
			}
			return
		}
		var data map[string][]opData
		mem := checkMem(ops)
		if mem == "vbcst" {
			// 可能存在纯 vreg 变体，稍后看看是否可以合并它们
			data = memOps
		} else {
			data = otherOps
		}
		opcode := inst.Opcode()
		if _, ok := data[opcode]; !ok {
			s := make([]opData, 1)
			s[0] = opData{inst, ops, mem}
			data[opcode] = s
		} else {
			data[opcode] = append(data[opcode], opData{inst, ops, mem})
		}
	})
	for _, s := range otherOps {
		for _, o := range s {
			addFields := map[string]string{}
			if o.mem == "noMem" {
				opcode := o.inst.Opcode()
				// 检查此操作是否存在 vbcst 变体
				// 首先检查操作码
				// 保持此逻辑与 [decodeOperands] 同步
				if ms, ok := memOps[opcode]; ok {
					feat1, ok1 := decodeCPUFeature(o.inst)
					// 然后检查是否存在这样的操作：对于所有 vreg
					// 形状，它们在相同索引处是相同的
					var feat1Match, feat2Match string
					matchIdx := -1
					var featMismatchCnt int
				outer:
					for i, m := range ms {
						// 它们的 CPU 特性应该首先匹配
						var featMismatch bool
						feat2, ok2 := decodeCPUFeature(m.inst)
						if !ok1 || !ok2 {
							continue
						}
						if feat1 != feat2 {
							featMismatch = true
							featMismatchCnt++
						}
						if len(o.ops) == len(m.ops) {
							for j := range o.ops {
								if reflect.TypeOf(o.ops[j]) == reflect.TypeOf(m.ops[j]) {
									v1, ok3 := o.ops[j].(operandVReg)
									v2, _ := m.ops[j].(operandVReg)
									if !ok3 {
										continue
									}
									if v1.vecShape != v2.vecShape {
										// 不匹配，跳过此 memOp
										continue outer
									}
								} else {
									_, ok3 := o.ops[j].(operandVReg)
									_, ok4 := m.ops[j].(operandMem)
									// 唯一的区别必须是 vreg 和 mem，没有其他情况。
									if !ok3 || !ok4 {
										// 不匹配，跳过此 memOp
										continue outer
									}
								}
							}
							// 找到匹配，提前退出
							matchIdx = i
							feat1Match = feat1
							feat2Match = feat2
							if featMismatchCnt > 1 {
								panic("检测到多个特性不匹配的 vbcst memops，simdgen 无法区分")
							}
							if !featMismatch {
								// 特性不匹配是可以的，但应优先考虑匹配的情况。
								break
							}
						}
					}
					// 从 memOps 中移除匹配项，它现在已合并到此纯 vreg 操作中
					if matchIdx != -1 {
						memOps[opcode] = append(memOps[opcode][:matchIdx], memOps[opcode][matchIdx+1:]...)
						// 通过添加新字段来完成合并
						// 目前我们只有 vbcst
						addFields["memFeatures"] = "vbcst"
						if feat1Match != feat2Match {
							addFields["memFeaturesData"] = fmt.Sprintf("feat1=%s;feat2=%s", feat1Match, feat2Match)
						}
					}
				}
			}
			appendDefs(o.inst, o.ops, addFields)
		}
	}
	for _, ms := range memOps {
		for _, m := range ms {
			if *Verbose {
				log.Printf("mem op not merged: %s, %v\n", m.inst.Opcode(), m)
			}
			appendDefs(m.inst, m.ops, nil)
		}
	}
	if err != nil {
		log.Fatalf("walk insts: %v", err)
	}

	if len(unknownFeatures) > 0 {
		if !*Verbose {
			nInst := 0
			for _, insts := range unknownFeatures {
				nInst += len(insts)
			}
			log.Printf("%d unhandled CPU features for %d instructions (use -v for details)", len(unknownFeatures), nInst)
		} else {
			keys := slices.SortedFunc(maps.Keys(unknownFeatures), func(a, b cpuFeatureKey) int {
				return cmp.Or(cmp.Compare(a.Extension, b.Extension),
					cmp.Compare(a.ISASet, b.ISASet))
			})
			for _, key := range keys {
				if key.ISASet == "" || key.ISASet == key.Extension {
					log.Printf("unhandled Extension %s", key.Extension)
				} else {
					log.Printf("unhandled Extension %s and ISASet %s", key.Extension, key.ISASet)
				}
				log.Printf("  opcodes: %s", slices.Sorted(maps.Keys(unknownFeatures[key])))
			}
		}
	}

	return defs
}

var (
	maskRequiredRe = regexp.MustCompile(`VPCOMPRESS[BWDQ]|VCOMPRESSP[SD]|VPEXPAND[BWDQ]|VEXPANDP[SD]`)
	maskOptionalRe = regexp.MustCompile(`VPCMP(EQ|GT|U)?[BWDQ]|VCMPP[SD]`)
)

func applyQuirks(inst *xeddata.Inst, ops []operand) {
	opc := inst.Opcode()
	switch {
	case maskRequiredRe.MatchString(opc):
		// 这些指令上的掩码被标记为可选的，但没有掩码该指令就没有意义。
		for i, op := range ops {
			if op, ok := op.(operandMask); ok {
				op.optional = false
				ops[i] = op
			}
		}

	case maskOptionalRe.MatchString(opc):
		// 相反，这些掩码应该被标记为可选的但实际上没有。
		for i, op := range ops {
			if op, ok := op.(operandMask); ok && op.action.r {
				op.optional = true
				ops[i] = op
			}
		}
	}
}

type operandCommon struct {
	action operandAction
}

// operandAction 定义此操作数是读取和/或写入。
//
// TODO: 这应该放在 [xeddata.Operand] 中吗？
type operandAction struct {
	r  bool // 读取
	w  bool // 写入
	cr bool // 读取是条件性的（意味着 r==true）
	cw bool // 写入是条件性的（意味着 w==true）
}

type operandMem struct {
	operandCommon
	vecShape
	elemBaseType scalarBaseType
	// 以下字段不会刷新到最终输出
	// 支持全向量广播；意味着操作数在宽度中指定了 "vv"（向量向量）类型，
	// 并且指令具有 TXT=BCASTSTR 属性。
	vbcst   bool
	unknown bool // 未知类型
}

type vecShape struct {
	elemBits  int    // 元素大小（位）
	bits      int    // 寄存器宽度（位）（总向量位数）
	fixedName string // 固定寄存器名称
}

type operandVReg struct { // 向量寄存器
	operandCommon
	vecShape
	elemBaseType scalarBaseType
}

type operandGReg struct { // 通用寄存器
	operandCommon
	vecShape
	elemBaseType scalarBaseType
}

// operandMask 是向量掩码。
//
// 无论实际掩码表示如何，此操作数的 [vecShape] 都对应于"逐位"类型的掩码。
// 即，elemBits 给出每个掩码元素覆盖的元素宽度，而 bits/elemBits 给出掩码
// 元素的总数。（bits 给出总位数，就好像这是一个逐位掩码，它本身可能没有意义。）
type operandMask struct {
	operandCommon
	vecShape
	// 掩码中的位数是 w/bits。

	allMasks bool // 如果设置，则无法推断大小，因为所有操作数都是掩码。

	// 掩码可以省略，在这种情况下默认为 K0/"无掩码"
	optional bool
}

type operandImm struct {
	operandCommon
	bits int // 立即数大小（位）
}

type operand interface {
	common() operandCommon
	addToDef(b *unify.DefBuilder)
}

func strVal(s any) *unify.Value {
	return unify.NewValue(unify.NewStringExact(fmt.Sprint(s)))
}

func (o operandCommon) common() operandCommon {
	return o
}

func (o operandMem) addToDef(b *unify.DefBuilder) {
	b.Add("class", strVal("memory"))
	if o.unknown {
		return
	}
	baseDomain, err := unify.NewStringRegex(o.elemBaseType.regex())
	if err != nil {
		panic("parsing baseRe: " + err.Error())
	}
	b.Add("base", unify.NewValue(baseDomain))
	b.Add("bits", strVal(o.bits))
	if o.elemBits != o.bits {
		b.Add("elemBits", strVal(o.elemBits))
	}
}

func (o operandVReg) addToDef(b *unify.DefBuilder) {
	baseDomain, err := unify.NewStringRegex(o.elemBaseType.regex())
	if err != nil {
		panic("parsing baseRe: " + err.Error())
	}
	b.Add("class", strVal("vreg"))
	b.Add("bits", strVal(o.bits))
	b.Add("base", unify.NewValue(baseDomain))
	// 如果 elemBits == bits，则向量可以是任何形状。例如，逻辑操作就是这种情况。
	if o.elemBits != o.bits {
		b.Add("elemBits", strVal(o.elemBits))
	}
	if o.fixedName != "" {
		b.Add("fixedReg", strVal(o.fixedName))
	}
}

func (o operandGReg) addToDef(b *unify.DefBuilder) {
	baseDomain, err := unify.NewStringRegex(o.elemBaseType.regex())
	if err != nil {
		panic("parsing baseRe: " + err.Error())
	}
	b.Add("class", strVal("greg"))
	b.Add("bits", strVal(o.bits))
	b.Add("base", unify.NewValue(baseDomain))
	if o.elemBits != o.bits {
		b.Add("elemBits", strVal(o.elemBits))
	}
	if o.fixedName != "" {
		b.Add("fixedReg", strVal(o.fixedName))
	}
}

func (o operandMask) addToDef(b *unify.DefBuilder) {
	b.Add("class", strVal("mask"))
	if o.allMasks {
		// 如果所有操作数都是掩码，则省略大小并让统一来确定掩码大小。
		return
	}
	b.Add("elemBits", strVal(o.elemBits))
	b.Add("bits", strVal(o.bits))
	if o.fixedName != "" {
		b.Add("fixedReg", strVal(o.fixedName))
	}
}

func (o operandImm) addToDef(b *unify.DefBuilder) {
	b.Add("class", strVal("immediate"))
	b.Add("bits", strVal(o.bits))
}

var actionEncoding = map[string]operandAction{
	"r":   {r: true},
	"cr":  {r: true, cr: true},
	"w":   {w: true},
	"cw":  {w: true, cw: true},
	"rw":  {r: true, w: true},
	"crw": {r: true, w: true, cr: true},
	"rcw": {r: true, w: true, cw: true},
}

func decodeOperand(db *xeddata.Database, operand string) (operand, error) {
	op, err := xeddata.NewOperand(db, operand)
	if err != nil {
		log.Fatalf("parsing operand %q: %v", operand, err)
	}
	if *flagDebugXED {
		fmt.Printf("  %+v\n", op)
	}

	if strings.HasPrefix(op.Name, "EMX_BROADCAST") {
		// 这指的是在 all-state.txt 中定义的一组宏，它们将 BCAST 操作数
		// 设置为各种固定值。但 BCAST 操作数本身是被抑制的和"内部的"，
		// 所以我认为我们可以忽略此操作数。
		return nil, nil
	}

	// TODO: 参见 xed_decoded_inst_operand_action。这可能需要更复杂。
	action, ok := actionEncoding[op.Action]
	if !ok {
		return nil, fmt.Errorf("unknown action %q", op.Action)
	}
	common := operandCommon{action: action}

	lhs := op.NameLHS()
	if strings.HasPrefix(lhs, "MEM") {
		// 看起来 XED 数据在 VPADDD 上有不一致，标记属性
		// VPBROADCASTD 而不是规范的 BCASTSTR。
		if op.Width == "vv" && (op.Attributes["TXT=BCASTSTR"] ||
			op.Attributes["TXT=VPBROADCASTD"]) {
			baseType, elemBits, ok := decodeType(op)
			if !ok {
				return nil, fmt.Errorf("failed to decode memory width %q", operand)
			}
			// 此操作数有两种可能的宽度（[bits]）：
			// 1. 与其他操作数相同
			// 2. 其他操作数的元素宽度（广播）
			// 默认为 2，稍后我们将在操作中设置一个新字段来指示此双宽度属性。
			shape := vecShape{elemBits: elemBits, bits: elemBits}
			return operandMem{
				operandCommon: common,
				vecShape:      shape,
				elemBaseType:  baseType,
				vbcst:         true,
				unknown:       false,
			}, nil
		}
		// TODO: 更好地解析 op.Width 以处理所有情况
		// 目前这至少会错过 VPBROADCAST。
		return operandMem{
			operandCommon: common,
			unknown:       true,
		}, nil
	} else if strings.HasPrefix(lhs, "REG") {
		if op.Width == "mskw" {
			// 掩码操作数不指定宽度。我们必须推断它。
			//
			// XED 使用标记 ZEROSTR 来表示掩码操作数是可选的，
			// 如果省略，则意味着 K0，即"无掩码"。
			return operandMask{
				operandCommon: common,
				optional:      op.Attributes["TXT=ZEROSTR"],
			}, nil
		} else {
			class, regBits, fixedReg := decodeReg(op)
			if class == NOT_REG_CLASS {
				return nil, fmt.Errorf("failed to decode register %q", operand)
			}
			baseType, elemBits, ok := decodeType(op)
			if !ok {
				return nil, fmt.Errorf("failed to decode register width %q", operand)
			}
			shape := vecShape{elemBits: elemBits, bits: regBits, fixedName: fixedReg}
			if class == VREG_CLASS {
				return operandVReg{
					operandCommon: common,
					vecShape:      shape,
					elemBaseType:  baseType,
				}, nil
			}
			// 通用寄存器
			m := min(shape.bits, shape.elemBits)
			shape.bits, shape.elemBits = m, m
			return operandGReg{
				operandCommon: common,
				vecShape:      shape,
				elemBaseType:  baseType,
			}, nil

		}
	} else if strings.HasPrefix(lhs, "IMM") {
		_, bits, ok := decodeType(op)
		if !ok {
			return nil, fmt.Errorf("failed to decode register width %q", operand)
		}
		return operandImm{
			operandCommon: common,
			bits:          bits,
		}, nil
	}

	// TODO: BASE 和 SEG
	return nil, fmt.Errorf("unknown operand LHS %q in %q", lhs, operand)
}

func decodeOperands(db *xeddata.Database, operands []string) (ops []operand, err error) {
	// 解码 XED 操作数描述。
	for _, o := range operands {
		op, err := decodeOperand(db, o)
		if err != nil {
			return nil, err
		}
		if op != nil {
			ops = append(ops, op)
		}
	}

	// XED 不编码掩码操作数的大小。如果有掩码操作数，
	// 尝试从其他操作数推断它们的大小。
	if err := inferMaskSizes(ops); err != nil {
		return nil, fmt.Errorf("%w in operands %+v", err, operands)
	}

	return ops, nil
}

func inferMaskSizes(ops []operand) error {
	// 这是一个启发式方法，在某些情况下会失败：
	//
	// - 像 KAND[BWDQ] 这样的掩码操作在 XED 中*没有*任何东西来指示掩码大小。
	//
	// - VINSERT*、VPSLL*、VPSRA* 和 VPSRL* 以及其他一些自然具有混合输入大小，
	//   XED 不指示掩码适用于哪些操作数。
	//
	// - VPDP* 和 VP4DP* 具有非常复杂的混合操作数模式。
	//
	// 我认为对于这些，我们可能只需要手写一个表来说明每个掩码适用于哪些操作数。
	inferMask := func(r, w bool) error {
		var masks []int
		var rSizes, wSizes, sizes []vecShape
		allMasks := true
		hasWMask := false
		for i, op := range ops {
			action := op.common().action
			if _, ok := op.(operandMask); ok {
				if action.r && action.w {
					return fmt.Errorf("unexpected rw mask")
				}
				if action.r == r || action.w == w {
					masks = append(masks, i)
				}
				if action.w {
					hasWMask = true
				}
			} else {
				allMasks = false
				if reg, ok := op.(operandVReg); ok {
					if action.r {
						rSizes = append(rSizes, reg.vecShape)
					}
					if action.w {
						wSizes = append(wSizes, reg.vecShape)
					}
				}
			}
		}
		if len(masks) == 0 {
			return nil
		}

		if r {
			sizes = rSizes
			if len(sizes) == 0 {
				sizes = wSizes
			}
		}
		if w {
			sizes = wSizes
			if len(sizes) == 0 {
				sizes = rSizes
			}
		}

		if len(sizes) == 0 {
			// 如果所有操作数都是掩码，则将掩码推断留给用户。
			if allMasks {
				for _, i := range masks {
					m := ops[i].(operandMask)
					m.allMasks = true
					ops[i] = m
				}
				return nil
			}
			return fmt.Errorf("cannot infer mask size: no register operands")
		}
		shape, ok := singular(sizes)
		if !ok {
			if !hasWMask && len(wSizes) == 1 && len(masks) == 1 {
				// 此模式看起来像谓词掩码，所以它的形状应该与输出对齐。
				// TODO: 验证这是一个安全的假设。
				shape = wSizes[0]
			} else {
				return fmt.Errorf("cannot infer mask size: multiple register sizes %v", sizes)
			}
		}
		for _, i := range masks {
			m := ops[i].(operandMask)
			m.vecShape = shape
			ops[i] = m
		}
		return nil
	}
	if err := inferMask(true, false); err != nil {
		return err
	}
	if err := inferMask(false, true); err != nil {
		return err
	}
	return nil
}

// addOperandsToDef 将 "in"、"inVariant" 和 "out" 添加到指令 Def。
//
// 如果 variant&instVariantMasked，则可选掩码输入操作数被添加到 inVariant 字段，
// 否则省略。
func addOperandsToDef(ops []operand, instDB *unify.DefBuilder, variant instVariant) {
	var inVals, inVar, outVals []*unify.Value
	asmPos := 0
	for _, op := range ops {
		var db unify.DefBuilder
		op.addToDef(&db)
		db.Add("asmPos", unify.NewValue(unify.NewStringExact(fmt.Sprint(asmPos))))

		action := op.common().action
		asmCount := 1 // 汇编操作数数量；0 或 1
		if action.r {
			inVal := unify.NewValue(db.Build())
			// 如果这是一个可选掩码，将其放入输入变体元组中。
			if mask, ok := op.(operandMask); ok && mask.optional {
				if variant&instVariantMasked != 0 {
					inVar = append(inVar, inVal)
				} else {
					// 此操作数根本不会出现在汇编中。
					asmCount = 0
				}
			} else {
				// 只是一个普通的输入操作数。
				inVals = append(inVals, inVal)
			}
		}
		if action.w {
			outVal := unify.NewValue(db.Build())
			outVals = append(outVals, outVal)
		}

		asmPos += asmCount
	}

	instDB.Add("in", unify.NewValue(unify.NewTuple(inVals...)))
	instDB.Add("inVariant", unify.NewValue(unify.NewTuple(inVar...)))
	instDB.Add("out", unify.NewValue(unify.NewTuple(outVals...)))
	memFeatures := checkMem(ops)
	if memFeatures != "noMem" {
		instDB.Add("memFeatures", unify.NewValue(unify.NewStringExact(memFeatures)))
	}
}

// checkMem 检查操作中内存操作数的形状并返回该形状。
// 保持此函数与 [decodeOperand] 同步。
func checkMem(ops []operand) string {
	memState := "noMem"
	var mem *operandMem
	memCnt := 0
	for _, op := range ops {
		if m, ok := op.(operandMem); ok {
			mem = &m
			memCnt++
		}
	}
	if mem != nil {
		if mem.unknown {
			memState = "unknown"
		} else if memCnt > 1 {
			memState = "tooManyMem"
		} else {
			// 目前我们只有 vbcst 情况。
			// 此形状表示 [bits] 字段有两个可能的值：
			// 1. 元素广播宽度，即其对等 vreg 操作数的 [elemBits]（解析的 XED 数据中的默认值）
			// 2. 完整向量宽度，即其对等 vreg 操作数的 [bits]（godefs 应该知道这一点）
			memState = "vbcst"
		}
	}
	return memState
}

func instToUVal(inst *xeddata.Inst, ops []operand, addFields map[string]string) []*unify.Value {
	feature, ok := decodeCPUFeature(inst)
	if !ok {
		return nil
	}

	var vals []*unify.Value
	vals = append(vals, instToUVal1(inst, ops, feature, instVariantNone, addFields))
	if hasOptionalMask(ops) {
		vals = append(vals, instToUVal1(inst, ops, feature, instVariantMasked, addFields))
	}
	return vals
}

func instToUVal1(inst *xeddata.Inst, ops []operand, feature string, variant instVariant, addFields map[string]string) *unify.Value {
	var db unify.DefBuilder
	db.Add("goarch", unify.NewValue(unify.NewStringExact("amd64")))
	db.Add("asm", unify.NewValue(unify.NewStringExact(inst.Opcode())))
	addOperandsToDef(ops, &db, variant)
	db.Add("cpuFeature", unify.NewValue(unify.NewStringExact(feature)))
	for k, v := range addFields {
		db.Add(k, unify.NewValue(unify.NewStringExact(v)))
	}

	if strings.Contains(inst.Pattern, "ZEROING=0") {
		// 这是一条 EVEX 指令，但 ".Z"（零合并）指令标志无效。
		// EVEX.z 必须为零。
		//
		// 这可能意味着几件事：
		//
		// - 指令的输出是掩码，所以合并模式没有意义。例如 VCMPPS。
		//
		// - 任何地方都没有涉及掩码。（也许在这种情况下也设置了 MASK=0？）
		//   例如 VINSERTPS。
		//
		// - 操作本身执行合并。例如带 mem 操作数的 VCOMPRESSPS。
		//
		// 可能还有其他原因。
		db.Add("zeroing", unify.NewValue(unify.NewStringExact("false")))
	}
	pos := unify.Pos{Path: inst.Pos.Path, Line: inst.Pos.Line}
	return unify.NewValuePos(db.Build(), pos)
}

// decodeCPUFeature 返回 inst 所需的 CPU 特性名称。这些与 simd 包中的
// "Has*" 特性检查的名称匹配。
func decodeCPUFeature(inst *xeddata.Inst) (string, bool) {
	key := cpuFeatureKey{
		Extension: inst.Extension,
		ISASet:    isaSetStrip.ReplaceAllLiteralString(inst.ISASet, ""),
	}
	feat, ok := cpuFeatureMap[key]
	if !ok {
		imap := unknownFeatures[key]
		if imap == nil {
			imap = make(map[string]struct{})
			unknownFeatures[key] = imap
		}
		imap[inst.Opcode()] = struct{}{}
		return "", false
	}
	if feat == "ignore" {
		return "", false
	}
	return feat, true
}

var isaSetStrip = regexp.MustCompile("_(128N?|256N?|512)$")

type cpuFeatureKey struct {
	Extension, ISASet string
}

// cpuFeatureMap 从 XED 的 "EXTENSION" 和 "ISA_SET" 映射到可在 SIMD API 中使用的
// CPU 特性名称。
var cpuFeatureMap = map[cpuFeatureKey]string{
	{"SHA", "SHA"}: "SHA",

	{"AVX", ""}:              "AVX",
	{"AVX_VNNI", "AVX_VNNI"}: "AVXVNNI",
	{"AVX2", ""}:             "AVX2",
	{"AVXAES", ""}:           "AVX, AES",

	// AVX-512 基础特性。我们将所有这些组合成一个 "AVX512" 特性。
	{"AVX512EVEX", "AVX512F"}:  "AVX512",
	{"AVX512EVEX", "AVX512CD"}: "AVX512",
	{"AVX512EVEX", "AVX512BW"}: "AVX512",
	{"AVX512EVEX", "AVX512DQ"}: "AVX512",
	// AVX512VL 不会显式出现在 ISASet 中。我猜它是由向量长度后缀暗示的。

	// AVX-512 扩展特性
	{"AVX512EVEX", "AVX512_BITALG"}:     "AVX512BITALG",
	{"AVX512EVEX", "AVX512_GFNI"}:       "AVX512GFNI",
	{"AVX512EVEX", "AVX512_VBMI2"}:      "AVX512VBMI2",
	{"AVX512EVEX", "AVX512_VBMI"}:       "AVX512VBMI",
	{"AVX512EVEX", "AVX512_VNNI"}:       "AVX512VNNI",
	{"AVX512EVEX", "AVX512_VPOPCNTDQ"}:  "AVX512VPOPCNTDQ",
	{"AVX512EVEX", "AVX512_VAES"}:       "AVX512VAES",
	{"AVX512EVEX", "AVX512_VPCLMULQDQ"}: "AVX512VPCLMULQDQ",

	// AVX 10.2（尚未支持）
	{"AVX512EVEX", "AVX10_2_RC"}: "ignore",
}

var unknownFeatures = map[cpuFeatureKey]map[string]struct{}{}

// hasOptionalMask 返回 ops 中是否有可选掩码操作数。
func hasOptionalMask(ops []operand) bool {
	for _, op := range ops {
		if op, ok := op.(operandMask); ok && op.optional {
			return true
		}
	}
	return false
}

func singular[T comparable](xs []T) (T, bool) {
	if len(xs) == 0 {
		return *new(T), false
	}
	for _, x := range xs[1:] {
		if x != xs[0] {
			return *new(T), false
		}
	}
	return xs[0], true
}

type fixedReg struct {
	class int
	name  string
	width int
}

var fixedRegMap = map[string]fixedReg{
	"XED_REG_XMM0": {VREG_CLASS, "x0", 128},
}

// decodeReg 返回类别（NOT_REG_CLASS、VREG_CLASS、GREG_CLASS、VREG_CLASS_FIXED、
// GREG_CLASS_FIXED）、位宽和寄存器名称（如果是固定的）。
// 如果操作数不能确定为寄存器，则类别为 NOT_REG_CLASS。
func decodeReg(op *xeddata.Operand) (class, width int, name string) {
	// op.Width 告诉我们总宽度，例如：
	//
	//    dq => 128 位（XMM）
	//    qq => 256 位（YMM）
	//    mskw => K
	//    z[iuf?](8|16|32|...) => 512 位（ZMM）
	//
	// 但编码确实很奇怪，不清楚这些是否*总是*意味着 XMM/YMM/ZMM，
	// 或者其他不规则的东西是否可以使用这些大宽度。
	// 因此，我们深入研究寄存器集本身。

	if !strings.HasPrefix(op.NameLHS(), "REG") {
		return NOT_REG_CLASS, 0, ""
	}
	// TODO: 我们不应该依赖宏命名约定。我们应该使用 all-dec-patterns.txt，
	// 但 xeddata 目前不支持该表。
	rhs := op.NameRHS()
	if !strings.HasSuffix(rhs, "()") {
		if fixedReg, ok := fixedRegMap[rhs]; ok {
			return fixedReg.class, fixedReg.width, fixedReg.name
		}
		return NOT_REG_CLASS, 0, ""
	}
	switch {
	case strings.HasPrefix(rhs, "XMM_"):
		return VREG_CLASS, 128, ""
	case strings.HasPrefix(rhs, "YMM_"):
		return VREG_CLASS, 256, ""
	case strings.HasPrefix(rhs, "ZMM_"):
		return VREG_CLASS, 512, ""
	case strings.HasPrefix(rhs, "GPR64_"), strings.HasPrefix(rhs, "VGPR64_"):
		return GREG_CLASS, 64, ""
	case strings.HasPrefix(rhs, "GPR32_"), strings.HasPrefix(rhs, "VGPR32_"):
		return GREG_CLASS, 32, ""
	}
	return NOT_REG_CLASS, 0, ""
}

var xtypeRe = regexp.MustCompile(`^([iuf])([0-9]+)$`)

// scalarBaseType 描述标量元素的基本类型。这是一个 Go 类型，
// 但没有位宽后缀（scalarBaseIntOrUint 除外）。
type scalarBaseType int

const (
	scalarBaseInt scalarBaseType = iota
	scalarBaseUint
	scalarBaseIntOrUint // 有符号或无符号未指定
	scalarBaseFloat
	scalarBaseComplex
	scalarBaseBFloat
	scalarBaseHFloat
)

func (s scalarBaseType) regex() string {
	switch s {
	case scalarBaseInt:
		return "int"
	case scalarBaseUint:
		return "uint"
	case scalarBaseIntOrUint:
		return "int|uint"
	case scalarBaseFloat:
		return "float"
	case scalarBaseComplex:
		return "complex"
	case scalarBaseBFloat:
		return "BFloat"
	case scalarBaseHFloat:
		return "HFloat"
	}
	panic(fmt.Sprintf("unknown scalar base type %d", s))
}

func decodeType(op *xeddata.Operand) (base scalarBaseType, bits int, ok bool) {
	// xtype 告诉你元素类型。i8、i16、i32、i64、f32 等。
	//
	// TODO: 像 AVX2 VPAND 这样的东西有一个 u256 的 xtype，因为它们与
	// 元素宽度无关。我是将其映射到所有宽度，还是只省略元素宽度并让
	// 统一来充实它？没有 u512（大概那些都是掩码的，所以元素宽度很重要）。
	// 这些都是 Category: LOGICAL，所以也许我们可以使用该信息？

	// 处理一些奇怪的情况。
	switch op.Xtype {
	// 由 Open Compute Project "OCP 8-bit Floating Point Specification (OFP8)"
	// 定义的 8 位浮点格式。
	case "bf8": // E5M2 浮点
		return scalarBaseBFloat, 8, true
	case "hf8": // E4M3 浮点
		return scalarBaseHFloat, 8, true
	case "bf16": // bfloat16 浮点
		return scalarBaseBFloat, 16, true
	case "2f16":
		// 由 2 个 float16 组成的复数。在 Go 中不存在，但我们可以说明它会是什么。
		return scalarBaseComplex, 32, true
	case "2i8", "2I8":
		// 这些只使用每个 16 位字段中的低 INT8。
		// 据我所知，"2I8" 是一个拼写错误。
		return scalarBaseInt, 8, true
	case "2u16", "2U16":
		// 一些 VPDP* 有它
		// TODO: "z" 是否意味着它有零化？
		return scalarBaseUint, 16, true
	case "2i16", "2I16":
		// 一些 VPDP* 有它
		return scalarBaseInt, 16, true
	case "4u8", "4U8":
		// 一些 VPDP* 有它
		return scalarBaseUint, 8, true
	case "4i8", "4I8":
		// 一些 VPDP* 有它
		return scalarBaseInt, 8, true
	}

	// 其余的遵循简单的模式。
	m := xtypeRe.FindStringSubmatch(op.Xtype)
	if m == nil {
		// TODO: 报告无法识别的 xtype
		return 0, 0, false
	}
	bits, _ = strconv.Atoi(m[2])
	switch m[1] {
	case "i", "u":
		// XED 对于什么是有符号、无符号或无关紧要的情况相当不一致，
		// 所以将它们合并在一起，让 Go 定义适当地缩小范围。
		// 也许有更好的方法来做到这一点。
		return scalarBaseIntOrUint, bits, true
	case "f":
		return scalarBaseFloat, bits, true
	default:
		panic("unreachable")
	}
}
