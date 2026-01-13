// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"bytes"
	"fmt"
	"log"
	"sort"
	"strings"
)

const simdMachineOpsTmpl = `
package main

func simdAMD64Ops(v11, v21, v2k, vkv, v2kv, v2kk, v31, v3kv, vgpv, vgp, vfpv, vfpkv, w11, w21, w2k, wkw, w2kw, w2kk, w31, w3kw, wgpw, wgp, wfpw, wfpkw,
	wkwload, v21load, v31load, v11load, w21load, w31load, w2kload, w2kwload, w11load, w3kwload, w2kkload, v31x0AtIn2 regInfo) []opData {
	return []opData{
{{- range .OpsData }}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", commutative: {{.Comm}}, typ: "{{.Type}}", resultInArg0: {{.ResultInArg0}}},
{{- end }}
{{- range .OpsDataImm }}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", aux: "UInt8", commutative: {{.Comm}}, typ: "{{.Type}}", resultInArg0: {{.ResultInArg0}}},
{{- end }}
{{- range .OpsDataLoad}}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", commutative: {{.Comm}}, typ: "{{.Type}}", aux: "SymOff", symEffect: "Read", resultInArg0: {{.ResultInArg0}}},
{{- end}}
{{- range .OpsDataImmLoad}}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", commutative: {{.Comm}}, typ: "{{.Type}}", aux: "SymValAndOff", symEffect: "Read", resultInArg0: {{.ResultInArg0}}},
{{- end}}
{{- range .OpsDataMerging }}
		{name: "{{.OpName}}Merging", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", commutative: false, typ: "{{.Type}}", resultInArg0: true},
{{- end }}
{{- range .OpsDataImmMerging }}
		{name: "{{.OpName}}Merging", argLength: {{.OpInLen}}, reg: {{.RegInfo}}, asm: "{{.Asm}}", aux: "UInt8", commutative: false, typ: "{{.Type}}", resultInArg0: true},
{{- end }}
	}
}
`

