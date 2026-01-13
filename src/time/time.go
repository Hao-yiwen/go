// 版权所有 2009 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// Package time 提供测量和显示时间的功能。
//
// 日历计算始终假定使用格里高利历（公历），
// 不考虑闰秒。
//
// # 单调时钟
//
// 操作系统同时提供"墙上时钟"和"单调时钟"。墙上时钟会因时钟同步而改变，
// 而单调时钟不会。一般规则是：墙上时钟用于显示时间，
// 单调时钟用于测量时间。本包没有拆分 API，
// 而是让 [time.Now] 返回的 Time 同时包含墙上时钟读数和单调时钟读数；
// 后续的时间显示操作使用墙上时钟读数，而后续的时间测量操作
// （特别是比较和减法）使用单调时钟读数。
//
// 例如，下面的代码总是计算出大约 20 毫秒的正向经过时间，
// 即使在计时操作期间墙上时钟被修改：
//
//	start := time.Now()
//	... 耗时 20 毫秒的操作 ...
//	t := time.Now()
//	elapsed := t.Sub(start)
//
// 其他惯用法，如 [time.Since](start)、[time.Until](deadline) 和
// time.Now().Before(deadline)，同样能够抵抗墙上时钟的重置。
//
// 本节的其余部分详细说明了操作如何使用单调时钟，
// 但使用本包并不需要理解这些细节。
//
// time.Now 返回的 Time 包含单调时钟读数。
// 如果 Time t 有单调时钟读数，t.Add 会将相同的时长同时添加到
// 墙上时钟读数和单调时钟读数来计算结果。
// 因为 t.AddDate(y, m, d)、t.Round(d) 和 t.Truncate(d) 是墙上时间计算，
// 它们总是从结果中剥离单调时钟读数。
// 因为 t.In、t.Local 和 t.UTC 用于影响墙上时间的解释，
// 它们也会从结果中剥离单调时钟读数。
// 剥离单调时钟读数的标准方法是使用 t = t.Round(0)。
//
// 如果 Time t 和 u 都包含单调时钟读数，则 t.After(u)、t.Before(u)、
// t.Equal(u)、t.Compare(u) 和 t.Sub(u) 操作仅使用单调时钟读数进行，
// 忽略墙上时钟读数。如果 t 或 u 中任一个不包含单调时钟读数，
// 这些操作将回退到使用墙上时钟读数。
//
// 在某些系统上，当计算机进入睡眠状态时，单调时钟会停止。
// 在这样的系统上，t.Sub(u) 可能无法准确反映 t 和 u 之间实际经过的时间。
// 这同样适用于其他进行时间减法的函数和方法，如 [Since]、[Until]、
// [Time.Before]、[Time.After]、[Time.Add]、[Time.Equal] 和 [Time.Compare]。
// 在某些情况下，你可能需要剥离单调时钟以获得准确的结果。
//
// 因为单调时钟读数在当前进程之外没有意义，
// t.GobEncode、t.MarshalBinary、t.MarshalJSON 和 t.MarshalText
// 生成的序列化形式会省略单调时钟读数，t.Format 也不提供其格式。
// 类似地，构造函数 [time.Date]、[time.Parse]、[time.ParseInLocation] 和 [time.Unix]，
// 以及反序列化器 t.GobDecode、t.UnmarshalBinary、
// t.UnmarshalJSON 和 t.UnmarshalText 创建的时间都不包含单调时钟读数。
//
// 单调时钟读数仅存在于 [Time] 值中。它不是 [Duration] 值
// 或 t.Unix 及相关方法返回的 Unix 时间的一部分。
//
// 注意，Go 的 == 运算符不仅比较时间瞬间，还比较 [Location] 和单调时钟读数。
// 有关 Time 值相等性测试的讨论，请参阅 Time 类型的文档。
//
// 出于调试目的，t.String 的结果会包含单调时钟读数（如果存在）。
// 如果 t != u 是由于不同的单调时钟读数导致的，
// 该差异将在打印 t.String() 和 u.String() 时可见。
//
// # 定时器精度
//
// [Timer] 的精度取决于 Go 运行时、操作系统和底层硬件。
// 在 Unix 上，精度约为 1 毫秒。
// 在 Windows 1803 及更新版本上，精度约为 0.5 毫秒。
// 在较旧的 Windows 版本上，默认精度约为 16 毫秒，
// 但可以使用 [golang.org/x/sys/windows.TimeBeginPeriod] 请求更高的精度。
package time

import (
	"errors"
	"math/bits"
	_ "unsafe" // 用于 go:linkname
)

// Time 表示具有纳秒精度的时间瞬间。
//
// 使用时间的程序通常应该将其作为值而非指针来存储和传递。
// 也就是说，时间变量和结构体字段应该是 [time.Time] 类型，而不是 *time.Time。
//
// Time 值可以被多个 goroutine 同时使用，但 [Time.GobDecode]、
// [Time.UnmarshalBinary]、[Time.UnmarshalJSON] 和 [Time.UnmarshalText]
// 方法不是并发安全的。
//
// 时间瞬间可以使用 [Time.Before]、[Time.After] 和 [Time.Equal] 方法进行比较。
// [Time.Sub] 方法计算两个时间瞬间的差值，返回 [Duration]。
// [Time.Add] 方法将 Time 和 Duration 相加，返回 Time。
//
// Time 类型的零值是公元 1 年 1 月 1 日 00:00:00.000000000 UTC。
// 由于这个时间在实践中不太可能出现，[Time.IsZero] 方法提供了
// 一种简单的方式来检测未显式初始化的时间。
//
// 每个时间都有一个关联的 [Location]。[Time.Local]、[Time.UTC] 和 Time.In 方法
// 返回具有特定 Location 的 Time。使用这些方法更改 Time 值的 Location
// 不会改变它表示的实际时间瞬间，只会改变解释它的时区。
//
// 由 [Time.GobEncode]、[Time.MarshalBinary]、[Time.AppendBinary]、
// [Time.MarshalJSON]、[Time.MarshalText] 和 [Time.AppendText] 方法保存的
// Time 值表示形式存储 [Time.Location] 的偏移量，但不存储位置名称。
// 因此它们会丢失夏令时信息。
//
// 除了必需的"墙上时钟"读数外，Time 可能还包含当前进程单调时钟的可选读数，
// 以便为比较或减法提供额外的精度。
// 详情请参阅包文档中的"单调时钟"部分。
//
// 注意，Go 的 == 运算符不仅比较时间瞬间，还比较 Location 和单调时钟读数。
// 因此，在将 Time 值用作 map 或数据库键之前，应该首先确保
// 所有值都设置了相同的 Location（可以通过使用 UTC 或 Local 方法实现），
// 并且已通过设置 t = t.Round(0) 剥离了单调时钟读数。
// 通常情况下，优先使用 t.Equal(u) 而不是 t == u，因为 t.Equal 使用
// 最准确的可用比较方式，并能正确处理只有一个参数具有单调时钟读数的情况。
type Time struct {
	// wall 和 ext 编码墙上时间的秒数、墙上时间的纳秒数，
	// 以及可选的单调时钟读数（以纳秒为单位）。
	//
	// 从高位到低位，wall 编码了一个 1 位标志（hasMonotonic）、
	// 一个 33 位秒数字段和一个 30 位墙上时间纳秒字段。
	// 纳秒字段的范围是 [0, 999999999]。
	// 如果 hasMonotonic 位为 0，则 33 位字段必须为零，
	// 完整的有符号 64 位墙上秒数（自公元 1 年 1 月 1 日起）存储在 ext 中。
	// 如果 hasMonotonic 位为 1，则 33 位字段保存自 1885 年 1 月 1 日起的
	// 33 位无符号墙上秒数，ext 保存有符号 64 位单调时钟读数（自进程启动以来的纳秒数）。
	wall uint64
	ext  int64

	// loc 指定应该用于确定与此 Time 对应的
	// 分钟、小时、月、日和年的 Location。
	// nil 表示 UTC。
	// 所有 UTC 时间都用 loc==nil 表示，永远不会是 loc==&utcLoc。
	loc *Location
}

const (
	hasMonotonic = 1 << 63
	maxWall      = wallToInternal + (1<<33 - 1) // 2157 年
	minWall      = wallToInternal               // 1885 年
	nsecMask     = 1<<30 - 1
	nsecShift    = 30
)

// 这些用于操作墙上时钟和单调时钟读数的辅助函数
// 使用指针接收器，即使它们不修改时间，
// 这样调用成本更低。

// nsec 返回时间的纳秒部分。
func (t *Time) nsec() int32 {
	return int32(t.wall & nsecMask)
}

// sec 返回自公元 1 年 1 月 1 日以来的秒数。
func (t *Time) sec() int64 {
	if t.wall&hasMonotonic != 0 {
		return wallToInternal + int64(t.wall<<1>>(nsecShift+1))
	}
	return t.ext
}

