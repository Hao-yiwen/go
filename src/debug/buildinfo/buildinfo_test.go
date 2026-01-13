// 版权所有 2021 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package buildinfo_test

import (
	"bytes"
	"debug/buildinfo"
	"debug/pe"
	"encoding/binary"
	"flag"
	"fmt"
	"internal/obscuretestdata"
	"internal/testenv"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var flagAll = flag.Bool("all", false, "test all supported GOOS/GOARCH platforms, instead of only the current platform")

// TestReadFile 确认 ReadFile 能够从支持的目标平台上的二进制文件中读取构建信息。
// 它在当前平台（如果设置了 -all 则在所有平台上）以各种配置构建一个简单的二进制文件，
// 并检查是否可以读取构建信息。
func TestReadFile(t *testing.T) {
	if testing.Short() {
		t.Skip("test requires compiling and linking, which may be slow")
	}
	testenv.MustHaveGoBuild(t)

	type platform struct{ goos, goarch string }
	platforms := []platform{
		{"aix", "ppc64"},
		{"darwin", "amd64"},
		{"darwin", "arm64"},
		{"linux", "386"},
		{"linux", "amd64"},
		{"windows", "386"},
		{"windows", "amd64"},
	}
	runtimePlatform := platform{runtime.GOOS, runtime.GOARCH}
	haveRuntimePlatform := false
	for _, p := range platforms {
		if p == runtimePlatform {
			haveRuntimePlatform = true
			break
		}
	}
	if !haveRuntimePlatform {
		platforms = append(platforms, runtimePlatform)
	}

	buildModes := []string{"pie", "exe"}
	if testenv.HasCGO() {
		buildModes = append(buildModes, "c-shared")
	}

	// 与 src/cmd/go/internal/work/init.go:buildModeInit 保持同步。
	badmode := func(goos, goarch, buildmode string) string {
		return fmt.Sprintf("-buildmode=%s not supported on %s/%s", buildmode, goos, goarch)
	}

	buildWithModules := func(t *testing.T, goos, goarch, buildmode string) string {
		dir := t.TempDir()
		gomodPath := filepath.Join(dir, "go.mod")
		gomodData := []byte("module example.com/m\ngo 1.18\n")
		if err := os.WriteFile(gomodPath, gomodData, 0666); err != nil {
			t.Fatal(err)
		}
		helloPath := filepath.Join(dir, "hello.go")
		helloData := []byte("package main\nfunc main() {}\n")
		if err := os.WriteFile(helloPath, helloData, 0666); err != nil {
			t.Fatal(err)
		}
		outPath := filepath.Join(dir, path.Base(t.Name()))
		cmd := exec.Command(testenv.GoToolPath(t), "build", "-o="+outPath, "-buildmode="+buildmode)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GO111MODULE=on", "GOOS="+goos, "GOARCH="+goarch)
		stderr := &strings.Builder{}
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			if badmodeMsg := badmode(goos, goarch, buildmode); strings.Contains(stderr.String(), badmodeMsg) {
				t.Skip(badmodeMsg)
			}
			t.Fatalf("failed building test file: %v\n%s", err, stderr.String())
		}
		return outPath
	}

	buildWithGOPATH := func(t *testing.T, goos, goarch, buildmode string) string {
		gopathDir := t.TempDir()
		pkgDir := filepath.Join(gopathDir, "src/example.com/m")
		if err := os.MkdirAll(pkgDir, 0777); err != nil {
			t.Fatal(err)
		}
		helloPath := filepath.Join(pkgDir, "hello.go")
		helloData := []byte("package main\nfunc main() {}\n")
		if err := os.WriteFile(helloPath, helloData, 0666); err != nil {
			t.Fatal(err)
		}
		outPath := filepath.Join(gopathDir, path.Base(t.Name()))
		cmd := exec.Command(testenv.GoToolPath(t), "build", "-o="+outPath, "-buildmode="+buildmode)
		cmd.Dir = pkgDir
		cmd.Env = append(os.Environ(), "GO111MODULE=off", "GOPATH="+gopathDir, "GOOS="+goos, "GOARCH="+goarch)
		stderr := &strings.Builder{}
		cmd.Stderr = stderr
		if err := cmd.Run(); err != nil {
			if badmodeMsg := badmode(goos, goarch, buildmode); strings.Contains(stderr.String(), badmodeMsg) {
				t.Skip(badmodeMsg)
			}
			t.Fatalf("failed building test file: %v\n%s", err, stderr.String())
		}
		return outPath
	}

	damageBuildInfo := func(t *testing.T, name string) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		i := bytes.Index(data, []byte("\xff Go buildinf:"))
		if i < 0 {
			t.Fatal("Go buildinf not found")
		}
		data[i+2] = 'N'
		if err := os.WriteFile(name, data, 0666); err != nil {
			t.Fatal(err)
		}
	}

	damageStringLen := func(t *testing.T, name string) {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		i := bytes.Index(data, []byte("\xff Go buildinf:"))
		if i < 0 {
			t.Fatal("Go buildinf not found")
		}
		verLen := data[i+32:]
		binary.PutUvarint(verLen, 16<<40) // 16TB 对任何人来说应该足够了。
		if err := os.WriteFile(name, data, 0666); err != nil {
			t.Fatal(err)
		}
	}

	goVersionRe := regexp.MustCompile("(?m)^go\t.*\n")
	buildRe := regexp.MustCompile("(?m)^build\t.*\n")
	cleanOutputForComparison := func(got string) string {
		// 删除或替换任何可能依赖于测试环境的内容，
		// 以便我们之后可以用字符串比较来检查输出。
		// 我们将删除除编译器之外的所有构建行，只是为了确保包含了构建行。
		got = goVersionRe.ReplaceAllString(got, "go\tGOVERSION\n")
		got = buildRe.ReplaceAllStringFunc(got, func(match string) string {
			if strings.HasPrefix(match, "build\t-compiler=") {
				return match
			}
			return ""
		})
		return got
	}

	cases := []struct {
		name    string
		build   func(t *testing.T, goos, goarch, buildmode string) string
		want    string
		wantErr string
	}{
		{
			name: "doesnotexist",
			build: func(t *testing.T, goos, goarch, buildmode string) string {
				return "doesnotexist.txt"
			},
			wantErr: "doesnotexist",
		},
		{
			name: "empty",
			build: func(t *testing.T, _, _, _ string) string {
				dir := t.TempDir()
				name := filepath.Join(dir, "empty")
				if err := os.WriteFile(name, nil, 0666); err != nil {
					t.Fatal(err)
				}
				return name
			},
			wantErr: "unrecognized file format",
		},
		{
			name:  "valid_modules",
			build: buildWithModules,
			want: "go\tGOVERSION\n" +
				"path\texample.com/m\n" +
				"mod\texample.com/m\t(devel)\t\n" +
				"build\t-compiler=gc\n",
		},
		{
			name: "invalid_modules",
			build: func(t *testing.T, goos, goarch, buildmode string) string {
				name := buildWithModules(t, goos, goarch, buildmode)
				damageBuildInfo(t, name)
				return name
			},
			wantErr: "not a Go executable",
		},
		{
			name: "invalid_str_len",
			build: func(t *testing.T, goos, goarch, buildmode string) string {
				name := buildWithModules(t, goos, goarch, buildmode)
				damageStringLen(t, name)
				return name
			},
			wantErr: "not a Go executable",
		},
		{
			name:  "valid_gopath",
			build: buildWithGOPATH,
			want: "go\tGOVERSION\n" +
				"path\texample.com/m\n" +
				"build\t-compiler=gc\n",
		},
		{
			name: "invalid_gopath",
			build: func(t *testing.T, goos, goarch, buildmode string) string {
				name := buildWithGOPATH(t, goos, goarch, buildmode)
				damageBuildInfo(t, name)
				return name
			},
			wantErr: "not a Go executable",
		},
	}

	for _, p := range platforms {
		t.Run(p.goos+"_"+p.goarch, func(t *testing.T) {
			if p != runtimePlatform && !*flagAll {
				t.Skipf("skipping platforms other than %s_%s because -all was not set", runtimePlatform.goos, runtimePlatform.goarch)
			}
			for _, mode := range buildModes {
				t.Run(mode, func(t *testing.T) {
					for _, tc := range cases {
						t.Run(tc.name, func(t *testing.T) {
							t.Parallel()
							name := tc.build(t, p.goos, p.goarch, mode)
							if info, err := buildinfo.ReadFile(name); err != nil {
								if tc.wantErr == "" {
									t.Fatalf("unexpected error: %v", err)
								} else if errMsg := err.Error(); !strings.Contains(errMsg, tc.wantErr) {
									t.Fatalf("got error %q; want error containing %q", errMsg, tc.wantErr)
								}
							} else {
								if tc.wantErr != "" {
									t.Fatalf("unexpected success; want error containing %q", tc.wantErr)
								}
								got := info.String()
								if clean := cleanOutputForComparison(got); got != tc.want && clean != tc.want {
									t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
								}
							}
						})
					}
				})
			}
		})
	}
}

