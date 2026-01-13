// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm) || wasip1

package mime

import (
	"bufio"
	"os"
	"strings"
)

func init() {
	osInitMime = initMimeUnix
}

// 有关 FreeDesktop 共享 MIME-info 数据库规范，请参阅
// https://specifications.freedesktop.org/shared-mime-info-spec/shared-mime-info-spec-0.21.html
var mimeGlobs = []string{
	"/usr/local/share/mime/globs2",
	"/usr/share/mime/globs2",
}

// unix 上 mime.types 文件的常见位置。
var typeFiles = []string{
	"/etc/mime.types",
	"/etc/apache2/mime.types",
	"/etc/apache/mime.types",
	"/etc/httpd/conf/mime.types",
}

func loadMimeGlobsFile(filename string) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// 每行的格式应为：weight:mimetype:glob[:morefields...]
		fields := strings.Split(scanner.Text(), ":")
		if len(fields) < 3 || len(fields[0]) < 1 || len(fields[2]) < 3 {
			continue
		} else if fields[0][0] == '#' || fields[2][0] != '*' || fields[2][1] != '.' {
			continue
		}

		extension := fields[2][1:]
		if strings.ContainsAny(extension, "?*[") {
			// 不是单纯的扩展名，而是 glob 模式。暂时忽略：
			// - 我们没有实现这种 glob 语法（可以转换为 path/filepath.Match）
			// - 支持带权重排序的 glob 会对所有查找产生性能影响，
			//   而 glob 条目很少见
			// - 尝试字面匹配 glob 元字符没有用处
			continue
		}
		if _, ok := mimeTypes.Load(extension); ok {
			// 我们已经见过这个扩展名了。
			// 文件按权重排序，所以我们保留第一个看到的条目。
			continue
		}

		setExtensionType(extension, fields[1])
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	return nil
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
		if len(fields) <= 1 || fields[0][0] == '#' {
			continue
		}
		mimeType := fields[0]
		for _, ext := range fields[1:] {
			if ext[0] == '#' {
				break
			}
			setExtensionType("."+ext, mimeType)
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
}

func initMimeUnix() {
	for _, filename := range mimeGlobs {
		if err := loadMimeGlobsFile(filename); err == nil {
			return // 如果找到 mimetype 数据库，停止检查更多文件。
		}
	}

	// 如果不存在系统生成的 mimetype 数据库，则回退。
	for _, filename := range typeFiles {
		loadMimeFile(filename)
	}
}

func initMimeForTests() map[string]string {
	mimeGlobs = []string{""}
	typeFiles = []string{"testdata/test.types"}
	return map[string]string{
		".T1":  "application/test",
		".t2":  "text/test; charset=utf-8",
		".png": "image/png",
	}
}
