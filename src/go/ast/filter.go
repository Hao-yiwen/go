// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package ast

import (
	"go/token"
	"slices"
)

// ----------------------------------------------------------------------------
// 导出过滤

// exportFilter 是用于提取导出节点的特殊过滤函数。
func exportFilter(name string) bool {
	return IsExported(name)
}

// FileExports 原地裁剪 Go 源文件的 AST，使得只保留导出的节点：
// 所有未导出的顶级标识符及其关联信息（如类型、初始值或函数体）
// 都会被移除。导出类型的非导出字段和方法会被剥离。
// [File.Comments] 列表不会改变。
//
// FileExports 报告是否存在导出的声明。
func FileExports(src *File) bool {
	return filterFile(src, exportFilter, true)
}

// PackageExports 原地裁剪 Go 包的 AST，使得只保留导出的节点。
// pkg.Files 列表不会改变，以便文件名和顶级包注释不会丢失。
//
// PackageExports 报告是否存在导出的声明；否则返回 false。
//
// 已弃用：改用类型检查器 [go/types] 而不是 [Package]；
// 参见 [Object]。或者使用 [FileExports]。
func PackageExports(pkg *Package) bool {
	return filterPackage(pkg, exportFilter, true)
}

// ----------------------------------------------------------------------------
// 通用过滤

type Filter func(string) bool

func filterIdentList(list []*Ident, f Filter) []*Ident {
	j := 0
	for _, x := range list {
		if f(x.Name) {
			list[j] = x
			j++
		}
	}
	return list[0:j]
}

// fieldName 假设 x 是匿名字段的类型，并返回相应的字段名。
// 如果 x 不是可接受的匿名字段，结果为 nil。
func fieldName(x Expr) *Ident {
	switch t := x.(type) {
	case *Ident:
		return t
	case *SelectorExpr:
		if _, ok := t.X.(*Ident); ok {
			return t.Sel
		}
	case *StarExpr:
		return fieldName(t.X)
	}
	return nil
}

func filterFieldList(fields *FieldList, filter Filter, export bool) (removedFields bool) {
	if fields == nil {
		return false
	}
	list := fields.List
	j := 0
	for _, f := range list {
		keepField := false
		if len(f.Names) == 0 {
			// 匿名字段
			name := fieldName(f.Type)
			keepField = name != nil && filter(name.Name)
		} else {
			n := len(f.Names)
			f.Names = filterIdentList(f.Names, filter)
			if len(f.Names) < n {
				removedFields = true
			}
			keepField = len(f.Names) > 0
		}
		if keepField {
			if export {
				filterType(f.Type, filter, export)
			}
			list[j] = f
			j++
		}
	}
	if j < len(list) {
		removedFields = true
	}
	fields.List = list[0:j]
	return
}

func filterCompositeLit(lit *CompositeLit, filter Filter, export bool) {
	n := len(lit.Elts)
	lit.Elts = filterExprList(lit.Elts, filter, export)
	if len(lit.Elts) < n {
		lit.Incomplete = true
	}
}

func filterExprList(list []Expr, filter Filter, export bool) []Expr {
	j := 0
	for _, exp := range list {
		switch x := exp.(type) {
		case *CompositeLit:
			filterCompositeLit(x, filter, export)
		case *KeyValueExpr:
			if x, ok := x.Key.(*Ident); ok && !filter(x.Name) {
				continue
			}
			if x, ok := x.Value.(*CompositeLit); ok {
				filterCompositeLit(x, filter, export)
			}
		}
		list[j] = exp
		j++
	}
	return list[0:j]
}

func filterParamList(fields *FieldList, filter Filter, export bool) bool {
	if fields == nil {
		return false
	}
	var b bool
	for _, f := range fields.List {
		if filterType(f.Type, filter, export) {
			b = true
		}
	}
	return b
}

func filterType(typ Expr, f Filter, export bool) bool {
	switch t := typ.(type) {
	case *Ident:
		return f(t.Name)
	case *ParenExpr:
		return filterType(t.X, f, export)
	case *ArrayType:
		return filterType(t.Elt, f, export)
	case *StructType:
		if filterFieldList(t.Fields, f, export) {
			t.Incomplete = true
		}
		return len(t.Fields.List) > 0
	case *FuncType:
		b1 := filterParamList(t.Params, f, export)
		b2 := filterParamList(t.Results, f, export)
		return b1 || b2
	case *InterfaceType:
		if filterFieldList(t.Methods, f, export) {
			t.Incomplete = true
		}
		return len(t.Methods.List) > 0
	case *MapType:
		b1 := filterType(t.Key, f, export)
		b2 := filterType(t.Value, f, export)
		return b1 || b2
	case *ChanType:
		return filterType(t.Value, f, export)
	}
	return false
}

