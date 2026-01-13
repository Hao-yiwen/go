// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package template

import (
	"maps"
	"reflect"
	"sync"
	"text/template/parse"
)

// common 保存了相关模板共享的信息。
type common struct {
	tmpl   map[string]*Template // 从名称到已定义模板的映射
	muTmpl sync.RWMutex         // 保护 tmpl
	option option
	// 我们使用两个映射，一个用于解析，一个用于执行。
	// 这种分离使 API 更清洁，因为它不会
	// 暴露反射给客户端。
	muFuncs    sync.RWMutex // 保护 parseFuncs 和 execFuncs
	parseFuncs FuncMap
	execFuncs  map[string]reflect.Value
}

// Template 是已解析模板的表示。*parse.Tree
// 字段仅导出以供 [html/template] 使用，应该被
// 所有其他客户端视为未导出。
type Template struct {
	name string
	*parse.Tree
	*common
	leftDelim  string
	rightDelim string
}

// New 分配一个具有给定名称的新的、未定义的模板。
func New(name string) *Template {
	t := &Template{
		name: name,
	}
	t.init()
	return t
}

// Name 返回模板的名称。
func (t *Template) Name() string {
	return t.name
}

// New 分配一个新的、未定义的模板，与给定的模板相关联，且具有相同的
// 分隔符。这种关联是传递的，允许一个模板
// 通过 {{template}} 动作调用另一个。
//
// 因为相关的模板共享底层数据，模板构造
// 无法安全地并行进行。一旦模板被构造，
// 就可以并行执行。
func (t *Template) New(name string) *Template {
	t.init()
	nt := &Template{
		name:       name,
		common:     t.common,
		leftDelim:  t.leftDelim,
		rightDelim: t.rightDelim,
	}
	return nt
}

// init 保证 t 有一个有效的 common 结构。
func (t *Template) init() {
	if t.common == nil {
		c := new(common)
		c.tmpl = make(map[string]*Template)
		c.parseFuncs = make(FuncMap)
		c.execFuncs = make(map[string]reflect.Value)
		t.common = c
	}
}

// Clone 返回模板的副本，包括所有相关的
// 模板。实际表示不被复制，但相关模板的命名空间
// 被复制，所以对副本中 [Template.Parse] 的进一步调用会添加
// 模板到副本而不是原始。Clone 可用于准备
// 公共模板并通过为其他模板添加变体定义
// 来在克隆后使用它们。
func (t *Template) Clone() (*Template, error) {
	nt := t.copy(nil)
	nt.init()
	if t.common == nil {
		return nt, nil
	}
	nt.option = t.option
	t.muTmpl.RLock()
	defer t.muTmpl.RUnlock()
	for k, v := range t.tmpl {
		if k == t.name {
			nt.tmpl[t.name] = nt
			continue
		}
		// 相关的模板共享 nt 的 common 结构。
		tmpl := v.copy(nt.common)
		nt.tmpl[k] = tmpl
	}
	t.muFuncs.RLock()
	defer t.muFuncs.RUnlock()
	maps.Copy(nt.parseFuncs, t.parseFuncs)
	maps.Copy(nt.execFuncs, t.execFuncs)
	return nt, nil
}

// copy 返回 t 的浅拷贝，common 设置为参数。
func (t *Template) copy(c *common) *Template {
	return &Template{
		name:       t.name,
		Tree:       t.Tree,
		common:     c,
		leftDelim:  t.leftDelim,
		rightDelim: t.rightDelim,
	}
}

// AddParseTree 将参数解析树与模板 t 相关联，给
// 它指定的名称。如果模板尚未定义，此树会变成
// 其定义。如果已定义且已有该名称，现有
// 定义被替换；否则创建、定义并返回新模板。
func (t *Template) AddParseTree(name string, tree *parse.Tree) (*Template, error) {
	t.init()
	t.muTmpl.Lock()
	defer t.muTmpl.Unlock()
	nt := t
	if name != t.name {
		nt = t.New(name)
	}
	// 即使 nt == t，我们也需要在 common.tmpl 映射中安装它。
	if t.associate(nt, tree) || nt.Tree == nil {
		nt.Tree = tree
	}
	return nt, nil
}

// Templates 返回与 t 相关的已定义模板的切片。
func (t *Template) Templates() []*Template {
	if t.common == nil {
		return nil
	}
	// 返回一个切片，以便我们不暴露该映射。
	t.muTmpl.RLock()
	defer t.muTmpl.RUnlock()
	m := make([]*Template, 0, len(t.tmpl))
	for _, v := range t.tmpl {
		m = append(m, v)
	}
	return m
}

// Delims 将动作分隔符设置为指定的字符串，用于
// [Template.Parse]、[Template.ParseFiles] 或 [Template.ParseGlob] 的后续调用。嵌套模板
// 定义将继承这些设置。空分隔符表示
// 对应的默认值：{{ 或 }}。
// 返回值是模板，所以调用可以链接。
func (t *Template) Delims(left, right string) *Template {
	t.init()
	t.leftDelim = left
	t.rightDelim = right
	return t
}

// Funcs 将参数映射的元素添加到模板的函数映射中。
// 必须在模板解析之前调用。
// 如果映射中的值不是具有适当返回
// 类型的函数，或者如果名称不能在模板中
// 语法上用作函数，则会 panic。
// 可以合法地覆盖映射的元素。返回值是模板，
// 所以调用可以链接。
func (t *Template) Funcs(funcMap FuncMap) *Template {
	t.init()
	t.muFuncs.Lock()
	defer t.muFuncs.Unlock()
	addValueFuncs(t.execFuncs, funcMap)
	addFuncs(t.parseFuncs, funcMap)
	return t
}

// Lookup 返回与 t 相关联的、具有给定名称的模板。
// 如果没有这样的模板或模板没有定义，则返回 nil。
func (t *Template) Lookup(name string) *Template {
	if t.common == nil {
		return nil
	}
	t.muTmpl.RLock()
	defer t.muTmpl.RUnlock()
	return t.tmpl[name]
}

// Parse 将文本作为 t 的模板体进行解析。
// 文本中的命名模板定义（{{define ...}} 或 {{block ...}} 语句）
// 定义与 t 相关的其他模板，并从
// t 本身的定义中移除。
//
// 可以在 Parse 的后续调用中重新定义模板。
// 模板定义的体仅包含空白和注释
// 被视为空，不会替换现有模板的体。
// 这允许使用 Parse 添加新的命名模板定义而无需
// 覆盖主模板体。
func (t *Template) Parse(text string) (*Template, error) {
	t.init()
	t.muFuncs.RLock()
	trees, err := parse.Parse(t.name, text, t.leftDelim, t.rightDelim, t.parseFuncs, builtins())
	t.muFuncs.RUnlock()
	if err != nil {
		return nil, err
	}
	// 将新解析的树（包括 t 的树）添加到我们的 common 结构中。
	for name, tree := range trees {
		if _, err := t.AddParseTree(name, tree); err != nil {
			return nil, err
		}
	}
	return t, nil
}

// associate 将新模板安装到与 t 相关的模板组中。
// 已知两者共享 common 结构。
// 布尔返回值报告是否将此树存储为 t.Tree。
func (t *Template) associate(new *Template, tree *parse.Tree) bool {
	if new.common != t.common {
		panic("internal error: associate not common")
	}
	if old := t.tmpl[new.name]; old != nil && parse.IsEmptyTree(tree.Root) && old.Tree != nil {
		// 如果存在该名称的模板，
		// 不要用空模板替换它。
		return false
	}
	t.tmpl[new.name] = new
	return true
}
