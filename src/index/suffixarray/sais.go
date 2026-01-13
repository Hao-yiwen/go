// Copyright 2019 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// 通过诱导排序（SAIS）构造后缀数组。
// 参见 Ge Nong、Sen Zhang 和 Wai Hong Chen，
// "两种线性时间后缀数组构造的高效算法"，
// 尤其是第 3 部分 (https://ieeexplore.ieee.org/document/5582081)。
// 另见 http://zork.net/~st/jottings/sais.html。
//
// 采用受 Yuta Mori 的 sais-lite 启发的优化
// (https://sites.google.com/site/yuta256/sais)。
//
// 以及其他新的优化。

// 许多这些函数由它们操作的类型的大小进行参数化。
// 生成器 gen.go 制作了这些函数的副本供其他大小使用。
// 具体来说：
//
// - 名称以 _8_32 结尾的函数接受 []byte 和 []int32 参数
//   并被复制成 _32_32、_8_64 和 _64_64 形式。
//   _32_32 和 _64_64_ 后缀被缩短为普通 _32 和 _64。
//   创建 _32_32 和 _64_64 形式时，会删除函数体中包含文本"byte-only"或"256"的任何行。
//   （这些行通常是 8 位特定的优化。）
//
// - 名称仅以 _32 结尾的函数在 []int32 上操作
//   并被复制成 _64 形式。（注意它可能仍然接受 []byte，
//   但不需要 []byte 被扩展为完整整数数组的函数版本。）

// 此代码的总体运行时间与输入大小呈线性关系：
// 它运行一系列线性遍历来将问题简化为
// 一个最多一半大小的子问题，递归调用自身，
// 然后运行一系列线性遍历来将子问题的答案
// 转换为原始问题的答案。
// 这给出 T(N) = O(N) + T(N/2) = O(N) + O(N/2) + O(N/4) + ... = O(N)。
//
// 代码的概述，标记了通过 O(N) 大小的数组的前向和后向扫描，是：
//
// sais_I_N
//	placeLMS_I_B
//		bucketMax_I_B
//			freq_I_B
//				<scan +text> (1)
//			<scan +freq> (2)
//		<scan -text, random bucket> (3)
//	induceSubL_I_B
//		bucketMin_I_B
//			freq_I_B
//				<scan +text, often optimized away> (4)
//			<scan +freq> (5)
//		<scan +sa, random text, random bucket> (6)
//	induceSubS_I_B
//		bucketMax_I_B
//			freq_I_B
//				<scan +text, often optimized away> (7)
//			<scan +freq> (8)
//		<scan -sa, random text, random bucket> (9)
//	assignID_I_B
//		<scan +sa, random text substrings> (10)
//	map_B
//		<scan -sa> (11)
//	recurse_B
//		(recursive call to sais_B_B for a subproblem of size at most 1/2 input, often much smaller)
//	unmap_I_B
//		<scan -text> (12)
//		<scan +sa> (13)
//	expand_I_B
//		bucketMax_I_B
//			freq_I_B
//				<scan +text, often optimized away> (14)
//			<scan +freq> (15)
//		<scan -sa, random text, random bucket> (16)
//	induceL_I_B
//		bucketMin_I_B
//			freq_I_B
//				<scan +text, often optimized away> (17)
//			<scan +freq> (18)
//		<scan +sa, random text, random bucket> (19)
//	induceS_I_B
//		bucketMax_I_B
//			freq_I_B
//				<scan +text, often optimized away> (20)
//			<scan +freq> (21)
//		<scan -sa, random text, random bucket> (22)
//
// 在这里，_B 表示后缀数组的大小（_32 或 _64），_I 表示输入大小（_8 或 _B）。
//
// 概述显示，对于递归的给定级别，通常有 22 次扫描
// O(N) 大小的数组。在顶层，操作 8 位输入文本时，
// 六个 freq 扫描是固定大小（256），而不是可能的
// 输入大小。此外，频率计数一次，
// 只要有空间就缓存（通常总是有空间，
// 在顶层也总是有空间），这消除了除了
// 第一个 freq_I_B 文本扫描之外的所有扫描（即 6 个中的 5 个）。
// 所以递归的顶层只做 22 - 6 - 5 = 11
// 个输入大小的扫描，典型级别做 16 次扫描。
//
// 线性扫描的成本远不及到文本的随机访问成本，
// 这些访问是在少数几次扫描中进行的
// （特别是上面标记的第 #6、#9、#16、#19、#22 次）。
// 在真实文本中，访问中没有太多但有一些局部性，
// 这是由于文本的重复结构
// （同样的原因使得 Burrows-Wheeler 压缩非常有效）。
// 对于随机输入，没有局部性，这使得那些
// 访问更加昂贵，尤其是一旦文本
// 不再适合缓存时。
// 例如，运行在 50 MB 的 Go 源代码上，induceSubL_8_32
// （仅在递归的顶层运行一次）
// 需要 0.44s，而在 50 MB 的随机输入上，需要 2.55s。
// 几乎所有的相对减速都由文本访问解释：
//
//		c0, c1 := text[k-1], text[k]
//
// 这一行在 Go 文本上运行 0.23s，在随机文本上运行 2.02s。

