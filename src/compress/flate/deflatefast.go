// 版权所有 2016 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package flate

import "math"

// 这种编码算法优先考虑速度而非输出大小，
// 基于 Snappy 的 LZ77 风格编码器：github.com/golang/snappy

const (
	tableBits  = 14             // 表中使用的位数。
	tableSize  = 1 << tableBits // 表的大小。
	tableMask  = tableSize - 1  // 表索引的掩码。冗余，但可以消除边界检查。
	tableShift = 32 - tableBits // 右移以获取 uint32 的最高有效 tableBits 位。

	// 达到此值时重置缓冲区偏移量。
	// 偏移量在块之间以 int32 值存储。
	// 由于我们检查的偏移量在缓冲区的开头，
	// 我们需要减去当前和输入缓冲区以避免 int32 溢出的风险。
	bufferReset = math.MaxInt32 - maxStoreBlockSize*2
)

func load32(b []byte, i int32) uint32 {
	b = b[i : i+4 : len(b)] // 帮助编译器消除下一行的边界检查。
	return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
}

func load64(b []byte, i int32) uint64 {
	b = b[i : i+8 : len(b)] // 帮助编译器消除下一行的边界检查。
	return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
		uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
}

func hash(u uint32) uint32 {
	return (u * 0x1e35a7bd) >> tableShift
}

// 这些常量由 Snappy 实现定义，以便其汇编实现可以快速路径处理
// 一些每次 16 字节的复制。它们在纯 Go 实现中不是必需的，
// 因为我们不使用那些相同的优化，但使用相同的阈值并没有真正的坏处。
const (
	inputMargin            = 16 - 1
	minNonLiteralBlockSize = 1 + 1 + inputMargin
)

type tableEntry struct {
	val    uint32 // 目标处的值
	offset int32
}

// deflateFast 维护匹配表和用于跨块匹配的前一个字节块。
type deflateFast struct {
	table [tableSize]tableEntry
	prev  []byte // 前一个块，如果未知则长度为零。
	cur   int32  // 当前匹配偏移量。
}

func newDeflateFast() *deflateFast {
	return &deflateFast{cur: maxStoreBlockSize, prev: make([]byte, 0, maxStoreBlockSize)}
}

// encode 编码 src 中给定的块，将标记追加到 dst 并返回结果。
func (e *deflateFast) encode(dst []token, src []byte) []token {
	// 确保 e.cur 不会回绕。
	if e.cur >= bufferReset {
		e.shiftOffsets()
	}

	// 这个检查不在 Snappy 实现中，但在那里，调用者而不是被调用者处理这种情况。
	if len(src) < minNonLiteralBlockSize {
		e.cur += maxStoreBlockSize
		e.prev = e.prev[:0]
		return emitLiteral(dst, src)
	}

	// sLimit 是停止查找偏移量/长度复制的时机。inputMargin 让我们在
	// 查找复制时在主循环中使用 emitLiteral 的快速路径。
	sLimit := int32(len(src) - inputMargin)

	// nextEmit 是 src 中下一个 emitLiteral 应该开始的位置。
	nextEmit := int32(0)
	s := int32(0)
	cv := load32(src, s)
	nextHash := hash(cv)

	for {
		// 从 C++ snappy 实现复制：
		//
		// 启发式匹配跳过：如果扫描了 32 个字节没有找到匹配，
		// 开始只查看每隔一个字节。如果再扫描（或跳过）32 个字节，
		// 查看每第三个字节，依此类推。当找到匹配时，
		// 立即返回查看每个字节。对于可压缩数据，
		// 由于更多的簿记工作，这是一个小的损失（约 5% 性能，约 0.1% 密度），
		// 但对于不可压缩数据（如 JPEG），这是一个巨大的胜利，
		// 因为压缩器很快"意识到"数据是不可压缩的，
		// 不再费心到处寻找匹配。
		//
		// "skip" 变量跟踪自上次匹配以来有多少字节；
		// 将其除以 32（即右移五位）得到每次迭代要前进的字节数。
		skip := int32(32)

		nextS := s
		var candidate tableEntry
		for {
			s = nextS
			bytesBetweenHashLookups := skip >> 5
			nextS = s + bytesBetweenHashLookups
			skip += bytesBetweenHashLookups
			if nextS > sLimit {
				goto emitRemainder
			}
			candidate = e.table[nextHash&tableMask]
			now := load32(src, nextS)
			e.table[nextHash&tableMask] = tableEntry{offset: s + e.cur, val: cv}
			nextHash = hash(now)

			offset := s - (candidate.offset - e.cur)
			if offset > maxMatchOffset || cv != candidate.val {
				// 超出范围或不匹配。
				cv = now
				continue
			}
			break
		}

		// 找到了 4 字节匹配。我们稍后会看是否有超过 4 字节匹配。
		// 但是，在匹配之前，src[nextEmit:s] 是不匹配的。将它们作为字面字节发出。
		dst = emitLiteral(dst, src[nextEmit:s])

		// 调用 emitCopy，然后看看另一个 emitCopy 是否可以是我们的下一步。
		// 重复直到我们在上次 emitCopy 调用消耗的内容之后找不到输入的匹配。
		//
		// 如果我们正常退出这个循环，那么我们接下来需要调用 emitLiteral，
		// 尽管我们还不知道字面量有多大。我们通过继续到主循环的下一次迭代来处理这个问题。
		// 如果我们接近耗尽输入，我们也可以通过 goto 退出这个循环。
		for {
			// 不变量：我们在 s 处有一个 4 字节匹配，不需要在 s 之前发出任何字面字节。

			// 尽可能延长 4 字节匹配。
			//
			s += 4
			t := candidate.offset - e.cur + 4
			l := e.matchLen(s, t, src)

			// matchToken 是 flate 中等同于 Snappy 的 emitCopy。(长度,偏移量)
			dst = append(dst, matchToken(uint32(l+4-baseMatchLength), uint32(s-t-baseMatchOffset)))
			s += l
			nextEmit = s
			if s >= sLimit {
				goto emitRemainder
			}

			// 我们现在可以立即从 s 开始工作，但为了提高压缩率，
			// 我们首先更新 s-1 和 s 处的哈希表。如果另一个 emitCopy
			// 不是我们的下一步，也在 s+1 处计算 nextHash。
			// 至少在 GOARCH=amd64 上，这三个哈希计算作为一次 load64 调用
			//（带有一些移位）比三次 load32 调用更快。
			x := load64(src, s-1)
			prevHash := hash(uint32(x))
			e.table[prevHash&tableMask] = tableEntry{offset: e.cur + s - 1, val: uint32(x)}
			x >>= 8
			currHash := hash(uint32(x))
			candidate = e.table[currHash&tableMask]
			e.table[currHash&tableMask] = tableEntry{offset: e.cur + s, val: uint32(x)}

			offset := s - (candidate.offset - e.cur)
			if offset > maxMatchOffset || uint32(x) != candidate.val {
				cv = uint32(x >> 8)
				nextHash = hash(cv)
				s++
				break
			}
		}
	}

emitRemainder:
	if int(nextEmit) < len(src) {
		dst = emitLiteral(dst, src[nextEmit:])
	}
	e.cur += int32(len(src))
	e.prev = e.prev[:len(src)]
	copy(e.prev, src)
	return dst
}

