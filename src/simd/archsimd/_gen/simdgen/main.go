// 版权所有 2025 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// simdgen 是一个生成 Go <-> asm SIMD 映射的实验性工具。
//
// 用法：simdgen [-xedPath=path] [-q=query] input.yaml...
//
// 如果提供了 -xedPath，则其中一个输入是从指定路径的 Intel XED 数据
// 生成的操作码定义的总和。
//
// 如果提供了输入 YAML 文件，每个文件都作为输入值读取。请参阅
// [unify.Closure.UnmarshalYAML] 或 "go doc unify.Closure.UnmarshalYAML"
// 了解这些文件的格式。
//
// TODO: 示例定义和值。
//
// 该命令对所有输入进行统一，并打印此统一的所有可能结果。
//
// 如果提供了 -q 标志，其字符串值将被解析为一个值，并作为统一的另一个输入。
// 这旨在作为"查询"结果的一种方式，通常是将其缩小到结果的一小部分。
//
// 典型用法：
//
//	go run . -xedPath $XEDPATH *.yaml
//
// 要仅查看从 XED 生成的定义，运行：
//
//	go run . -xedPath $XEDPATH
//
// （这是因为如果只有一个输入，就没有东西可以与之统一，
// 所以结果就是它本身。）
//
// 要仅查看 VPADDQ 的定义：
//
//	go run . -xedPath $XEDPATH -q '{asm: VPADDQ}'
//
// simdgen 还可以生成 SIMD 映射的 Go 定义：
// 要将 go 文件生成到 go root，运行：
//
//	go run . -xedPath $XEDPATH -o godefs -goroot $PATH/TO/go go.yaml categories.yaml types.yaml
//
// types.yaml 已经编写好了，它指定了向量的形状。
// categories.yaml 和 go.yaml 包含与 types.yaml 和 XED
// 数据统一的定义，你可以在 ops/AddSub/ 中找到示例。
//
// 生成 Go 定义时，simdgen 做了 3 个"魔法"：
// - 它将掩码操作（设置了 op 的 [Masked] 字段）拆分为常量和非常量：
//   - 一个是正常的掩码操作，即原始操作
//   - 另一个将其掩码操作数的 [Const] 字段设置为 "K0"。
//   - 这样用户就不需要提供单独的 "K0" 掩码操作定义。
//
// - 它对具有重复项的内置函数名称进行去重：
//   - 如果有两个共享相同签名的操作，一个是 AVX512，另一个
//     是 AVX512 之前的，则会选择另一个。
//   - 当某些操作在 AVX512 之前和之后都有定义时，这种情况经常发生。
//     这样用户就不需要为 AVX512 对应物提供单独的 "K0" 操作。
//
// - 它将 op 的 [ConstImm] 字段复制到其立即数操作数的 [Const] 字段。
//   - 这样用户就不需要提供冗长的 op 定义，而只需要
//     常量立即数字段不同。这对于减少带有立即数控制谓词的
//     比较的冗长性很有用。
//
// 这 3 个魔法可以通过启用 -nosplitmask、-nodedup 或
// -noconstimmporting 标志来禁用。
//
// simdgen 目前仅支持 amd64，-arch=$OTHERARCH 将触发致命错误。
package main

// 重要 TODO：
//
// - 这可能产生重复项，这也可能导致环境合并效率降低。
// 添加哈希并使用它进行去重。注意这在调试跟踪中的显示方式，
// 因为如果我们不显示它的发生可能会使事情变得混乱。
//
// - 我需要 Closure、Value 和 Domain 吗？感觉我应该只需要两种类型。

import (
	"cmp"
	"flag"
	"fmt"
	"log"
	"maps"
	"os"
	"path/filepath"
	"runtime/pprof"
	"slices"
	"strings"

	"simd/archsimd/_gen/unify"

	"gopkg.in/yaml.v3"
)

var (
	xedPath               = flag.String("xedPath", "", "load XED datafiles from `path`")
	flagQ                 = flag.String("q", "", "query: read `def` as another input (skips final validation)")
	flagO                 = flag.String("o", "yaml", "output type: yaml, godefs (generate definitions into a Go source tree")
	flagGoDefRoot         = flag.String("goroot", ".", "the path to the Go dev directory that will receive the generated files")
	FlagNoDedup           = flag.Bool("nodedup", false, "disable deduplicating godefs of 2 qualifying operations from different extensions")
	FlagNoConstImmPorting = flag.Bool("noconstimmporting", false, "disable const immediate porting from op to imm operand")
	FlagArch              = flag.String("arch", "amd64", "the target architecture")

	Verbose = flag.Bool("v", false, "verbose")

	flagDebugXED   = flag.Bool("debug-xed", false, "show XED instructions")
	flagDebugUnify = flag.Bool("debug-unify", false, "print unification trace")
	flagDebugHTML  = flag.String("debug-html", "", "write unification trace to `file.html`")
	FlagReportDup  = flag.Bool("reportdup", false, "report the duplicate godefs")

	flagCPUProfile = flag.String("cpuprofile", "", "write CPU profile to `file`")
	flagMemProfile = flag.String("memprofile", "", "write memory profile to `file`")
)

