// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 此文件包含调用解析器的导出入口点。

package parser

import (
	"bytes"
	"errors"
	"go/ast"
	"go/token"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// 如果 src != nil，readSource 会尽可能将 src 转换为 []byte；
// 否则返回错误。如果 src == nil，readSource 返回读取
// filename 指定的文件的结果。
func readSource(filename string, src any) ([]byte, error) {
	if src != nil {
		switch s := src.(type) {
		case string:
			return []byte(s), nil
		case []byte:
			return s, nil
		case *bytes.Buffer:
			// 是 io.Reader，但 src 已经以 []byte 形式提供
			if s != nil {
				return s.Bytes(), nil
			}
		case io.Reader:
			return io.ReadAll(s)
		}
		return nil, errors.New("invalid source")
	}
	return os.ReadFile(filename)
}

// Mode 值是一组标志（或 0）。
// 它们控制解析的源代码量和其他可选的解析器功能。
type Mode uint

const (
	PackageClauseOnly    Mode             = 1 << iota // 在包子句后停止解析
	ImportsOnly                                       // 在导入声明后停止解析
	ParseComments                                     // 解析注释并将其添加到 AST
	Trace                                             // 打印解析产生式的追踪
	DeclarationErrors                                 // 报告声明错误
	SpuriousErrors                                    // 与 AllErrors 相同，用于向后兼容
	SkipObjectResolution                              // 跳过已弃用的标识符解析；参见 ParseFile
	AllErrors            = SpuriousErrors             // 报告所有错误（不仅仅是不同行上的前 10 个）
)

// ParseFile 解析单个 Go 源文件的源代码，并返回对应的 [ast.File] 节点。
// 源代码可以通过源文件的文件名或 src 参数提供。
//
// 如果 src != nil，ParseFile 从 src 解析源，文件名仅在记录位置信息时使用。
// src 参数的参数类型必须是字符串、[]byte 或 [io.Reader]。
// 如果 src == nil，ParseFile 解析由 filename 指定的文件。
//
// mode 参数控制解析的源文本量和其他可选的解析器功能。
// 如果设置了 [SkipObjectResolution] 模式位（推荐），
// 解析的对象解析阶段将被跳过，导致 File.Scope、File.Unresolved 和
// 所有 Ident.Obj 字段为 nil。这些字段已弃用；有关详细信息，请参见 [ast.Object]。
//
// 位置信息被记录在文件集 fset 中，不得为 nil。
//
// 如果无法读取源，返回的 AST 为 nil，错误指示具体的失败。
// 如果读取了源但发现语法错误，结果是一个部分 AST
// (包含表示错误源代码片段的 [ast.Bad]* 节点)。
// 多个错误通过按源位置排序的 scanner.ErrorList 返回。
func ParseFile(fset *token.FileSet, filename string, src any, mode Mode) (f *ast.File, err error) {
	if fset == nil {
		panic("parser.ParseFile: no token.FileSet provided (fset == nil)")
	}

	// get source
	text, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	file := fset.AddFile(filename, -1, len(text))

	var p parser
	defer func() {
		if e := recover(); e != nil {
			// resume same panic if it's not a bailout
			bail, ok := e.(bailout)
			if !ok {
				panic(e)
			} else if bail.msg != "" {
				p.errors.Add(p.file.Position(bail.pos), bail.msg)
			}
		}

		// set result values
		if f == nil {
			// source is not a valid Go source file - satisfy
			// ParseFile API and return a valid (but) empty
			// *ast.File
			f = &ast.File{
				Name:  new(ast.Ident),
				Scope: ast.NewScope(nil),
			}
		}

		// Ensure the start/end are consistent,
		// whether parsing succeeded or not.
		f.FileStart = token.Pos(file.Base())
		f.FileEnd = file.End()

		p.errors.Sort()
		err = p.errors.Err()
	}()

	// parse source
	p.init(file, text, mode)
	f = p.parseFile()

	return
}

// ParseDir calls [ParseFile] for all files with names ending in ".go" in the
// directory specified by path and 返回一个map of package name -> package
// AST with all the packages found.
//
// If filter != nil, only the files with [fs.FileInfo] entries passing through
// the filter (and ending in ".go") are considered. The mode bits are passed
// to [ParseFile] unchanged. Position information is recorded in fset, which
// must not be nil.
//
// If the directory couldn't be read, a nil map and the respective error are
// returned. If a parse error occurred, a non-nil but incomplete map and the
// first error encountered are returned.
//
// Deprecated: ParseDir does not consider build tags when associating
// files with packages. For precise information about the relationship
// between packages and files, use golang.org/x/tools/go/packages,
// which can also optionally parse and type-check the files too.
func ParseDir(fset *token.FileSet, path string, filter func(fs.FileInfo) bool, mode Mode) (pkgs map[string]*ast.Package, first error) {
	list, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	pkgs = make(map[string]*ast.Package)
	for _, d := range list {
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".go") {
			continue
		}
		if filter != nil {
			info, err := d.Info()
			if err != nil {
				return nil, err
			}
			if !filter(info) {
				continue
			}
		}
		filename := filepath.Join(path, d.Name())
		if src, err := ParseFile(fset, filename, nil, mode); err == nil {
			name := src.Name.Name
			pkg, found := pkgs[name]
			if !found {
				pkg = &ast.Package{
					Name:  name,
					Files: make(map[string]*ast.File),
				}
				pkgs[name] = pkg
			}
			pkg.Files[filename] = src
		} else if first == nil {
			first = err
		}
	}

	return
}

// ParseExprFrom 是一个 convenience function for parsing an expression.
// The arguments have the same meaning as for [ParseFile], but the source must
// be a valid Go (type or value) expression. Specifically, fset must not
// be nil.
//
// If the source couldn't be read, the returned AST is nil and the error
// 指示 specific failure. If the source was read but syntax
// errors were found, the result 是一个 partial AST (with [ast.Bad]* nodes
// representing the fragments of erroneous source code). Multiple errors
// are returned via a scanner.ErrorList which is sorted by source position.
func ParseExprFrom(fset *token.FileSet, filename string, src any, mode Mode) (expr ast.Expr, err error) {
	if fset == nil {
		panic("parser.ParseExprFrom: no token.FileSet provided (fset == nil)")
	}

	// get source
	text, err := readSource(filename, src)
	if err != nil {
		return nil, err
	}

	var p parser
	defer func() {
		if e := recover(); e != nil {
			// resume same panic if it's not a bailout
			bail, ok := e.(bailout)
			if !ok {
				panic(e)
			} else if bail.msg != "" {
				p.errors.Add(p.file.Position(bail.pos), bail.msg)
			}
		}
		p.errors.Sort()
		err = p.errors.Err()
	}()

	// parse expr
	file := fset.AddFile(filename, -1, len(text))
	p.init(file, text, mode)
	expr = p.parseRhs()

	// If a semicolon was inserted, consume it;
	// report an error if there's more tokens.
	if p.tok == token.SEMICOLON && p.lit == "\n" {
		p.next()
	}
	p.expect(token.EOF)

	return
}

// ParseExpr 是一个 convenience function for obtaining the AST of an expression x.
// The position information recorded in the AST is undefined. The filename used
// in error messages 是 empty string.
//
// If syntax errors were found, the result 是一个 partial AST (with [ast.Bad]* nodes
// representing the fragments of erroneous source code). Multiple errors are
// returned via a scanner.ErrorList which is sorted by source position.
func ParseExpr(x string) (ast.Expr, error) {
	return ParseExprFrom(token.NewFileSet(), "", []byte(x), 0)
}