// unixSec 返回自 1970 年 1 月 1 日以来的秒数（Unix 时间）。
func (t *Time) unixSec() int64 { return t.sec() + internalToUnix }

// addSec 将 d 秒添加到时间。
func (t *Time) addSec(d int64) {
	if t.wall&hasMonotonic != 0 {
		sec := int64(t.wall << 1 >> (nsecShift + 1))
		dsec := sec + d
		if 0 <= dsec && dsec <= 1<<33-1 {
			t.wall = t.wall&nsecMask | uint64(dsec)<<nsecShift | hasMonotonic
			return
		}
		// 墙上秒数现在超出了压缩字段的范围。
		// 移动到 ext。
		t.stripMono()
	}

	// 检查 t.ext 和 d 的和是否溢出，并正确处理。
	sum := t.ext + d
	if (sum > t.ext) == (d > 0) {
		t.ext = sum
	} else if d > 0 {
		t.ext = 1<<63 - 1
	} else {
		t.ext = -(1<<63 - 1)
	}
}

// setLoc 设置与时间关联的位置。
func (t *Time) setLoc(loc *Location) {
	if loc == &utcLoc {
		loc = nil
	}
	t.stripMono()
	t.loc = loc
}

// stripMono 剥离 t 中的单调时钟读数。
func (t *Time) stripMono() {
	if t.wall&hasMonotonic != 0 {
		t.ext = t.sec()
		t.wall &= nsecMask
	}
}

// setMono 设置 t 中的单调时钟读数。
// 如果 t 无法保存单调时钟读数（因为其墙上时间太大），
// setMono 不执行任何操作。
func (t *Time) setMono(m int64) {
	if t.wall&hasMonotonic == 0 {
		sec := t.ext
		if sec < minWall || maxWall < sec {
			return
		}
		t.wall |= hasMonotonic | uint64(sec-minWall)<<nsecShift
	}
	t.ext = m
}

// mono 返回 t 的单调时钟读数。
// 如果没有读数则返回 0。
// 此函数仅用于测试，
// 所以技术上 0 也是有效的单调时钟读数这一点是可以接受的。
func (t *Time) mono() int64 {
	if t.wall&hasMonotonic == 0 {
		return 0
	}
	return t.ext
}

// IsZero 报告 t 是否表示零时间瞬间，
// 即公元 1 年 1 月 1 日 00:00:00 UTC。
func (t Time) IsZero() bool {
	// 如果 t.wall 中设置了 hasMonotonic，则时间不可能早于 1885 年，所以不可能是公元 1 年。
	// 如果 hasMonotonic 为零，则 wall 中除纳秒字段外的所有位都应该为 0。
	// 所以如果没有纳秒则 t.wall == 0，如果没有秒则 t.ext == 0。
	// 这等效于 t.sec() == 0 && t.nsec() == 0，但效率更高。
	return t.wall == 0 && t.ext == 0
}

// After 报告时间瞬间 t 是否在 u 之后。
func (t Time) After(u Time) bool {
	if t.wall&u.wall&hasMonotonic != 0 {
		return t.ext > u.ext
	}
	ts := t.sec()
	us := u.sec()
	return ts > us || ts == us && t.nsec() > u.nsec()
}

// Before 报告时间瞬间 t 是否在 u 之前。
func (t Time) Before(u Time) bool {
	if t.wall&u.wall&hasMonotonic != 0 {
		return t.ext < u.ext
	}
	ts := t.sec()
	us := u.sec()
	return ts < us || ts == us && t.nsec() < u.nsec()
}

// Compare 比较时间瞬间 t 和 u。如果 t 在 u 之前，返回 -1；
// 如果 t 在 u 之后，返回 +1；如果相同，返回 0。
func (t Time) Compare(u Time) int {
	var tc, uc int64
	if t.wall&u.wall&hasMonotonic != 0 {
		tc, uc = t.ext, u.ext
	} else {
		tc, uc = t.sec(), u.sec()
		if tc == uc {
			tc, uc = int64(t.nsec()), int64(u.nsec())
		}
	}
	switch {
	case tc < uc:
		return -1
	case tc > uc:
		return +1
	}
	return 0
}

// Equal 报告 t 和 u 是否表示同一时间瞬间。
// 即使两个时间在不同的位置，它们也可以相等。
// 例如，6:00 +0200 和 4:00 UTC 是 Equal 的。
// 有关使用 == 比较 Time 值的陷阱，请参阅 Time 类型的文档；
// 大多数代码应该使用 Equal 代替。
func (t Time) Equal(u Time) bool {
	if t.wall&u.wall&hasMonotonic != 0 {
		return t.ext == u.ext
	}
	return t.sec() == u.sec() && t.nsec() == u.nsec()
}

// Month 指定一年中的月份（January = 1，...）。
type Month int

const (
	January Month = 1 + iota
	February
	March
	April
	May
	June
	July
	August
	September
	October
	November
	December
)

// String 返回月份的英文名称（"January"、"February"，...）。
func (m Month) String() string {
	if January <= m && m <= December {
		return longMonthNames[m-1]
	}
	buf := make([]byte, 20)
	n := fmtInt(buf, uint64(m))
	return "%!Month(" + string(buf[n:]) + ")"
}

// Weekday 指定一周中的某一天（Sunday = 0，...）。
type Weekday int

const (
	Sunday Weekday = iota
	Monday
	Tuesday
	Wednesday
	Thursday
	Friday
	Saturday
)

// String 返回星期的英文名称（"Sunday"、"Monday"，...）。
func (d Weekday) String() string {
	if Sunday <= d && d <= Saturday {
		return longDayNames[d]
	}
	buf := make([]byte, 20)
	n := fmtInt(buf, uint64(d))
	return "%!Weekday(" + string(buf[n:]) + ")"
}

