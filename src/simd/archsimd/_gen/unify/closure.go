// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unify

import (
	"fmt"
	"iter"
	"maps"
	"slices"
)

type Closure struct {
	val *Value
	env envSet
}

func NewSum(vs ...*Value) Closure {
	id := &ident{name: "sum"}
	return Closure{NewValue(Var{id}), topEnv.bind(id, vs...)}
}

// IsBottom 返回 c 是否包含没有值。
func (c Closure) IsBottom() bool {
	return c.val.Domain == nil
}

// Summands 返回 c 的顶级 Values。这假设 c 的顶级
// 被构造为和，主要用于调试。
func (c Closure) Summands() iter.Seq[*Value] {
	return func(yield func(*Value) bool) {
		var rec func(v *Value, env envSet) bool
		rec = func(v *Value, env envSet) bool {
			switch d := v.Domain.(type) {
			case Var:
				parts := env.partitionBy(d.id)
				for _, part := range parts {
					// It may be a sum of sums. Walk into this value.
					if !rec(part.value, part.env) {
						return false
					}
				}
				return true
			default:
				return yield(v)
			}
		}
		rec(c.val, c.env)
	}
}

// All 通过从环境替换变量来枚举 c 的所有可能的具体值。
//
// 例如，枚举此值
//
//	a: !sum [1, 2]
//	b: !sum [3, 4]
//
// 结果为
//
//   - {a: 1, b: 3}
//   - {a: 1, b: 4}
//   - {a: 2, b: 3}
//   - {a: 2, b: 4}
func (c Closure) All() iter.Seq[*Value] {
	// 为了枚举所有可能的变量下的所有具体值
	// 绑定，我们使用"非确定性延续传递风格"来
	// 实现这个。我们使用 CPS 来遍历 Value 树，线程化
	// （可能缩小的）环境通过该 CPS 跟随欧拉
	// 巡回。当环境允许多个选择时，我们调用相同的
	// 每个选择的延续。与 yield 函数类似，
	// 延续可以返回 false 来停止非确定性遍历。
	return func(yield func(*Value) bool) {
		c.val.all1(c.env, func(v *Value, e envSet) bool {
			return yield(v)
		})
	}
}

func (v *Value) all1(e envSet, cont func(*Value, envSet) bool) bool {
	switch d := v.Domain.(type) {
	default:
		panic(fmt.Sprintf("unknown domain type %T", d))

	case nil:
		return true

	case Top, String:
		return cont(v, e)

	case Def:
		fields := d.keys()
		// 我们可以重用此 parts 片，因为我们通过
		// 状态空间进行 DFS。（否则，我们必须进行一些混乱的线程化
		// 不可变类似于切片的值通过 allElt。）
		parts := make(map[string]*Value, len(fields))

		// TODO: 如果在此 Def 下没有 Vars 或 Sums，那么没有什么可以
		// 改变 Value 或 env，所以我们可以只调用 cont(v, e)。
		var allElt func(elt int, e envSet) bool
		allElt = func(elt int, e envSet) bool {
			if elt == len(fields) {
				// 从具体部分构建新 Def。Clone parts 因为
				// 我们可能在其他非确定性分支上重用它。
				nVal := newValueFrom(Def{maps.Clone(parts)}, v)
				return cont(nVal, e)
			}

			return d.fields[fields[elt]].all1(e, func(v *Value, e envSet) bool {
				parts[fields[elt]] = v
				return allElt(elt+1, e)
			})
		}
		return allElt(0, e)

	case Tuple:
		// 本质上与 Def 相同。
		if d.repeat != nil {
			// 我们对此无能为力。
			return cont(v, e)
		}
		parts := make([]*Value, len(d.vs))
		var allElt func(elt int, e envSet) bool
		allElt = func(elt int, e envSet) bool {
			if elt == len(d.vs) {
				// 从具体部分构建新元组。Clone parts 因为
				// 我们可能在其他非确定性分支上重用它。
				nVal := newValueFrom(Tuple{vs: slices.Clone(parts)}, v)
				return cont(nVal, e)
			}

			return d.vs[elt].all1(e, func(v *Value, e envSet) bool {
				parts[elt] = v
				return allElt(elt+1, e)
			})
		}
		return allElt(0, e)

	case Var:
		// 遍历此变量可以绑定的每种方式。
		for _, ePart := range e.partitionBy(d.id) {
			// d.id 在此环境分区中不再绑定。我们稍后可能
			// 在欧拉巡回中需要它，所以将其绑定回这个单一
			// 值。
			env := ePart.env.bind(d.id, ePart.value)
			if !ePart.value.all1(env, cont) {
				return false
			}
		}
		return true
	}
}
