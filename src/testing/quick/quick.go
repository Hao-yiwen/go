// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package quick 实现了帮助进行黑盒测试的实用函数。
//
// testing/quick 包已冻结，不接受新功能。
package quick

import (
	"flag"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"strings"
	"time"
)

var defaultMaxCount *int = flag.Int("quickchecks", 100, "每次检查的默认迭代次数")

// Generator 可以生成其自身类型的随机值。
type Generator interface {
	// Generate 使用 size 作为大小提示，返回该方法所在类型的随机实例。
	Generate(rand *rand.Rand, size int) reflect.Value
}

// randFloat32 生成一个取 float32 完整范围的随机浮点数。
func randFloat32(rand *rand.Rand) float32 {
	f := rand.Float64() * math.MaxFloat32
	if rand.Int()&1 == 1 {
		f = -f
	}
	return float32(f)
}

// randFloat64 生成一个取 float64 完整范围的随机浮点数。
func randFloat64(rand *rand.Rand) float64 {
	f := rand.Float64() * math.MaxFloat64
	if rand.Int()&1 == 1 {
		f = -f
	}
	return f
}

// randInt64 返回一个随机 int64。
func randInt64(rand *rand.Rand) int64 {
	return int64(rand.Uint64())
}

// complexSize 是包含其他值的任意值的最大长度。
const complexSize = 50

// Value 返回给定类型的任意值。
// 如果该类型实现了 [Generator] 接口，将使用该接口。
// 注意：要为结构体创建任意值，所有字段必须被导出。
func Value(t reflect.Type, rand *rand.Rand) (value reflect.Value, ok bool) {
	return sizedValue(t, rand, complexSize)
}

// sizedValue 返回给定类型的任意值。size 提示
// 用于收缩作为间接级别的函数，以便
// 递归数据结构会终止。
func sizedValue(t reflect.Type, rand *rand.Rand, size int) (value reflect.Value, ok bool) {
	if m, ok := reflect.TypeAssert[Generator](reflect.Zero(t)); ok {
		return m.Generate(rand, size), true
	}

	v := reflect.New(t).Elem()
	switch concrete := t; concrete.Kind() {
	case reflect.Bool:
		v.SetBool(rand.Int()&1 == 0)
	case reflect.Float32:
		v.SetFloat(float64(randFloat32(rand)))
	case reflect.Float64:
		v.SetFloat(randFloat64(rand))
	case reflect.Complex64:
		v.SetComplex(complex(float64(randFloat32(rand)), float64(randFloat32(rand))))
	case reflect.Complex128:
		v.SetComplex(complex(randFloat64(rand), randFloat64(rand)))
	case reflect.Int16:
		v.SetInt(randInt64(rand))
	case reflect.Int32:
		v.SetInt(randInt64(rand))
	case reflect.Int64:
		v.SetInt(randInt64(rand))
	case reflect.Int8:
		v.SetInt(randInt64(rand))
	case reflect.Int:
		v.SetInt(randInt64(rand))
	case reflect.Uint16:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Uint32:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Uint64:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Uint8:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Uint:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Uintptr:
		v.SetUint(uint64(randInt64(rand)))
	case reflect.Map:
		numElems := rand.Intn(size)
		v.Set(reflect.MakeMap(concrete))
		for i := 0; i < numElems; i++ {
			key, ok1 := sizedValue(concrete.Key(), rand, size)
			value, ok2 := sizedValue(concrete.Elem(), rand, size)
			if !ok1 || !ok2 {
				return reflect.Value{}, false
			}
			v.SetMapIndex(key, value)
		}
	case reflect.Pointer:
		if rand.Intn(size) == 0 {
			v.SetZero() // Generate nil pointer.
		} else {
			elem, ok := sizedValue(concrete.Elem(), rand, size)
			if !ok {
				return reflect.Value{}, false
			}
			v.Set(reflect.New(concrete.Elem()))
			v.Elem().Set(elem)
		}
	case reflect.Slice:
		numElems := rand.Intn(size)
		sizeLeft := size - numElems
		v.Set(reflect.MakeSlice(concrete, numElems, numElems))
		for i := 0; i < numElems; i++ {
			elem, ok := sizedValue(concrete.Elem(), rand, sizeLeft)
			if !ok {
				return reflect.Value{}, false
			}
			v.Index(i).Set(elem)
		}
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			elem, ok := sizedValue(concrete.Elem(), rand, size)
			if !ok {
				return reflect.Value{}, false
			}
			v.Index(i).Set(elem)
		}
	case reflect.String:
		numChars := rand.Intn(complexSize)
		codePoints := make([]rune, numChars)
		for i := 0; i < numChars; i++ {
			codePoints[i] = rune(rand.Intn(0x10ffff))
		}
		v.SetString(string(codePoints))
	case reflect.Struct:
		n := v.NumField()
		// 将 sizeLeft 均匀分配到结构体字段中。
		sizeLeft := size
		if n > sizeLeft {
			sizeLeft = 1
		} else if n > 0 {
			sizeLeft /= n
		}
		for i := 0; i < n; i++ {
			elem, ok := sizedValue(concrete.Field(i).Type, rand, sizeLeft)
			if !ok {
				return reflect.Value{}, false
			}
			v.Field(i).Set(elem)
		}
	default:
		return reflect.Value{}, false
	}

	return v, true
}