// 时间计算
//
// Time 的零值被定义为
//	公元 1 年 1 月 1 日 00:00:00.000000000 UTC
// 原因是：(1) 它看起来像一个零值，或者说是日期能达到的最接近零的值
// （1-1-1 00:00:00 UTC），(2) 在实践中不太可能出现，适合作为"未设置"的标记值，
// 不像 1970 年 1 月 1 日那样常见，(3) 即使在 UTC 以西的时区也有非负年份，
// 而 1-1-0 00:00:00 UTC 在纽约会是 12-31-(-1) 19:00:00。
//
// Time 的零值并不强制时间表示使用特定的纪元。例如，要在内部使用 Unix 纪元，
// 我们可以定义为区分零值和 1970 年 1 月 1 日，
// 该时间将由 sec=-1, nsec=1e9 表示。但是，它确实建议了一种表示方式，
// 即使用 1-1-1 00:00:00 UTC 作为纪元，这也是我们的做法。
//
// Add 和 Sub 计算不受纪元选择的影响。
//
// 表示计算——年、月、分钟等——都大量依赖于除以正常数和取模运算。
// 对于日历计算，我们希望这些除法向下取整，即使对于负值也是如此，
// 这样余数始终为正，但 Go 的除法（像大多数硬件除法指令一样）向零取整。
// 我们仍然可以进行这些计算，然后针对负的被除数调整结果，
// 但反复编写这种调整很烦人。相反，我们可以改用一个很久以前的不同纪元，
// 使得我们关心的所有时间都是正的，这样向零取整和向下取整就一致了。
// 这些表示例程已经需要添加时区偏移，所以添加到备用纪元的转换成本很低。
// 例如，有一个非负时间 t 意味着我们可以写
//
//	sec = t % 60
//
// 而不是到处写
//
//	sec = t % 60
//	if sec < 0 {
//		sec += 60
//	}
//
// 日历以精确的 400 年为周期运行：为 1970-2369 打印的 400 年日历
// 同样适用于 2370-2769。甚至星期几都能对应上。
// 选择周期边界使得例外年份总是尽可能推迟，这简化了日期计算：
// 公元 0 年 3 月 1 日就是这样一天：
// 第一个闰日（2 月 29 日）在四年减一天之后，
// 第一个没有 2 月 29 日的 4 的倍数年在 100 年减一天之后，
// 第一个有 2 月 29 日的 100 的倍数年在 400 年减一天之后。
// 对于任何 Y = 0 mod 400 的年份 Y，3 月 1 日也是这样一天。
//
// 最后，如果 Unix 纪元和很久以前的纪元之间的差值可以用 int64 常量表示，
// 会很方便。
//
// 这三个考虑因素——选择尽可能早的纪元，从等于 0 mod 400 的年份的 3 月 1 日开始，
// 并且不早于 1970 年 2⁶³ 秒——将我们带到了公元前 292277022400 年。
// 我们将这一时刻称为绝对零时刻，将自该年以来以 uint64 秒数测量的时间称为绝对时间。
//
// 以自公元 1 年以来的 int64 秒数测量的时间——Time 的 sec 字段使用的表示——
// 称为内部时间。
//
// 以自 1970 年以来的 int64 秒数测量的时间称为 Unix 时间。
//
// 直接使用公元 1 年作为绝对纪元很诱人，定义例程仅对年份 >= 1 有效。
// 但是，在 UTC 以西的时区显示纪元时，这些例程将无效，因为那是公元 0 年。
// 说在一半的时区中正确打印零时间不受支持似乎是站不住脚的。
// 相比之下，在公元前 292277022400 年错误处理某些时间是可以接受的。
//
// 所有这些对 API 的客户端都是不透明的，如果有更好的实现，可以更改。
//
// 日期计算使用 Cassio Neri 和 Lorenz Schneider 的以下巧妙数学实现，
// 论文为"Euclidean affine functions and their application to calendar algorithms"，
// SP&E 2023。https://doi.org/10.1002/spe.3172
//
// 定义"日历除法"(f, f°, f*) 为将一个时间单位转换为更大单位的整数和余数并返回的
// 三元函数组。例如，在没有闰年的日历中，(d/365, d%365, y*365) 是
// 将天数转换为年数的日历除法：
//
//	(f)  year := days/365
//	(f°) yday := days%365
//	(f*) days := year*365 (+ yday)
//
// 注意 f* 通常是容易编写的函数：它是反转更复杂除法的日历乘法。
//
// Neri 和 Schneider 证明，当 f* 采用以下形式时
//
//	f*(n) = (a n + b) / c
//
// 使用 a ≥ c > 0 的整数向下取整除法，
// 他们称之为欧几里得仿射函数或 EAF，则：
//
//	f(n) = (c n + c - b - 1) / a
//	f°(n) = (c n + c - b - 1) % a / c
//
// 这为任何可以用 EAF 形式编写日历乘法的日历除法提供了相当直接的计算。
// 因为纪元已移至 3 月 1 日，所有日历乘法都可以用 EAF 形式编写。
// 当日期被分解为 [century, cyear, amonth, mday] 时，
// 其中 century、cyear 和 mday 从 0 开始，
// amonth 从 3 开始（March = 3，...，January = 13，February = 14），
// 用 EAF 形式编写的日历乘法是：
//
//	yday = (153 (amonth-3) + 2) / 5 = (153 amonth - 457) / 5
//	cday = 365 cyear + cyear/4 = 1461 cyear / 4
//	centurydays = 36524 century + century/4 = 146097 century / 4
//	days = centurydays + cday + yday + mday.
//
// 每个方程只能处理一个周期循环，所以年份计算必须分成 [century, cyear]，
// 同时处理 100 年周期和 400 年周期。
//
// yday 计算不是显而易见的，但源于以下事实：
// 从三月到一月的日历重复 5 个月 153 天的周期 31、30、31、30、31
// （我们不关心二月，因为 yday 只计算二月 1 日之前的天数，
// 因为二月是最后一个月）。
//
// 使用从 f* 推导 f 和 f° 的规则，这些乘法转换为这些除法：
//
//	century := (4 days + 3) / 146097
//	cdays := (4 days + 3) % 146097 / 4
//	cyear := (4 cdays + 3) / 1461
//	ayday := (4 cdays + 3) % 1461 / 4
//	amonth := (5 ayday + 461) / 153
//	mday := (5 ayday + 461) % 153 / 5
//
// ayday 和 amonth 中的 a 代表绝对（以 3 月 1 日为基准），
// 以区别于标准的 yday（以 1 月 1 日为基准）。
//
// 计算完这些后，我们可以使用无分支数学从 3 月 1 日日历
// 转换到标准的 1 月 1 日日历，假设从 bool 到 int 0 或 1 的
// 无分支转换，这里记为 int(b)：
//
//	isJanFeb := int(yday >= marchThruDecember)
//	month := amonth - isJanFeb*12
//	year := century*100 + cyear + isJanFeb
//	isLeap := int(cyear%4 == 0) & (int(cyear != 0) | int(century%4 == 0))
//	day := 1 + mday
//	yday := 1 + ayday + 31 + 28 + isLeap&^isJanFeb - 365*isJanFeb
//
// isLeap 是标准的闰年规则，但分离的年份形式使所有除法都简化为二进制掩码。
// 注意 day 和 yday 是从 1 开始的，与 mday 和 ayday 不同。

// 为了保持各种单位的分离，我们为每种单位定义整数类型。
// 这些类型永远不会存储在接口中，也不会被分配，
// 所以它们的类型信息不会出现在 Go 二进制文件中。
const (
	secondsPerMinute = 60
	secondsPerHour   = 60 * secondsPerMinute
	secondsPerDay    = 24 * secondsPerHour
	secondsPerWeek   = 7 * secondsPerDay
	daysPer400Years  = 365*400 + 97

	// 从 3 月 1 日到年底的天数
	marchThruDecember = 31 + 30 + 31 + 30 + 31 + 31 + 30 + 31 + 30 + 31

	// absoluteYears 是我们从内部时间减去以获得绝对时间的年数。
	// 此值必须是 0 mod 400，它定义了上面"时间计算"注释中提到的
	// "绝对零时刻"：-absoluteYears 年 3 月 1 日。
	// 绝对纪元之前的日期将无法正确计算，
	// 但除此之外，该值可以根据需要更改。
	absoluteYears = 292277022400

	// 零 Time 的年份。
	// 下面的 unixToInternal 计算假定此值。
	internalYear = 1

	// 在内部时间和绝对时间或 Unix 时间之间转换的偏移量。
	absoluteToInternal int64 = -(absoluteYears*365.2425 + marchThruDecember) * secondsPerDay
	internalToAbsolute       = -absoluteToInternal

	unixToInternal int64 = (1969*365 + 1969/4 - 1969/100 + 1969/400) * secondsPerDay
	internalToUnix int64 = -unixToInternal

	absoluteToUnix = absoluteToInternal + internalToUnix
	unixToAbsolute = unixToInternal + internalToAbsolute

	wallToInternal int64 = (1884*365 + 1884/4 - 1884/100 + 1884/400) * secondsPerDay
)

// absSeconds 计算自绝对零时刻以来的秒数。
type absSeconds uint64

// absDays 计算自绝对零时刻以来的天数。
type absDays uint64

// absCentury 计算自绝对零时刻以来的世纪数。
type absCentury uint64

// absCyear 计算自世纪开始以来的年数。
type absCyear int

// absYday 计算自年初以来的天数。
// 注意绝对年份从 3 月 1 日开始。
type absYday int

// absMonth 计算自年初以来的月数。
// absMonth=0 表示三月。
type absMonth int

// absLeap 是一个单比特（0 或 1），表示给定年份是否为闰年。
type absLeap int

// absJanFeb 是一个单比特（0 或 1），表示给定的日期是否在一月或二月。
// 这是一个特殊情况，因为绝对年份从三月开始（与正常日历年不同）。
type absJanFeb int

// dateToAbsDays 接受标准的年/月/日，并返回从绝对纪元到该日的天数。
// days 参数可以超出范围，特别是可以为负数。
func dateToAbsDays(year int64, month Month, day int) absDays {
	// 参见上面的"时间计算"注释。
	amonth := uint32(month)
	janFeb := uint32(0)
	if amonth < 3 {
		janFeb = 1
	}
	amonth += 12 * janFeb
	y := uint64(year) - uint64(janFeb) + absoluteYears

	// 对于 amonth 在 [3,14] 范围内，我们想要：
	//
	//	ayday := (153*amonth - 457) / 5
	//
	// （参见上面的"时间计算"注释
	// 以及 Neri 和 Schneider 的第 7 节。）
	//
	// 这等效于：
	//
	//	ayday := (979*amonth - 2919) >> 5
	//
	// 后一种形式使用的指令更少，
	// 所以使用它，节省几个周期。
	// 参见 Neri 和 Schneider 的第 8.3 节
	// 了解更多关于此优化的信息。
	//
	// （注意没有节省除法，因为编译器
	// 在所有情况下都不使用除法来实现 / 5。）
	ayday := (979*amonth - 2919) >> 5

	century := y / 100
	cyear := uint32(y % 100)
	cday := 1461 * cyear / 4
	centurydays := 146097 * century / 4

	return absDays(centurydays + uint64(int64(cday+ayday)+int64(day)-1))
}

// days 将绝对秒数转换为绝对天数。
func (abs absSeconds) days() absDays {
	return absDays(abs / secondsPerDay)
}