func filterSpec(spec Spec, f Filter, export bool) bool {
	switch s := spec.(type) {
	case *ValueSpec:
		s.Names = filterIdentList(s.Names, f)
		s.Values = filterExprList(s.Values, f, export)
		if len(s.Names) > 0 {
			if export {
				filterType(s.Type, f, export)
			}
			return true
		}
	case *TypeSpec:
		if f(s.Name.Name) {
			if export {
				filterType(s.Type, f, export)
			}
			return true
		}
		if !export {
			// 对于通用过滤（不仅仅是导出），
			// 即使名称未被过滤掉也要过滤类型。
			// 如果类型包含过滤后的元素，
			// 保留该声明。
			return filterType(s.Type, f, export)
		}
	}
	return false
}

func filterSpecList(list []Spec, f Filter, export bool) []Spec {
	j := 0
	for _, s := range list {
		if filterSpec(s, f, export) {
			list[j] = s
			j++
		}
	}
	return list[0:j]
}

// FilterDecl 通过移除未通过过滤器 f 的所有名称（包括结构体字段
// 和接口方法名称，但不包括参数列表中的名称）来原地裁剪 Go 声明的 AST。
//
// FilterDecl 报告过滤后是否还剩下任何已声明的名称。
func FilterDecl(decl Decl, f Filter) bool {
	return filterDecl(decl, f, false)
}

func filterDecl(decl Decl, f Filter, export bool) bool {
	switch d := decl.(type) {
	case *GenDecl:
		d.Specs = filterSpecList(d.Specs, f, export)
		return len(d.Specs) > 0
	case *FuncDecl:
		return f(d.Name.Name)
	}
	return false
}

// FilterFile 通过从顶级声明中移除未通过过滤器 f 的所有名称
// （包括结构体字段和接口方法名称，但不包括参数列表中的名称）
// 来原地裁剪 Go 文件的 AST。如果声明随后变为空，则从 AST 中移除该声明。
// Import 声明总是被移除。[File.Comments] 列表不会改变。
//
// FilterFile 报告过滤后是否还剩下任何顶级声明。
func FilterFile(src *File, f Filter) bool {
	return filterFile(src, f, false)
}

func filterFile(src *File, f Filter, export bool) bool {
	j := 0
	for _, d := range src.Decls {
		if filterDecl(d, f, export) {
			src.Decls[j] = d
			j++
		}
	}
	src.Decls = src.Decls[0:j]
	return j > 0
}

// FilterPackage 通过从顶级声明中移除未通过过滤器 f 的所有名称
// （包括结构体字段和接口方法名称，但不包括参数列表中的名称）
// 来原地裁剪 Go 包的 AST。如果声明随后变为空，则从 AST 中移除该声明。
// pkg.Files 列表不会改变，以便文件名和顶级包注释不会丢失。
//
// FilterPackage 报告过滤后是否还剩下任何顶级声明。
//
// 已弃用：改用类型检查器 [go/types] 而不是 [Package]；
// 参见 [Object]。或者使用 [FilterFile]。
func FilterPackage(pkg *Package, f Filter) bool {
	return filterPackage(pkg, f, false)
}

func filterPackage(pkg *Package, f Filter, export bool) bool {
	hasDecls := false
	for _, src := range pkg.Files {
		if filterFile(src, f, export) {
			hasDecls = true
		}
	}
	return hasDecls
}

// ----------------------------------------------------------------------------
// 包文件合并

// MergeMode 标志控制 [MergePackageFiles] 的行为。
//
// 已弃用：改用类型检查器 [go/types] 而不是 [Package]；
// 参见 [Object]。
type MergeMode uint

// 已弃用：改用类型检查器 [go/types] 而不是 [Package]；
// 参见 [Object]。
const (
	// 如果设置，则排除重复的函数声明。
	FilterFuncDuplicates MergeMode = 1 << iota
	// 如果设置，则排除未与特定 AST 节点关联的注释
	// （作为 Doc 或 Comment）。
	FilterUnassociatedComments
	// 如果设置，则排除重复的 import 声明。
	FilterImportDuplicates
)

// nameOf 返回给定函数声明的函数名（foo）或方法名（foo.bar）。
// 如果接收者的 AST 不正确，则假定它是一个函数。
func nameOf(f *FuncDecl) string {
	if r := f.Recv; r != nil && len(r.List) == 1 {
		// 看起来像是正确的接收者声明
		t := r.List[0].Type
		// 解引用指针接收者类型
		if p, _ := t.(*StarExpr); p != nil {
			t = p.X
		}
		// 接收者类型必须是类型名
		if p, _ := t.(*Ident); p != nil {
			return p.Name + "." + f.Name.Name
		}
		// 否则假定是函数
	}
	return f.Name.Name
}

// separator 是一个空的 // 风格注释，当不同的注释组
// 连接成单个组时穿插其间
var separator = &Comment{token.NoPos, "//"}

