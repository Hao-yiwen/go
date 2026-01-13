// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package main

import (
	"bytes"
	"fmt"
	"sort"
)

const simdGenericOpsTmpl = `
package main

func simdGenericOps() []opData {
	return []opData{
{{- range .Ops }}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, commutative: {{.Comm}}},
{{- end }}
{{- range .OpsImm }}
		{name: "{{.OpName}}", argLength: {{.OpInLen}}, commutative: {{.Comm}}, aux: "UInt8"},
{{- end }}
	}
}
`

// writeSIMDGenericOps 生成通用操作并将其写入指定目录中的 simdAMD64ops.go。
func writeSIMDGenericOps(ops []Operation) *bytes.Buffer {
	t := templateOf(simdGenericOpsTmpl, "simdgenericOps")
	buffer := new(bytes.Buffer)
	buffer.WriteString(generatedHeader)

	type genericOpsData struct {
		OpName  string
		OpInLen int
		Comm    bool
	}
	type opData struct {
		Ops    []genericOpsData
		OpsImm []genericOpsData
	}
	var opsData opData
	for _, op := range ops {
		if op.NoGenericOps != nil && *op.NoGenericOps == "true" {
			continue
		}
		if op.SkipMaskedMethod() {
			continue
		}
		_, _, _, immType, gOp := op.shape()
		gOpData := genericOpsData{gOp.GenericName(), len(gOp.In), op.Commutative}
		if immType == VarImm || immType == ConstVarImm {
			opsData.OpsImm = append(opsData.OpsImm, gOpData)
		} else {
			opsData.Ops = append(opsData.Ops, gOpData)
		}
	}
	sort.Slice(opsData.Ops, func(i, j int) bool {
		return compareNatural(opsData.Ops[i].OpName, opsData.Ops[j].OpName) < 0
	})
	sort.Slice(opsData.OpsImm, func(i, j int) bool {
		return compareNatural(opsData.OpsImm[i].OpName, opsData.OpsImm[j].OpName) < 0
	})

	err := t.Execute(buffer, opsData)
	if err != nil {
		panic(fmt.Errorf("failed to execute template: %w", err))
	}

	return buffer
}
