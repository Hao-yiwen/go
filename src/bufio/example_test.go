// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bufio_test

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

func ExampleWriter() {
	w := bufio.NewWriter(os.Stdout)
	fmt.Fprint(w, "Hello, ")
	fmt.Fprint(w, "world!")
	w.Flush() // 不要忘记 flush！
	// Output: Hello, world!
}

func ExampleWriter_AvailableBuffer() {
	w := bufio.NewWriter(os.Stdout)
	for _, i := range []int64{1, 2, 3, 4} {
		b := w.AvailableBuffer()
		b = strconv.AppendInt(b, i, 10)
		b = append(b, ' ')
		w.Write(b)
	}
	w.Flush()
	// Output: 1 2 3 4
}

// ExampleWriter_ReadFrom 演示如何使用 Writer 的 ReadFrom 方法。
func ExampleWriter_ReadFrom() {
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)

	data := "Hello, world!\nThis is a ReadFrom example."
	reader := strings.NewReader(data)

	n, err := writer.ReadFrom(reader)
	if err != nil {
		fmt.Println("ReadFrom Error:", err)
		return
	}

	if err = writer.Flush(); err != nil {
		fmt.Println("Flush Error:", err)
		return
	}

	fmt.Println("Bytes written:", n)
	fmt.Println("Buffer contents:", buf.String())
	// Output:
	// Bytes written: 41
	// Buffer contents: Hello, world!
	// This is a ReadFrom example.
}

// Scanner 的最简单用法，将标准输入作为一组行读取。
func ExampleScanner_lines() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		fmt.Println(scanner.Text()) // Println 会加回最后的 '\n'
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading standard input:", err)
	}
}

// 将最近的 Scan 调用作为 []byte 返回。
func ExampleScanner_Bytes() {
	scanner := bufio.NewScanner(strings.NewReader("gopher"))
	for scanner.Scan() {
		fmt.Println(len(scanner.Bytes()) == 6)
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "shouldn't see an error scanning a string")
	}
	// Output:
	// true
}

// 使用 Scanner 通过扫描
// 输入作为空格分隔 token 的序列来实现简单的字数统计工具。
func ExampleScanner_words() {
	// 人工输入源。
	const input = "Now is the winter of our discontent,\nMade glorious summer by this sun of York.\n"
	scanner := bufio.NewScanner(strings.NewReader(input))
	// 为扫描操作设置分割函数。
	scanner.Split(bufio.ScanWords)
	// 计算单词。
	count := 0
	for scanner.Scan() {
		count++
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}
	fmt.Printf("%d\n", count)
	// Output: 15
}

// 使用带有自定义分割函数的 Scanner（通过包装 ScanWords 构建）来验证
// 32 位十进制输入。
func ExampleScanner_custom() {
	// 人工输入源。
	const input = "1234 5678 1234567901234567890"
	scanner := bufio.NewScanner(strings.NewReader(input))
	// 通过包装现有的 ScanWords 函数来创建自定义分割函数。
	split := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		advance, token, err = bufio.ScanWords(data, atEOF)
		if err == nil && token != nil {
			_, err = strconv.ParseInt(string(token), 10, 32)
		}
		return
	}
	// 为扫描操作设置分割函数。
	scanner.Split(split)
	// 验证输入
	for scanner.Scan() {
		fmt.Printf("%s\n", scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Invalid input: %s", err)
	}
	// Output:
	// 1234
	// 5678
	// Invalid input: strconv.ParseInt: parsing "1234567901234567890": value out of range
}

// 使用带有自定义分割函数的 Scanner 来解析逗号分隔的
// 列表，其中最后一个值为空。
func ExampleScanner_emptyFinalToken() {
	// 逗号分隔的列表；最后一项为空。
	const input = "1,2,3,4,"
	scanner := bufio.NewScanner(strings.NewReader(input))
	// 定义一个在逗号处分隔的分割函数。
	onComma := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		for i := 0; i < len(data); i++ {
			if data[i] == ',' {
				return i + 1, data[:i], nil
			}
		}
		if !atEOF {
			return 0, nil, nil
		}
		// 有一个最终的 token 要被传送，可能是空字符串。
		// 在这里返回 bufio.ErrFinalToken 告诉 Scan 之后没有更多 token
		// 但不会从 Scan 本身触发返回的错误。
		return 0, data, bufio.ErrFinalToken
	}
	scanner.Split(onComma)
	// 扫描。
	for scanner.Scan() {
		fmt.Printf("%q ", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}
	// Output: "1" "2" "3" "4" ""
}

// 使用带有自定义分割函数的 Scanner 来解析逗号分隔的
// 列表，其中最后一个值为空，但在 token "STOP" 处停止。
func ExampleScanner_earlyStop() {
	onComma := func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		i := bytes.IndexByte(data, ',')
		if i == -1 {
			if !atEOF {
				return 0, nil, nil
			}
			// 如果我们已到达末尾，返回最后一个 token。
			return 0, data, bufio.ErrFinalToken
		}
		// 如果 token 是 "STOP"，停止扫描并忽略其余部分。
		if string(data[:i]) == "STOP" {
			return i + 1, nil, bufio.ErrFinalToken
		}
		// 否则，返回逗号前的 token。
		return i + 1, data[:i], nil
	}
	const input = "1,2,STOP,4,"
	scanner := bufio.NewScanner(strings.NewReader(input))
	scanner.Split(onComma)
	for scanner.Scan() {
		fmt.Printf("Got a token %q\n", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "reading input:", err)
	}
	// Output:
	// Got a token "1"
	// Got a token "2"
}