// MergePackageFiles 通过合并属于包的文件的 AST 来创建文件 AST。
// mode 标志控制合并行为。
//
// 已弃用：此函数规范不完善且有无法修复的 bug；
// 另外 [Package] 也已弃用。
func MergePackageFiles(pkg *Package, mode MergeMode) *File {
	// 计算所有包文件中的包文档、注释和声明的数量。
	// 同时计算排序后的文件名列表，以便后续迭代可以始终以相同顺序进行。
	ndocs := 0
	ncomments := 0
	ndecls := 0
	filenames := make([]string, len(pkg.Files))
	var minPos, maxPos token.Pos
	i := 0
	for filename, f := range pkg.Files {
		filenames[i] = filename
		i++
		if f.Doc != nil {
			ndocs += len(f.Doc.List) + 1 // +1 用于分隔符
		}
		ncomments += len(f.Comments)
		ndecls += len(f.Decls)
		if i == 0 || f.FileStart < minPos {
			minPos = f.FileStart
		}
		if i == 0 || f.FileEnd > maxPos {
			maxPos = f.FileEnd
		}
	}
	slices.Sort(filenames)

	// 将所有包文件的包注释收集到单个 CommentGroup 中 - 收集的包文档。
	// 通常应该只有一个文件有包注释；但收集额外的注释比丢弃它们更好。
	var doc *CommentGroup
	var pos token.Pos
	if ndocs > 0 {
		list := make([]*Comment, ndocs-1) // -1：第一组前没有分隔符
		i := 0
		for _, filename := range filenames {
			f := pkg.Files[filename]
			if f.Doc != nil {
				if i > 0 {
					// 不是第一组 - 添加分隔符
					list[i] = separator
					i++
				}
				for _, c := range f.Doc.List {
					list[i] = c
					i++
				}
				if f.Package > pos {
					// 保留最大的包子句位置作为
					// 合并文件的包子句位置。
					pos = f.Package
				}
			}
		}
		doc = &CommentGroup{list}
	}

	// 从所有包文件收集声明。
	var decls []Decl
	if ndecls > 0 {
		decls = make([]Decl, ndecls)
		funcs := make(map[string]int) // 函数名 -> decls 索引的映射
		i := 0                        // 当前索引
		n := 0                        // 过滤条目的数量
		for _, filename := range filenames {
			f := pkg.Files[filename]
			for _, d := range f.Decls {
				if mode&FilterFuncDuplicates != 0 {
					// 语言实体可能在不同的包文件中声明多次；
					// 只有在构建时声明才必须唯一。
					// 目前，排除函数的多次声明 - 保留有文档的那个。
					//
					// TODO(gri): 如果多次声明很常见，
					//            将此过滤扩展到其他实体（const、type、vars）。
					if f, isFun := d.(*FuncDecl); isFun {
						name := nameOf(f)
						if j, exists := funcs[name]; exists {
							// 函数已声明
							if decls[j] != nil && decls[j].(*FuncDecl).Doc == nil {
								// 现有声明没有文档；
								// 忽略现有声明
								decls[j] = nil
							} else {
								// 忽略新声明
								d = nil
							}
							n++ // 过滤了一个条目
						} else {
							funcs[name] = i
						}
					}
				}
				decls[i] = d
				i++
			}
		}

		// 如果条目被过滤，则从 decls 列表中消除 nil 条目。
		// 我们使用第二次遍历来做这件事，以避免扰乱源代码中
		// 原始的声明顺序（否则，这也会使单个文件中
		// 单调递增的位置信息无效）。
		if n > 0 {
			i = 0
			for _, d := range decls {
				if d != nil {
					decls[i] = d
					i++
				}
			}
			decls = decls[0:i]
		}
	}

	// 从所有包文件收集 import spec。
	var imports []*ImportSpec
	if mode&FilterImportDuplicates != 0 {
		seen := make(map[string]bool)
		for _, filename := range filenames {
			f := pkg.Files[filename]
			for _, imp := range f.Imports {
				if path := imp.Path.Value; !seen[path] {
					// TODO: 考虑处理以下情况：
					// - 存在 2 个具有相同导入路径但
					//   本地名称不同的导入（可能应该保留两个）
					// - 存在 2 个导入但只有一个有注释
					// - 存在 2 个导入且都有（可能不同的）注释
					imports = append(imports, imp)
					seen[path] = true
				}
			}
		}
	} else {
		// 按文件名迭代以确保确定性顺序。
		for _, filename := range filenames {
			f := pkg.Files[filename]
			imports = append(imports, f.Imports...)
		}
	}

	// 从所有包文件收集注释。
	var comments []*CommentGroup
	if mode&FilterUnassociatedComments == 0 {
		comments = make([]*CommentGroup, ncomments)
		i := 0
		for _, filename := range filenames {
			f := pkg.Files[filename]
			i += copy(comments[i:], f.Comments)
		}
	}

	// TODO(gri) 需要计算未解析的标识符！
	return &File{doc, pos, NewIdent(pkg.Name), decls, minPos, maxPos, pkg.Scope, imports, nil, comments, ""}
}
