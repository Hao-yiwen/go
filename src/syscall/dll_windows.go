// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package syscall

import (
	"internal/syscall/windows/sysdll"
	"sync"
	"sync/atomic"
	"unsafe"
)

// 使用双下划线以避免与自动生成函数的名称冲突。
//go:cgo_import_dynamic syscall.__LoadLibraryExW LoadLibraryExW%3 "kernel32.dll"
//go:cgo_import_dynamic syscall.__GetProcAddress GetProcAddress%2 "kernel32.dll"

var (
	__LoadLibraryExW unsafe.Pointer
	__GetProcAddress unsafe.Pointer
)

// DLLError 描述 DLL 加载失败的原因。
type DLLError struct {
	Err     error
	ObjName string
	Msg     string
}

func (e *DLLError) Error() string { return e.Msg }

func (e *DLLError) Unwrap() error { return e.Err }

// 注意：对于下面的 Syscall 函数：
//
// //go:uintptrkeepalive 因为 uintptr 参数可能是需要在调用者中保持存活的转换指针。
//
// //go:nosplit 因为栈复制不考虑 uintptrkeepalive，所以栈不能增长。
// 栈复制不能盲目假设所有 uintptr 参数都是指针，因为某些值可能看起来像指针，
// 但实际上不是指针，调整它们的值会破坏调用。

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall(trap, nargs, a1, a2, a3 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3)
}

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall6(trap, nargs, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3, a4, a5, a6)
}

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall9(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9)
}

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall12(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12)
}

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall15(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15)
}

// 已弃用：请使用 [SyscallN] 代替。
//
//go:nosplit
//go:uintptrkeepalive
func Syscall18(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15, a16, a17, a18 uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(trap, nargs, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15, a16, a17, a18)
}

// SyscallN 使用参数 args 执行过程 p。
//
// 更多信息请参阅 [Proc.Call]。
//
//go:nosplit
//go:uintptrkeepalive
func SyscallN(p uintptr, args ...uintptr) (r1, r2 uintptr, err Errno) {
	return syscalln(p, uintptr(len(args)), args...)
}

// syscalln 在 runtime/syscall_windows.go 中实现。
//
//go:noescape
func syscalln(fn, n uintptr, args ...uintptr) (r1, r2 uintptr, err Errno)

// 注意：对于下面的 loadlibrary、loadlibrary 和 getprocaddress 函数：
//
// //go:linkname 作为链接器 -checklinkname 的允许列表，
// 因为 golang.org/x/sys/windows 使用 linkname 链接这些函数。

//go:linkname loadlibrary
func loadlibrary(filename *uint16) (uintptr, Errno) {
	handle, _, err := SyscallN(uintptr(__LoadLibraryExW), uintptr(unsafe.Pointer(filename)), 0, 0)
	if handle != 0 {
		err = 0
	}
	return handle, err
}

//go:linkname loadsystemlibrary
func loadsystemlibrary(filename *uint16) (uintptr, Errno) {
	const _LOAD_LIBRARY_SEARCH_SYSTEM32 = 0x00000800
	handle, _, err := SyscallN(uintptr(__LoadLibraryExW), uintptr(unsafe.Pointer(filename)), 0, _LOAD_LIBRARY_SEARCH_SYSTEM32)
	if handle != 0 {
		err = 0
	}
	return handle, err
}

//go:linkname getprocaddress
func getprocaddress(handle uintptr, procname *uint8) (uintptr, Errno) {
	proc, _, err := SyscallN(uintptr(__GetProcAddress), handle, uintptr(unsafe.Pointer(procname)))
	if proc != 0 {
		err = 0
	}
	return proc, err
}

// DLL 实现对单个 DLL 的访问。
type DLL struct {
	Name   string
	Handle Handle
}

// LoadDLL 将指定名称的 DLL 文件加载到内存中。
//
// 如果 name 不是绝对路径，也不是 Go 使用的已知系统 DLL，
// Windows 将在多个位置搜索该 DLL，可能导致 DLL 预加载攻击。
//
// 使用 golang.org/x/sys/windows 中的 [LazyDLL] 来安全地加载系统 DLL。
func LoadDLL(name string) (*DLL, error) {
	namep, err := UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	var h uintptr
	var e Errno
	if sysdll.IsSystemDLL[name] {
		h, e = loadsystemlibrary(namep)
	} else {
		h, e = loadlibrary(namep)
	}
	if e != 0 {
		return nil, &DLLError{
			Err:     e,
			ObjName: name,
			Msg:     "Failed to load " + name + ": " + e.Error(),
		}
	}
	d := &DLL{
		Name:   name,
		Handle: Handle(h),
	}
	return d, nil
}

// MustLoadDLL 类似于 [LoadDLL]，但如果加载操作失败则 panic。
func MustLoadDLL(name string) *DLL {
	d, e := LoadDLL(name)
	if e != nil {
		panic(e)
	}
	return d
}

// FindProc 在 [DLL] d 中搜索名为 name 的过程，如果找到则返回 [*Proc]。
// 如果搜索失败则返回错误。
func (d *DLL) FindProc(name string) (proc *Proc, err error) {
	namep, err := BytePtrFromString(name)
	if err != nil {
		return nil, err
	}
	a, e := getprocaddress(uintptr(d.Handle), namep)
	if e != 0 {
		return nil, &DLLError{
			Err:     e,
			ObjName: name,
			Msg:     "Failed to find " + name + " procedure in " + d.Name + ": " + e.Error(),
		}
	}
	p := &Proc{
		Dll:  d,
		Name: name,
		addr: a,
	}
	return p, nil
}

