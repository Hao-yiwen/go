// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package bzip2

import (
	"cmp"
	"slices"
)

// huffmanTree 是一个二叉树，通过逐位导航来到达一个符号。
type huffmanTree struct {
	// nodes 包含树中所有非叶节点。nodes[0] 是树的根节点，
	// nextNode 包含构建树时要使用的 nodes 的下一个元素的索引。
	nodes    []huffmanNode
	nextNode int
}

// huffmanNode 是树中的一个节点。left 和 right 包含指向树的 nodes 切片的索引。
// 如果 left 或 right 是 invalidNodeValue，则子节点是叶节点，
// 其值在 leftValue/rightValue 中。
//
// 符号是 uint16 类型，因为 bzip2 不仅在树中编码 MTF 索引，
// 还编码两个用于游程长度编码的魔法值和一个 EOF 符号。
// 因此有超过 256 个可能的符号。
type huffmanNode struct {
	left, right           uint16
	leftValue, rightValue uint16
}

// invalidNodeValue 是一个无效索引，用于标记树中的叶节点。
const invalidNodeValue = 0xffff

// Decode 从给定的 bitReader 读取位并导航树，直到找到一个符号。
func (t *huffmanTree) Decode(br *bitReader) (v uint16) {
	nodeIndex := uint16(0) // node 0 是树的根节点。

	for {
		node := &t.nodes[nodeIndex]

		var bit uint16
		if br.bits > 0 {
			// 获取下一位 - 快速路径。
			br.bits--
			bit = uint16(br.n>>(br.bits&63)) & 1
		} else {
			// 获取下一位 - 慢速路径。
			// 使用 ReadBits 从底层 io.ByteReader 检索单个位。
			bit = uint16(br.ReadBits(1))
		}

		// 通过使两个加载都无条件，欺骗编译器生成条件移动而不是分支。
		l, r := node.left, node.right

		if bit == 1 {
			nodeIndex = l
		} else {
			nodeIndex = r
		}

		if nodeIndex == invalidNodeValue {
			// 我们找到了一个叶节点。使用 bit 的值来决定是左值还是右值。
			l, r := node.leftValue, node.rightValue
			if bit == 1 {
				v = l
			} else {
				v = r
			}
			return
		}
	}
}

// newHuffmanTree 从包含每个符号的码长的切片构建一个 Huffman 树。
// 最大码长为 32 位。
func newHuffmanTree(lengths []uint8) (huffmanTree, error) {
	// 有很多可能的树为每个符号分配相同的码长（例如，考虑沿中间反射一棵树）。
	// 由于码长分配决定了树的效率，这些树中的每一棵都同样好。
	// 为了最小化构建树所需的信息量，bzip2 使用规范树，
	// 这样只需给定码长分配就可以重建它。

	if len(lengths) < 2 {
		panic("newHuffmanTree: too few symbols")
	}

	var t huffmanTree

	// 首先我们按码长升序排序码长分配，使用符号值来打破平局。
	pairs := make([]huffmanSymbolLengthPair, len(lengths))
	for i, length := range lengths {
		pairs[i].value = uint16(i)
		pairs[i].length = length
	}

	slices.SortFunc(pairs, func(a, b huffmanSymbolLengthPair) int {
		if c := cmp.Compare(a.length, b.length); c != 0 {
			return c
		}
		return cmp.Compare(a.value, b.value)
	})

	// 现在我们为符号分配编码，从最长的编码开始。
	// 我们将编码打包到 uint32 中，在最高有效位端。
	// 所以分支从 MSB 向下取。这使得以后排序变得容易。
	code := uint32(0)
	length := uint8(32)

	codes := make([]huffmanCode, len(lengths))
	for i := len(pairs) - 1; i >= 0; i-- {
		if length > pairs[i].length {
			length = pairs[i].length
		}
		codes[i].code = code
		codes[i].codeLen = length
		codes[i].value = pairs[i].value
		// 我们需要"递增"编码，这意味着将 |code| 视为 |length| 位数字。
		code += 1 << (32 - length)
	}

	// 现在我们可以按编码排序，这样每个分支的左半部分会递归地分组在一起。
	slices.SortFunc(codes, func(a, b huffmanCode) int {
		return cmp.Compare(a.code, b.code)
	})

	t.nodes = make([]huffmanNode, len(codes))
	_, err := buildHuffmanNode(&t, codes, 0)
	return t, err
}

// huffmanSymbolLengthPair 包含一个符号及其码长。
type huffmanSymbolLengthPair struct {
	value  uint16
	length uint8
}

// huffmanCode 包含一个符号、其编码和码长。
type huffmanCode struct {
	code    uint32
	codeLen uint8
	value   uint16
}

// buildHuffmanNode 接受一个已排序的 huffmanCodes 切片，并在给定层级构建
// Huffman 树中的一个节点。它返回新构建节点的索引。
func buildHuffmanNode(t *huffmanTree, codes []huffmanCode, level uint32) (nodeIndex uint16, err error) {
	test := uint32(1) << (31 - level)

	// 我们必须搜索编码列表以找到左右两侧之间的分界点。
	firstRightIndex := len(codes)
	for i, code := range codes {
		if code.code&test != 0 {
			firstRightIndex = i
			break
		}
	}

	left := codes[:firstRightIndex]
	right := codes[firstRightIndex:]

	if len(left) == 0 || len(right) == 0 {
		// Huffman 树中有一个多余的层级，表明编码器中有 bug。
		// 然而，这个 bug 已在实际中被观察到，所以我们处理它。

		// 如果这个函数是递归调用的，那么我们知道 len(codes) >= 2，
		// 否则，我们会命中下面的"叶节点"情况，而不会递归。
		//
		// 然而，对于初始调用，len(codes) 可能是零或一。
		// 这两种情况都是无效的，因为零长度树无法编码任何内容，
		// 长度为 1 的树只能编码 EOF，因此是多余的。我们拒绝这两种情况。
		if len(codes) < 2 {
			return 0, StructuralError("empty Huffman tree")
		}

		// 在这种情况下，递归并不总是减少 codes 的长度，
		// 所以我们需要通过另一种机制来确保终止。
		if level == 31 {
			// 由于 len(codes) >= 2，值在所有 32 位上匹配的唯一方式
			// 是它们相等，这是无效的。这确保我们永远不会进入无限递归。
			return 0, StructuralError("equal symbols in Huffman tree")
		}

		if len(left) == 0 {
			return buildHuffmanNode(t, right, level+1)
		}
		return buildHuffmanNode(t, left, level+1)
	}

	nodeIndex = uint16(t.nextNode)
	node := &t.nodes[t.nextNode]
	t.nextNode++

	if len(left) == 1 {
		// 叶节点
		node.left = invalidNodeValue
		node.leftValue = left[0].value
	} else {
		node.left, err = buildHuffmanNode(t, left, level+1)
	}

	if err != nil {
		return
	}

	if len(right) == 1 {
		// 叶节点
		node.right = invalidNodeValue
		node.rightValue = right[0].value
	} else {
		node.right, err = buildHuffmanNode(t, right, level+1)
	}

	return
}
