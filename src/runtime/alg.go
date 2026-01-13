// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package runtime

import (
	"internal/abi"
	"internal/byteorder"
	"internal/cpu"
	"internal/goarch"
	"internal/runtime/sys"
	"unsafe"
)

const (
	// 在 Wasm 上我们使用 32 位哈希，参见 hash32.go。
	hashSize = (1-goarch.IsWasm)*goarch.PtrSize + goarch.IsWasm*4
	c0       = uintptr((8-hashSize)/4*2860486313 + (hashSize-4)/4*33054211828000289)
	c1       = uintptr((8-hashSize)/4*3267000013 + (hashSize-4)/4*23344194077549503)
)

func trimHash(h uintptr) uintptr {
	if goarch.IsWasm != 0 {
		// 在 Wasm 上，尽管 uintptr 是 64 位的，我们仍使用 32 位哈希。
		// memhash* 总是返回一个高 32 位为 0 的 uintptr（参见 hash32.go）。
		// 我们在其他手动计算哈希的地方也会截断哈希值，例如在 interhash 中。
		return uintptr(uint32(h))
	}
	return h
}

func memhash0(p unsafe.Pointer, h uintptr) uintptr {
	return h
}

func memhash8(p unsafe.Pointer, h uintptr) uintptr {
	return memhash(p, h, 1)
}

func memhash16(p unsafe.Pointer, h uintptr) uintptr {
	return memhash(p, h, 2)
}

func memhash128(p unsafe.Pointer, h uintptr) uintptr {
	return memhash(p, h, 16)
}

//go:nosplit
func memhash_varlen(p unsafe.Pointer, h uintptr) uintptr {
	ptr := sys.GetClosurePtr()
	size := *(*uintptr)(unsafe.Pointer(ptr + unsafe.Sizeof(h)))
	return memhash(p, h, size)
}

// 运行时变量，用于检查当前运行的处理器是否
// 实际支持基于 AES 的哈希实现所使用的指令。
var useAeshash bool

// 在 asm_*.s 中

// memhash 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/aacfactory/fns
//   - github.com/dgraph-io/ristretto
//   - github.com/minio/simdjson-go
//   - github.com/nbd-wtf/go-nostr
//   - github.com/outcaste-io/ristretto
//   - github.com/puzpuzpuz/xsync/v2
//   - github.com/puzpuzpuz/xsync/v3
//   - github.com/authzed/spicedb
//   - github.com/pingcap/badger
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname memhash
func memhash(p unsafe.Pointer, h, s uintptr) uintptr

func memhash32(p unsafe.Pointer, h uintptr) uintptr

func memhash64(p unsafe.Pointer, h uintptr) uintptr

// strhash 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/aristanetworks/goarista
//   - github.com/bytedance/sonic
//   - github.com/bytedance/go-tagexpr/v2
//   - github.com/cloudwego/dynamicgo
//   - github.com/v2fly/v2ray-core/v5
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname strhash
func strhash(p unsafe.Pointer, h uintptr) uintptr

func strhashFallback(a unsafe.Pointer, h uintptr) uintptr {
	x := (*stringStruct)(a)
	return memhashFallback(x.str, h, uintptr(x.len))
}

// 注意：由于 NaN != NaN，一个 map 可以包含任意数量的
// 以 NaN 为键的（大多是无用的）条目。
// 为了避免长哈希链，我们为 NaN 分配一个随机数作为哈希值。

func f32hash(p unsafe.Pointer, h uintptr) uintptr {
	f := *(*float32)(p)
	switch {
	case f == 0:
		return trimHash(c1 * (c0 ^ h)) // +0, -0
	case f != f:
		return trimHash(c1 * (c0 ^ h ^ uintptr(rand()))) // 任何类型的 NaN
	default:
		return memhash(p, h, 4)
	}
}

func f64hash(p unsafe.Pointer, h uintptr) uintptr {
	f := *(*float64)(p)
	switch {
	case f == 0:
		return trimHash(c1 * (c0 ^ h)) // +0, -0
	case f != f:
		return trimHash(c1 * (c0 ^ h ^ uintptr(rand()))) // 任何类型的 NaN
	default:
		return memhash(p, h, 8)
	}
}

func c64hash(p unsafe.Pointer, h uintptr) uintptr {
	x := (*[2]float32)(p)
	return f32hash(unsafe.Pointer(&x[1]), f32hash(unsafe.Pointer(&x[0]), h))
}

func c128hash(p unsafe.Pointer, h uintptr) uintptr {
	x := (*[2]float64)(p)
	return f64hash(unsafe.Pointer(&x[1]), f64hash(unsafe.Pointer(&x[0]), h))
}