//go:generate go run gen.go

package suffixarray

// text_32 返回输入文本的后缀数组。
// 它要求 len(text) 适应 int32，
// 并且调用者将 sa 清零。
func text_32(text []byte, sa []int32) {
	if int(int32(len(text))) != len(text) || len(text) != len(sa) {
		panic("suffixarray: misuse of text_32")
	}
	sais_8_32(text, 256, sa, make([]int32, 2*256))
}

// sais_8_32 计算文本的后缀数组。
// 文本必须只包含 [0, textMax) 中的值。
// 后缀数组存储在 sa 中，调用者
// 必须确保已清零。
// 调用者还必须提供临时空间 tmp
// 且 len(tmp) ≥ textMax。如果 len(tmp) ≥ 2*textMax
// 则算法运行稍快。
// 如果 sais_8_32 修改 tmp，返回时设置 tmp[0] = -1。
func sais_8_32(text []byte, textMax int, sa, tmp []int32) {
	if len(sa) != len(text) || len(tmp) < textMax {
		panic("suffixarray: misuse of sais_8_32")
	}

	// 平凡的基础情况。排序 0 或 1 个东西很容易。
	if len(text) == 0 {
		return
	}
	if len(text) == 1 {
		sa[0] = 0
		return
	}

	// 建立由文本字符索引的切片，
	// 保存字符频率和桶排序偏移。
	// 如果临时空间只足够一个切片，
	// 我们将其用作桶偏移，
	// 每次需要时重新计算字符频率。
	var freq, bucket []int32
	if len(tmp) >= 2*textMax {
		freq, bucket = tmp[:textMax], tmp[textMax:2*textMax]
		freq[0] = -1 // 标记为未初始化
	} else {
		freq, bucket = nil, tmp[:textMax]
	}

	// SAIS 算法。
	// 这些调用中的每一个都进行一次通过 sa 的扫描。
	// 有关每个函数在算法中的角色，请参见各个函数的文档。
	numLMS := placeLMS_8_32(text, sa, freq, bucket)
	if numLMS <= 1 {
		// 0 或 1 个项已排序。什么都不做。
	} else {
		induceSubL_8_32(text, sa, freq, bucket)
		induceSubS_8_32(text, sa, freq, bucket)
		length_8_32(text, sa, numLMS)
		maxID := assignID_8_32(text, sa, numLMS)
		if maxID < numLMS {
			map_32(sa, numLMS)
			recurse_32(sa, tmp, numLMS, maxID)
			unmap_8_32(text, sa, numLMS)
		} else {
			// 如果 maxID == numLMS，则每个 LMS 子串
			// 是唯一的，所以两个 LMS 后缀的相对顺序
			// 仅由前导 LMS 子串确定。
			// 也就是说，LMS 后缀排序顺序与
			// （更简单的）LMS 子串排序顺序相匹配。
			// 将原始 LMS 子串顺序复制到
			// 后缀数组目标。
			copy(sa, sa[len(sa)-numLMS:])
		}
		expand_8_32(text, freq, bucket, sa, numLMS)
	}
	induceL_8_32(text, sa, freq, bucket)
	induceS_8_32(text, sa, freq, bucket)

	// 向调用者标记我们已覆盖 tmp。
	tmp[0] = -1
}

// freq_8_32 返回文本的字符频率，
// 作为由字符值索引的切片。
// 如果 freq 为 nil，freq_8_32 使用并返回 bucket。
// 如果 freq 非 nil，freq_8_32 假定 freq[0] >= 0
// 表示频率已计算。
// 如果频率数据被覆盖或未初始化，
// 调用者必须设置 freq[0] = -1 以强制重新计算
// 下次需要时。
func freq_8_32(text []byte, freq, bucket []int32) []int32 {
	if freq != nil && freq[0] >= 0 {
		return freq // 已计算
	}
	if freq == nil {
		freq = bucket
	}

	freq = freq[:256] // 消除下面 freq[c] 的边界检查
	clear(freq)
	for _, c := range text {
		freq[c]++
	}
	return freq
}