// Config 结构包含运行测试的选项。
type Config struct {
	// MaxCount 设置最大迭代次数。
	// 如果为零，则使用 MaxCountScale。
	MaxCount int
	// MaxCountScale 是应用于
	// 默认最大值的非负缩放因子。
	// 计数为零意味着默认值，通常为 100
	// 但可以通过 -quickchecks 标志设置。
	MaxCountScale float64
	// Rand 指定随机数来源。
	// 如果为 nil，将使用默认伪随机源。
	Rand *rand.Rand
	// Values 指定一个函数来生成
	// 与被测试函数的参数相同的任意 reflect.Values 切片。
	// 如果为 nil，则使用顶级 Value 函数生成它们。
	Values func([]reflect.Value, *rand.Rand)
}

var defaultConfig Config

// getRand 返回用于给定 Config 的 *rand.Rand。
func (c *Config) getRand() *rand.Rand {
	if c.Rand == nil {
		return rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return c.Rand
}

// getMaxCount 返回给定 Config 要运行的最大迭代次数。
func (c *Config) getMaxCount() (maxCount int) {
	maxCount = c.MaxCount
	if maxCount == 0 {
		if c.MaxCountScale != 0 {
			maxCount = int(c.MaxCountScale * float64(*defaultMaxCount))
		} else {
			maxCount = *defaultMaxCount
		}
	}

	return
}

// SetupError 是检查的使用方式中出现错误的结果，与
// 被测试的函数无关。
type SetupError string

func (s SetupError) Error() string { return string(s) }

// CheckError 是 Check 找到错误的结果。
type CheckError struct {
	Count int
	In    []any
}

func (s *CheckError) Error() string {
	return fmt.Sprintf("#%d: failed on input %s", s.Count, toString(s.In))
}

// CheckEqualError 是 [CheckEqual] 找到错误的结果。
type CheckEqualError struct {
	CheckError
	Out1 []any
	Out2 []any
}

func (s *CheckEqualError) Error() string {
	return fmt.Sprintf("#%d: failed on input %s. Output 1: %s. Output 2: %s", s.Count, toString(s.In), toString(s.Out1), toString(s.Out2))
}

// Check 查找函数 f（返回 bool 的任何函数）的输入，
// 使得 f 返回 false。它对 f 重复调用，为每个参数使用任意值。
// 如果 f 在给定输入上返回 false，
// Check 会将该输入作为 *[CheckError] 返回。
// 例如：
//
//	func TestOddMultipleOfThree(t *testing.T) {
//		f := func(x int) bool {
//			y := OddMultipleOfThree(x)
//			return y%2 == 1 && y%3 == 0
//		}
//		if err := quick.Check(f, nil); err != nil {
//			t.Error(err)
//		}
//	}
func Check(f any, config *Config) error {
	if config == nil {
		config = &defaultConfig
	}

	fVal, fType, ok := functionAndType(f)
	if !ok {
		return SetupError("argument is not a function")
	}

	if fType.NumOut() != 1 {
		return SetupError("function does not return one value")
	}
	if fType.Out(0).Kind() != reflect.Bool {
		return SetupError("function does not return a bool")
	}

	arguments := make([]reflect.Value, fType.NumIn())
	rand := config.getRand()
	maxCount := config.getMaxCount()

	for i := 0; i < maxCount; i++ {
		err := arbitraryValues(arguments, fType, config, rand)
		if err != nil {
			return err
		}

		if !fVal.Call(arguments)[0].Bool() {
			return &CheckError{i + 1, toInterfaces(arguments)}
		}
	}

	return nil
}

// CheckEqual 查找 f 和 g 返回不同结果的输入。
// 它对 f 和 g 重复调用，为每个参数使用任意值。
// 如果 f 和 g 返回不同的答案，CheckEqual 返回 *[CheckEqualError]
// 来描述输入和输出。
func CheckEqual(f, g any, config *Config) error {
	if config == nil {
		config = &defaultConfig
	}

	x, xType, ok := functionAndType(f)
	if !ok {
		return SetupError("f is not a function")
	}
	y, yType, ok := functionAndType(g)
	if !ok {
		return SetupError("g is not a function")
	}

	if xType != yType {
		return SetupError("functions have different types")
	}

	arguments := make([]reflect.Value, xType.NumIn())
	rand := config.getRand()
	maxCount := config.getMaxCount()

	for i := 0; i < maxCount; i++ {
		err := arbitraryValues(arguments, xType, config, rand)
		if err != nil {
			return err
		}

		xOut := toInterfaces(x.Call(arguments))
		yOut := toInterfaces(y.Call(arguments))

		if !reflect.DeepEqual(xOut, yOut) {
			return &CheckEqualError{CheckError{i + 1, toInterfaces(arguments)}, xOut, yOut}
		}
	}

	return nil
}

// arbitraryValues 写入 Values 到 args，使得 args 包含适合
// 调用 f 的 Values。
func arbitraryValues(args []reflect.Value, f reflect.Type, config *Config, rand *rand.Rand) (err error) {
	if config.Values != nil {
		config.Values(args, rand)
		return
	}

	for j := 0; j < len(args); j++ {
		var ok bool
		args[j], ok = Value(f.In(j), rand)
		if !ok {
			err = SetupError(fmt.Sprintf("cannot create arbitrary value of type %s for argument %d", f.In(j), j))
			return
		}
	}

	return
}

func functionAndType(f any) (v reflect.Value, t reflect.Type, ok bool) {
	v = reflect.ValueOf(f)
	ok = v.Kind() == reflect.Func
	if !ok {
		return
	}
	t = v.Type()
	return
}

func toInterfaces(values []reflect.Value) []any {
	ret := make([]any, len(values))
	for i, v := range values {
		ret[i] = v.Interface()
	}
	return ret
}

func toString(interfaces []any) string {
	s := make([]string, len(interfaces))
	for i, v := range interfaces {
		s[i] = fmt.Sprintf("%#v", v)
	}
	return strings.Join(s, ", ")
}
