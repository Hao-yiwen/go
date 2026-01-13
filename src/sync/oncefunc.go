// 版权所有 2022 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package sync

// OnceFunc 返回一个只调用 f 一次的函数。返回的函数可以被并发调用。
//
// 如果 f 发生 panic，返回的函数在每次调用时都会以相同的值 panic。
func OnceFunc(f func()) func() {
	// 使用结构体以便只有一次堆分配。
	d := struct {
		f     func()
		once  Once
		valid bool
		p     any
	}{
		f: f,
	}
	return func() {
		d.once.Do(func() {
			defer func() {
				d.f = nil // 调用后不再保持 f 存活。
				d.p = recover()
				if !d.valid {
					// 立即重新 panic，这样在第一次调用时
					// 用户可以获得进入 f 的完整堆栈跟踪。
					panic(d.p)
				}
			}()
			d.f()
			d.valid = true // 仅在 f 没有 panic 时设置。
		})
		if !d.valid {
			panic(d.p)
		}
	}
}

// OnceValue 返回一个只调用 f 一次并返回 f 返回值的函数。
// 返回的函数可以被并发调用。
//
// 如果 f 发生 panic，返回的函数在每次调用时都会以相同的值 panic。
func OnceValue[T any](f func() T) func() T {
	// 使用结构体以便只有一次堆分配。
	d := struct {
		f      func() T
		once   Once
		valid  bool
		p      any
		result T
	}{
		f: f,
	}
	return func() T {
		d.once.Do(func() {
			defer func() {
				d.f = nil
				d.p = recover()
				if !d.valid {
					panic(d.p)
				}
			}()
			d.result = d.f()
			d.valid = true
		})
		if !d.valid {
			panic(d.p)
		}
		return d.result
	}
}

// OnceValues 返回一个只调用 f 一次并返回 f 返回的多个值的函数。
// 返回的函数可以被并发调用。
//
// 如果 f 发生 panic，返回的函数在每次调用时都会以相同的值 panic。
func OnceValues[T1, T2 any](f func() (T1, T2)) func() (T1, T2) {
	// 使用结构体以便只有一次堆分配。
	d := struct {
		f     func() (T1, T2)
		once  Once
		valid bool
		p     any
		r1    T1
		r2    T2
	}{
		f: f,
	}
	return func() (T1, T2) {
		d.once.Do(func() {
			defer func() {
				d.f = nil
				d.p = recover()
				if !d.valid {
					panic(d.p)
				}
			}()
			d.r1, d.r2 = d.f()
			d.valid = true
		})
		if !d.valid {
			panic(d.p)
		}
		return d.r1, d.r2
	}
}
