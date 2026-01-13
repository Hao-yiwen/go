// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// dsa 包实现了数字签名算法，如 FIPS 186-3 中所定义。
//
// 本包中的 DSA 操作不使用常数时间算法实现。
//
// 已弃用：DSA 是一种遗留算法，应使用现代替代方案（如
// 由 crypto/ed25519 包实现的 Ed25519）代替。具有 1024 位模数
// (L1024N160 参数) 的密钥在密码学上较弱，而较大的密钥不被广泛支持。
// 注意 FIPS 186-5 不再批准 DSA 用于签名生成。
package dsa

import (
	"errors"
	"io"
	"math/big"

	"crypto/internal/fips140only"
	"crypto/internal/rand"
)

// Parameters 表示密钥的域参数。这些参数可以
// 在许多密钥之间共享。Q 的位长度必须是 8 的倍数。
type Parameters struct {
	P, Q, G *big.Int
}

// PublicKey 代表一个 DSA 公钥。
type PublicKey struct {
	Parameters
	Y *big.Int
}

// PrivateKey 代表一个 DSA 私钥。
type PrivateKey struct {
	PublicKey
	X *big.Int
}

// ErrInvalidPublicKey 在公钥不能被此代码使用时返回。
// FIPS 对 DSA 密钥的格式要求非常严格，但其他代码可能
// 不那么严格。因此，在使用可能由其他代码生成的密钥时，
// 必须处理此错误。
var ErrInvalidPublicKey = errors.New("crypto/dsa: invalid public key")

// ParameterSizes 是 DSA 参数集中素数的可接受位长度的枚举。
// 参见 FIPS 186-3，第 4.2 节。
type ParameterSizes int

const (
	L1024N160 ParameterSizes = iota
	L2048N224
	L2048N256
	L3072N256
)

// numMRTests 是我们执行的 Miller-Rabin 素性测试的次数。我们
// 从 FIPS 186-3 的表 C.1 中选择最大推荐数。
const numMRTests = 64

// GenerateParameters 将一组随机的、有效的 DSA 参数放入 params。
// 这个函数即使在快速机器上也可能需要很多秒。
func GenerateParameters(params *Parameters, rand io.Reader, sizes ParameterSizes) error {
	if fips140only.Enforced() {
		return errors.New("crypto/dsa: use of DSA is not allowed in FIPS 140-only mode")
	}

	// 此函数不完全遵循 FIPS 186-3，因为它不
	// 使用验证种子来生成素数。验证
	// 种子似乎没有被导出或其他代码使用，
	// 省略它使代码更清晰。

	var L, N int
	switch sizes {
	case L1024N160:
		L = 1024
		N = 160
	case L2048N224:
		L = 2048
		N = 224
	case L2048N256:
		L = 2048
		N = 256
	case L3072N256:
		L = 3072
		N = 256
	default:
		return errors.New("crypto/dsa: invalid ParameterSizes")
	}

	qBytes := make([]byte, N/8)
	pBytes := make([]byte, L/8)

	q := new(big.Int)
	p := new(big.Int)
	rem := new(big.Int)
	one := new(big.Int)
	one.SetInt64(1)

GeneratePrimes:
	for {
		if _, err := io.ReadFull(rand, qBytes); err != nil {
			return err
		}

		qBytes[len(qBytes)-1] |= 1
		qBytes[0] |= 0x80
		q.SetBytes(qBytes)

		if !q.ProbablyPrime(numMRTests) {
			continue
		}

		for i := 0; i < 4*L; i++ {
			if _, err := io.ReadFull(rand, pBytes); err != nil {
				return err
			}

			pBytes[len(pBytes)-1] |= 1
			pBytes[0] |= 0x80

			p.SetBytes(pBytes)
			rem.Mod(p, q)
			rem.Sub(rem, one)
			p.Sub(p, rem)
			if p.BitLen() < L {
				continue
			}

			if !p.ProbablyPrime(numMRTests) {
				continue
			}

			params.P = p
			params.Q = q
			break GeneratePrimes
		}
	}

	h := new(big.Int)
	h.SetInt64(2)
	g := new(big.Int)

	pm1 := new(big.Int).Sub(p, one)
	e := new(big.Int).Div(pm1, q)

	for {
		g.Exp(h, e, p)
		if g.Cmp(one) == 0 {
			h.Add(h, one)
			continue
		}

		params.G = g
		return nil
	}
}

// GenerateKey 生成一个公钥和私钥对。[PrivateKey] 的
// Parameters 必须已经有效（参见 [GenerateParameters]）。
func GenerateKey(priv *PrivateKey, rand io.Reader) error {
	if fips140only.Enforced() {
		return errors.New("crypto/dsa: use of DSA is not allowed in FIPS 140-only mode")
	}

	if priv.P == nil || priv.Q == nil || priv.G == nil {
		return errors.New("crypto/dsa: parameters not set up before generating key")
	}

	x := new(big.Int)
	xBytes := make([]byte, priv.Q.BitLen()/8)

	for {
		_, err := io.ReadFull(rand, xBytes)
		if err != nil {
			return err
		}
		x.SetBytes(xBytes)
		if x.Sign() != 0 && x.Cmp(priv.Q) < 0 {
			break
		}
	}

	priv.X = x
	priv.Y = new(big.Int)
	priv.Y.Exp(priv.G, x, priv.P)
	return nil
}