// bucketMin_8_32 将 bucket[c] 的最小索引存储到
// 桶排序中字符 c 的桶。
func bucketMin_8_32(text []byte, freq, bucket []int32) {
	freq = freq_8_32(text, freq, bucket)
	freq = freq[:256]     // 建立 len(freq) = 256，所以下面 0 ≤ i < 256
	bucket = bucket[:256] // 消除下面 bucket[i] 的边界检查
	total := int32(0)
	for i, n := range freq {
		bucket[i] = total
		total += n
	}
}

// bucketMax_8_32 将 bucket[c] 的最大索引存储到
// 桶排序中字符 c 的桶。
// 字符 c 的桶索引是 [min, max)。
// 也就是说，max 是该桶中最后一个索引之后的一个。
func bucketMax_8_32(text []byte, freq, bucket []int32) {
	freq = freq_8_32(text, freq, bucket)
	freq = freq[:256]     // 建立 len(freq) = 256，所以下面 0 ≤ i < 256
	bucket = bucket[:256] // 消除下面 bucket[i] 的边界检查
	total := int32(0)
	for i, n := range freq {
		total += n
		bucket[i] = total
	}
}

// SAIS 算法以一系列通过 sa 的扫描进行。
// 以下每个函数都实现一次扫描，
// 函数在这里按照它们在算法中执行的顺序显示。

// placeLMS_8_32 将文本的 LMS 子串的最后一个字符的索引放入 sa，
// 排序到它们正确桶的最右端
// 在后缀数组中。
//
// 文本末尾的虚想哨兵字符
// 是最后一个 LMS 子串的最后一个字符，但是
// 虚想哨兵字符没有桶，
// 它的值比任何实际字符都小。
// 因此，调用者必须假装 sa[-1] == len(text)。
//
// LMS 子串字符的文本索引始终是 ≥ 1
// （第一个 LMS 子串必须前面有一个或多个 L 类型
// 不属于任何 LMS 子串的字符），
// 所以使用 0 作为"不存在"后缀数组条目是安全的，
// 无论是在这个函数中还是在大多数后来的函数中
// （直到下面的 induceL_8_32）。
func placeLMS_8_32(text []byte, sa, freq, bucket []int32) int {
	bucketMax_8_32(text, freq, bucket)

	numLMS := 0
	lastB := int32(-1)
	bucket = bucket[:256] // 消除下面 bucket[c1] 的边界检查

	// 接下来的代码块（直到空白行）向后循环
	// 遍历文本，在每个位置 i 处停止执行代码体
	// 使得 text[i] 是 L 字符且 text[i+1] 是 S 字符。
	// 也就是说，i+1 是 LMS 子串开始的位置。
	// 这些可以被提取到一个带回调的函数中，
	// 但会有显著的速度成本。相反，我们只需在
	// 这个源文件中写这七行几次。下面的副本
	// 参考由这个原始部分建立的模式作为
	// "LMS 子串迭代器"。
	//
	// 在每次通过文本的扫描中，c0、c1 是文本的连续字符。
	// 在这个向后扫描中，c0 == text[i] 且 c1 == text[i+1]。
	// 通过向后扫描，我们可以跟踪当前
	// 位置根据通常的定义是 S 类型还是 L 类型：
	//
	//	- 位置 len(text) 是 S 类型，text[len(text)] == -1（哨兵）
	//	- 位置 i 是 S 类型，如果 text[i] < text[i+1]，或如果 text[i] == text[i+1] && i+1 是 S 类型。
	//	- 位置 i 是 L 类型，如果 text[i] > text[i+1]，或如果 text[i] == text[i+1] && i+1 是 L 类型。
	//
	// 向后扫描让我们维护当前类型，
	// 当我们看到 c0 != c1 时更新它，否则保留不变。
	// 我们想要识别所有前面有 L 的 S 位置。
	// 根据定义，位置 len(text) 就是这样一个位置，但我们
	// 无处记录它，所以我们通过虚假地
	// 在循环开始时设置 isTypeS = false 来消除它。
	c0, c1, isTypeS := byte(0), byte(0), false
	for i := len(text) - 1; i >= 0; i-- {
		c0, c1 = text[i], c0
		if c0 < c1 {
			isTypeS = true
		} else if c0 > c1 && isTypeS {
			isTypeS = false

			// 为 LMS 子串的开始对索引 i+1 进行桶操作。
			b := bucket[c1] - 1
			bucket[c1] = b
			sa[b] = int32(i + 1)
			lastB = b
			numLMS++
		}
	}

	// 我们记录了 LMS 子串的开始，但真正想要的是结尾。
	// 幸运的是，有两个差异，开始索引和结尾索引是相同的。
	// 第一个差异是最右边 LMS 子串的结尾索引是 len(text)，
	// 所以调用者必须假装 sa[-1] == len(text)，如上所述。
	// 第二个差异是第一个最左边的 LMS 子串开始索引
	// 不是早期 LMS 子串的结尾，所以作为一个优化，我们可以省略
	// 该最左边的 LMS 子串开始索引（我们写的最后一个）。
	//
	// 例外：如果 numLMS <= 1，调用者根本不会麻烦
	// 递归，会将结果视为包含 LMS 子串开始。
	// 在这种情况下，我们不删除最后一个条目。
	if numLMS > 1 {
		sa[lastB] = 0
	}
	return numLMS
}

