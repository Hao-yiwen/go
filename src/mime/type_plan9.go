// 版权所有 2013 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package mime

import (
	"bufio"
	"os"
	"strings"
)

func init() {
	osInitMime = initMimePlan9
}

func initMimePlan9() {
	for _, filename := range typeFiles {
		loadMimeFile(filename)
	}
}

var typeFiles = []string{
	"/sys/lib/mimetype",
}

func initMimeForTests() map[string]string {
	typeFiles = []string{"testdata/test.types.plan9"}
	return map[string]string{
		".t1":  "application/test",
		".t2":  "text/test; charset=utf-8",
		".pNg": "image/png",
	}
}

func loadMimeFile(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) <= 2 || fields[0][0] != '.' {
			continue
		}
		if fields[1] == "-" || fields[2] == "-" {
			continue
		}
		setExtensionType(fields[0], fields[1]+"/"+fields[2])
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}