// Test117 验证旧的 1.18 之前格式的解析是否正常工作。
func Test117(t *testing.T) {
	b, err := obscuretestdata.ReadFile("testdata/go117/go117.base64")
	if err != nil {
		t.Fatalf("ReadFile got err %v, want nil", err)
	}

	info, err := buildinfo.Read(bytes.NewReader(b))
	if err != nil {
		t.Fatalf("Read got err %v, want nil", err)
	}

	if info.GoVersion != "go1.17" {
		t.Errorf("GoVersion got %s want go1.17", info.GoVersion)
	}
	if info.Path != "example.com/go117" {
		t.Errorf("Path got %s want example.com/go117", info.Path)
	}
	if info.Main.Path != "example.com/go117" {
		t.Errorf("Main.Path got %s want example.com/go117", info.Main.Path)
	}
}

// TestNotGo 验证解析非 Go 二进制文件时返回正确的错误。
func TestNotGo(t *testing.T) {
	b, err := obscuretestdata.ReadFile("testdata/notgo/notgo.base64")
	if err != nil {
		t.Fatalf("ReadFile got err %v, want nil", err)
	}

	_, err = buildinfo.Read(bytes.NewReader(b))
	if err == nil {
		t.Fatalf("Read got nil err, want non-nil")
	}

	// 这里精确的错误文本不是关键，但我们希望得到类似
	// errNotGoExe 的错误，而不是例如文件读取错误。
	if !strings.Contains(err.Error(), "not a Go executable") {
		t.Errorf("ReadFile got err %v want not a Go executable", err)
	}
}