// induceSubL_8_32 将 LMS 子串的 L 类型文本索引插入
// 到 sa 中，假设 LMS 子串的最后一个字符
// 已经插入到 sa 中，按最后一个字符排序，并在
// 对应字符桶的右端（不是左端）。
// 每个 LMS 子串具有形式（作为正则表达式）/S+L+S/：
// 一个或多个 S 类型、一个或多个 L 类型、最后一个 S 类型。
// induceSubL_8_32 只留下最左边的 L 类型文本
// 每个 LMS 子串的索引。也就是说，它删除最后的 S 类型
// 条目中存在的索引，并插入然后删除
// 内部 L 类型索引。
// （induceSubS_8_32 只需要最左边的 L 类型索引。）
func induceSubL_8_32(text []byte, sa, freq, bucket []int32) {
	// 初始化字符桶左侧的位置。
	bucketMin_8_32(text, freq, bucket)
	bucket = bucket[:256] // 消除下面 bucket[cB] 的边界检查

	// 当我们从左到右扫描数组时，每个 sa[i] = j > 0 是一个正确
	// 排序的后缀数组条目（对于 text[j:]），我们知道 j-1 是 L 类型。
	// 因为 j-1 是 L 类型，现在将其插入 sa 会正确排序。
	// 但我们想区分 j-1 与 j-2 为 L 类型还是 S 类型。
	// 我们可以处理前者，但想为调用者保留后者。
	// 我们通过否定 j-1 如果它前面是 S 类型来记录差异。
	// 无论如何，插入（进入 text[j-1] 桶）保证
	// 在 sa[i´] 处发生，其中 i´ > i，也就是说，在 sa 的部分
	// 我们还没有扫描。单次通过因此看到索引 j、j-1、j-2、j-3，
	// 等等，以排序但不一定相邻的顺序，直到找到
	// 前面是 S 类型索引的那个，此时必须停止。
	//
	// 当我们通过数组扫描时，我们清除已处理的条目（sa[i] > 0）为零，
	// 并将 sa[i] < 0 翻转为 -sa[i]，所以循环完成时 sa 包含
	// 只是每个 LMS 子串的最左边 L 类型索引。
	//
	// 后缀数组 sa 因此同时用作输入、输出，
	// 以及一个奇迹般量身定制的工作队列。

	// placeLMS_8_32 省略了隐式条目 sa[-1] == len(text)，
	// 对应于识别的 L 类型索引 len(text)-1。
	// 在 sa 正确的从左到右扫描之前处理它。
	// 有关注释，请查看循环中的主体。
	k := len(text) - 1
	c0, c1 := text[k-1], text[k]
	if c0 < c1 {
		k = -k
	}

	// 缓存最近使用的桶索引：
	// 我们按排序顺序处理后缀
	// 并访问由
	// 排序顺序之前的字节索引的桶，
	// 仍有非常好的局部性。
	// 不变式：b 是 bucket[cB] 的缓存、可能是脏副本。
	cB := c1
	b := bucket[cB]
	sa[b] = int32(k)
	b++

	for i := 0; i < len(sa); i++ {
		j := int(sa[i])
		if j == 0 {
			// 跳过空条目。
			continue
		}
		if j < 0 {
			// 为调用者保留发现的 S 类型索引。
			sa[i] = int32(-j)
			continue
		}
		sa[i] = 0

		// 索引 j 在工作队列上，意味着 k := j-1 是 L 类型，
		// 所以我们现在可以正确地将 k 放入 sa。
		// 如果 k-1 是 L 类型，为稍后在此循环中处理将 k 排队。
		// 如果 k-1 是 S 类型（text[k-1] < text[k]），为调用者排队 -k 保存。
		k := j - 1
		c0, c1 := text[k-1], text[k]
		if c0 < c1 {
			k = -k
		}

		if cB != c1 {
			bucket[cB] = b
			cB = c1
			b = bucket[cB]
		}
		sa[b] = int32(k)
		b++
	}
}