const simdPackage = "simd/archsimd"

func main() {
	flag.Parse()

	if *flagCPUProfile != "" {
		f, err := os.Create(*flagCPUProfile)
		if err != nil {
			log.Fatalf("-cpuprofile: %s", err)
		}
		defer f.Close()
		pprof.StartCPUProfile(f)
		defer pprof.StopCPUProfile()
	}
	if *flagMemProfile != "" {
		f, err := os.Create(*flagMemProfile)
		if err != nil {
			log.Fatalf("-memprofile: %s", err)
		}
		defer func() {
			pprof.WriteHeapProfile(f)
			f.Close()
		}()
	}

	var inputs []unify.Closure

	if *FlagArch != "amd64" {
		log.Fatalf("simdgen only supports amd64")
	}

	// 将 XED 加载到定义集中。
	if *xedPath != "" {
		xedDefs := loadXED(*xedPath)
		inputs = append(inputs, unify.NewSum(xedDefs...))
	}

	// 加载查询。
	if *flagQ != "" {
		r := strings.NewReader(*flagQ)
		def, err := unify.Read(r, "<query>", unify.ReadOpts{})
		if err != nil {
			log.Fatalf("parsing -q: %s", err)
		}
		inputs = append(inputs, def)
	}

	// 加载定义文件。
	must := make(map[*unify.Value]struct{})
	for _, path := range flag.Args() {
		defs, err := unify.ReadFile(path, unify.ReadOpts{})
		if err != nil {
			log.Fatal(err)
		}
		inputs = append(inputs, defs)

		if filepath.Base(path) == "go.yaml" {
			// 这些必须全部在最终结果中使用
			for def := range defs.Summands() {
				must[def] = struct{}{}
			}
		}
	}

	// 准备统一
	if *flagDebugUnify {
		unify.Debug.UnifyLog = os.Stderr
	}
	if *flagDebugHTML != "" {
		f, err := os.Create(*flagDebugHTML)
		if err != nil {
			log.Fatal(err)
		}
		unify.Debug.HTML = f
		defer f.Close()
	}

	// 统一！
	unified, err := unify.Unify(inputs...)
	if err != nil {
		log.Fatal(err)
	}

	// 验证结果。
	//
	// 如果这是命令行查询，则不验证，因为这往往会消除许多必需的定义，
	// 并且用于定义可能无法枚举的情况。
	if *flagQ == "" && len(must) > 0 {
		validate(unified, must)
	}

	// 打印结果。
	switch *flagO {
	case "yaml":
		// 生成一个看起来像编码切片的结果，但是流式输出。
		fmt.Println("!sum")
		var val1 [1]*unify.Value
		for val := range unified.All() {
			val1[0] = val
			// 我们每次都必须创建一个新的编码器，否则它会在每个对象之间
			// 打印文档分隔符。
			enc := yaml.NewEncoder(os.Stdout)
			if err := enc.Encode(val1); err != nil {
				log.Fatal(err)
			}
			enc.Close()
		}
	case "godefs":
		if err := writeGoDefs(*flagGoDefRoot, unified); err != nil {
			log.Fatalf("Failed writing godefs: %+v", err)
		}
	}

	if !*Verbose && *xedPath != "" {
		if operandRemarks == 0 {
			fmt.Fprintf(os.Stderr, "XED decoding generated no errors, which is unusual.\n")
		} else {
			fmt.Fprintf(os.Stderr, "XED decoding generated %d \"errors\" which is not cause for alarm, use -v for details.\n", operandRemarks)
		}
	}
}

func validate(cl unify.Closure, required map[*unify.Value]struct{}) {
	// 验证：
	// 1. 所有最终定义都是精确的
	// 2. 所有必需的定义都被使用
	for def := range cl.All() {
		if _, ok := def.Domain.(unify.Def); !ok {
			fmt.Fprintf(os.Stderr, "%s: expected Def, got %T\n", def.PosString(), def.Domain)
			continue
		}

		if !def.Exact() {
			fmt.Fprintf(os.Stderr, "%s: def not reduced to an exact value, why is %s:\n", def.PosString(), def.WhyNotExact())
			fmt.Fprintf(os.Stderr, "\t%s\n", strings.ReplaceAll(def.String(), "\n", "\n\t"))
		}

		for root := range def.Provenance() {
			delete(required, root)
		}
	}
	// 报告未使用的定义
	unused := slices.SortedFunc(maps.Keys(required),
		func(a, b *unify.Value) int {
			return cmp.Or(
				cmp.Compare(a.Pos().Path, b.Pos().Path),
				cmp.Compare(a.Pos().Line, b.Pos().Line),
			)
		})
	for _, def := range unused {
		// TODO: Can we say anything more actionable? This is always a problem
		// with unification: if it fails, it's very hard to point a finger at
		// any particular reason. We could go back and try unifying this again
		// with each subset of the inputs (starting with individual inputs) to
		// at least say "it doesn't unify with anything in x.yaml". That's a lot
		// of work, but if we have trouble debugging unification failure it may
		// be worth it.
		fmt.Fprintf(os.Stderr, "%s: def required, but did not unify (%v)\n",
			def.PosString(), def)
	}
}