// FuzzIssue57002 是 golang.org/issue/57002 的回归测试。
//
// issue 57002 的原因是当 pointerSize 未被检查时，
// 读取可能会因切片边界越界而 panic
func FuzzIssue57002(f *testing.F) {
	// 来自 issue 的输入
	f.Add([]byte{0x4d, 0x5a, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x50, 0x45, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x20, 0x20, 0x20, 0x20, 0x0, 0x0, 0x0, 0x0, 0x20, 0x3f, 0x0, 0x20, 0x0, 0x0, 0x20, 0x20, 0x20, 0x20, 0x20, 0xff, 0x20, 0x20, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xb, 0x20, 0x20, 0x20, 0xfc, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x9, 0x0, 0x0, 0x0, 0x20, 0x0, 0x0, 0x0, 0x20, 0x20, 0x20, 0x20, 0x20, 0xef, 0x20, 0xff, 0xbf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xf, 0x0, 0x2, 0x0, 0x20, 0x0, 0x0, 0x9, 0x0, 0x4, 0x0, 0x20, 0xf6, 0x0, 0xd3, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x1, 0x0, 0x0, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0xa, 0x20, 0xa, 0x20, 0x20, 0x20, 0xff, 0x20, 0x20, 0xff, 0x20, 0x47, 0x6f, 0x20, 0x62, 0x75, 0x69, 0x6c, 0x64, 0x69, 0x6e, 0x66, 0x3a, 0xde, 0xb5, 0xdf, 0xff, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x6, 0x7f, 0x7f, 0x7f, 0x20, 0xf4, 0xb2, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x1, 0x0, 0x0, 0xb, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x20, 0x20, 0x0, 0x0, 0x0, 0x0, 0x5, 0x0, 0x20, 0x20, 0x20, 0x20, 0x0, 0x0, 0x0, 0x0, 0x20, 0x3f, 0x27, 0x20, 0x0, 0xd, 0x0, 0xa, 0x20, 0x20, 0x20, 0x20, 0x20, 0xff, 0x20, 0x20, 0xff, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x0, 0x20, 0x20, 0x0, 0x0, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x5c, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20, 0x20})
	f.Fuzz(func(t *testing.T, input []byte) {
		buildinfo.Read(bytes.NewReader(input))
	})
}