func interhash(p unsafe.Pointer, h uintptr) uintptr {
	a := (*iface)(p)
	tab := a.tab
	if tab == nil {
		return h
	}
	t := tab.Type
	if t.Equal == nil {
		// 在这里检查可哈希性。我们可以在 typehash 内部做这个检查，
		// 但我们希望在错误文本中报告最顶层的类型
		// （例如，在一个包含切片类型字段的结构体中，
		// 我们希望报告结构体，而不是切片）。
		panic(errorString("hash of unhashable type " + toRType(t).string()))
	}
	if t.IsDirectIface() {
		return trimHash(c1 * typehash(t, unsafe.Pointer(&a.data), h^c0))
	} else {
		return trimHash(c1 * typehash(t, a.data, h^c0))
	}
}

// nilinterhash 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/anacrolix/stm
//   - github.com/aristanetworks/goarista
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname nilinterhash
func nilinterhash(p unsafe.Pointer, h uintptr) uintptr {
	a := (*eface)(p)
	t := a._type
	if t == nil {
		return h
	}
	if t.Equal == nil {
		// 参见上面 interhash 中的注释。
		panic(errorString("hash of unhashable type " + toRType(t).string()))
	}
	if t.IsDirectIface() {
		return trimHash(c1 * typehash(t, unsafe.Pointer(&a.data), h^c0))
	} else {
		return trimHash(c1 * typehash(t, a.data, h^c0))
	}
}

// typehash 计算地址 p 处类型 t 的对象的哈希值。
// h 是种子。
// 这个函数很少使用。大多数 map 使用固定函数（如 f32hash）
// 或编译器生成的函数（如用于 struct { x, y string } 这样的类型）进行哈希。
// 这个实现较慢但更通用，用于接口类型的哈希计算
// （从上面的 interhash 或 nilinterhash 调用）或用于
// reflect.MapOf 生成的 map 中的哈希计算（下面的 reflect_typehash）。
// 注意：这个函数必须与编译器生成的函数完全匹配。参见 issue 37716。
//
// typehash 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/puzpuzpuz/xsync/v2
//   - github.com/puzpuzpuz/xsync/v3
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname typehash
func typehash(t *_type, p unsafe.Pointer, h uintptr) uintptr {
	if t.TFlag&abi.TFlagRegularMemory != 0 {
		// 特殊处理指针大小，参见 issue 37086。
		switch t.Size_ {
		case 4:
			return memhash32(p, h)
		case 8:
			return memhash64(p, h)
		default:
			return memhash(p, h, t.Size_)
		}
	}
	switch t.Kind() {
	case abi.Float32:
		return f32hash(p, h)
	case abi.Float64:
		return f64hash(p, h)
	case abi.Complex64:
		return c64hash(p, h)
	case abi.Complex128:
		return c128hash(p, h)
	case abi.String:
		return strhash(p, h)
	case abi.Interface:
		i := (*interfacetype)(unsafe.Pointer(t))
		if len(i.Methods) == 0 {
			return nilinterhash(p, h)
		}
		return interhash(p, h)
	case abi.Array:
		a := (*arraytype)(unsafe.Pointer(t))
		for i := uintptr(0); i < a.Len; i++ {
			h = typehash(a.Elem, add(p, i*a.Elem.Size_), h)
		}
		return h
	case abi.Struct:
		s := (*structtype)(unsafe.Pointer(t))
		for _, f := range s.Fields {
			if f.Name.IsBlank() {
				continue
			}
			h = typehash(f.Typ, add(p, f.Offset), h)
		}
		return h
	default:
		// 这种情况不应该发生，因为 typehash 只应该用可比较类型调用。
		panic(errorString("hash of unhashable type " + toRType(t).string()))
	}
}

//go:linkname reflect_typehash reflect.typehash
func reflect_typehash(t *_type, p unsafe.Pointer, h uintptr) uintptr {
	return typehash(t, p, h)
}

