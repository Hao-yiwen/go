// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package unify

import (
	"fmt"
	"iter"
	"reflect"
	"strings"
)

// An envSet 是环境的不可变集合，其中每个环境是
// 从 [ident] 到 [Value] 的映射。
//
// 为了保持紧凑，我们使用类似于
// 关系代数的代数表示。原子是零、单位或单个绑定：
//
// - 单个绑定 {x: v} 是一个环境集，由单个
// 环境组成，将单个 ident x 绑定到单个值 v。
//
// - Zero (0) 是空集。
//
// - Unit (1) 是一个环境集，由单个空环境
// （无绑定）组成。
//
// 从这些，我们使用和与叉积构建更复杂的环境集：
//
// - 和，E + F，只是两个环境集的并集：E ∪ F
//
// - 叉积，E ⨯ F，是两个环境的笛卡尔积
// 集，接着连接每对环境：{e ⊕ f | (e, f) ∊ E ⨯ F}
//
// 两个环境的连接，e ⊕ f，是包含所有的环境
// e 或 f 中的绑定。为了检测错误，如果
// 标识符在 e 和 f 中都绑定是错误的（但是，
// 请参见下面我们可以做什么不同的地方）。
//
// 环境集形成可交换的半环，因此遵守通常的
// 可交换半环规则：
//
//	e + 0 = e
//	e ⨯ 0 = 0
//	e ⨯ 1 = e
//	e + f = f + e
//	e ⨯ f = f ⨯ e
//
// 此外，环境集加法和乘法幂等
// 因为 + 和 ⨯ 本身定义为集的术语：
//
//	e + e = e
//	e ⨯ e = e
//
// # 示例
//
// 为了表示 {{x: 1, y: 1}, {x: 2, y: 2}}，我们构建两个环境并
// 将它们求和：
//
//	({x: 1} ⨯ {y: 1}) + ({x: 2} ⨯ {y: 2})
//
// 如果我们添加第三个变量 z，可以是 1 或 2，独立于 x 和 y，我们
// 得到四个逻辑环境：
//
//	{x: 1, y: 1, z: 1}
//	{x: 2, y: 2, z: 1}
//	{x: 1, y: 1, z: 2}
//	{x: 2, y: 2, z: 2}
//
// 这可以表示为所有四个环境的和，但因为 z 是
// 独立的，我们可以使用更紧凑的表示：
//
//	(({x: 1} ⨯ {y: 1}) + ({x: 2} ⨯ {y: 2})) ⨯ ({z: 1} + {z: 2})
//
// # 广义叉积
//
// 虽然叉积当前仅限于不相交的环境，但我们
// 可以将连接两个环境的定义推广为：
//
//	{xₖ: vₖ} ⊕ {xₖ: wₖ} = {xₖ: vₖ ∩ wₖ}（其中未绑定的 ident 绑定到 [Top] 值，⟙）
//
// 其中 v ∩ w 是 v 和 w 的统一。这本身可以粗化为
//
//	v ∩ w = v if w = ⟙
//	      = w if v = ⟙
//	      = v if v = w
//	      = 0 otherwise
//
// 我们可以使用此规则来实现替换。例如，E ⨯ {x: 1}
// 将环境集 E 缩小到仅 x 绑定到 1 的环境。但
// 我们目前不这样做。
type envSet struct {
	root *envExpr
}

type envExpr struct {
	// TODO: 这个树形数据结构可能不理想，因为它
	// 涉及大量的遍历来查找事物，我们通常必须进行深度
	// 重写以分区。一些扁平化数组风格的
	// 表示会更好吗，可能与 ident 使用索引结合？
	// 我们甚至可以将其与不可变数组抽象（ala
	// Clojure）相结合，可以实现更高效的构造操作。

	kind envExprKind

	// For envBinding
	id  *ident
	val *Value

	// 对于和或积。Len 必须 >= 2，没有元素可以
	// 与此节点具有相同的类型。
	operands []*envExpr
}