// induceSubS_8_32 将 LMS 子串的 S 类型文本索引插入
// 到 sa，假设最左边的 L 类型文本索引已经
// 插入到 sa，按 LMS 子串后缀排序，并在
// 对应字符桶的左端。
// 每个 LMS 子串具有形式（作为正则表达式）/S+L+S/：
// 一个或多个 S 类型、一个或多个 L 类型、最后一个 S 类型。
// induceSubS_8_32 只留下最左边的 S 类型文本
// 每个 LMS 子串的索引，按排序顺序，在 sa 的右端。
// 也就是说，它删除条目中存在的 L 类型索引，
// 并插入然后删除内部 S 类型索引，
// 将 LMS 子串开始索引打包到 sa[len(sa)-numLMS:]。
// （只有 LMS 子串开始索引由递归处理。）
func induceSubS_8_32(text []byte, sa, freq, bucket []int32) {
	// 初始化字符桶右侧的位置。
	bucketMax_8_32(text, freq, bucket)
	bucket = bucket[:256] // 消除下面 bucket[cB] 的边界检查

	// 类似于上面的 induceSubL_8_32，
	// 当我们从右到左扫描数组时，每个 sa[i] = j > 0 是一个正确
	// 排序的后缀数组条目（对于 text[j:]），我们知道 j-1 是 S 类型。
	// 因为 j-1 是 S 类型，现在将其插入 sa 会正确排序。
	// 但我们想区分 j-1 与 j-2 为 S 类型还是 L 类型。
	// 我们可以处理前者，但想为调用者保留后者。
	// 我们通过否定 j-1 如果它前面是 L 类型来记录差异。
	// 无论如何，插入（进入 text[j-1] 桶）保证
	// 在 sa[i´] 处发生，其中 i´ < i，也就是说，在 sa 的部分
	// 我们还没有扫描。单次通过因此看到索引 j、j-1、j-2、j-3，
	// 等等，以排序但不一定相邻的顺序，直到找到
	// 前面是 L 类型索引的那个，此时必须停止。
	// 那个索引（前面是 L 类型）是一个 LMS 子串开始。
	//
	// 当我们通过数组扫描时，我们清除已处理的条目（sa[i] > 0）为零，
	// 并将 sa[i] < 0 翻转为 -sa[i] 并紧凑到 sa 顶部，
	// 所以循环完成时 sa 的顶部正好包含
	// LMS 子串开始索引，按 LMS 子串排序。

	// 缓存最近使用的桶索引：
	cB := byte(0)
	b := bucket[cB]

	top := len(sa)
	for i := len(sa) - 1; i >= 0; i-- {
		j := int(sa[i])
		if j == 0 {
			// 跳过空条目。
			continue
		}
		sa[i] = 0
		if j < 0 {
			// 为调用者保留发现的 LMS 子串开始索引。
			top--
			sa[top] = int32(-j)
			continue
		}

		// 索引 j 在工作队列上，意味着 k := j-1 是 S 类型，
		// 所以我们现在可以正确地将 k 放入 sa。
		// 如果 k-1 是 S 类型，为稍后在此循环中处理将 k 排队。
		// 如果 k-1 是 L 类型（text[k-1] > text[k]），为调用者排队 -k 保存。
		k := j - 1
		c1 := text[k]
		c0 := text[k-1]
		if c0 > c1 {
			k = -k
		}

		if cB != c1 {
			bucket[cB] = b
			cB = c1
			b = bucket[cB]
		}
		b--
		sa[b] = int32(k)
	}
}

