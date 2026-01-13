// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package mime 实现了 MIME 规范的部分功能。
package mime

import (
	"fmt"
	"slices"
	"strings"
	"sync"
)

var (
	mimeTypes      sync.Map // map[string]string; ".Z" => "application/x-compress"
	mimeTypesLower sync.Map // map[string]string; ".z" => "application/x-compress"

	// extensions 从 MIME 类型映射到小写文件扩展名列表：
	// "image/jpeg" => [".jfif", ".jpg", ".jpeg", ".pjp", ".pjpeg"]
	extensionsMu sync.Mutex // 保护 extensions 的存储操作（但不包括加载操作）。
	extensions   sync.Map   // map[string][]string; 切片值只追加不修改。
)

// setMimeTypes 由 initMime 的非测试路径和测试使用。
func setMimeTypes(lowerExt, mixExt map[string]string) {
	mimeTypes.Clear()
	mimeTypesLower.Clear()
	extensions.Clear()

	for k, v := range lowerExt {
		mimeTypesLower.Store(k, v)
	}
	for k, v := range mixExt {
		mimeTypes.Store(k, v)
	}

	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	for k, v := range lowerExt {
		justType, _, err := ParseMediaType(v)
		if err != nil {
			panic(err)
		}
		var exts []string
		if ei, ok := extensions.Load(justType); ok {
			exts = ei.([]string)
		}
		extensions.Store(justType, append(exts, k))
	}
}

// 如果 Firefox 和 Chrome 在各自的列表中都包含某个类型，则该类型会在此处列出。
// 当两者存在冲突时，使用 IANA 列出的媒体类型进行解决冲突
// https://www.iana.org/assignments/media-types/media-types.xhtml
//
// Chrome 的 MIME 类型到文件扩展名的映射定义在
// https://chromium.googlesource.com/chromium/src.git/+/refs/heads/main/net/base/mime_util.cc
//
// Firefox 的 MIME 类型可在以下位置找到
// https://github.com/mozilla-firefox/firefox/blob/main/netwerk/mime/nsMimeTypes.h
// 文件扩展名的映射在
// https://github.com/mozilla-firefox/firefox/blob/main/uriloader/exthandler/nsExternalHelperAppService.cpp
var builtinTypesLower = map[string]string{
	".ai":    "application/postscript",
	".apk":   "application/vnd.android.package-archive",
	".apng":  "image/apng",
	".avif":  "image/avif",
	".bin":   "application/octet-stream",
	".bmp":   "image/bmp",
	".com":   "application/octet-stream",
	".css":   "text/css; charset=utf-8",
	".csv":   "text/csv; charset=utf-8",
	".doc":   "application/msword",
	".docx":  "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".ehtml": "text/html; charset=utf-8",
	".eml":   "message/rfc822",
	".eps":   "application/postscript",
	".exe":   "application/octet-stream",
	".flac":  "audio/flac",
	".gif":   "image/gif",
	".gz":    "application/gzip",
	".htm":   "text/html; charset=utf-8",
	".html":  "text/html; charset=utf-8",
	".ico":   "image/vnd.microsoft.icon",
	".ics":   "text/calendar; charset=utf-8",
	".jfif":  "image/jpeg",
	".jpeg":  "image/jpeg",
	".jpg":   "image/jpeg",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".m4a":   "audio/mp4",
	".mjs":   "text/javascript; charset=utf-8",
	".mp3":   "audio/mpeg",
	".mp4":   "video/mp4",
	".oga":   "audio/ogg",
	".ogg":   "audio/ogg",
	".ogv":   "video/ogg",
	".opus":  "audio/ogg",
	".pdf":   "application/pdf",
	".pjp":   "image/jpeg",
	".pjpeg": "image/jpeg",
	".png":   "image/png",
	".ppt":   "application/vnd.ms-powerpoint",
	".pptx":  "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".ps":    "application/postscript",
	".rdf":   "application/rdf+xml",
	".rtf":   "application/rtf",
	".shtml": "text/html; charset=utf-8",
	".svg":   "image/svg+xml",
	".text":  "text/plain; charset=utf-8",
	".tif":   "image/tiff",
	".tiff":  "image/tiff",
	".txt":   "text/plain; charset=utf-8",
	".vtt":   "text/vtt; charset=utf-8",
	".wasm":  "application/wasm",
	".wav":   "audio/wav",
	".webm":  "audio/webm",
	".webp":  "image/webp",
	".xbl":   "text/xml; charset=utf-8",
	".xbm":   "image/x-xbitmap",
	".xht":   "application/xhtml+xml",
	".xhtml": "application/xhtml+xml",
	".xls":   "application/vnd.ms-excel",
	".xlsx":  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xml":   "text/xml; charset=utf-8",
	".xsl":   "text/xml; charset=utf-8",
	".zip":   "application/zip",
}

