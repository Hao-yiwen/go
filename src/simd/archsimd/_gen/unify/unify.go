// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package unify 实现结构化值的统一。
//
// A [Value] 代表可能无限的具体值集合，其中值可以是字符串 ([String])、
// 值的元组 ([Tuple]) 或字符串键映射值称为"def"([Def])。这些集合可以进一步
// 由变量 ([Var]) 约束。[Value] 与变量的绑定组合是 [Closure]。
//
// [Unify] 找到满足两个或多个其他 [Closure] 的 [Closure]。这可以被认为是
// 这些 Closure 的值所代表的集合的交集，或这些 Closure 的最大下界/下确界。
// 如果不存在这样的 Closure，统一的结果是"底"或空集。
//
// # 示例
//
// 正则表达式"a*"是零个或多个"a"的字符串的无限集合。"a*"可以与"a"或"aa"或"aaa"统一，
// 结果分别为"a"、"aa"或"aaa"。但是，尝试将"a*"与"b"统一失败
// 因为没有满足两者的值。
//
// Sums 直接表达集合。例如，!sum [a, b] 是包含"a"和"b"的集合。
// 将其与 !sum [b, c] 统一会得到"b"。这也使得
// 很容易演示统一不一定是单个具体值。例如，
// 统一 !sum [a, b, c] 与 !sum [b, c, d] 会得到两个具体值："b"和"c"。
//
// 特殊值 _ 或"top"表示所有可能的值。将 _ 与
// 任何值 x 统一的结果是 x。
//
// 统一复合值——元组和 def——会统一它们的元素。
//
// 值 [a*, aa] 是一个无限的元组集合。如果我们将其与
// 值 [aaa, a*] 统一，满足两者的唯一可能值是 [aaa, aa]。
// 同样，这是由这两个值描述的集合的交集。
//
// Def 类似于元组，但它们由字符串索引且没有
// 固定长度。例如，{x: a, y: b} 是一个有两个字段的 def。任何字段
// 如果在 def 中未提及，则隐式为 top。因此，将其与 {y: b, z:
// c} 统一会得到 {x: a, y: b, z: c}。
//
// 变量约束值。例如，值 [$x, $x] 代表所有
// 第一个和第二个值相同的元组，但不会以其他方式
// 约束该值。因此，此集合包括 [a, a] 以及 [[b, c, d],
// [b, c, d]]，但不包括 [a, b]。
//
// Sums 在内部实现为同时绑定到
// sum 的所有值的新鲜变量。即 !sum [a, b] 实际上是 $var（其中
// var 是某个新鲜名称），在环境 $var=a | $var=b 下关闭。
package unify

import (
	"errors"
	"fmt"
	"slices"
)

// Unify 计算满足每个输入 Closure 的 Closure。如果不存在这样的
// Closure，则返回底。
func Unify(closures ...Closure) (Closure, error) {
	if len(closures) == 0 {
		return Closure{topValue, topEnv}, nil
	}

	var trace *tracer
	if Debug.UnifyLog != nil || Debug.HTML != nil {
		trace = &tracer{
			logw:     Debug.UnifyLog,
			saveTree: Debug.HTML != nil,
		}
	}

	unified := closures[0]
	for _, c := range closures[1:] {
		var err error
		uf := newUnifier()
		uf.tracer = trace
		e := crossEnvs(unified.env, c.env)
		unified.val, unified.env, err = unified.val.unify(c.val, e, false, uf)
		if Debug.HTML != nil {
			uf.writeHTML(Debug.HTML)
		}
		if err != nil {
			return Closure{}, err
		}
	}

	return unified, nil
}

type unifier struct {
	*tracer
}

func newUnifier() *unifier {
	return &unifier{}
}

// errDomains 是在 unify 和 unify1 之间使用的哨兵错误，表示
// unify1 无法统一两个值的域。
var errDomains = errors.New("cannot unify domains")

func (v *Value) unify(w *Value, e envSet, swap bool, uf *unifier) (*Value, envSet, error) {
	if swap {
		// 将值按顺序排列。这恰好是一个方便的控制点
		// 来执行此操作。
		v, w = w, v
	}

	uf.traceUnify(v, w, e)

	d, e2, err := v.unify1(w, e, false, uf)
	if err == errDomains {
		// 尝试另一个顺序。
		d, e2, err = w.unify1(v, e, true, uf)
		if err == errDomains {
			// 好的，我们真的不能统一这些。
			err = fmt.Errorf("cannot unify %T (%s) and %T (%s): kind mismatch", v.Domain, v.PosString(), w.Domain, w.PosString())
		}
	}
	if err != nil {
		uf.traceDone(nil, envSet{}, err)
		return nil, envSet{}, err
	}
	res := unified(d, v, w)
	uf.traceDone(res, e2, nil)
	if d == nil {
		// 双重检查底值是否也有底环境。
		if !e2.isEmpty() {
			panic("bottom Value has non-bottom environment")
		}
	}

	return res, e2, nil
}