// split 将天数拆分为世纪、世纪内年份、年内天数。
func (days absDays) split() (century absCentury, cyear absCyear, ayday absYday) {
	// 参见上面的"时间计算"注释。
	d := 4*uint64(days) + 3
	century = absCentury(d / 146097)

	// 这应该是
	//	cday := uint32(d % 146097) / 4
	//	cd := 4*cday + 3
	// 也就是说
	//	cday := uint32(d % 146097) >> 2
	//	cd := cday<<2 + 3
	// 但当然 (x>>2<<2)+3 == x|3，
	// 所以改用那个。
	cd := uint32(d%146097) | 3

	// 对于 cdays 在 [0,146097]（100 年）范围内，我们想要：
	//
	//	cyear := (4 cdays + 3) / 1461
	//	yday := (4 cdays + 3) % 1461 / 4
	//
	// （参见上面的"时间计算"注释
	// 以及 Neri 和 Schneider 的第 7 节。）
	//
	// 这等效于：
	//
	//	cyear := (2939745 cdays) >> 32
	//	yday := (2939745 cdays) & 0xFFFFFFFF / 2939745 / 4
	//
	// 所以改用那个，节省几个周期。
	// 参见 Neri 和 Schneider 的第 8.3 节
	// 了解更多关于此优化的信息。
	hi, lo := bits.Mul32(2939745, cd)
	cyear = absCyear(hi)
	ayday = absYday(lo / 2939745 / 4)
	return
}

// split 将 ayday 拆分为绝对月份和标准的（从 1 开始的）月内天数。
func (ayday absYday) split() (m absMonth, mday int) {
	// 参见上面的"时间计算"注释。
	//
	// 对于 yday 在 [0,366] 范围内，
	//
	//	amonth := (5 yday + 461) / 153
	//	mday := (5 yday + 461) % 153 / 5
	//
	// 等效于：
	//
	//	amonth = (2141 yday + 197913) >> 16
	//	mday = (2141 yday + 197913) & 0xFFFF / 2141
	//
	// 所以改用那个，节省几个周期。
	// 参见 Neri 和 Schneider 的第 8.3 节。
	d := 2141*uint32(ayday) + 197913
	return absMonth(d >> 16), 1 + int((d&0xFFFF)/2141)
}

// janFeb 如果以 3 月 1 日为基准的 ayday 在一月或二月，则返回 1，否则返回 0。
func (ayday absYday) janFeb() absJanFeb {
	// 参见上面的"时间计算"注释。
	jf := absJanFeb(0)
	if ayday >= marchThruDecember {
		jf = 1
	}
	return jf
}

// month 返回 (m, janFeb) 对应的标准 Month。
func (m absMonth) month(janFeb absJanFeb) Month {
	// 参见上面的"时间计算"注释。
	return Month(m) - Month(janFeb)*12
}

// leap 如果 (century, cyear) 是闰年则返回 1，否则返回 0。
func (century absCentury) leap(cyear absCyear) absLeap {
	// 参见上面的"时间计算"注释。
	y4ok := 0
	if cyear%4 == 0 {
		y4ok = 1
	}
	y100ok := 0
	if cyear != 0 {
		y100ok = 1
	}
	y400ok := 0
	if century%4 == 0 {
		y400ok = 1
	}
	return absLeap(y4ok & (y100ok | y400ok))
}

// year 返回 (century, cyear, janFeb) 对应的标准年份。
func (century absCentury) year(cyear absCyear, janFeb absJanFeb) int {
	// 参见上面的"时间计算"注释。
	return int(uint64(century)*100-absoluteYears) + int(cyear) + int(janFeb)
}

// yday 返回 (ayday, janFeb, leap) 对应的标准的从 1 开始的年内天数。
func (ayday absYday) yday(janFeb absJanFeb, leap absLeap) int {
	// 参见上面的"时间计算"注释。
	return int(ayday) + (1 + 31 + 28) + int(leap)&^int(janFeb) - 365*int(janFeb)
}

// date 将天数转换为标准的年、月、日。
func (days absDays) date() (year int, month Month, day int) {
	century, cyear, ayday := days.split()
	amonth, day := ayday.split()
	janFeb := ayday.janFeb()
	year = century.year(cyear, janFeb)
	month = amonth.month(janFeb)
	return
}

// yearYday 将天数转换为标准年份和从 1 开始的年内天数。
func (days absDays) yearYday() (year, yday int) {
	century, cyear, ayday := days.split()
	janFeb := ayday.janFeb()
	year = century.year(cyear, janFeb)
	yday = ayday.yday(janFeb, century.leap(cyear))
	return
}

// absSec 返回时间 t 的绝对秒数，已按时区偏移调整。
// 在计算表示属性（如 Month 或 Hour）时调用此方法。
// 我们更想叫它 abs，但有一些指向 abs 的 linkname 使这变得有问题。
// 参见下面的 timeAbs。
func (t Time) absSec() absSeconds {
	l := t.loc
	// 尽可能避免函数调用。
	if l == nil || l == &localLoc {
		l = l.get()
	}
	sec := t.unixSec()
	if l != &utcLoc {
		if l.cacheZone != nil && l.cacheStart <= sec && sec < l.cacheEnd {
			sec += int64(l.cacheZone.offset)
		} else {
			_, offset, _, _, _ := l.lookup(sec)
			sec += int64(offset)
		}
	}
	return absSeconds(sec + (unixToInternal + internalToAbsolute))
}

// locabs 是 Zone 和 abs 方法的组合，
// 从单次时区查找中提取两个返回值。
func (t Time) locabs() (name string, offset int, abs absSeconds) {
	l := t.loc
	if l == nil || l == &localLoc {
		l = l.get()
	}
	// 如果命中本地时间缓存，则避免函数调用。
	sec := t.unixSec()
	if l != &utcLoc {
		if l.cacheZone != nil && l.cacheStart <= sec && sec < l.cacheEnd {
			name = l.cacheZone.name
			offset = l.cacheZone.offset
		} else {
			name, offset, _, _, _ = l.lookup(sec)
		}
		sec += int64(offset)
	} else {
		name = "UTC"
	}
	abs = absSeconds(sec + (unixToInternal + internalToAbsolute))
	return
}

// Date 返回 t 所在的年、月、日。
func (t Time) Date() (year int, month Month, day int) {
	return t.absSec().days().date()
}

// Year 返回 t 所在的年份。
func (t Time) Year() int {
	century, cyear, ayday := t.absSec().days().split()
	janFeb := ayday.janFeb()
	return century.year(cyear, janFeb)
}

// Month 返回 t 指定的年份中的月份。
func (t Time) Month() Month {
	_, _, ayday := t.absSec().days().split()
	amonth, _ := ayday.split()
	return amonth.month(ayday.janFeb())
}

// Day 返回 t 指定的月份中的日期。
func (t Time) Day() int {
	_, _, ayday := t.absSec().days().split()
	_, day := ayday.split()
	return day
}

// Weekday 返回 t 指定的星期几。
func (t Time) Weekday() Weekday {
	return t.absSec().days().weekday()
}

// weekday 返回 days 指定的星期几。
func (days absDays) weekday() Weekday {
	// 绝对年份的 3 月 1 日，就像 2000 年的 3 月 1 日一样，是星期三。
	return Weekday((uint64(days) + uint64(Wednesday)) % 7)
}

// ISOWeek 返回 t 所在的 ISO 8601 年份和周数。
// Week 的范围是 1 到 53。第 n 年的 1 月 1 日到 1 月 3 日可能属于
// 第 n-1 年的第 52 或 53 周，12 月 29 日到 12 月 31 日可能属于
// 第 n+1 年的第 1 周。
func (t Time) ISOWeek() (year, week int) {
	// 根据规则，日历年的第一个日历周是包含该年第一个星期四的周，
	// 最后一周是紧接下一个日历年第一个日历周之前的周。
	// 详情参见 https://www.iso.org/obp/ui#iso:std:iso:8601:-1:ed-1:v1:en:term:3.1.1.23

	// 周从星期一开始
	// Monday Tuesday Wednesday Thursday Friday Saturday Sunday
	// 1      2       3         4        5      6        7
	// +3     +2      +1        0        -1     -2       -3
	// 到星期四的偏移
	days := t.absSec().days()
	thu := days + absDays(Thursday-((days-1).weekday()+1))
	year, yday := thu.yearYday()
	return year, (yday-1)/7 + 1
}

// Clock 返回 t 指定的一天内的小时、分钟和秒。
func (t Time) Clock() (hour, min, sec int) {
	return t.absSec().clock()
}

// clock 返回 abs 指定的一天内的小时、分钟和秒。
func (abs absSeconds) clock() (hour, min, sec int) {
	sec = int(abs % secondsPerDay)
	hour = sec / secondsPerHour
	sec -= hour * secondsPerHour
	min = sec / secondsPerMinute
	sec -= min * secondsPerMinute
	return
}

// Hour 返回 t 指定的一天内的小时，范围是 [0, 23]。
func (t Time) Hour() int {
	return int(t.absSec()%secondsPerDay) / secondsPerHour
}

// Minute 返回 t 指定的一小时内的分钟偏移，范围是 [0, 59]。
func (t Time) Minute() int {
	return int(t.absSec()%secondsPerHour) / secondsPerMinute
}

// Second 返回 t 指定的一分钟内的秒偏移，范围是 [0, 59]。
func (t Time) Second() int {
	return int(t.absSec() % secondsPerMinute)
}

// Nanosecond 返回 t 指定的一秒内的纳秒偏移，
// 范围是 [0, 999999999]。
func (t Time) Nanosecond() int {
	return int(t.nsec())
}

