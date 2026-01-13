// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"
)

type tplRuleData struct {
	tplName        string // 例如 "sftimm"
	GoOp           string // 例如 "ShiftAllLeft"
	GoType         string // 例如 "Uint32x8"
	Args           string // 例如 "x y"
	Asm            string // 例如 "VPSLLD256"
	ArgsOut        string // 例如 "x y"
	MaskInConvert  string // 例如 "VPMOVVec32x8ToM"
	MaskOutConvert string // 例如 "VPMOVMToVec32x8"
	ElementSize    int    // 例如 32
	Size           int    // 例如 128
	ArgsLoadAddr   string // [Args] 的最后一个 vreg 参数是具体的 "(VMOVDQUload* ptr mem)"，可能包含掩码。
	ArgsAddr       string // [Args] 的最后一个 vreg 参数被替换为 "ptr"，可能包含掩码，末尾有 "mem"。
	FeatCheck      string // 例如 "v.Block.CPUfeatures.hasFeature(CPUavx512)" -- 用于 ssa/_gen 规则文件。
}

var (
	ruleTemplates = template.Must(template.New("simdRules").Parse(`
{{define "pureVreg"}}({{.GoOp}}{{.GoType}} {{.Args}}) => ({{.Asm}} {{.ArgsOut}})
{{end}}
{{define "maskIn"}}({{.GoOp}}{{.GoType}} {{.Args}} mask) => ({{.Asm}} {{.ArgsOut}} ({{.MaskInConvert}} <types.TypeMask> mask))
{{end}}
{{define "maskOut"}}({{.GoOp}}{{.GoType}} {{.Args}}) => ({{.MaskOutConvert}} ({{.Asm}} {{.ArgsOut}}))
{{end}}
{{define "maskInMaskOut"}}({{.GoOp}}{{.GoType}} {{.Args}} mask) => ({{.MaskOutConvert}} ({{.Asm}} {{.ArgsOut}} ({{.MaskInConvert}} <types.TypeMask> mask)))
{{end}}
{{define "sftimm"}}({{.Asm}} x (MOVQconst [c])) => ({{.Asm}}const [uint8(c)] x)
{{end}}
{{define "masksftimm"}}({{.Asm}} x (MOVQconst [c]) mask) => ({{.Asm}}const [uint8(c)] x mask)
{{end}}
{{define "vregMem"}}({{.Asm}} {{.ArgsLoadAddr}}) && canMergeLoad(v, l) && clobber(l) => ({{.Asm}}load {{.ArgsAddr}})
{{end}}
{{define "vregMemFeatCheck"}}({{.Asm}} {{.ArgsLoadAddr}}) && {{.FeatCheck}} && canMergeLoad(v, l) && clobber(l)=> ({{.Asm}}load {{.ArgsAddr}})
{{end}}
`))
)

func (d tplRuleData) MaskOptimization(asmCheck map[string]bool) string {
	asmNoMask := d.Asm
	if i := strings.Index(asmNoMask, "Masked"); i == -1 {
		return ""
	}
	asmNoMask = strings.ReplaceAll(asmNoMask, "Masked", "")
	if asmCheck[asmNoMask] == false {
		return ""
	}

	for _, nope := range []string{"VMOVDQU", "VPCOMPRESS", "VCOMPRESS", "VPEXPAND", "VEXPAND", "VPBLENDM", "VMOVUP"} {
		if strings.HasPrefix(asmNoMask, nope) {
			return ""
		}
	}

	size := asmNoMask[len(asmNoMask)-3:]
	if strings.HasSuffix(asmNoMask, "const") {
		sufLen := len("128const")
		size = asmNoMask[len(asmNoMask)-sufLen:][:3]
	}
	switch size {
	case "128", "256", "512":
	default:
		panic("Unexpected operation size on " + d.Asm)
	}

	switch d.ElementSize {
	case 8, 16, 32, 64:
	default:
		panic(fmt.Errorf("Unexpected operation width %d on %v", d.ElementSize, d.Asm))
	}

	return fmt.Sprintf("(VMOVDQU%dMasked%s (%s %s) mask) => (%s %s mask)\n", d.ElementSize, size, asmNoMask, d.Args, d.Asm, d.Args)
}