type envExprKind byte

const (
	envZero envExprKind = iota
	envUnit
	envProduct
	envSum
	envBinding
)

var (
	// topEnv 是 [envSet] 的单位值（乘法恒等式）。
	topEnv = envSet{envExprUnit}
	// bottomEnv 是 [envSet] 的零值（加法恒等式）。
	bottomEnv = envSet{envExprZero}

	envExprZero = &envExpr{kind: envZero}
	envExprUnit = &envExpr{kind: envUnit}
)

// bind 将 id 绑定到 e 中的每个 vals。
//
// 如果 id 已在 e 中绑定，则会崩溃。
//
// 环境通常初始化时通过从 [topEnv] 开始
// 并一次或多次调用 bind 构造。
func (e envSet) bind(id *ident, vals ...*Value) envSet {
	if e.isEmpty() {
		return bottomEnv
	}

	// TODO: 如果任何 vals 是 _，我们应该删除该 val 吗？我们
	// 对于 id 缺失于 e 是否意味着 id 无效或
	// 意味着 id 是 _ 有些不一致。

	// 检查 id 不在 e 中。
	for range e.root.bindings(id) {
		panic("id " + id.name + " already present in environment")
	}

	// 创建所有值的和。
	bindings := make([]*envExpr, 0, 1)
	for _, val := range vals {
		bindings = append(bindings, &envExpr{kind: envBinding, id: id, val: val})
	}

	// 将其乘入。
	return envSet{newEnvExprProduct(e.root, newEnvExprSum(bindings...))}
}

func (e envSet) isEmpty() bool {
	return e.root.kind == envZero
}

// bindings 产生 e 中具有给定 id 的所有 [envBinding] 节点。如果 id 为 nil，
// 则产生所有绑定节点。
func (e *envExpr) bindings(id *ident) iter.Seq[*envExpr] {
	// 这只是一个前序遍历，碰巧这是我们唯一需要的
	// 前序遍历。
	return func(yield func(*envExpr) bool) {
		var rec func(e *envExpr) bool
		rec = func(e *envExpr) bool {
			if e.kind == envBinding && (id == nil || e.id == id) {
				if !yield(e) {
					return false
				}
			}
			for _, o := range e.operands {
				if !rec(o) {
					return false
				}
			}
			return true
		}
		rec(e)
	}
}

// newEnvExprProduct 从 exprs 构造一个积节点，进行
// 简化。它不检查绑定是否不相交。
func newEnvExprProduct(exprs ...*envExpr) *envExpr {
	factors := make([]*envExpr, 0, 2)
	for _, expr := range exprs {
		switch expr.kind {
		case envZero:
			return envExprZero
		case envUnit:
			// 对积无影响
		case envProduct:
			factors = append(factors, expr.operands...)
		default:
			factors = append(factors, expr)
		}
	}

	if len(factors) == 0 {
		return envExprUnit
	} else if len(factors) == 1 {
		return factors[0]
	}
	return &envExpr{kind: envProduct, operands: factors}
}

// newEnvExprSum 从 exprs 构造一个和节点，进行简化。
func newEnvExprSum(exprs ...*envExpr) *envExpr {
	// TODO: 如果所有 envs 都是积（或绑定），对任何公共项进行因式分解。
	// 例如，x * y + x * z ==> x * (y + z)。对于绑定
	// 项很容易做到，但对于更一般的项更难。

	var have smallSet[*envExpr]
	terms := make([]*envExpr, 0, 2)
	for _, expr := range exprs {
		switch expr.kind {
		case envZero:
			// 对和无影响
		case envSum:
			for _, expr1 := range expr.operands {
				if have.Add(expr1) {
					terms = append(terms, expr1)
				}
			}
		default:
			if have.Add(expr) {
				terms = append(terms, expr)
			}
		}
	}

	if len(terms) == 0 {
		return envExprZero
	} else if len(terms) == 1 {
		return terms[0]
	}
	return &envExpr{kind: envSum, operands: terms}
}

