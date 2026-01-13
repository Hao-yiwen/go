// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// elliptic 包实现了标准 NIST P-224、P-256、P-384 和 P-521
// 素域上的椭圆曲线。
//
// 此包的直接使用已弃用，除了使用 [crypto/ecdsa] 所需的 [P224]、[P256]、[P384] 和
// [P521] 值。大多数其他用途应迁移到更高效和更安全的 [crypto/ecdh]，或者
// 第三方模块以获得低级功能。
package elliptic

import (
	"io"
	"math/big"
	"sync"
)

// Curve 代表一条短形式 Weierstrass 曲线，其中 a=-3。
//
// 当输入不是曲线上的点时，Add、Double 和 ScalarMult 的行为未定义。
//
// 注意，惯例中的无穷远点 (0, 0) 不被视为在
// 曲线上，尽管它可以由 Add、Double、ScalarMult 或
// ScalarBaseMult 返回（但不由 [Unmarshal] 或 [UnmarshalCompressed] 函数返回）。
//
// 使用由 [P224]、[P256]、[P384] 和 [P521] 返回的以外的
// Curve 实现已弃用。
type Curve interface {
	// Params 返回曲线的参数。
	Params() *CurveParams

	// IsOnCurve 报告给定的 (x,y) 是否在曲线上。
	//
	// 已弃用：这是一个低级不安全的 API。对于 ECDH，使用 crypto/ecdh
	// 包。crypto/ecdh 中 NIST 曲线的 NewPublicKey 方法接受
	// 与 Unmarshal 函数相同的编码，并执行曲线上检查。
	IsOnCurve(x, y *big.Int) bool

	// Add 返回 (x1,y1) 和 (x2,y2) 的和。
	//
	// 已弃用：这是一个低级不安全的 API。
	Add(x1, y1, x2, y2 *big.Int) (x, y *big.Int)

	// Double 返回 2*(x,y)。
	//
	// 已弃用：这是一个低级不安全的 API。
	Double(x1, y1 *big.Int) (x, y *big.Int)

	// ScalarMult 返回 k*(x,y)，其中 k 是大端形式的整数。
	//
	// 已弃用：这是一个低级不安全的 API。对于 ECDH，使用 crypto/ecdh
	// 包。ScalarMult 的大多数用途可以被替换为调用 crypto/ecdh 中
	// NIST 曲线的 ECDH 方法。
	ScalarMult(x1, y1 *big.Int, k []byte) (x, y *big.Int)

	// ScalarBaseMult 返回 k*G，其中 G 是群的基点，
	// k 是大端形式的整数。
	//
	// 已弃用：这是一个低级不安全的 API。对于 ECDH，使用 crypto/ecdh
	// 包。ScalarBaseMult 的大多数用途可以被替换为调用
	// crypto/ecdh 中 PrivateKey.PublicKey 方法。
	ScalarBaseMult(k []byte) (x, y *big.Int)
}

var mask = []byte{0xff, 0x1, 0x3, 0x7, 0xf, 0x1f, 0x3f, 0x7f}

// GenerateKey 返回一个公钥/私钥对。私钥是
// 使用给定的读取器生成的，该读取器必须返回随机数据。
//
// 已弃用：对于 ECDH，使用 [crypto/ecdh] 包的 GenerateKey 方法；
// 对于 ECDSA，使用 crypto/ecdsa 包的 GenerateKey 函数。
func GenerateKey(curve Curve, rand io.Reader) (priv []byte, x, y *big.Int, err error) {
	N := curve.Params().N
	bitSize := N.BitLen()
	byteLen := (bitSize + 7) / 8
	priv = make([]byte, byteLen)

	for x == nil {
		_, err = io.ReadFull(rand, priv)
		if err != nil {
			return
		}
		// 在基础字段的大小不是整数个字节的情况下，我们必须屏蔽掉多余的位。
		priv[0] &= mask[bitSize%8]
		// 这是因为在测试中，rand 会返回全零，而我们不
		// 想要得到无穷远点并无限循环。
		priv[1] ^= 0x42

		// 如果标量超出范围，采样另一个随机数。
		if new(big.Int).SetBytes(priv).Cmp(N) >= 0 {
			continue
		}

		x, y = curve.ScalarBaseMult(priv)
	}
	return
}

// Marshal 将曲线上的点转换为 SEC 1 第 2.0 版第 2.3.3 节指定的未压缩形式。
// 如果点不在曲线上（或是惯例中的无穷远点），行为未定义。
//
// 已弃用：对于 ECDH，使用 crypto/ecdh 包。此函数返回一个编码
// 等价于 crypto/ecdh 中 PublicKey.Bytes 的编码。
func Marshal(curve Curve, x, y *big.Int) []byte {
	panicIfNotOnCurve(curve, x, y)

	byteLen := (curve.Params().BitSize + 7) / 8

	ret := make([]byte, 1+2*byteLen)
	ret[0] = 4 // 未压缩点

	x.FillBytes(ret[1 : 1+byteLen])
	y.FillBytes(ret[1+byteLen : 1+2*byteLen])

	return ret
}

// MarshalCompressed 将曲线上的点转换为 SEC 1 第 2.0 版第 2.3.3 节指定的压缩形式。
// 如果点不在曲线上（或是惯例中的无穷远点），行为未定义。
func MarshalCompressed(curve Curve, x, y *big.Int) []byte {
	panicIfNotOnCurve(curve, x, y)
	byteLen := (curve.Params().BitSize + 7) / 8
	compressed := make([]byte, 1+byteLen)
	compressed[0] = byte(y.Bit(0)) | 2
	x.FillBytes(compressed[1:])
	return compressed
}