var once sync.Once // 保护 initMime

var testInitMime, osInitMime func()

func initMime() {
	if fn := testInitMime; fn != nil {
		fn()
	} else {
		setMimeTypes(builtinTypesLower, builtinTypesLower)
		osInitMime()
	}
}

// TypeByExtension 返回与文件扩展名 ext 关联的 MIME 类型。
// 扩展名 ext 应以点开头，如 ".html"。
// 当 ext 没有关联的类型时，TypeByExtension 返回 ""。
//
// 扩展名首先进行区分大小写的查找，然后进行不区分大小写的查找。
//
// 内置表较小，但在 unix 上会通过本地系统的 MIME-info 数据库或 mime.types 文件进行扩充，
// 如果这些文件存在于以下一个或多个位置：
//
//	/usr/local/share/mime/globs2
//	/usr/share/mime/globs2
//	/etc/mime.types
//	/etc/apache2/mime.types
//	/etc/apache/mime.types
//	/etc/httpd/conf/mime.types
//
// 在 Windows 上，MIME 类型从注册表中提取。
//
// 文本类型的 charset 参数默认设置为 "utf-8"。
func TypeByExtension(ext string) string {
	once.Do(initMime)

	// 区分大小写的查找。
	if v, ok := mimeTypes.Load(ext); ok {
		return v.(string)
	}

	// 不区分大小写的查找。
	// 乐观地假设是短 ASCII 扩展名，这种情况下不会分配内存。
	var buf [10]byte
	lower := buf[:0]
	const utf8RuneSelf = 0x80 // 来自 utf8 包，但不导入它。
	for i := 0; i < len(ext); i++ {
		c := ext[i]
		if c >= utf8RuneSelf {
			// 慢路径。
			si, _ := mimeTypesLower.Load(strings.ToLower(ext))
			s, _ := si.(string)
			return s
		}
		if 'A' <= c && c <= 'Z' {
			lower = append(lower, c+('a'-'A'))
		} else {
			lower = append(lower, c)
		}
	}
	si, _ := mimeTypesLower.Load(string(lower))
	s, _ := si.(string)
	return s
}

// ExtensionsByType 返回已知与 MIME 类型 typ 关联的扩展名。
// 返回的每个扩展名都以点开头，如 ".html"。
// 当 typ 没有关联的扩展名时，ExtensionsByType 返回 nil 切片。
//
// 内置表较小，但在 unix 上会通过本地系统的 MIME-info 数据库或 mime.types 文件进行扩充，
// 如果这些文件存在于以下一个或多个位置：
//
//	/usr/local/share/mime/globs2
//	/usr/share/mime/globs2
//	/etc/mime.types
//	/etc/apache2/mime.types
//	/etc/apache/mime.types
//	/etc/httpd/conf/mime.types
//
// 在 Windows 上，扩展名从注册表中提取。
func ExtensionsByType(typ string) ([]string, error) {
	justType, _, err := ParseMediaType(typ)
	if err != nil {
		return nil, err
	}

	once.Do(initMime)
	s, ok := extensions.Load(justType)
	if !ok {
		return nil, nil
	}
	ret := append([]string(nil), s.([]string)...)
	slices.Sort(ret)
	return ret, nil
}

// AddExtensionType 将扩展名 ext 关联的 MIME 类型设置为 typ。
// 扩展名应以点开头，如 ".html"。
func AddExtensionType(ext, typ string) error {
	if !strings.HasPrefix(ext, ".") {
		return fmt.Errorf("mime: extension %q missing leading dot", ext)
	}
	once.Do(initMime)
	return setExtensionType(ext, typ)
}

func setExtensionType(extension, mimeType string) error {
	justType, param, err := ParseMediaType(mimeType)
	if err != nil {
		return err
	}
	if strings.HasPrefix(mimeType, "text/") && param["charset"] == "" {
		param["charset"] = "utf-8"
		mimeType = FormatMediaType(mimeType, param)
	}
	extLower := strings.ToLower(extension)

	mimeTypes.Store(extension, mimeType)
	mimeTypesLower.Store(extLower, mimeType)

	extensionsMu.Lock()
	defer extensionsMu.Unlock()
	var exts []string
	if ei, ok := extensions.Load(justType); ok {
		exts = ei.([]string)
	}
	for _, v := range exts {
		if v == extLower {
			return nil
		}
	}
	extensions.Store(justType, append(exts, extLower))
	return nil
}