func crossEnvs(env1, env2 envSet) envSet {
	// 确认 envs 有不相交的 ident。
	var ids1 smallSet[*ident]
	for e := range env1.root.bindings(nil) {
		ids1.Add(e.id)
	}
	for e := range env2.root.bindings(nil) {
		if ids1.Has(e.id) {
			panic(fmt.Sprintf("%s bound on both sides of cross-product", e.id.name))
		}
	}

	return envSet{newEnvExprProduct(env1.root, env2.root)}
}

func unionEnvs(envs ...envSet) envSet {
	exprs := make([]*envExpr, len(envs))
	for i := range envs {
		exprs[i] = envs[i].root
	}
	return envSet{newEnvExprSum(exprs...)}
}

// envPartition 是 env 的一个子集，其中 id 在所有
// 确定性环境中绑定到值。
type envPartition struct {
	id    *ident
	value *Value
	env   envSet
}

// partitionBy 按 id 的不同绑定分割 e，并从每个
// 分区中移除 id。
//
// 如果 e 中有 id 未绑定的环境，它们将不会
// 反映在任何分区中。
//
// 如果 e 是底，它会崩溃，因为尝试对空环境
// 集进行分区几乎肯定表示错误。
func (e envSet) partitionBy(id *ident) []envPartition {
	if e.isEmpty() {
		// 我们可以返回零个分区，但来到这里
		// 几乎肯定表示错误。
		panic("cannot partition empty environment set")
	}

	// 为 id 的每个值发出一个分区。
	var seen smallSet[*Value]
	var parts []envPartition
	for n := range e.root.bindings(id) {
		if !seen.Add(n.val) {
			// 已为此值发出分区。
			continue
		}

		parts = append(parts, envPartition{
			id:    id,
			value: n.val,
			env:   envSet{e.root.substitute(id, n.val)},
		})
	}

	return parts
}

// substitute 用 1 替换 id 到 val 的绑定，用 0 替换 id 到任何
// 其他值的绑定，并简化结果。
func (e *envExpr) substitute(id *ident, val *Value) *envExpr {
	switch e.kind {
	default:
		panic("bad kind")

	case envZero, envUnit:
		return e

	case envBinding:
		if e.id != id {
			return e
		} else if e.val != val {
			return envExprZero
		} else {
			return envExprUnit
		}

	case envProduct, envSum:
		// 替换每个操作数。有时，这不会改变任何内容，因此我们
		// 延迟构建新操作数列表。
		var nOperands []*envExpr
		for i, op := range e.operands {
			nOp := op.substitute(id, val)
			if nOperands == nil && op != nOp {
				// 操作数分歧；初始化 nOperands。
				nOperands = make([]*envExpr, 0, len(e.operands))
				nOperands = append(nOperands, e.operands[:i]...)
			}
			if nOperands != nil {
				nOperands = append(nOperands, nOp)
			}
		}
		if nOperands == nil {
			// 没有改变。
			return e
		}
		if e.kind == envProduct {
			return newEnvExprProduct(nOperands...)
		} else {
			return newEnvExprSum(nOperands...)
		}
	}
}

// A smallSet 是一个为小规模堆栈分配优化的集合。
type smallSet[T comparable] struct {
	array [32]T
	n     int

	m map[T]struct{}
}

// Has 返回 val 是否在集合中。
func (s *smallSet[T]) Has(val T) bool {
	arr := s.array[:s.n]
	for i := range arr {
		if arr[i] == val {
			return true
		}
	}
	_, ok := s.m[val]
	return ok
}

