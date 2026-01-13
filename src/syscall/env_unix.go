// 版权所有 2010 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

//go:build unix || (js && wasm) || plan9 || wasip1

// Unix 环境变量。

package syscall

import (
	"runtime"
	"sync"
)

var (
	// envLock 保护 env 和 envs。
	envLock sync.RWMutex

	// env 将环境变量映射到其在 envs 中的首次出现位置。
	env map[string]int

	// envs 由运行时提供。元素应为 "key=value" 形式。
	// 空字符串表示已删除（或要忽略的重复项）。
	envs []string = runtime_envs()
)

func runtime_envs() []string // 在 runtime 包中

var copyenv = sync.OnceFunc(func() {
	env = make(map[string]int)
	for i, s := range envs {
		for j := 0; j < len(s); j++ {
			if s[j] == '=' {
				key := s[:j]
				if _, ok := env[key]; !ok {
					env[key] = i // 首次出现的键
				} else {
					// 清除重复的键。这允许 Unsetenv 安全地只删除
					// 第一个项目，而不必担心取消隐藏后面的项目，
					// 这可能会带来安全问题。
					envs[i] = ""
				}
				break
			}
		}
	}
})

func Unsetenv(key string) error {
	copyenv()

	envLock.Lock()
	defer envLock.Unlock()

	if i, ok := env[key]; ok {
		envs[i] = ""
		delete(env, key)
	}
	runtimeUnsetenv(key)
	return nil
}

func Getenv(key string) (value string, found bool) {
	copyenv()
	if len(key) == 0 {
		return "", false
	}

	envLock.RLock()
	defer envLock.RUnlock()

	i, ok := env[key]
	if !ok {
		return "", false
	}
	s := envs[i]
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[i+1:], true
		}
	}
	return "", false
}

func Setenv(key, value string) error {
	copyenv()
	if len(key) == 0 {
		return EINVAL
	}
	for i := 0; i < len(key); i++ {
		if key[i] == '=' || key[i] == 0 {
			return EINVAL
		}
	}
	// 在 Plan 9 上，null 被用作分隔符，例如在 $path 中。
	if runtime.GOOS != "plan9" {
		for i := 0; i < len(value); i++ {
			if value[i] == 0 {
				return EINVAL
			}
		}
	}

	envLock.Lock()
	defer envLock.Unlock()

	i, ok := env[key]
	kv := key + "=" + value
	if ok {
		envs[i] = kv
	} else {
		i = len(envs)
		envs = append(envs, kv)
	}
	env[key] = i
	runtimeSetenv(key, value)
	return nil
}

func Clearenv() {
	copyenv()

	envLock.Lock()
	defer envLock.Unlock()

	runtimeClearenv(env)

	env = make(map[string]int)
	envs = []string{}
}

func Environ() []string {
	copyenv()
	envLock.RLock()
	defer envLock.RUnlock()
	a := make([]string, 0, len(envs))
	for _, env := range envs {
		if env != "" {
			a = append(a, env)
		}
	}
	return a
}