func emitLiteral(dst []token, lit []byte) []token {
	for _, v := range lit {
		dst = append(dst, literalToken(uint32(v)))
	}
	return dst
}

// matchLen 返回 src[s:] 和 src[t:] 之间的匹配长度。
// t 可以是负数，表示匹配从 e.prev 开始。
// 我们假设 src[s-4:s] 和 src[t-4:t] 已经匹配。
func (e *deflateFast) matchLen(s, t int32, src []byte) int32 {
	s1 := int(s) + maxMatchLength - 4
	if s1 > len(src) {
		s1 = len(src)
	}

	// 如果我们在当前块内
	if t >= 0 {
		b := src[t:]
		a := src[s:s1]
		b = b[:len(a)]
		// 尽可能延长匹配。
		for i := range a {
			if a[i] != b[i] {
				return int32(i)
			}
		}
		return int32(len(a))
	}

	// 我们在前一个块中找到了匹配。
	tp := int32(len(e.prev)) + t
	if tp < 0 {
		return 0
	}

	// 尽可能延长匹配。
	a := src[s:s1]
	b := e.prev[tp:]
	if len(b) > len(a) {
		b = b[:len(a)]
	}
	a = a[:len(b)]
	for i := range b {
		if a[i] != b[i] {
			return int32(i)
		}
	}

	// 如果我们达到了限制，我们匹配了前一个块中允许的所有内容并返回。
	n := int32(len(b))
	if int(s+n) == s1 {
		return n
	}

	// 继续在当前块中寻找更多匹配。
	a = src[s+n : s1]
	b = src[:len(a)]
	for i := range a {
		if a[i] != b[i] {
			return int32(i) + n
		}
	}
	return int32(len(a)) + n
}

// Reset 重置编码历史。
// 这确保不会与前一个块进行匹配。
func (e *deflateFast) reset() {
	e.prev = e.prev[:0]
	// 增加偏移量，使所有匹配都将失败距离检查。
	// 表中不应有 >= e.cur 的内容。
	e.cur += maxMatchOffset

	// 防止 e.cur 回绕。
	if e.cur >= bufferReset {
		e.shiftOffsets()
	}
}

// shiftOffsets 将所有匹配偏移量向下移动。
// 这只在罕见的情况下调用，以防止整数溢出。
//
// 见 https://golang.org/issue/18636 和 https://github.com/golang/go/issues/34121。
func (e *deflateFast) shiftOffsets() {
	if len(e.prev) == 0 {
		// 我们没有历史记录；只需清除表。
		clear(e.table[:])
		e.cur = maxMatchOffset + 1
		return
	}

	// 将表中所有还没有太远的内容向下移动。
	for i := range e.table[:] {
		v := e.table[i].offset - e.cur + maxMatchOffset + 1
		if v < 0 {
			// 我们想将 e.cur 重置为 maxMatchOffset + 1，所以我们需要
			// 将所有表条目向下移动 (e.cur - (maxMatchOffset + 1))。
			// 因为我们忽略 > maxMatchOffset 的匹配，我们可以将
			// 任何负偏移量限制为 0。
			v = 0
		}
		e.table[i].offset = v
	}
	e.cur = maxMatchOffset + 1
}