// length_8_32 计算并记录文本中每个 LMS 子串的长度。
// 索引 j 处的 LMS 子串的长度存储在 sa[j/2]，
// 避免已存储在 sa 上半部分的 LMS 子串索引。
// （如果索引 j 是 LMS 子串开始，则索引 j-1 是 L 类型，不能是。）
// 有两个例外，为下面的 name_8_32 优化。
//
// 首先，最后的 LMS 子串被记录为长度 0，这否则是
// 不可能的，而不是给它一个包括隐式哨兵的长度。
// 这确保最后的 LMS 子串的长度不等于所有其他的
// 因此可以被检测为不同而无需文本比较
// （它不相等，因为它是唯一以隐式哨兵结尾的，
// 文本比较会有问题，因为隐式哨兵
// 实际上不在 text[len(text)] 处）。
//
// 其次，为了完全避免文本比较，如果 LMS 子串非常短，
// sa[j/2] 记录其实际文本而不是其长度，所以如果两个这样的
// 子串有匹配的"长度"，根本不需要读取文本。
// "非常短"的定义是文本字节必须打包成 uint32，
// 无符号编码 e 必须是 ≥ len(text)，所以它可以
// 从有效长度中区分。
func length_8_32(text []byte, sa []int32, numLMS int) {
	end := 0 // 当前 LMS 子串结尾的索引（0 表示最后的 LMS 子串）

	// N 个文本字节编码为"长度"字的编码
	// 为每个字节加 1，将它们打包到底部
	// 字的 N*8 位，然后按位反转结果。
	// 也就是说，文本序列 A B C（十六进制 41 42 43）
	// 编码为 ^uint32(0x42_43_44)。
	// LMS 子串永远不能以 0xFF 开始或结尾。
	// 添加 1 确保编码的字节序列永远不会
	// 以 0x00 开始或结尾，所以存在的字节可以
	// 与顶部位中的零填充区分，
	// 所以长度不需要单独编码。
	// 反转字节增加了
	// 4 字节编码仍然是 ≥ len(text) 的机会。
	// 特别是，如果第一个字节是 ASCII（<= 0x7E，所以 +1 <= 0x7F）
	// 那么反转的高位将被设置，
	// 使其明确不是有效长度（这将是负数）。
	//
	// cx 保存预反转编码（打包的增量字节）。
	cx := uint32(0) // byte-only

	// 这一节（直到空白行）是"LMS 子串迭代器"，
	// 在上面的 placeLMS_8_32 中描述，添加了一行来维护 cx。
	c0, c1, isTypeS := byte(0), byte(0), false
	for i := len(text) - 1; i >= 0; i-- {
		c0, c1 = text[i], c0
		cx = cx<<8 | uint32(c1+1) // byte-only
		if c0 < c1 {
			isTypeS = true
		} else if c0 > c1 && isTypeS {
			isTypeS = false

			// 索引 j = i+1 是 LMS 子串的开始。
			// 计算长度或编码文本以存储在 sa[j/2]。
			j := i + 1
			var code int32
			if end == 0 {
				code = 0
			} else {
				code = int32(end - j)
				if code <= 32/8 && ^cx >= uint32(len(text)) { // byte-only
					code = int32(^cx) // byte-only
				} // byte-only
			}
			sa[j>>1] = code
			end = j + 1
			cx = uint32(c1 + 1) // byte-only
		}
	}
}

// assignID_8_32 为 LMS 子串的集合分配密集 ID 编号
// 尊重字符串排序和相等性，
// 返回最大分配的 ID。
// 例如，给定输入"ababab"，LMS 子串
// 是"aba"、"aba"和"ab"，重新编号为 2 2 1。
// sa[len(sa)-numLMS:] 保存 LMS 子串索引
// 按字符串顺序排序，所以分配数字我们可以
// 逐个考虑，删除相邻的重复。
// 索引 j 处 LMS 子串的新 ID 写入 sa[j/2]，
// 覆盖之前存储在那里的长度（由上面的 length_8_32）。
func assignID_8_32(text []byte, sa []int32, numLMS int) int {
	id := 0
	lastLen := int32(-1) // impossible
	lastPos := int32(0)
	for _, j := range sa[len(sa)-numLMS:] {
		// Is the LMS-substring at index j new, or is it the same as the last one we saw?
		n := sa[j/2]
		if n != lastLen {
			goto New
		}
		if uint32(n) >= uint32(len(text)) {
			// "长度"实际上是编码的完整文本，并且它们匹配。
			goto Same
		}
		{
			// 比较实际文本。
			n := int(n)
			this := text[j:][:n]
			last := text[lastPos:][:n]
			for i := 0; i < n; i++ {
				if this[i] != last[i] {
					goto New
				}
			}
			goto Same
		}
	New:
		id++
		lastPos = j
		lastLen = n
	Same:
		sa[j/2] = int32(id)
	}
	return id
}

// map_32 将文本中的 LMS 子串映射到其新 ID，
// 为递归产生子问题。
// 映射本身主要由 assignID_8_32 应用：
// sa[i] 要么是 0、索引 2*i 处 LMS 子串的 ID，
// 要么是索引 2*i+1 处 LMS 子串的 ID。
// 要产生子问题，我们只需要删除零
// 并将 ID 更改为 ID-1（我们的 ID 从 1 开始，但文本字符从 0 开始）。
//
// map_32 打包结果，这是递归的输入，
// 到 sa 的顶部，所以递归结果可以存储
// 在 sa 的底部，这为 expand_8_32 设置得很好。
func map_32(sa []int32, numLMS int) {
	w := len(sa)
	for i := len(sa) / 2; i >= 0; i-- {
		j := sa[i]
		if j > 0 {
			w--
			sa[w] = j - 1
		}
	}
}