// TestIssue54968 是 golang.org/issue/54968 的回归测试。
//
// issue 54968 的原因是当第一个 buildInfoMagic 无效时，
// 会进入无限循环。
func TestIssue54968(t *testing.T) {
	t.Parallel()

	const (
		paddingSize    = 200
		buildInfoAlign = 16
	)
	buildInfoMagic := []byte("\xff Go buildinf:")

	// 构造一个有效的 PE 头。
	var buf bytes.Buffer

	buf.Write([]byte{'M', 'Z'})
	buf.Write(bytes.Repeat([]byte{0}, 0x3c-2))
	// 在位置 0x3c，存根包含 PE 签名的文件偏移量。
	binary.Write(&buf, binary.LittleEndian, int32(0x3c+4))

	buf.Write([]byte{'P', 'E', 0, 0})

	binary.Write(&buf, binary.LittleEndian, pe.FileHeader{NumberOfSections: 1})

	sh := pe.SectionHeader32{
		Name:             [8]uint8{'t', 0},
		SizeOfRawData:    uint32(paddingSize + len(buildInfoMagic)),
		PointerToRawData: uint32(buf.Len()),
	}
	sh.PointerToRawData = uint32(buf.Len() + binary.Size(sh))

	binary.Write(&buf, binary.LittleEndian, sh)

	start := buf.Len()
	buf.Write(bytes.Repeat([]byte{0}, paddingSize+len(buildInfoMagic)))
	data := buf.Bytes()

	if _, err := pe.NewFile(bytes.NewReader(data)); err != nil {
		t.Fatalf("need a valid PE header for the misaligned buildInfoMagic test: %s", err)
	}

	// 在头部之后放置 buildInfoMagic。
	for i := 1; i < paddingSize-len(buildInfoMagic); i++ {
		// 只测试未对齐的 buildInfoMagic。
		if i%buildInfoAlign == 0 {
			continue
		}

		t.Run(fmt.Sprintf("start_at_%d", i), func(t *testing.T) {
			d := data[:start]
			// 故意构造未对齐的 buildInfoMagic。
			d = append(d, bytes.Repeat([]byte{0}, i)...)
			d = append(d, buildInfoMagic...)
			d = append(d, bytes.Repeat([]byte{0}, paddingSize-i)...)

			_, err := buildinfo.Read(bytes.NewReader(d))

			wantErr := "not a Go executable"
			if err == nil {
				t.Errorf("got error nil; want error containing %q", wantErr)
			} else if errMsg := err.Error(); !strings.Contains(errMsg, wantErr) {
				t.Errorf("got error %q; want error containing %q", errMsg, wantErr)
			}
		})
	}
}

func FuzzRead(f *testing.F) {
	go117, err := obscuretestdata.ReadFile("testdata/go117/go117.base64")
	if err != nil {
		f.Errorf("Error reading go117: %v", err)
	}
	f.Add(go117)

	notgo, err := obscuretestdata.ReadFile("testdata/notgo/notgo.base64")
	if err != nil {
		f.Errorf("Error reading notgo: %v", err)
	}
	f.Add(notgo)

	f.Fuzz(func(t *testing.T, in []byte) {
		buildinfo.Read(bytes.NewReader(in))
	})
}
