// 版权所有 2016 The Go 作者。保留所有权利。
// 本源代码的使用受 BSD 风格许可证管制，
// 该许可证可在 LICENSE 文件中找到。

//go:build (linux && cgo) || (darwin && cgo) || (freebsd && cgo)

package plugin

/*
#cgo linux LDFLAGS: -ldl
#include <dlfcn.h>
#include <limits.h>
#include <stdlib.h>
#include <stdint.h>

#include <stdio.h>

static uintptr_t pluginOpen(const char* path, char** err) {
	void* h = dlopen(path, RTLD_NOW|RTLD_GLOBAL);
	if (h == NULL) {
		*err = (char*)dlerror();
	}
	return (uintptr_t)h;
}

static void* pluginLookup(uintptr_t h, const char* name, char** err) {
	void* r = dlsym((void*)h, name);
	if (r == NULL) {
		*err = (char*)dlerror();
	}
	return r;
}
*/
import "C"

import (
	"errors"
	"sync"
	"unsafe"
)

func open(name string) (*Plugin, error) {
	cPath := make([]byte, C.PATH_MAX+1)
	cRelName := make([]byte, len(name)+1)
	copy(cRelName, name)
	if C.realpath(
		(*C.char)(unsafe.Pointer(&cRelName[0])),
		(*C.char)(unsafe.Pointer(&cPath[0]))) == nil {
		return nil, errors.New(`plugin.Open("` + name + `"): realpath failed`)
	}

	filepath := C.GoString((*C.char)(unsafe.Pointer(&cPath[0])))

	pluginsMu.Lock()
	if p := plugins[filepath]; p != nil {
		pluginsMu.Unlock()
		if p.err != "" {
			return nil, errors.New(`plugin.Open("` + name + `"): ` + p.err + ` (previous failure)`)
		}
		<-p.loaded
		return p, nil
	}
	var cErr *C.char
	h := C.pluginOpen((*C.char)(unsafe.Pointer(&cPath[0])), &cErr)
	if h == 0 {
		pluginsMu.Unlock()
		return nil, errors.New(`plugin.Open("` + name + `"): ` + C.GoString(cErr))
	}
	// TODO(crawshaw): 查找插件笔记，确认它是一个 Go 插件
	// 并且它是用正确的工具链构建的。
	if len(name) > 3 && name[len(name)-3:] == ".so" {
		name = name[:len(name)-3]
	}
	if plugins == nil {
		plugins = make(map[string]*Plugin)
	}
	pluginpath, syms, initTasks, errstr := lastmoduleinit()
	if errstr != "" {
		plugins[filepath] = &Plugin{
			pluginpath: pluginpath,
			err:        errstr,
		}
		pluginsMu.Unlock()
		return nil, errors.New(`plugin.Open("` + name + `"): ` + errstr)
	}
	// 这个函数可以从插件的 init 函数调用。
	// 在映射中放置一个占位符，以便后续的打开操作可以等待它。
	p := &Plugin{
		pluginpath: pluginpath,
		loaded:     make(chan struct{}),
	}
	plugins[filepath] = p
	pluginsMu.Unlock()

	doInit(initTasks)

	// 填充每个插件符号的值。
	updatedSyms := map[string]any{}
	for symName, sym := range syms {
		isFunc := symName[0] == '.'
		if isFunc {
			delete(syms, symName)
			symName = symName[1:]
		}

		fullName := pluginpath + "." + symName
		cname := make([]byte, len(fullName)+1)
		copy(cname, fullName)

		p := C.pluginLookup(h, (*C.char)(unsafe.Pointer(&cname[0])), &cErr)
		if p == nil {
			return nil, errors.New(`plugin.Open("` + name + `"): could not find symbol ` + symName + `: ` + C.GoString(cErr))
		}
		valp := (*[2]unsafe.Pointer)(unsafe.Pointer(&sym))
		if isFunc {
			(*valp)[1] = unsafe.Pointer(&p)
		} else {
			(*valp)[1] = p
		}
		// 我们不能在迭代期间添加到 syms，因为我们会处理
		// 某些符号两次，无法判断符号是否是函数
		updatedSyms[symName] = sym
	}
	p.syms = updatedSyms

	close(p.loaded)
	return p, nil
}

func lookup(p *Plugin, symName string) (Symbol, error) {
	if s := p.syms[symName]; s != nil {
		return s, nil
	}
	return nil, errors.New("plugin: symbol " + symName + " not found in plugin " + p.pluginpath)
}

var (
	pluginsMu sync.Mutex
	plugins   map[string]*Plugin
)

// lastmoduleinit 在 runtime 包中定义。
func lastmoduleinit() (pluginpath string, syms map[string]any, inittasks []*initTask, errstr string)

// doInit 在 runtime 包中定义。
//
//go:linkname doInit runtime.doInit
func doInit(t []*initTask)

type initTask struct {
	// 在 runtime.initTask 中定义的字段。我们在
	// 这个包中只处理指向 initTask 的指针，
	// 所以内容无关。
}