// YearDay 返回 t 指定的年内天数，非闰年范围是 [1,365]，
// 闰年范围是 [1,366]。
func (t Time) YearDay() int {
	_, yday := t.absSec().days().yearYday()
	return yday
}

// Duration 表示两个时间瞬间之间经过的时间，
// 以 int64 纳秒计数表示。这种表示方式将
// 可表示的最大时长限制为大约 290 年。
type Duration int64

const (
	minDuration Duration = -1 << 63
	maxDuration Duration = 1<<63 - 1
)

// 常用时长。没有定义 Day 或更大单位，
// 以避免在夏令时时区转换时产生混淆。
//
// 要计算 [Duration] 中的单位数量，使用除法：
//
//	second := time.Second
//	fmt.Print(int64(second/time.Millisecond)) // 打印 1000
//
// 要将整数单位数转换为 Duration，使用乘法：
//
//	seconds := 10
//	fmt.Print(time.Duration(seconds)*time.Second) // 打印 10s
const (
	Nanosecond  Duration = 1
	Microsecond          = 1000 * Nanosecond
	Millisecond          = 1000 * Microsecond
	Second               = 1000 * Millisecond
	Minute               = 60 * Second
	Hour                 = 60 * Minute
)

// String 返回表示时长的字符串，格式为 "72h3m0.5s"。
// 前导零单位被省略。作为特殊情况，小于一秒的时长
// 使用更小的单位（毫秒、微秒或纳秒）格式化，以确保
// 前导数字非零。零时长格式化为 0s。
func (d Duration) String() string {
	// 这是可内联的，以利用"函数外联"。
	// 因此，调用者可以决定字符串是否必须堆分配。
	var arr [32]byte
	n := d.format(&arr)
	return string(arr[n:])
}

// format 将 d 的表示格式化到 buf 的末尾，
// 并返回第一个字符的偏移量。
func (d Duration) format(buf *[32]byte) int {
	// 最大时间是 2540400h10m10.000000000s
	w := len(buf)

	u := uint64(d)
	neg := d < 0
	if neg {
		u = -u
	}

	if u < uint64(Second) {
		// 特殊情况：如果时长小于一秒，
		// 使用更小的单位，如 1.2ms
		var prec int
		w--
		buf[w] = 's'
		w--
		switch {
		case u == 0:
			buf[w] = '0'
			return w
		case u < uint64(Microsecond):
			// 打印纳秒
			prec = 0
			buf[w] = 'n'
		case u < uint64(Millisecond):
			// 打印微秒
			prec = 3
			// U+00B5 'µ' 微符号 == 0xC2 0xB5
			w-- // 需要两个字节的空间。
			copy(buf[w:], "µ")
		default:
			// 打印毫秒
			prec = 6
			buf[w] = 'm'
		}
		w, u = fmtFrac(buf[:w], u, prec)
		w = fmtInt(buf[:w], u)
	} else {
		w--
		buf[w] = 's'

		w, u = fmtFrac(buf[:w], u, 9)

		// u 现在是整数秒
		w = fmtInt(buf[:w], u%60)
		u /= 60

		// u 现在是整数分钟
		if u > 0 {
			w--
			buf[w] = 'm'
			w = fmtInt(buf[:w], u%60)
			u /= 60

			// u 现在是整数小时
			// 在小时处停止，因为天的长度可能不同。
			if u > 0 {
				w--
				buf[w] = 'h'
				w = fmtInt(buf[:w], u)
			}
		}
	}

	if neg {
		w--
		buf[w] = '-'
	}

	return w
}

// fmtFrac 将 v/10**prec 的小数部分（例如 ".12345"）格式化到
// buf 的末尾，省略尾随零。当小数为 0 时也省略小数点。
// 返回输出字节开始的索引和 v/10**prec 的值。
func fmtFrac(buf []byte, v uint64, prec int) (nw int, nv uint64) {
	// 省略尾随零，包括小数点。
	w := len(buf)
	print := false
	for i := 0; i < prec; i++ {
		digit := v % 10
		print = print || digit != 0
		if print {
			w--
			buf[w] = byte(digit) + '0'
		}
		v /= 10
	}
	if print {
		w--
		buf[w] = '.'
	}
	return w, v
}

// fmtInt 将 v 格式化到 buf 的末尾。
// 返回输出开始的索引。
func fmtInt(buf []byte, v uint64) int {
	w := len(buf)
	if v == 0 {
		w--
		buf[w] = '0'
	} else {
		for v > 0 {
			w--
			buf[w] = byte(v%10) + '0'
			v /= 10
		}
	}
	return w
}

// Nanoseconds 以整数纳秒计数返回时长。
func (d Duration) Nanoseconds() int64 { return int64(d) }

// Microseconds 以整数微秒计数返回时长。
func (d Duration) Microseconds() int64 { return int64(d) / 1e3 }

// Milliseconds 以整数毫秒计数返回时长。
func (d Duration) Milliseconds() int64 { return int64(d) / 1e6 }

// 这些方法返回 float64，因为主要用例是打印像 1.5s 这样的浮点数，
// 截断为整数会使它们在这些情况下无用。
// 我们自己分离整数和小数部分，可以保证
// 将返回的 float64 转换为整数时的舍入方式与纯整数转换相同，
// 即使在例如 float64(d.Nanoseconds())/1e9 会有不同舍入的情况下也是如此。

// Seconds 以浮点数秒返回时长。
func (d Duration) Seconds() float64 {
	sec := d / Second
	nsec := d % Second
	return float64(sec) + float64(nsec)/1e9
}

// Minutes 以浮点数分钟返回时长。
func (d Duration) Minutes() float64 {
	min := d / Minute
	nsec := d % Minute
	return float64(min) + float64(nsec)/(60*1e9)
}

// Hours 以浮点数小时返回时长。
func (d Duration) Hours() float64 {
	hour := d / Hour
	nsec := d % Hour
	return float64(hour) + float64(nsec)/(60*60*1e9)
}

// Truncate 返回将 d 向零舍入到 m 的倍数的结果。
// 如果 m <= 0，Truncate 返回不变的 d。
func (d Duration) Truncate(m Duration) Duration {
	if m <= 0 {
		return d
	}
	return d - d%m
}

// lessThanHalf 报告 x+x < y 是否成立，但避免溢出，
// 假设 x 和 y 都是正数（Duration 是有符号的）。
func lessThanHalf(x, y Duration) bool {
	return uint64(x)+uint64(x) < uint64(y)
}

// Round 返回将 d 舍入到最接近 m 的倍数的结果。
// 中间值的舍入行为是远离零舍入。
// 如果结果超过 [Duration] 可存储的最大（或最小）值，
// Round 返回最大（或最小）时长。
// 如果 m <= 0，Round 返回不变的 d。
func (d Duration) Round(m Duration) Duration {
	if m <= 0 {
		return d
	}
	r := d % m
	if d < 0 {
		r = -r
		if lessThanHalf(r, m) {
			return d + r
		}
		if d1 := d - m + r; d1 < d {
			return d1
		}
		return minDuration // overflow
	}
	if lessThanHalf(r, m) {
		return d - r
	}
	if d1 := d + m - r; d1 > d {
		return d1
	}
	return maxDuration // 溢出
}

// Abs 返回 d 的绝对值。
// 作为特殊情况，Duration([math.MinInt64]) 被转换为 Duration([math.MaxInt64])，
// 其幅度减少 1 纳秒。
func (d Duration) Abs() Duration {
	switch {
	case d >= 0:
		return d
	case d == minDuration:
		return maxDuration
	default:
		return -d
	}
}

// Add 返回时间 t+d。
func (t Time) Add(d Duration) Time {
	dsec := int64(d / 1e9)
	nsec := t.nsec() + int32(d%1e9)
	if nsec >= 1e9 {
		dsec++
		nsec -= 1e9
	} else if nsec < 0 {
		dsec--
		nsec += 1e9
	}
	t.wall = t.wall&^nsecMask | uint64(nsec) // 更新 nsec
	t.addSec(dsec)
	if t.wall&hasMonotonic != 0 {
		te := t.ext + int64(d)
		if d < 0 && te > t.ext || d > 0 && te < t.ext {
			// 单调时钟读数现在超出范围；降级为仅墙上时钟。
			t.stripMono()
		} else {
			t.ext = te
		}
	}
	return t
}

// Sub 返回时长 t-u。如果结果超过 [Duration] 可存储的最大（或最小）值，
// 将返回最大（或最小）时长。
// 要计算 t-d（d 是时长），使用 t.Add(-d)。
func (t Time) Sub(u Time) Duration {
	if t.wall&u.wall&hasMonotonic != 0 {
		return subMono(t.ext, u.ext)
	}
	d := Duration(t.sec()-u.sec())*Second + Duration(t.nsec()-u.nsec())
	// 检查溢出或下溢。
	switch {
	case u.Add(d).Equal(t):
		return d // d 是正确的
	case t.Before(u):
		return minDuration // t - u 是负的超出范围
	default:
		return maxDuration // t - u 是正的超出范围
	}
}