// recurse_32 递归调用 sais_32 来解决我们构建的子问题。
// 子问题在 sa 的右端，后缀数组结果将被
// 写在 sa 的左端，sa 的中间可用作
// 临时频率和桶存储。
func recurse_32(sa, oldTmp []int32, numLMS, maxID int) {
	dst, saTmp, text := sa[:numLMS], sa[numLMS:len(sa)-numLMS], sa[len(sa)-numLMS:]

	// 为递归调用设置临时空间。
	// 我们必须向 sais_32 传递一个至少有 maxID 个条目的 tmp 缓冲区。
	//
	// 子问题的长度保证最多为 len(sa)/2，
	// 所以 sa 可以同时保存子问题及其后缀数组。
	// 几乎总是，子问题的长度 < len(sa)/3，
	// 在这种情况下，sa 中间有一个子问题大小的部分
	// 我们可以重用来做临时空间（saTmp）。
	// 当从 sais_8_32 调用 recurse_32 时，oldTmp 长度为 512
	// （来自 text_32），saTmp 通常会大得多，所以我们将使用 saTmp。
	// 当更深的递归回到 recurse_32 时，现在 oldTmp 是
	// 最顶层递归的 saTmp，通常大于
	// 当前的 saTmp（因为当前 sa 越来越小
	// 随着递归变得更深），我们继续重用那个最顶层的
	// 大 saTmp 而不是提供的较小的。
	//
	// 为什么子问题的长度如此频繁地略低于 len(sa)/3？
	// 参见 Nong、Zhang 和 Chen，第 3.6 节以获得合理的解释。
	// 简而言之，len(sa)/2 的情况对应于 SLSLSLSLSLSL 模式
	// 在输入中，较大和较小输入字节的完美交替。
	// 真实文本不会这样做。如果每个 L 类型索引随机后跟
	// L 类型或 S 类型索引，那么一半的子串将
	// 是 SLS 形式，但另一半将更长。其中的一半，
	// 一半（总体上四分之一）将是 SLLS；八分之一将是 SLLLS，等等。
	// 不计每个中的最后一个 S（与下一个中的第一个 S 重叠），
	// 这合计为平均长度 2×½ + 3×¼ + 4×⅛ + ... = 3。
	// 我们需要的空间进一步减少，因为很多
	// 像 SLS 这样的短模式通常是相同的字符序列
	// 在整个文本中重复，相对于 numLMS 减少 maxID。
	//
	// 对于短输入，平均值可能不利于我们，但然后我们
	// 通常可以回到使用顶层调用中可用的长度 512 tmp。
	// （另外，短分配不会是大问题。）
	//
	// 对于病态输入，我们回到分配长度为
	// max(maxID, numLMS/2) 的新 tmp。这个递归级别需要 maxID，
	// 递归的所有更深级别将需要不超过 numLMS/2，
	// 所以这一个分配保证足以处理整个堆栈
	// 的递归调用。
	tmp := oldTmp
	if len(tmp) < len(saTmp) {
		tmp = saTmp
	}
	if len(tmp) < numLMS {
		// TestSAIS/forcealloc 到达此代码。
		n := maxID
		if n < numLMS/2 {
			n = numLMS / 2
		}
		tmp = make([]int32, n)
	}

	// sais_32 要求调用者安排清除 dst，
	// 因为一般来说，调用者可能知道 dst 是
	// 新分配且已清除。但这个不是。
	clear(dst)
	sais_32(text, maxID, dst, tmp)
}

// unmap_8_32 将子问题解映射回原始。
// sa[:numLMS] 是 LMS 子串编号，已经不重要了。
// sa[len(sa)-numLMS:] 是那些 LMS 子串编号的排序列表。
// 关键部分是如果列表说 K，那表示第 K 个子串。
// 我们可以用 LMS 子串的索引替换 sa[:numLMS]。
// 然后如果列表说 K，它真的表示 sa[K]。
// 将列表映射回 LMS 子串索引后，
// 我们可以将那些放入正确的桶。
func unmap_8_32(text []byte, sa []int32, numLMS int) {
	unmap := sa[len(sa)-numLMS:]
	j := len(unmap)

	// "LMS 子串迭代器"（参见上面的 placeLMS_8_32）。
	c0, c1, isTypeS := byte(0), byte(0), false
	for i := len(text) - 1; i >= 0; i-- {
		c0, c1 = text[i], c0
		if c0 < c1 {
			isTypeS = true
		} else if c0 > c1 && isTypeS {
			isTypeS = false

			// 填充逆映射。
			j--
			unmap[j] = int32(i + 1)
		}
	}

	// 对子问题后缀数组应用逆映射。
	sa = sa[:numLMS]
	for i := 0; i < len(sa); i++ {
		sa[i] = unmap[sa[i]]
	}
}