// writeSIMDMachineOps 生成机器操作并将其写入指定目录中的 simdAMD64ops.go。
func writeSIMDMachineOps(ops []Operation) *bytes.Buffer {
	t := templateOf(simdMachineOpsTmpl, "simdAMD64Ops")
	buffer := new(bytes.Buffer)
	buffer.WriteString(generatedHeader)

	type opData struct {
		OpName       string
		Asm          string
		OpInLen      int
		RegInfo      string
		Comm         bool
		Type         string
		ResultInArg0 bool
	}
	type machineOpsData struct {
		OpsData           []opData
		OpsDataImm        []opData
		OpsDataLoad       []opData
		OpsDataImmLoad    []opData
		OpsDataMerging    []opData
		OpsDataImmMerging []opData
	}

	regInfoSet := map[string]bool{
		"v11": true, "v21": true, "v2k": true, "v2kv": true, "v2kk": true, "vkv": true, "v31": true, "v3kv": true, "vgpv": true, "vgp": true, "vfpv": true, "vfpkv": true,
		"w11": true, "w21": true, "w2k": true, "w2kw": true, "w2kk": true, "wkw": true, "w31": true, "w3kw": true, "wgpw": true, "wgp": true, "wfpw": true, "wfpkw": true,
		"wkwload": true, "v21load": true, "v31load": true, "v11load": true, "w21load": true, "w31load": true, "w2kload": true, "w2kwload": true, "w11load": true,
		"w3kwload": true, "w2kkload": true, "v31x0AtIn2": true}
	opsData := make([]opData, 0)
	opsDataImm := make([]opData, 0)
	opsDataLoad := make([]opData, 0)
	opsDataImmLoad := make([]opData, 0)
	opsDataMerging := make([]opData, 0)
	opsDataImmMerging := make([]opData, 0)

	// 确定要使用的指令的"最佳"版本
	best := make(map[string]Operation)
	var mOpOrder []string
	countOverrides := func(s []Operand) int {
		a := 0
		for _, o := range s {
			if o.OverwriteBase != nil {
				a++
			}
		}
		return a
	}
	for _, op := range ops {
		_, _, maskType, _, gOp := op.shape()
		asm := machineOpName(maskType, gOp)
		other, ok := best[asm]
		if !ok {
			best[asm] = op
			mOpOrder = append(mOpOrder, asm)
			continue
		}
		if !op.Commutative && other.Commutative { // 如果存在非交换版本的操作，则非交换版本优先。
			best[asm] = op
			continue
		}
		// 检查 "op" 是否比 "other" 更好
		if countOverrides(op.In)+countOverrides(op.Out) < countOverrides(other.In)+countOverrides(other.Out) {
			best[asm] = op
		}
	}

	regInfoErrs := make([]error, 0)
	regInfoMissing := make(map[string]bool, 0)
	for _, asm := range mOpOrder {
		op := best[asm]
		shapeIn, shapeOut, maskType, _, gOp := op.shape()

		// TODO: 我们所有的掩码操作现在都是置零的，我们需要生成带有合并掩码的机器操作，也许可以
		// 在这里复制一个并添加 "Merging" 后缀。重写规则将需要它们。
		makeRegInfo := func(op Operation, mem memShape) (string, error) {
			regInfo, err := op.regShape(mem)
			if err != nil {
				panic(err)
			}
			regInfo, err = rewriteVecAsScalarRegInfo(op, regInfo)
			if err != nil {
				if mem == NoMem || mem == InvalidMem {
					panic(err)
				}
				return "", err
			}
			if regInfo == "v01load" {
				regInfo = "vload"
			}
			// 使 AVX512 操作使用高位寄存器
			if strings.Contains(op.CPUFeature, "AVX512") {
				regInfo = strings.ReplaceAll(regInfo, "v", "w")
			}
			if _, ok := regInfoSet[regInfo]; !ok {
				regInfoErrs = append(regInfoErrs, fmt.Errorf("unsupported register constraint, please update the template and AMD64Ops.go: %s.  Op is %s", regInfo, op))
				regInfoMissing[regInfo] = true
			}
			return regInfo, nil
		}
		regInfo, err := makeRegInfo(op, NoMem)
		if err != nil {
			panic(err)
		}
		var outType string
		if shapeOut == OneVregOut || shapeOut == OneVregOutAtIn || gOp.Out[0].OverwriteClass != nil {
			// 如果发生类覆盖，那实际上不是掩码而是向量寄存器。
			outType = fmt.Sprintf("Vec%d", *gOp.Out[0].Bits)
		} else if shapeOut == OneGregOut {
			outType = gOp.GoType() // 这是一个直接的 Go 类型，不是 VecNNN 类型
		} else if shapeOut == OneKmaskOut {
			outType = "Mask"
		} else {
			panic(fmt.Errorf("simdgen does not recognize this output shape: %d", shapeOut))
		}
		resultInArg0 := false
		if shapeOut == OneVregOutAtIn {
			resultInArg0 = true
		}
		var memOpData *opData
		regInfoMerging := regInfo
		hasMerging := false
		if op.MemFeatures != nil && *op.MemFeatures == "vbcst" {
			// 目前我们只有 vbcst 情况
			// 创建一个完整的向量内存变体。
			opMem := rewriteLastVregToMem(op)
			regInfo, err := makeRegInfo(opMem, VregMemIn)
			if err != nil {
				// 如果不为空就跳过它。
				// 错误可能由 [checkVecAsScalar] 触发。
				// TODO: 使 [checkVecAsScalar] 能够识别内存操作。
				if *Verbose {
					log.Printf("Seen error: %e", err)
				}
			} else {
				memOpData = &opData{asm + "load", gOp.Asm, len(gOp.In) + 1, regInfo, false, outType, resultInArg0}
			}
		}
		hasMerging = gOp.hasMaskedMerging(maskType, shapeOut)
		if hasMerging && !resultInArg0 {
			// 我们必须在这里复制切片，因为当没有重新切片时，
			// 排序操作会从其他别名中可见。
			newIn := make([]Operand, len(op.In), len(op.In)+1)
			copy(newIn, op.In)
			op.In = newIn
			op.In = append(op.In, op.Out[0])
			op.sortOperand()
			regInfoMerging, err = makeRegInfo(op, NoMem)
			if err != nil {
				panic(err)
			}
		}

		if shapeIn == OneImmIn || shapeIn == OneKmaskImmIn {
			opsDataImm = append(opsDataImm, opData{asm, gOp.Asm, len(gOp.In), regInfo, gOp.Commutative, outType, resultInArg0})
			if memOpData != nil {
				if *op.MemFeatures != "vbcst" {
					panic("simdgen only knows vbcst for mem ops for now")
				}
				opsDataImmLoad = append(opsDataImmLoad, *memOpData)
			}
			if hasMerging {
				mergingLen := len(gOp.In)
				if !resultInArg0 {
					mergingLen++
				}
				opsDataImmMerging = append(opsDataImmMerging, opData{asm, gOp.Asm, mergingLen, regInfoMerging, gOp.Commutative, outType, resultInArg0})
			}
		} else {
			opsData = append(opsData, opData{asm, gOp.Asm, len(gOp.In), regInfo, gOp.Commutative, outType, resultInArg0})
			if memOpData != nil {
				if *op.MemFeatures != "vbcst" {
					panic("simdgen only knows vbcst for mem ops for now")
				}
				opsDataLoad = append(opsDataLoad, *memOpData)
			}
			if hasMerging {
				mergingLen := len(gOp.In)
				if !resultInArg0 {
					mergingLen++
				}
				opsDataMerging = append(opsDataMerging, opData{asm, gOp.Asm, mergingLen, regInfoMerging, gOp.Commutative, outType, resultInArg0})
			}
		}
	}
	if len(regInfoErrs) != 0 {
		for _, e := range regInfoErrs {
			log.Printf("Errors: %e\n", e)
		}
		panic(fmt.Errorf("these regInfo unseen: %v", regInfoMissing))
	}
	sort.Slice(opsData, func(i, j int) bool {
		return compareNatural(opsData[i].OpName, opsData[j].OpName) < 0
	})
	sort.Slice(opsDataImm, func(i, j int) bool {
		return compareNatural(opsDataImm[i].OpName, opsDataImm[j].OpName) < 0
	})
	sort.Slice(opsDataLoad, func(i, j int) bool {
		return compareNatural(opsDataLoad[i].OpName, opsDataLoad[j].OpName) < 0
	})
	sort.Slice(opsDataImmLoad, func(i, j int) bool {
		return compareNatural(opsDataImmLoad[i].OpName, opsDataImmLoad[j].OpName) < 0
	})
	sort.Slice(opsDataMerging, func(i, j int) bool {
		return compareNatural(opsDataMerging[i].OpName, opsDataMerging[j].OpName) < 0
	})
	sort.Slice(opsDataImmMerging, func(i, j int) bool {
		return compareNatural(opsDataImmMerging[i].OpName, opsDataImmMerging[j].OpName) < 0
	})
	err := t.Execute(buffer, machineOpsData{opsData, opsDataImm, opsDataLoad, opsDataImmLoad,
		opsDataMerging, opsDataImmMerging})
	if err != nil {
		panic(fmt.Errorf("failed to execute template: %w", err))
	}

	return buffer
}