func subMono(t, u int64) Duration {
	d := Duration(t - u)
	if d < 0 && t > u {
		return maxDuration // t - u 是正的超出范围
	}
	if d > 0 && t < u {
		return minDuration // t - u 是负的超出范围
	}
	return d
}

// Since 返回自 t 以来经过的时间。
// 它是 time.Now().Sub(t) 的简写。
func Since(t Time) Duration {
	if t.wall&hasMonotonic != 0 && !runtimeIsBubbled() {
		// 常见情况优化：如果 t 有单调时间，则 Sub 将只使用它。
		return subMono(runtimeNano()-startNano, t.ext)
	}
	return Now().Sub(t)
}

// Until 返回到 t 为止的时长。
// 它是 t.Sub(time.Now()) 的简写。
func Until(t Time) Duration {
	if t.wall&hasMonotonic != 0 && !runtimeIsBubbled() {
		// 常见情况优化：如果 t 有单调时间，则 Sub 将只使用它。
		return subMono(t.ext, runtimeNano()-startNano)
	}
	return t.Sub(Now())
}

// AddDate 返回将给定的年数、月数和天数添加到 t 后对应的时间。
// 例如，AddDate(-1, 2, 3) 应用于 2011 年 1 月 1 日
// 返回 2010 年 3 月 4 日。
//
// 注意，日期从根本上与时区相关联，日历周期（如天）没有固定的时长。
// AddDate 使用 Time 值的 Location 来确定这些时长。
// 这意味着相同的 AddDate 参数可能会根据基础 Time 值及其 Location
// 产生不同的绝对时间偏移。例如，AddDate(0, 0, 1) 应用于
// 3 月 27 日 12:00 总是返回 3 月 28 日 12:00。在某些地点和某些年份，
// 这是 24 小时的偏移。在其他情况下，由于夏令时转换，是 23 小时的偏移。
//
// AddDate 以与 Date 相同的方式规范化其结果，
// 因此，例如，在 10 月 31 日加一个月会得到
// 12 月 1 日，即 11 月 31 日的规范化形式。
func (t Time) AddDate(years int, months int, days int) Time {
	year, month, day := t.Date()
	hour, min, sec := t.Clock()
	return Date(year+years, month+Month(months), day+days, hour, min, sec, int(t.nsec()), t.Location())
}

// daysBefore 返回非闰年中月份 m 之前的天数。
// daysBefore(December+1) 返回 365。
func daysBefore(m Month) int {
	adj := 0
	if m >= March {
		adj = -2
	}

	// 在二月之后进行 -2 调整后，
	// 我们需要计算以下累计和：
	//	0  31  30  31  30  31  30  31  31  30  31  30  31
	// 即：
	//	0  31  61  92 122 153 183 214 245 275 306 336 367
	// 这几乎完全是 367/12×(m-1)，除了偶尔的偏差，
	// 这表明可能存在 (a×m + b)/c 形式的整数近似。
	// 对小的 a、b、c 进行暴力搜索发现
	// (214×m - 211) / 7 可以完美计算该函数。
	return (214*int(m)-211)/7 + adj
}

func daysIn(m Month, year int) int {
	if m == February {
		if isLeap(year) {
			return 29
		}
		return 28
	}
	// 排除二月的特殊情况后，模式是
	//	31 30 31 30 31 30 31 31 30 31 30 31
	// 添加 m&1 产生基本交替；
	// 添加 (m>>3)&1 从八月开始反转交替。
	return 30 + int((m+m>>3)&1)
}

// 由 runtime 包提供。
//
// now 返回当前实际时间，被 runtimeNow 取代，后者在适当时返回
// 伪造的 synctest 时钟。
//
// now 本应是内部细节，
// 但广泛使用的包通过 linkname 访问它。
// 耻辱榜的著名成员包括：
//   - gitee.com/quant1x/gox
//   - github.com/phuslu/log
//   - github.com/sethvargo/go-limiter
//   - github.com/ulule/limiter/v3
//
// 不要删除或更改类型签名。
// 参见 go.dev/issue/67401。
func now() (sec int64, nsec int32, mono int64)

// runtimeNow 返回当前时间。
// 在 synctest.Run bubble 内调用时，返回组的伪造时钟。
//
//go:linkname runtimeNow
func runtimeNow() (sec int64, nsec int32, mono int64)

// runtimeNano 以纳秒为单位返回运行时时钟的当前值。
// 在 synctest.Run bubble 内调用时，返回组的伪造时钟。
//
//go:linkname runtimeNano
func runtimeNano() int64

//go:linkname runtimeIsBubbled
func runtimeIsBubbled() bool

// 单调时间报告为相对于 startNano 的偏移量。
// 我们将 startNano 初始化为 runtimeNano() - 1，这样在
// 单调时间分辨率相当低的系统上（例如 Windows 2008，
// 其默认分辨率似乎为 15ms），
// 我们可以避免报告单调时间为 0。
// （调用者可能想使用 0 作为"未设置时间"。）
var startNano int64 = runtimeNano() - 1

// x/tools 在其测试中使用 time.Now 的 linkname。无害。
//go:linkname Now

// Now 返回当前本地时间。
func Now() Time {
	sec, nsec, mono := runtimeNow()
	if mono == 0 {
		return Time{uint64(nsec), sec + unixToInternal, Local}
	}
	mono -= startNano
	sec += unixToInternal - minWall
	if uint64(sec)>>33 != 0 {
		// 存储单调时间时，秒字段溢出了可用的 33 位。
		// 这将在 2157 年 3 月 16 日之后为真。
		return Time{uint64(nsec), sec + minWall, Local}
	}
	return Time{hasMonotonic | uint64(sec)<<nsecShift | uint64(nsec), mono, Local}
}

func unixTime(sec int64, nsec int32) Time {
	return Time{uint64(nsec), sec + unixToInternal, Local}
}

// UTC 返回将位置设置为 UTC 的 t。
func (t Time) UTC() Time {
	t.setLoc(&utcLoc)
	return t
}

// Local 返回将位置设置为本地时间的 t。
func (t Time) Local() Time {
	t.setLoc(Local)
	return t
}

// In 返回表示相同时间瞬间的 t 的副本，但副本的位置信息
// 被设置为 loc 以用于显示目的。
//
// 如果 loc 为 nil，In 会 panic。
func (t Time) In(loc *Location) Time {
	if loc == nil {
		panic("time: missing Location in call to Time.In")
	}
	t.setLoc(loc)
	return t
}

// Location 返回与 t 关联的时区信息。
func (t Time) Location() *Location {
	l := t.loc
	if l == nil {
		l = UTC
	}
	return l
}

// Zone 计算时间 t 生效的时区，返回时区的缩写名称（如 "CET"）
// 及其相对于 UTC 向东的偏移秒数。
func (t Time) Zone() (name string, offset int) {
	name, offset, _, _, _ = t.loc.lookup(t.unixSec())
	return
}

// ZoneBounds 返回时间 t 生效的时区的边界。
// 该时区从 start 开始，下一个时区从 end 开始。
// 如果时区从时间的开始处开始，start 将作为零 Time 返回。
// 如果时区永远持续，end 将作为零 Time 返回。
// 返回的时间的 Location 将与 t 相同。
func (t Time) ZoneBounds() (start, end Time) {
	_, _, startSec, endSec, _ := t.loc.lookup(t.unixSec())
	if startSec != alpha {
		start = unixTime(startSec, 0)
		start.setLoc(t.loc)
	}
	if endSec != omega {
		end = unixTime(endSec, 0)
		end.setLoc(t.loc)
	}
	return
}

// Unix 将 t 作为 Unix 时间返回，即自 1970 年 1 月 1 日 UTC 以来经过的秒数。
// 结果不依赖于与 t 关联的位置。
// 类 Unix 操作系统通常将时间记录为 32 位秒计数，
// 但由于这里的方法返回 64 位值，它在过去或未来数十亿年内都有效。
func (t Time) Unix() int64 {
	return t.unixSec()
}

// UnixMilli 将 t 作为 Unix 时间返回，即自 1970 年 1 月 1 日 UTC 以来经过的毫秒数。
// 如果以毫秒为单位的 Unix 时间无法用 int64 表示（1970 年前后超过 2.92 亿年的日期），
// 则结果未定义。结果不依赖于与 t 关联的位置。
func (t Time) UnixMilli() int64 {
	return t.unixSec()*1e3 + int64(t.nsec())/1e6
}

// UnixMicro 将 t 作为 Unix 时间返回，即自 1970 年 1 月 1 日 UTC 以来经过的微秒数。
// 如果以微秒为单位的 Unix 时间无法用 int64 表示（公元前 290307 年之前或
// 公元 294246 年之后的日期），则结果未定义。
// 结果不依赖于与 t 关联的位置。
func (t Time) UnixMicro() int64 {
	return t.unixSec()*1e6 + int64(t.nsec())/1e3
}