func (v *Value) unify1(w *Value, e envSet, swap bool, uf *unifier) (Domain, envSet, error) {
	// TODO: 如果出错，请将位置信息附加到它。

	vd, wd := v.Domain, w.Domain

	// 底返回底，并消除所有可能的环境。
	if vd == nil || wd == nil {
		return nil, bottomEnv, nil
	}

	// Top 总是返回另一个。
	if _, ok := vd.(Top); ok {
		return wd, e, nil
	}

	// 变量
	if vd, ok := vd.(Var); ok {
		return vd.unify(w, e, swap, uf)
	}

	// 复合值
	if vd, ok := vd.(Def); ok {
		if wd, ok := wd.(Def); ok {
			return vd.unify(wd, e, swap, uf)
		}
	}
	if vd, ok := vd.(Tuple); ok {
		if wd, ok := wd.(Tuple); ok {
			return vd.unify(wd, e, swap, uf)
		}
	}

	// 标量值
	if vd, ok := vd.(String); ok {
		if wd, ok := wd.(String); ok {
			res := vd.unify(wd)
			if res == nil {
				e = bottomEnv
			}
			return res, e, nil
		}
	}

	return nil, envSet{}, errDomains
}

func (d Def) unify(o Def, e envSet, swap bool, uf *unifier) (Domain, envSet, error) {
	out := Def{fields: make(map[string]*Value)}

	// 检查 d 的键与 o 的键。
	for key, dv := range d.All() {
		ov, ok := o.fields[key]
		if !ok {
			// ov 隐式为 Top。绕过统一。
			out.fields[key] = dv
			continue
		}
		exit := uf.enter("%s", key)
		res, e2, err := dv.unify(ov, e, swap, uf)
		exit.exit()
		if err != nil {
			return nil, envSet{}, err
		} else if res.Domain == nil {
			// 不匹配。
			return nil, bottomEnv, nil
		}
		out.fields[key] = res
		e = e2
	}
	// 检查我们还没有检查过的 o 的键。这些都隐式匹配
	// 因为我们知道 d 中对应的字段都是 Top。
	for key, dv := range o.All() {
		if _, ok := d.fields[key]; !ok {
			out.fields[key] = dv
		}
	}
	return out, e, nil
}

func (v Tuple) unify(w Tuple, e envSet, swap bool, uf *unifier) (Domain, envSet, error) {
	if v.repeat != nil && w.repeat != nil {
		// 由于我们延迟生成这些的内容，没有太多我们
		// 可以做的，只是将它们放在列表上稍后统一。
		return Tuple{repeat: concat(v.repeat, w.repeat)}, e, nil
	}

	// 展开任何重复的元组。
	tuples := make([]Tuple, 0, 2)
	if v.repeat == nil {
		tuples = append(tuples, v)
	} else {
		v2, e2 := v.doRepeat(e, len(w.vs))
		tuples = append(tuples, v2...)
		e = e2
	}
	if w.repeat == nil {
		tuples = append(tuples, w)
	} else {
		w2, e2 := w.doRepeat(e, len(v.vs))
		tuples = append(tuples, w2...)
		e = e2
	}

	// 现在统一所有元组（通常这将只是 2 个元组）
	out := tuples[0]
	for _, t := range tuples[1:] {
		if len(out.vs) != len(t.vs) {
			uf.logf("tuple length mismatch")
			return nil, bottomEnv, nil
		}
		zs := make([]*Value, len(out.vs))
		for i, v1 := range out.vs {
			exit := uf.enter("%d", i)
			z, e2, err := v1.unify(t.vs[i], e, swap, uf)
			exit.exit()
			if err != nil {
				return nil, envSet{}, err
			} else if z.Domain == nil {
				return nil, bottomEnv, nil
			}
			zs[i] = z
			e = e2
		}
		out = Tuple{vs: zs}
	}

	return out, e, nil
}

// doRepeat 从重复的元组创建固定长度的元组。调用者应该
// 统一返回的元组。
func (v Tuple) doRepeat(e envSet, n int) ([]Tuple, envSet) {
	res := make([]Tuple, len(v.repeat))
	for i, gen := range v.repeat {
		res[i].vs = make([]*Value, n)
		for j := range n {
			res[i].vs[j], e = gen(e)
		}
	}
	return res, e
}

// unify 与两个 [String] 的域相交。如果它可以证明这个
// 域是空的，它返回 nil（底）。
//
// TODO: 考虑将文字和正则表达式分割为两个域。
func (v String) unify(w String) Domain {
	// 统一是对称的，所以按字符串类型的顺序排列它们，以便我们只需
	// 处理一半的情况。
	if v.kind > w.kind {
		v, w = w, v
	}

	switch v.kind {
	case stringRegex:
		switch w.kind {
		case stringRegex:
			// 构造对所有正则表达式的匹配
			return String{kind: stringRegex, re: slices.Concat(v.re, w.re)}
		case stringExact:
			for _, re := range v.re {
				if !re.MatchString(w.exact) {
					return nil
				}
			}
			return w
		}
	case stringExact:
		if v.exact != w.exact {
			return nil
		}
		return v
	}
	panic("bad string kind")
}

func concat[T any](s1, s2 []T) []T {
	// 如果可能，重用 s1 或 s2。
	if len(s1) == 0 {
		return s2
	}
	return append(s1[:len(s1):len(s1)], s2...)
}