// fermatInverse 使用费马方法计算 k 在 GF(P) 中的逆。
// 这比欧几里得方法（在 math/big.Int.ModInverse 中实现）
// 有更好的常数时间特性，尽管 math/big 本身不是严格的
// 常数时间，所以它不是完美的。
func fermatInverse(k, P *big.Int) *big.Int {
	two := big.NewInt(2)
	pMinus2 := new(big.Int).Sub(P, two)
	return new(big.Int).Exp(k, pMinus2, P)
}

// Sign 使用私钥 priv 对任意长度的哈希（应该是对更大消息的哈希结果）
// 进行签名。它返回签名作为整数对。私钥的安全性取决于
// rand 的熵。
//
// 注意 FIPS 186-3 第 4.6 节指定哈希应截断为
// 子群的字节长度。此函数不自行执行该
// 截断。
//
// 从 Go 1.26 开始，总是使用安全的随机字节来源，
// 除非设置了 GODEBUG=cryptocustomrand=1，否则忽略 Reader。
// 此设置将在未来的 Go 版本中移除。
// 改为使用 [testing/cryptotest.SetGlobalRandom]。
//
// 请注意，使用受攻击者控制的 [PrivateKey] 调用 Sign
// 可能需要任意数量的 CPU。
func Sign(random io.Reader, priv *PrivateKey, hash []byte) (r, s *big.Int, err error) {
	if fips140only.Enforced() {
		return nil, nil, errors.New("crypto/dsa: use of DSA is not allowed in FIPS 140-only mode")
	}

	random = rand.CustomReader(random)

	// FIPS 186-3，第 4.6 节

	n := priv.Q.BitLen()
	if priv.Q.Sign() <= 0 || priv.P.Sign() <= 0 || priv.G.Sign() <= 0 || priv.X.Sign() <= 0 || n%8 != 0 {
		err = ErrInvalidPublicKey
		return
	}
	n >>= 3

	var attempts int
	for attempts = 10; attempts > 0; attempts-- {
		k := new(big.Int)
		buf := make([]byte, n)
		for {
			_, err = io.ReadFull(random, buf)
			if err != nil {
				return
			}
			k.SetBytes(buf)
			// priv.Q 必须 >= 128，因为上面的测试
			// 要求它 > 0，并且
			//    ceil(log_2(Q)) mod 8 = 0
			// 因此此循环将快速终止。
			if k.Sign() > 0 && k.Cmp(priv.Q) < 0 {
				break
			}
		}

		kInv := fermatInverse(k, priv.Q)

		r = new(big.Int).Exp(priv.G, k, priv.P)
		r.Mod(r, priv.Q)

		if r.Sign() == 0 {
			continue
		}

		z := k.SetBytes(hash)

		s = new(big.Int).Mul(priv.X, r)
		s.Add(s, z)
		s.Mod(s, priv.Q)
		s.Mul(s, kInv)
		s.Mod(s, priv.Q)

		if s.Sign() != 0 {
			break
		}
	}

	// 只有退化的私钥才需要超过少数几次
	// 尝试。
	if attempts == 0 {
		return nil, nil, ErrInvalidPublicKey
	}

	return
}

// Verify 使用公钥 pub 验证哈希的签名 r、s。它
// 报告签名是否有效。
//
// 注意 FIPS 186-3 第 4.6 节指定哈希应截断为
// 子群的字节长度。此函数不自行执行该
// 截断。
func Verify(pub *PublicKey, hash []byte, r, s *big.Int) bool {
	if fips140only.Enforced() {
		panic("crypto/dsa: use of DSA is not allowed in FIPS 140-only mode")
	}

	// FIPS 186-3，第 4.7 节

	if pub.P.Sign() == 0 {
		return false
	}

	if r.Sign() < 1 || r.Cmp(pub.Q) >= 0 {
		return false
	}
	if s.Sign() < 1 || s.Cmp(pub.Q) >= 0 {
		return false
	}

	w := new(big.Int).ModInverse(s, pub.Q)
	if w == nil {
		return false
	}

	n := pub.Q.BitLen()
	if n%8 != 0 {
		return false
	}
	z := new(big.Int).SetBytes(hash)

	u1 := new(big.Int).Mul(z, w)
	u1.Mod(u1, pub.Q)
	u2 := w.Mul(r, w)
	u2.Mod(u2, pub.Q)
	v := u1.Exp(pub.G, u1, pub.P)
	u2.Exp(pub.Y, u2, pub.P)
	v.Mul(v, u2)
	v.Mod(v, pub.P)
	v.Mod(v, pub.Q)

	return v.Cmp(r) == 0
}