// UnixNano 将 t 作为 Unix 时间返回，即自 1970 年 1 月 1 日 UTC 以来经过的纳秒数。
// 如果以纳秒为单位的 Unix 时间无法用 int64 表示（1678 年之前或
// 2262 年之后的日期），则结果未定义。注意这意味着对零 Time 调用 UnixNano
// 的结果是未定义的。结果不依赖于与 t 关联的位置。
func (t Time) UnixNano() int64 {
	return (t.unixSec())*1e9 + int64(t.nsec())
}

const (
	timeBinaryVersionV1 byte = iota + 1 // 用于一般情况
	timeBinaryVersionV2                 // 仅用于 LMT
)

// AppendBinary 实现 [encoding.BinaryAppender] 接口。
func (t Time) AppendBinary(b []byte) ([]byte, error) {
	var offsetMin int16 // UTC 以东的分钟数。-1 表示 UTC。
	var offsetSec int8
	version := timeBinaryVersionV1

	if t.Location() == UTC {
		offsetMin = -1
	} else {
		_, offset := t.Zone()
		if offset%60 != 0 {
			version = timeBinaryVersionV2
			offsetSec = int8(offset % 60)
		}

		offset /= 60
		if offset < -32768 || offset == -1 || offset > 32767 {
			return b, errors.New("Time.MarshalBinary: unexpected zone offset")
		}
		offsetMin = int16(offset)
	}

	sec := t.sec()
	nsec := t.nsec()
	b = append(b,
		version,       // 字节 0：版本
		byte(sec>>56), // 字节 1-8：秒
		byte(sec>>48),
		byte(sec>>40),
		byte(sec>>32),
		byte(sec>>24),
		byte(sec>>16),
		byte(sec>>8),
		byte(sec),
		byte(nsec>>24), // 字节 9-12：纳秒
		byte(nsec>>16),
		byte(nsec>>8),
		byte(nsec),
		byte(offsetMin>>8), // 字节 13-14：时区偏移（分钟）
		byte(offsetMin),
	)
	if version == timeBinaryVersionV2 {
		b = append(b, byte(offsetSec))
	}
	return b, nil
}

// MarshalBinary 实现 [encoding.BinaryMarshaler] 接口。
func (t Time) MarshalBinary() ([]byte, error) {
	b, err := t.AppendBinary(make([]byte, 0, 16))
	if err != nil {
		return nil, err
	}
	return b, nil
}

// UnmarshalBinary 实现 [encoding.BinaryUnmarshaler] 接口。
func (t *Time) UnmarshalBinary(data []byte) error {
	buf := data
	if len(buf) == 0 {
		return errors.New("Time.UnmarshalBinary: no data")
	}

	version := buf[0]
	if version != timeBinaryVersionV1 && version != timeBinaryVersionV2 {
		return errors.New("Time.UnmarshalBinary: unsupported version")
	}

	wantLen := /*version*/ 1 + /*sec*/ 8 + /*nsec*/ 4 + /*zone offset*/ 2
	if version == timeBinaryVersionV2 {
		wantLen++
	}
	if len(buf) != wantLen {
		return errors.New("Time.UnmarshalBinary: invalid length")
	}

	buf = buf[1:]
	sec := int64(buf[7]) | int64(buf[6])<<8 | int64(buf[5])<<16 | int64(buf[4])<<24 |
		int64(buf[3])<<32 | int64(buf[2])<<40 | int64(buf[1])<<48 | int64(buf[0])<<56

	buf = buf[8:]
	nsec := int32(buf[3]) | int32(buf[2])<<8 | int32(buf[1])<<16 | int32(buf[0])<<24

	buf = buf[4:]
	offset := int(int16(buf[1])|int16(buf[0])<<8) * 60
	if version == timeBinaryVersionV2 {
		offset += int(buf[2])
	}

	*t = Time{}
	t.wall = uint64(nsec)
	t.ext = sec

	if offset == -1*60 {
		t.setLoc(&utcLoc)
	} else if _, localoff, _, _, _ := Local.lookup(t.unixSec()); offset == localoff {
		t.setLoc(Local)
	} else {
		t.setLoc(FixedZone("", offset))
	}

	return nil
}

// TODO(rsc): 在 Go 2 中移除 GobEncoder、GobDecoder、MarshalJSON、UnmarshalJSON。
// 相同的语义将由通用的 MarshalBinary、MarshalText、
// UnmarshalBinary、UnmarshalText 提供。

// GobEncode 实现 gob.GobEncoder 接口。
func (t Time) GobEncode() ([]byte, error) {
	return t.MarshalBinary()
}

// GobDecode 实现 gob.GobDecoder 接口。
func (t *Time) GobDecode(data []byte) error {
	return t.UnmarshalBinary(data)
}

// MarshalJSON 实现 [encoding/json.Marshaler] 接口。
// 时间是带亚秒精度的 RFC 3339 格式的带引号字符串。
// 如果时间戳无法表示为有效的 RFC 3339
// （例如，年份超出范围），则报告错误。
func (t Time) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(RFC3339Nano)+len(`""`))
	b = append(b, '"')
	b, err := t.appendStrictRFC3339(b)
	b = append(b, '"')
	if err != nil {
		return nil, errors.New("Time.MarshalJSON: " + err.Error())
	}
	return b, nil
}

// UnmarshalJSON 实现 [encoding/json.Unmarshaler] 接口。
// 时间必须是 RFC 3339 格式的带引号字符串。
func (t *Time) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	// TODO(https://go.dev/issue/47353): 正确取消转义 JSON 字符串。
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return errors.New("Time.UnmarshalJSON: input is not a JSON string")
	}
	data = data[len(`"`) : len(data)-len(`"`)]
	var err error
	*t, err = parseStrictRFC3339(data)
	return err
}

func (t Time) appendTo(b []byte, errPrefix string) ([]byte, error) {
	b, err := t.appendStrictRFC3339(b)
	if err != nil {
		return nil, errors.New(errPrefix + err.Error())
	}
	return b, nil
}

// AppendText 实现 [encoding.TextAppender] 接口。
// 时间以带亚秒精度的 RFC 3339 格式进行格式化。
// 如果时间戳无法表示为有效的 RFC 3339
// （例如，年份超出范围），则返回错误。
func (t Time) AppendText(b []byte) ([]byte, error) {
	return t.appendTo(b, "Time.AppendText: ")
}

// MarshalText 实现 [encoding.TextMarshaler] 接口。输出
// 与调用 [Time.AppendText] 方法的结果相同。
//
// 更多信息请参阅 [Time.AppendText]。
func (t Time) MarshalText() ([]byte, error) {
	return t.appendTo(make([]byte, 0, len(RFC3339Nano)), "Time.MarshalText: ")
}

// UnmarshalText 实现 [encoding.TextUnmarshaler] 接口。
// 时间必须是 RFC 3339 格式。
func (t *Time) UnmarshalText(data []byte) error {
	var err error
	*t, err = parseStrictRFC3339(data)
	return err
}

// Unix 返回对应于给定 Unix 时间的本地 Time，
// 即自 1970 年 1 月 1 日 UTC 以来的 sec 秒和 nsec 纳秒。
// nsec 可以超出 [0, 999999999] 范围。
// 并非所有 sec 值都有对应的时间值。例如
// 1<<63-1（最大的 int64 值）就没有。
func Unix(sec int64, nsec int64) Time {
	if nsec < 0 || nsec >= 1e9 {
		n := nsec / 1e9
		sec += n
		nsec -= n * 1e9
		if nsec < 0 {
			nsec += 1e9
			sec--
		}
	}
	return unixTime(sec, int32(nsec))
}

// UnixMilli 返回对应于给定 Unix 时间的本地 Time，
// 即自 1970 年 1 月 1 日 UTC 以来的 msec 毫秒。
func UnixMilli(msec int64) Time {
	return Unix(msec/1e3, (msec%1e3)*1e6)
}

// UnixMicro 返回对应于给定 Unix 时间的本地 Time，
// 即自 1970 年 1 月 1 日 UTC 以来的 usec 微秒。
func UnixMicro(usec int64) Time {
	return Unix(usec/1e6, (usec%1e6)*1e3)
}

// IsDST 报告配置位置中的时间是否处于夏令时。
func (t Time) IsDST() bool {
	_, _, _, _, isDST := t.loc.lookup(t.Unix())
	return isDST
}

func isLeap(year int) bool {
	// year%4 == 0 && (year%100 != 0 || year%400 == 0)
	// 低 2 位必须为零。
	// 对于 25 的倍数，低 4 位必须为零。
	// 感谢 Cassio Neri 提供此技巧。
	mask := 0xf
	if year%25 != 0 {
		mask = 3
	}
	return year&mask == 0
}