// expand_8_32 将压缩的、排序的 LMS 后缀索引分发
// 从 sa[:numLMS] 到 sa 中相应桶的顶部，
// 保留排序顺序并为 L 类型索引腾出空间
// 由 induceL_8_32 插入排序序列。
func expand_8_32(text []byte, freq, bucket, sa []int32, numLMS int) {
	bucketMax_8_32(text, freq, bucket)
	bucket = bucket[:256] // 消除下面 bucket[c] 的边界检查

	// 向后循环通过 sa，总是跟踪
	// 下一个要从 sa[:numLMS] 填充的索引。
	// 当我们到达一个时，填充它。
	// 将其余槽归零；它们内部有死值。
	x := numLMS - 1
	saX := sa[x]
	c := text[saX]
	b := bucket[c] - 1
	bucket[c] = b

	for i := len(sa) - 1; i >= 0; i-- {
		if i != int(b) {
			sa[i] = 0
			continue
		}
		sa[i] = saX

		// 加载下一个条目以放下（如果有的话）。
		if x > 0 {
			x--
			saX = sa[x] // TODO 边界检查
			c = text[saX]
			b = bucket[c] - 1
			bucket[c] = b
		}
	}
}

// induceL_8_32 将 L 类型文本索引插入到 sa，
// 假设最左边的 S 类型索引已插入
// 到 sa，按排序顺序，在右桶的一半。
// 它在 sa 中保留所有 L 类型索引，但
// 最左边的 L 类型索引被否定，以标记它们
// 供 induceS_8_32 处理。
func induceL_8_32(text []byte, sa, freq, bucket []int32) {
	// 初始化字符桶左侧的位置。
	bucketMin_8_32(text, freq, bucket)
	bucket = bucket[:256] // 消除下面 bucket[cB] 的边界检查

	// 此扫描类似于上面 induceSubL_8_32 中的扫描。
	// 那个安排清除除最左边 L 类型索引外的所有内容。
	// 此扫描保留所有 L 类型索引和原始 S 类型
	// 索引，但它否定正的最左边 L 类型索引
	// （induceS_8_32 需要处理的那些）。

	// expand_8_32 省略了隐式条目 sa[-1] == len(text)，
	// 对应于标识的 L 类型索引 len(text)-1。
	// 在 sa 正确的从左到右扫描之前处理它。
	// 有关注释，请查看循环中的主体。
	k := len(text) - 1
	c0, c1 := text[k-1], text[k]
	if c0 < c1 {
		k = -k
	}

	// 缓存最近使用的桶索引。
	cB := c1
	b := bucket[cB]
	sa[b] = int32(k)
	b++

	for i := 0; i < len(sa); i++ {
		j := int(sa[i])
		if j <= 0 {
			// 跳过空条目或否定的条目（包括否定的零）。
			continue
		}

		// 索引 j 在工作队列上，意味着 k := j-1 是 L 类型，
		// 所以我们现在可以正确地将 k 放入 sa。
		// 如果 k-1 是 L 类型，为稍后在此循环中处理将 k 排队。
		// 如果 k-1 是 S 类型（text[k-1] < text[k]），为调用者排队 -k 保存。
		// 如果 k 是零，k-1 不存在，所以我们只需要将其保留
		// 给调用者。调用者无法区分
		// 空槽和非空零，但无需
		// 区分它们：最后的后缀数组最终会
		// 在某处有一个零，那将是真实的零。
		k := j - 1
		c1 := text[k]
		if k > 0 {
			if c0 := text[k-1]; c0 < c1 {
				k = -k
			}
		}

		if cB != c1 {
			bucket[cB] = b
			cB = c1
			b = bucket[cB]
		}
		sa[b] = int32(k)
		b++
	}
}

func induceS_8_32(text []byte, sa, freq, bucket []int32) {
	// 初始化字符桶右侧的位置。
	bucketMax_8_32(text, freq, bucket)
	bucket = bucket[:256] // 消除下面 bucket[cB] 的边界检查

	cB := byte(0)
	b := bucket[cB]

	for i := len(sa) - 1; i >= 0; i-- {
		j := int(sa[i])
		if j >= 0 {
			// 跳过未标记的条目。
			// （此循环看不到空条目；0 表示真实零索引。）
			continue
		}

		// 负的 j 是工作队列条目；重写为最终后缀数组的正 j。
		j = -j
		sa[i] = int32(j)

		// 索引 j 在工作队列上（编码为 -j，但现在已解码），
		// 意味着 k := j-1 是 L 类型，
		// 所以我们现在可以正确地将 k 放入 sa。
		// 如果 k-1 是 S 类型，为稍后在此循环中处理排队 -k。
		// 如果 k-1 是 L 类型（text[k-1] > text[k]），排队 k 为调用者保存。
		// 如果 k 是零，k-1 不存在，所以我们只需要将其保留
		// 给调用者。
		k := j - 1
		c1 := text[k]
		if k > 0 {
			if c0 := text[k-1]; c0 <= c1 {
				k = -k
			}
		}

		if cB != c1 {
			bucket[cB] = b
			cB = c1
			b = bucket[cB]
		}
		b--
		sa[b] = int32(k)
	}
}