// MustFindProc 类似于 [DLL.FindProc]，但如果搜索失败则 panic。
func (d *DLL) MustFindProc(name string) *Proc {
	p, e := d.FindProc(name)
	if e != nil {
		panic(e)
	}
	return p
}

// Release 从内存中卸载 [DLL] d。
func (d *DLL) Release() (err error) {
	return FreeLibrary(d.Handle)
}

// Proc 实现对 [DLL] 内部过程的访问。
type Proc struct {
	Dll  *DLL
	Name string
	addr uintptr
}

// Addr 返回 p 表示的过程的地址。
// 返回值可以传递给 Syscall 来运行该过程。
func (p *Proc) Addr() uintptr {
	return p.addr
}

// Call 使用参数 a 执行过程 p。
//
// 返回的错误始终非空，由 GetLastError 的结果构造。
// 调用者必须先检查主返回值以决定是否发生错误
// （根据被调用的特定函数的语义），然后再查询错误。
// 错误始终具有 [Errno] 类型。
//
// 在 amd64 上，Call 可以传递和返回浮点值。要传递 C 类型 "float" 的参数 x，
// 使用 uintptr(math.Float32bits(x))。要传递 C 类型 "double" 的参数，
// 使用 uintptr(math.Float64bits(x))。浮点返回值在 r2 中返回。
// C 类型 "float" 的返回值是 [math.Float32frombits](uint32(r2))。
// 对于 C 类型 "double"，是 [math.Float64frombits](uint64(r2))。
//
//go:uintptrescapes
func (p *Proc) Call(a ...uintptr) (uintptr, uintptr, error) {
	return SyscallN(p.Addr(), a...)
}

// LazyDLL 实现对单个 [DLL] 的访问。
// 它将延迟加载 DLL，直到第一次调用其 [LazyDLL.Handle] 方法
// 或其 [LazyProc] 的 Addr 方法。
//
// LazyDLL 与 [LoadDLL] 文档中记载的一样，
// 容易受到 DLL 预加载攻击。
//
// 使用 golang.org/x/sys/windows 中的 LazyDLL 来安全地加载系统 DLL。
type LazyDLL struct {
	mu   sync.Mutex
	dll  *DLL // DLL 加载后非空
	Name string
}

// Load 将 DLL 文件 d.Name 加载到内存中。如果失败则返回错误。
// 如果 DLL 已经加载到内存中，Load 不会尝试加载。
func (d *LazyDLL) Load() error {
	// 无竞态版本：
	// if d.dll == nil {
	if atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&d.dll))) == nil {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.dll == nil {
			dll, e := LoadDLL(d.Name)
			if e != nil {
				return e
			}
			// 无竞态版本：
			// d.dll = dll
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&d.dll)), unsafe.Pointer(dll))
		}
	}
	return nil
}

// mustLoad 类似于 Load，但如果搜索失败则 panic。
func (d *LazyDLL) mustLoad() {
	e := d.Load()
	if e != nil {
		panic(e)
	}
}

// Handle 返回 d 的模块句柄。
func (d *LazyDLL) Handle() uintptr {
	d.mustLoad()
	return uintptr(d.dll.Handle)
}

// NewProc 返回用于访问 [DLL] d 中指定名称过程的 [LazyProc]。
func (d *LazyDLL) NewProc(name string) *LazyProc {
	return &LazyProc{l: d, Name: name}
}

// NewLazyDLL 创建与 [DLL] 文件关联的新 [LazyDLL]。
func NewLazyDLL(name string) *LazyDLL {
	return &LazyDLL{Name: name}
}

// LazyProc 实现对 [LazyDLL] 内部过程的访问。
// 它延迟查找直到调用 [LazyProc.Addr]、[LazyProc.Call] 或 [LazyProc.Find] 方法。
type LazyProc struct {
	mu   sync.Mutex
	Name string
	l    *LazyDLL
	proc *Proc
}

// Find 在 [DLL] 中搜索名为 p.Name 的过程。如果搜索失败则返回错误。
// 如果过程已经找到并加载到内存中，Find 不会搜索。
func (p *LazyProc) Find() error {
	// 无竞态版本：
	// if p.proc == nil {
	if atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&p.proc))) == nil {
		p.mu.Lock()
		defer p.mu.Unlock()
		if p.proc == nil {
			e := p.l.Load()
			if e != nil {
				return e
			}
			proc, e := p.l.dll.FindProc(p.Name)
			if e != nil {
				return e
			}
			// 无竞态版本：
			// p.proc = proc
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&p.proc)), unsafe.Pointer(proc))
		}
	}
	return nil
}

// mustFind 类似于 Find，但如果搜索失败则 panic。
func (p *LazyProc) mustFind() {
	e := p.Find()
	if e != nil {
		panic(e)
	}
}

// Addr 返回 p 表示的过程的地址。
// 返回值可以传递给 Syscall 来运行该过程。
func (p *LazyProc) Addr() uintptr {
	p.mustFind()
	return p.proc.Addr()
}

// Call 使用参数 a 执行过程 p。更多信息请参阅 Proc.Call 的文档。
//
//go:uintptrescapes
func (p *LazyProc) Call(a ...uintptr) (r1, r2 uintptr, lastErr error) {
	p.mustFind()
	return p.proc.Call(a...)
}