func memequal0(p, q unsafe.Pointer) bool {
	return true
}
func memequal8(p, q unsafe.Pointer) bool {
	return *(*int8)(p) == *(*int8)(q)
}
func memequal16(p, q unsafe.Pointer) bool {
	return *(*int16)(p) == *(*int16)(q)
}
func memequal32(p, q unsafe.Pointer) bool {
	return *(*int32)(p) == *(*int32)(q)
}
func memequal64(p, q unsafe.Pointer) bool {
	return *(*int64)(p) == *(*int64)(q)
}
func memequal128(p, q unsafe.Pointer) bool {
	return *(*[2]int64)(p) == *(*[2]int64)(q)
}
func f32equal(p, q unsafe.Pointer) bool {
	return *(*float32)(p) == *(*float32)(q)
}
func f64equal(p, q unsafe.Pointer) bool {
	return *(*float64)(p) == *(*float64)(q)
}
func c64equal(p, q unsafe.Pointer) bool {
	return *(*complex64)(p) == *(*complex64)(q)
}
func c128equal(p, q unsafe.Pointer) bool {
	return *(*complex128)(p) == *(*complex128)(q)
}
func strequal(p, q unsafe.Pointer) bool {
	return *(*string)(p) == *(*string)(q)
}
func interequal(p, q unsafe.Pointer) bool {
	x := *(*iface)(p)
	y := *(*iface)(q)
	return x.tab == y.tab && ifaceeq(x.tab, x.data, y.data)
}
func nilinterequal(p, q unsafe.Pointer) bool {
	x := *(*eface)(p)
	y := *(*eface)(q)
	return x._type == y._type && efaceeq(x._type, x.data, y.data)
}
func efaceeq(t *_type, x, y unsafe.Pointer) bool {
	if t == nil {
		return true
	}
	eq := t.Equal
	if eq == nil {
		panic(errorString("comparing uncomparable type " + toRType(t).string()))
	}
	if t.IsDirectIface() {
		// 直接接口类型包括 ptr、chan、map、func，以及它们的单元素结构体/数组。
		// map 和 func 不可比较，所以它们不会到达这里。
		// ptr、chan 和单元素项可以直接使用 == 进行比较。
		return x == y
	}
	return eq(x, y)
}
func ifaceeq(tab *itab, x, y unsafe.Pointer) bool {
	if tab == nil {
		return true
	}
	t := tab.Type
	eq := t.Equal
	if eq == nil {
		panic(errorString("comparing uncomparable type " + toRType(t).string()))
	}
	if t.IsDirectIface() {
		// 参见 efaceeq 中的注释。
		return x == y
	}
	return eq(x, y)
}

// 哈希质量测试的测试适配器（参见 hash_test.go）
//
// stringHash 应该是一个内部实现细节，
// 但许多广泛使用的包通过 linkname 访问它。
// 耻辱榜上的知名成员包括：
//   - github.com/k14s/starlark-go
//
// 请勿删除或更改类型签名。
// 参见 go.dev/issue/67401。
//
//go:linkname stringHash
func stringHash(s string, seed uintptr) uintptr {
	return strhash(noescape(unsafe.Pointer(&s)), seed)
}

func bytesHash(b []byte, seed uintptr) uintptr {
	s := (*slice)(unsafe.Pointer(&b))
	return memhash(s.array, seed, uintptr(s.len))
}

func int32Hash(i uint32, seed uintptr) uintptr {
	return memhash32(noescape(unsafe.Pointer(&i)), seed)
}

func int64Hash(i uint64, seed uintptr) uintptr {
	return memhash64(noescape(unsafe.Pointer(&i)), seed)
}

func efaceHash(i any, seed uintptr) uintptr {
	return nilinterhash(noescape(unsafe.Pointer(&i)), seed)
}

func ifaceHash(i interface {
	F()
}, seed uintptr) uintptr {
	return interhash(noescape(unsafe.Pointer(&i)), seed)
}

const hashRandomBytes = goarch.PtrSize / 4 * 64

// 在 asm_{386,amd64,arm64}.s 中用于初始化哈希函数的种子
var aeskeysched [hashRandomBytes]byte

// 在 hash{32,64}.go 中用于初始化哈希函数的种子
var hashkey [4]uintptr

func alginit() {
	// 如果存在所需的指令，则安装 AES 哈希算法。
	if (GOARCH == "386" || GOARCH == "amd64") &&
		cpu.X86.HasAES && // AESENC
		cpu.X86.HasSSSE3 && // PSHUFB
		cpu.X86.HasSSE41 { // PINSR{D,Q}
		initAlgAES()
		return
	}
	if GOARCH == "arm64" && cpu.ARM64.HasAES {
		initAlgAES()
		return
	}
	for i := range hashkey {
		hashkey[i] = uintptr(bootstrapRand())
	}
}

func initAlgAES() {
	useAeshash = true
	// 使用随机数据初始化，使哈希冲突难以被人为构造。
	key := (*[hashRandomBytes / 8]uint64)(unsafe.Pointer(&aeskeysched))
	for i := range key {
		key[i] = bootstrapRand()
	}
}

// 注意：这些函数使用本机字节序执行读取。
func readUnaligned32(p unsafe.Pointer) uint32 {
	q := (*[4]byte)(p)
	if goarch.BigEndian {
		return byteorder.BEUint32(q[:])
	}
	return byteorder.LEUint32(q[:])
}

func readUnaligned64(p unsafe.Pointer) uint64 {
	q := (*[8]byte)(p)
	if goarch.BigEndian {
		return byteorder.BEUint64(q[:])
	}
	return byteorder.LEUint64(q[:])
}