// unmarshaler 由具有自己的常数时间 Unmarshal 的曲线实现。
//
// Marshal/MarshalCompressed 没有等价的接口，因为
// 它不涉及任何数学操作，只有 FillBytes 和 Bit。
type unmarshaler interface {
	Unmarshal([]byte) (x, y *big.Int)
	UnmarshalCompressed([]byte) (x, y *big.Int)
}

// 断言已知的曲线实现 unmarshaler。
var _ = []unmarshaler{p224, p256, p384, p521}

// Unmarshal 将 [Marshal] 序列化的点转换为 x, y 对。如果
// 点不是未压缩形式、不在曲线上或
// 是无穷远点，则是错误。出错时，x = nil。
//
// 已弃用：对于 ECDH，使用 crypto/ecdh 包。此函数接受与
// crypto/ecdh 中 NewPublicKey 方法等价的编码。
func Unmarshal(curve Curve, data []byte) (x, y *big.Int) {
	if c, ok := curve.(unmarshaler); ok {
		return c.Unmarshal(data)
	}

	byteLen := (curve.Params().BitSize + 7) / 8
	if len(data) != 1+2*byteLen {
		return nil, nil
	}
	if data[0] != 4 { // 未压缩形式
		return nil, nil
	}
	p := curve.Params().P
	x = new(big.Int).SetBytes(data[1 : 1+byteLen])
	y = new(big.Int).SetBytes(data[1+byteLen:])
	if x.Cmp(p) >= 0 || y.Cmp(p) >= 0 {
		return nil, nil
	}
	if !curve.IsOnCurve(x, y) {
		return nil, nil
	}
	return
}

// UnmarshalCompressed 将 [MarshalCompressed] 序列化的点转换为
// x, y 对。如果点不是压缩形式、不在
// 曲线上或是无穷远点，则是错误。出错时，x = nil。
func UnmarshalCompressed(curve Curve, data []byte) (x, y *big.Int) {
	if c, ok := curve.(unmarshaler); ok {
		return c.UnmarshalCompressed(data)
	}

	byteLen := (curve.Params().BitSize + 7) / 8
	if len(data) != 1+byteLen {
		return nil, nil
	}
	if data[0] != 2 && data[0] != 3 { // 压缩形式
		return nil, nil
	}
	p := curve.Params().P
	x = new(big.Int).SetBytes(data[1:])
	if x.Cmp(p) >= 0 {
		return nil, nil
	}
	// y² = x³ - 3x + b
	y = curve.Params().polynomial(x)
	y = y.ModSqrt(y, p)
	if y == nil {
		return nil, nil
	}
	if byte(y.Bit(0)) != data[0]&1 {
		y.Neg(y).Mod(y, p)
	}
	if !curve.IsOnCurve(x, y) {
		return nil, nil
	}
	return
}

func panicIfNotOnCurve(curve Curve, x, y *big.Int) {
	// (0, 0) 按惯例是无穷远点。在它上操作是可以的，
	// 尽管 IsOnCurve 有文档记录为它返回 false。参见问题 37294。
	if x.Sign() == 0 && y.Sign() == 0 {
		return
	}

	if !curve.IsOnCurve(x, y) {
		panic("crypto/elliptic: attempted operation on invalid point")
	}
}

var initonce sync.Once

func initAll() {
	initP224()
	initP256()
	initP384()
	initP521()
}

// P224 返回实现 NIST P-224（FIPS 186-3，第 D.2.2 节）的 [Curve]，
// 也称为 secp224r1。此 [Curve] 的 CurveParams.Name 是 "P-224"。
//
// 此函数的多次调用将返回相同的值，因此可以
// 用于相等性检查和 switch 语句。
//
// 密码学操作使用常数时间算法实现。
func P224() Curve {
	initonce.Do(initAll)
	return p224
}

// P256 返回实现 NIST P-256（FIPS 186-3，第 D.2.3 节）的 [Curve]，
// 也称为 secp256r1 或 prime256v1。此 [Curve] 的 CurveParams.Name 是
// "P-256"。
//
// 此函数的多次调用将返回相同的值，因此可以
// 用于相等性检查和 switch 语句。
//
// 密码学操作使用常数时间算法实现。
func P256() Curve {
	initonce.Do(initAll)
	return p256
}

// P384 返回实现 NIST P-384（FIPS 186-3，第 D.2.4 节）的 [Curve]，
// 也称为 secp384r1。此 [Curve] 的 CurveParams.Name 是 "P-384"。
//
// 此函数的多次调用将返回相同的值，因此可以
// 用于相等性检查和 switch 语句。
//
// 密码学操作使用常数时间算法实现。
func P384() Curve {
	initonce.Do(initAll)
	return p384
}

// P521 返回实现 NIST P-521（FIPS 186-3，第 D.2.5 节）的 [Curve]，
// 也称为 secp521r1。此 [Curve] 的 CurveParams.Name 是 "P-521"。
//
// 此函数的多次调用将返回相同的值，因此可以
// 用于相等性检查和 switch 语句。
//
// 密码学操作使用常数时间算法实现。
func P521() Curve {
	initonce.Do(initAll)
	return p521
}