// SSA 重写规则需要按从最具体到最不具体的顺序出现。这个映射用于实现该目的。
var tmplOrder = map[string]int{
	"masksftimm":    0,
	"sftimm":        1,
	"maskInMaskOut": 2,
	"maskOut":       3,
	"maskIn":        4,
	"pureVreg":      5,
	"vregMem":       6,
}

func compareTplRuleData(x, y tplRuleData) int {
	if c := compareNatural(x.GoOp, y.GoOp); c != 0 {
		return c
	}
	if c := compareNatural(x.GoType, y.GoType); c != 0 {
		return c
	}
	if c := compareNatural(x.Args, y.Args); c != 0 {
		return c
	}
	if x.tplName == y.tplName {
		return 0
	}
	xo, xok := tmplOrder[x.tplName]
	yo, yok := tmplOrder[y.tplName]
	if !xok {
		panic(fmt.Errorf("Unexpected template name %s, please add to tmplOrder", x.tplName))
	}
	if !yok {
		panic(fmt.Errorf("Unexpected template name %s, please add to tmplOrder", y.tplName))
	}
	return xo - yo
}

// writeSIMDRules 生成 SSA 的降低和重写规则，并将其写入指定目录中的 simdAMD64.rules。
func writeSIMDRules(ops []Operation) *bytes.Buffer {
	buffer := new(bytes.Buffer)
	buffer.WriteString(generatedHeader + "\n")

	// asm -> 掩码合并规则
	maskedMergeOpts := make(map[string]string)
	s2n := map[int]string{8: "B", 16: "W", 32: "D", 64: "Q"}
	asmCheck := map[string]bool{}
	var allData []tplRuleData
	var optData []tplRuleData    // 用于掩码窥孔优化和其他杂项
	var memOptData []tplRuleData // 用于内存窥孔优化
	memOpSeen := make(map[string]bool)

	for _, opr := range ops {
		opInShape, opOutShape, maskType, immType, gOp := opr.shape()
		asm := machineOpName(maskType, gOp)
		vregInCnt := len(gOp.In)
		if maskType == OneMask {
			vregInCnt--
		}

		data := tplRuleData{
			GoOp: gOp.Go,
			Asm:  asm,
		}

		if vregInCnt == 1 {
			data.Args = "x"
			data.ArgsOut = data.Args
		} else if vregInCnt == 2 {
			data.Args = "x y"
			data.ArgsOut = data.Args
		} else if vregInCnt == 3 {
			data.Args = "x y z"
			data.ArgsOut = data.Args
		} else {
			panic(fmt.Errorf("simdgen does not support more than 3 vreg in inputs"))
		}
		if immType == ConstImm {
			data.ArgsOut = fmt.Sprintf("[%s] %s", *opr.In[0].Const, data.ArgsOut)
		} else if immType == VarImm {
			data.Args = fmt.Sprintf("[a] %s", data.Args)
			data.ArgsOut = fmt.Sprintf("[a] %s", data.ArgsOut)
		} else if immType == ConstVarImm {
			data.Args = fmt.Sprintf("[a] %s", data.Args)
			data.ArgsOut = fmt.Sprintf("[a+%s] %s", *opr.In[0].Const, data.ArgsOut)
		}

		goType := func(op Operation) string {
			if op.OperandOrder != nil {
				switch *op.OperandOrder {
				case "21Type1", "231Type1":
					// Permute uses operand[1] for method receiver.
					return *op.In[1].Go
				}
			}
			return *op.In[0].Go
		}
		var tplName string
		// 如果发生类覆盖，那实际上不是掩码而是向量寄存器。
		if opOutShape == OneVregOut || opOutShape == OneVregOutAtIn || gOp.Out[0].OverwriteClass != nil {
			switch opInShape {
			case OneImmIn:
				tplName = "pureVreg"
				data.GoType = goType(gOp)
			case PureVregIn:
				tplName = "pureVreg"
				data.GoType = goType(gOp)
			case OneKmaskImmIn:
				fallthrough
			case OneKmaskIn:
				tplName = "maskIn"
				data.GoType = goType(gOp)
				rearIdx := len(gOp.In) - 1
				// 掩码在末尾。
				width := *gOp.In[rearIdx].ElemBits
				data.MaskInConvert = fmt.Sprintf("VPMOVVec%dx%dToM", width, *gOp.In[rearIdx].Lanes)
				data.ElementSize = width
			case PureKmaskIn:
				panic(fmt.Errorf("simdgen does not support pure k mask instructions, they should be generated by compiler optimizations"))
			}
		} else if opOutShape == OneGregOut {
			tplName = "pureVreg" // TODO 这将是错误的
			data.GoType = goType(gOp)
		} else {
			// OneKmaskOut 情况
			data.MaskOutConvert = fmt.Sprintf("VPMOVMToVec%dx%d", *gOp.Out[0].ElemBits, *gOp.In[0].Lanes)
			switch opInShape {
			case OneImmIn:
				fallthrough
			case PureVregIn:
				tplName = "maskOut"
				data.GoType = goType(gOp)
			case OneKmaskImmIn:
				fallthrough
			case OneKmaskIn:
				tplName = "maskInMaskOut"
				data.GoType = goType(gOp)
				rearIdx := len(gOp.In) - 1
				data.MaskInConvert = fmt.Sprintf("VPMOVVec%dx%dToM", *gOp.In[rearIdx].ElemBits, *gOp.In[rearIdx].Lanes)
			case PureKmaskIn:
				panic(fmt.Errorf("simdgen does not support pure k mask instructions, they should be generated by compiler optimizations"))
			}
		}

		if gOp.SpecialLower != nil {
			if *gOp.SpecialLower == "sftimm" {
				if data.GoType[0] == 'I' {
					// 仅对有符号类型执行此操作，对无符号类型是重复重写
					sftImmData := data
					if tplName == "maskIn" {
						sftImmData.tplName = "masksftimm"
					} else {
						sftImmData.tplName = "sftimm"
					}
					allData = append(allData, sftImmData)
					asmCheck[sftImmData.Asm+"const"] = true
				}
			} else {
				panic("simdgen sees unknwon special lower " + *gOp.SpecialLower + ", maybe implement it?")
			}
		}
		if gOp.MemFeatures != nil && *gOp.MemFeatures == "vbcst" {
			// 完整性检查
			selected := true
			for _, a := range gOp.In {
				if a.TreatLikeAScalarOfSize != nil {
					selected = false
					break
				}
			}
			if _, ok := memOpSeen[data.Asm]; ok {
				selected = false
			}
			if selected {
				memOpSeen[data.Asm] = true
				lastVreg := gOp.In[vregInCnt-1]
				// 完整性检查
				if lastVreg.Class != "vreg" {
					panic(fmt.Errorf("simdgen expects vbcst replaced operand to be a vreg, but %v found", lastVreg))
				}
				memOpData := data
				// 从参数中移除最后一个 vreg 并将其更改为 load。
				origArgs := data.Args[:len(data.Args)-1]
				// 准备立即数参数。
				immArg := ""
				immArgCombineOff := " [off] "
				if immType != NoImm && immType != InvalidImm {
					_, after, found := strings.Cut(origArgs, "]")
					if found {
						origArgs = after
					}
					immArg = "[c] "
					immArgCombineOff = " [makeValAndOff(int32(uint8(c)),off)] "
				}
				memOpData.ArgsLoadAddr = immArg + origArgs + fmt.Sprintf("l:(VMOVDQUload%d {sym} [off] ptr mem)", *lastVreg.Bits)
				// 从参数中移除最后一个 vreg 并将其更改为 "ptr"。
				memOpData.ArgsAddr = "{sym}" + immArgCombineOff + origArgs + "ptr"
				if maskType == OneMask {
					memOpData.ArgsAddr += " mask"
					memOpData.ArgsLoadAddr += " mask"
				}
				memOpData.ArgsAddr += " mem"
				if gOp.MemFeaturesData != nil {
					_, feat2 := getVbcstData(*gOp.MemFeaturesData)
					knownFeatChecks := map[string]string{
						"AVX":    "v.Block.CPUfeatures.hasFeature(CPUavx)",
						"AVX2":   "v.Block.CPUfeatures.hasFeature(CPUavx2)",
						"AVX512": "v.Block.CPUfeatures.hasFeature(CPUavx512)",
					}
					memOpData.FeatCheck = knownFeatChecks[feat2]
					memOpData.tplName = "vregMemFeatCheck"
				} else {
					memOpData.tplName = "vregMem"
				}
				memOptData = append(memOptData, memOpData)
				asmCheck[memOpData.Asm+"load"] = true
			}
		}
		// 生成掩码合并优化规则
		if gOp.hasMaskedMerging(maskType, opOutShape) {
			// TODO: 处理自定义操作数顺序和特殊降低。
			maskElem := gOp.In[len(gOp.In)-1]
			if maskElem.Bits == nil {
				panic("mask has no bits")
			}
			if maskElem.ElemBits == nil {
				panic("mask has no elemBits")
			}
			if maskElem.Lanes == nil {
				panic("mask has no lanes")
			}
			switch *maskElem.Bits {
			case 128, 256:
				// VPBLENDVB 情况。
				noMaskName := machineOpName(NoMask, gOp)
				ruleExisting, ok := maskedMergeOpts[noMaskName]
				rule := fmt.Sprintf("(VPBLENDVB%d dst (%s %s) mask) && v.Block.CPUfeatures.hasFeature(CPUavx512) => (%sMerging dst %s (VPMOVVec%dx%dToM <types.TypeMask> mask))\n",
					*maskElem.Bits, noMaskName, data.Args, data.Asm, data.Args, *maskElem.ElemBits, *maskElem.Lanes)
				if ok && ruleExisting != rule {
					panic(fmt.Sprintf("multiple masked merge rules for one op:\n%s\n%s\n", ruleExisting, rule))
				} else {
					maskedMergeOpts[noMaskName] = rule
				}
			case 512:
				// VPBLENDM[BWDQ] 情况。
				noMaskName := machineOpName(NoMask, gOp)
				ruleExisting, ok := maskedMergeOpts[noMaskName]
				rule := fmt.Sprintf("(VPBLENDM%sMasked%d dst (%s %s) mask) => (%sMerging dst %s mask)\n",
					s2n[*maskElem.ElemBits], *maskElem.Bits, noMaskName, data.Args, data.Asm, data.Args)
				if ok && ruleExisting != rule {
					panic(fmt.Sprintf("multiple masked merge rules for one op:\n%s\n%s\n", ruleExisting, rule))
				} else {
					maskedMergeOpts[noMaskName] = rule
				}
			}
		}

		if tplName == "pureVreg" && data.Args == data.ArgsOut {
			data.Args = "..."
			data.ArgsOut = "..."
		}
		data.tplName = tplName
		if opr.NoGenericOps != nil && *opr.NoGenericOps == "true" ||
			opr.SkipMaskedMethod() {
			optData = append(optData, data)
			continue
		}
		allData = append(allData, data)
		asmCheck[data.Asm] = true
	}

	slices.SortFunc(allData, compareTplRuleData)

	for _, data := range allData {
		if err := ruleTemplates.ExecuteTemplate(buffer, data.tplName, data); err != nil {
			panic(fmt.Errorf("failed to execute template %s for %s: %w", data.tplName, data.GoOp+data.GoType, err))
		}
	}

	seen := make(map[string]bool)

	for _, data := range optData {
		if data.tplName == "maskIn" {
			rule := data.MaskOptimization(asmCheck)
			if seen[rule] {
				continue
			}
			seen[rule] = true
			buffer.WriteString(rule)
		}
	}

	maskedMergeOptsRules := []string{}
	for asm, rule := range maskedMergeOpts {
		if !asmCheck[asm] {
			continue
		}
		maskedMergeOptsRules = append(maskedMergeOptsRules, rule)
	}
	slices.Sort(maskedMergeOptsRules)
	for _, rule := range maskedMergeOptsRules {
		buffer.WriteString(rule)
	}

	for _, data := range memOptData {
		if err := ruleTemplates.ExecuteTemplate(buffer, data.tplName, data); err != nil {
			panic(fmt.Errorf("failed to execute template %s for %s: %w", data.tplName, data.Asm, err))
		}
	}

	return buffer
}