// Add 将 val 添加到集合中，如果添加了（之前不存在）则返回 true。
// 存在）。
func (s *smallSet[T]) Add(val T) bool {
	// 测试存在。
	if s.Has(val) {
		return false
	}

	// 添加它
	if s.n < len(s.array) {
		s.array[s.n] = val
		s.n++
	} else {
		if s.m == nil {
			s.m = make(map[T]struct{})
		}
		s.m[val] = struct{}{}
	}
	return true
}

type ident struct {
	_    [0]func() // Not comparable (only compare *ident)
	name string
}

type Var struct {
	id *ident
}

func (d Var) Exact() bool {
	// 这些不能出现在具体的 Values 中。
	panic("Exact called on non-concrete Value")
}

func (d Var) WhyNotExact() string {
	// 这些不能出现在具体的 Values 中。
	return "WhyNotExact called on non-concrete Value"
}

func (d Var) decode(rv reflect.Value) error {
	return &inexactError{"var", rv.Type().String()}
}

func (d Var) unify(w *Value, e envSet, swap bool, uf *unifier) (Domain, envSet, error) {
	// TODO: 输入中的 !sums 中的 Vars 可能有大量的值。
	// 统一这些可能会更有效，一些索引用于
	// 我们可以提取的任何精确值，如精确为字符串的 Def 字段。
	// 也许我们尝试生成一个是/否/可能匹配数组，然后我们只需
	// 对可能的进行更深层次的评估。我们可能可以缓存这个
	// 在 envTerm 上。特殊处理 Var/Var 统一也可能有帮助
	// 选择要索引还是枚举哪一个。

	if vd, ok := w.Domain.(Var); ok && d.id == vd.id {
		// 统一 $x 与 $x 得到 $x。如果我们下降到这个，我们会有
		// 问题，因为我们从环境中删除 $x 以保持自己
		// 诚实，然后在另一边找不到它。
		//
		// TODO: 我不确定这是否是正确的修复。
		return vd, e, nil
	}

	// 我们需要在每个可能的环境中将 w 与 d 的值统一。我们
	// 可以通过按 d 的值对环境进行分组来节省一些工作，因为
	// 这里会有很多冗余。
	var nEnvs []envSet
	envParts := e.partitionBy(d.id)
	for i, envPart := range envParts {
		exit := uf.enterVar(d.id, i)
		// 每个分支在逻辑上获得初始环境的自己的副本
		// （缩小到仅此变量的绑定），每个分支
		// 可能导致对该起始环境的不同更改。
		res, e2, err := w.unify(envPart.value, envPart.env, swap, uf)
		exit.exit()
		if err != nil {
			return nil, envSet{}, err
		}
		if res.Domain == nil {
			// 这个分支完全未能统一，所以它消失了。
			continue
		}
		nEnv := e2.bind(d.id, res)
		nEnvs = append(nEnvs, nEnv)
	}

	if len(nEnvs) == 0 {
		// 所有分支失败
		return nil, bottomEnv, nil
	}

	// 这的效果完全在环境中捕获。我们可以返回
	// 相同的 Bind 节点。
	return d, unionEnvs(nEnvs...), nil
}

// An identPrinter 将 [ident] 映射到唯一的字符串名称。
type identPrinter struct {
	ids   map[*ident]string
	idGen map[string]int
}

func (p *identPrinter) unique(id *ident) string {
	if p.ids == nil {
		p.ids = make(map[*ident]string)
		p.idGen = make(map[string]int)
	}

	name, ok := p.ids[id]
	if !ok {
		gen := p.idGen[id.name]
		p.idGen[id.name]++
		if gen == 0 {
			name = id.name
		} else {
			name = fmt.Sprintf("%s#%d", id.name, gen)
		}
		p.ids[id] = name
	}

	return name
}

func (p *identPrinter) slice(ids []*ident) string {
	var strs []string
	for _, id := range ids {
		strs = append(strs, p.unique(id))
	}
	return fmt.Sprintf("[%s]", strings.Join(strs, ", "))
}
