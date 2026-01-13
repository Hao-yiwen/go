// 此代码由 "stringer -type attr" 生成；请勿编辑。

package template

import "strconv"

func _() {
	// "invalid array index" 编译器错误表示常量值已更改。
	// 重新运行 stringer 命令以重新生成它们。
	var x [1]struct{}
	_ = x[attrNone-0]
	_ = x[attrScript-1]
	_ = x[attrScriptType-2]
	_ = x[attrStyle-3]
	_ = x[attrURL-4]
	_ = x[attrSrcset-5]
}

const _attr_name = "attrNoneattrScriptattrScriptTypeattrStyleattrURLattrSrcset"

var _attr_index = [...]uint8{0, 8, 18, 32, 41, 48, 58}

func (i attr) String() string {
	if i >= attr(len(_attr_index)-1) {
		return "attr(" + strconv.FormatInt(int64(i), 10) + ")"
	}
	return _attr_name[_attr_index[i]:_attr_index[i+1]]
}