// norm 返回 nhi、nlo 使得
//
//	hi * base + lo == nhi * base + nlo
//	0 <= nlo < base
func norm(hi, lo, base int) (nhi, nlo int) {
	if lo < 0 {
		n := (-lo-1)/base + 1
		hi -= n
		lo += n * base
	}
	if lo >= base {
		n := lo / base
		hi += n
		lo -= n * base
	}
	return hi, lo
}

// Date 返回对应于
//
//	yyyy-mm-dd hh:mm:ss + nsec 纳秒
//
// 在给定位置该时间的适当时区中的 Time。
//
// month、day、hour、min、sec 和 nsec 值可以超出
// 它们通常的范围，在转换过程中将被规范化。
// 例如，10 月 32 日会转换为 11 月 1 日。
//
// 夏令时转换会跳过或重复时间。
// 例如，在美国，2011 年 3 月 13 日凌晨 2:15 从未发生过，
// 而 2011 年 11 月 6 日凌晨 1:15 发生了两次。在这种情况下，
// 时区的选择以及因此的时间是不明确的。
// Date 返回一个在转换涉及的两个时区之一中正确的时间，
// 但不保证是哪一个。
//
// 如果 loc 为 nil，Date 会 panic。
func Date(year int, month Month, day, hour, min, sec, nsec int, loc *Location) Time {
	if loc == nil {
		panic("time: missing Location in call to Date")
	}

	// 规范化月份，溢出到年份。
	m := int(month) - 1
	year, m = norm(year, m, 12)
	month = Month(m) + 1

	// 规范化 nsec、sec、min、hour，溢出到天。
	sec, nsec = norm(sec, nsec, 1e9)
	min, sec = norm(min, sec, 60)
	hour, min = norm(hour, min, 60)
	day, hour = norm(day, hour, 24)

	// 转换为绝对时间然后 Unix 时间。
	unix := int64(dateToAbsDays(int64(year), month, day))*secondsPerDay +
		int64(hour*secondsPerHour+min*secondsPerMinute+sec) +
		absoluteToUnix

	// 查找预期时间的时区偏移，以便我们可以调整到 UTC。
	// 查找函数期望 UTC，所以我们首先传入 unix，
	// 希望它不会太接近时区转换，
	// 如果是的话再进行调整。
	_, offset, start, end, _ := loc.lookup(unix)
	if offset != 0 {
		utc := unix - int64(offset)
		// 如果 utc 对于我们找到的时区有效，则我们有正确的偏移。
		// 如果不是，我们通过在位置中查找 utc 来获取正确的偏移。
		if utc < start || utc >= end {
			_, offset, _, _, _ = loc.lookup(utc)
		}
		unix -= int64(offset)
	}

	t := unixTime(unix, int32(nsec))
	t.setLoc(loc)
	return t
}

// Truncate 返回将 t 向下舍入到 d 的倍数（自零时间起）的结果。
// 如果 d <= 0，Truncate 返回剥离了任何单调时钟读数但其他不变的 t。
//
// Truncate 将时间作为自零时间以来的绝对时长进行操作；
// 它不对时间的表示形式进行操作。因此，Truncate(Hour) 可能返回
// 一个分钟非零的时间，取决于时间的 Location。
func (t Time) Truncate(d Duration) Time {
	t.stripMono()
	if d <= 0 {
		return t
	}
	_, r := div(t, d)
	return t.Add(-r)
}

// Round 返回将 t 舍入到最接近 d 的倍数（自零时间起）的结果。
// 中间值的舍入行为是向上舍入。
// 如果 d <= 0，Round 返回剥离了任何单调时钟读数但其他不变的 t。
//
// Round 将时间作为自零时间以来的绝对时长进行操作；
// 它不对时间的表示形式进行操作。因此，Round(Hour) 可能返回
// 一个分钟非零的时间，取决于时间的 Location。
func (t Time) Round(d Duration) Time {
	t.stripMono()
	if d <= 0 {
		return t
	}
	_, r := div(t, d)
	if lessThanHalf(r, d) {
		return t.Add(-r)
	}
	return t.Add(d - r)
}

// div 将 t 除以 d 并返回商的奇偶性和余数。
// 我们不再使用商的奇偶性（向上舍入而不是向偶数舍入），
// 但如果我们改变主意，它仍在这里。
func div(t Time, d Duration) (qmod2 int, r Duration) {
	neg := false
	nsec := t.nsec()
	sec := t.sec()
	if sec < 0 {
		// 对绝对值进行操作。
		neg = true
		sec = -sec
		nsec = -nsec
		if nsec < 0 {
			nsec += 1e9
			sec-- // -- 之前 sec >= 1 所以安全
		}
	}

	switch {
	// 特殊情况：2d 整除 1 秒。
	case d < Second && Second%(d+d) == 0:
		qmod2 = int(nsec/int32(d)) & 1
		r = Duration(nsec % int32(d))

	// 特殊情况：d 是 1 秒的倍数。
	case d%Second == 0:
		d1 := int64(d / Second)
		qmod2 = int(sec/d1) & 1
		r = Duration(sec%d1)*Second + Duration(nsec)

	// 一般情况。
	// 如果应用更多技巧，这可能会更快，
	// 但它实际上只是为了避免 API 中的特殊情况限制。
	// 没有人会关心这些情况。
	default:
		// 将纳秒计算为 128 位数字。
		sec := uint64(sec)
		tmp := (sec >> 32) * 1e9
		u1 := tmp >> 32
		u0 := tmp << 32
		tmp = (sec & 0xFFFFFFFF) * 1e9
		u0x, u0 := u0, u0+tmp
		if u0 < u0x {
			u1++
		}
		u0x, u0 = u0, u0+uint64(nsec)
		if u0 < u0x {
			u1++
		}

		// 通过对递减的 k 减去 r<<k 来计算余数。
		// 商的奇偶性是我们在最后一轮是否进行减法。
		d1 := uint64(d)
		for d1>>63 != 1 {
			d1 <<= 1
		}
		d0 := uint64(0)
		for {
			qmod2 = 0
			if u1 > d1 || u1 == d1 && u0 >= d0 {
				// 减法
				qmod2 = 1
				u0x, u0 = u0, u0-d0
				if u0 > u0x {
					u1--
				}
				u1 -= d1
			}
			if d1 == 0 && d0 == uint64(d) {
				break
			}
			d0 >>= 1
			d0 |= (d1 & 1) << 63
			d1 >>= 1
		}
		r = Duration(u0)
	}

	if neg && r != 0 {
		// 如果输入是负数且不是 d 的精确倍数，我们计算了 q、r 使得
		//	q*d + r = -t
		// 但正确的答案由 -(q-1)、d-r 给出：
		//	q*d + r = -t
		//	-q*d - r = t
		//	-(q-1)*d + (d - r) = t
		qmod2 ^= 1
		r = d - r
	}
	return
}

// 令人遗憾的 Linkname 兼容性
//
// timeAbs、absDate 和 absClock 模仿旧的内部细节，不再使用。
// 广泛使用的包通过 linkname 访问这些以获得"更快"的时间例程。
// 耻辱榜的著名成员包括：
//   - gitee.com/quant1x/gox
//   - github.com/phuslu/log
//
// phuslu 硬编码了 'Unix time + 9223372028715321600' [原文如此]
// 作为 absDate 和 absClock 的输入，使用旧的以 1 月 1 日为基准的
// 绝对时间。
// quant1x 通过 linkname 访问 time.Time.abs 方法并将
// 该方法的结果传递给 absDate 和 absClock。
//
// 保持这两个都能工作迫使我们在这里提供这三个例程，
// 使用旧的以 1 月 1 日为基准的纪元而不是新的以 3 月 1 日为基准的纪元。
// 而且 time.Time.abs 被 linkname 访问的事实意味着我们必须将
// 当前的 abs 方法命名为不同的名称（上面定义的 time.Time.absSec），
// 以便能够在这里提供这些旧例程的模拟。
//
// 如果这些喜欢 linkname 的包没有引用，这些代码都不会链接到二进制文件中。
// 特别是，尽管它的名字是 time.Time.abs，
// 它并不出现在 time.Time 方法表中。
//
// 不要删除这些例程或它们的 linkname，或更改
// 类型签名或参数的含义。

//go:linkname legacyTimeTimeAbs time.Time.abs
func legacyTimeTimeAbs(t Time) uint64 {
	return uint64(t.absSec() - marchThruDecember*secondsPerDay)
}

//go:linkname legacyAbsClock time.absClock
func legacyAbsClock(abs uint64) (hour, min, sec int) {
	return absSeconds(abs + marchThruDecember*secondsPerDay).clock()
}

//go:linkname legacyAbsDate time.absDate
func legacyAbsDate(abs uint64, full bool) (year int, month Month, day int, yday int) {
	d := absSeconds(abs + marchThruDecember*secondsPerDay).days()
	year, month, day = d.date()
	_, yday = d.yearYday()
	yday-- // yearYday is 1-based, old API was 0-based
	return
}
