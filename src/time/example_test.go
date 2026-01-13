// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package time_test

import (
	"fmt"
	"math"
	"time"
)

func expensiveCall() {}

func ExampleDuration() {
	t0 := time.Now()
	expensiveCall()
	t1 := time.Now()
	fmt.Printf("The call took %v to run.\n", t1.Sub(t0))
}

func ExampleDuration_Round() {
	d, err := time.ParseDuration("1h15m30.918273645s")
	if err != nil {
		panic(err)
	}

	round := []time.Duration{
		time.Nanosecond,
		time.Microsecond,
		time.Millisecond,
		time.Second,
		2 * time.Second,
		time.Minute,
		10 * time.Minute,
		time.Hour,
	}

	for _, r := range round {
		fmt.Printf("d.Round(%6s) = %s\n", r, d.Round(r).String())
	}
	// Output:
	// d.Round(   1ns) = 1h15m30.918273645s
	// d.Round(   1µs) = 1h15m30.918274s
	// d.Round(   1ms) = 1h15m30.918s
	// d.Round(    1s) = 1h15m31s
	// d.Round(    2s) = 1h15m30s
	// d.Round(  1m0s) = 1h16m0s
	// d.Round( 10m0s) = 1h20m0s
	// d.Round(1h0m0s) = 1h0m0s
}

func ExampleDuration_String() {
	fmt.Println(1*time.Hour + 2*time.Minute + 300*time.Millisecond)
	fmt.Println(300 * time.Millisecond)
	// Output:
	// 1h2m0.3s
	// 300ms
}

func ExampleDuration_Truncate() {
	d, err := time.ParseDuration("1h15m30.918273645s")
	if err != nil {
		panic(err)
	}

	trunc := []time.Duration{
		time.Nanosecond,
		time.Microsecond,
		time.Millisecond,
		time.Second,
		2 * time.Second,
		time.Minute,
		10 * time.Minute,
		time.Hour,
	}

	for _, t := range trunc {
		fmt.Printf("d.Truncate(%6s) = %s\n", t, d.Truncate(t).String())
	}
	// Output:
	// d.Truncate(   1ns) = 1h15m30.918273645s
	// d.Truncate(   1µs) = 1h15m30.918273s
	// d.Truncate(   1ms) = 1h15m30.918s
	// d.Truncate(    1s) = 1h15m30s
	// d.Truncate(    2s) = 1h15m30s
	// d.Truncate(  1m0s) = 1h15m0s
	// d.Truncate( 10m0s) = 1h10m0s
	// d.Truncate(1h0m0s) = 1h0m0s
}

func ExampleParseDuration() {
	hours, _ := time.ParseDuration("10h")
	complex, _ := time.ParseDuration("1h10m10s")
	micro, _ := time.ParseDuration("1µs")
	// 该包也接受微的不正确但常见的前缀 u。
	micro2, _ := time.ParseDuration("1us")

	fmt.Println(hours)
	fmt.Println(complex)
	fmt.Printf("There are %.0f seconds in %v.\n", complex.Seconds(), complex)
	fmt.Printf("There are %d nanoseconds in %v.\n", micro.Nanoseconds(), micro)
	fmt.Printf("There are %6.2e seconds in %v.\n", micro2.Seconds(), micro2)
	// Output:
	// 10h0m0s
	// 1h10m10s
	// There are 4210 seconds in 1h10m10s.
	// There are 1000 nanoseconds in 1µs.
	// There are 1.00e-06 seconds in 1µs.
}

func ExampleSince() {
	start := time.Now()
	expensiveCall()
	elapsed := time.Since(start)
	fmt.Printf("The call took %v to run.\n", elapsed)
}

func ExampleUntil() {
	futureTime := time.Now().Add(5 * time.Second)
	durationUntil := time.Until(futureTime)
	fmt.Printf("Duration until future time: %.0f seconds", math.Ceil(durationUntil.Seconds()))
	// Output: Duration until future time: 5 seconds
}

func ExampleDuration_Abs() {
	positiveDuration := 5 * time.Second
	negativeDuration := -3 * time.Second
	minInt64CaseDuration := time.Duration(math.MinInt64)

	absPositive := positiveDuration.Abs()
	absNegative := negativeDuration.Abs()
	absSpecial := minInt64CaseDuration.Abs() == time.Duration(math.MaxInt64)

	fmt.Printf("Absolute value of positive duration: %v\n", absPositive)
	fmt.Printf("Absolute value of negative duration: %v\n", absNegative)
	fmt.Printf("Absolute value of MinInt64 equal to MaxInt64: %t\n", absSpecial)

	// Output:
	// Absolute value of positive duration: 5s
	// Absolute value of negative duration: 3s
	// Absolute value of MinInt64 equal to MaxInt64: true
}

func ExampleDuration_Hours() {
	h, _ := time.ParseDuration("4h30m")
	fmt.Printf("I've got %.1f hours of work left.", h.Hours())
	// Output: I've got 4.5 hours of work left.
}

func ExampleDuration_Microseconds() {
	u, _ := time.ParseDuration("1s")
	fmt.Printf("One second is %d microseconds.\n", u.Microseconds())
	// Output:
	// One second is 1000000 microseconds.
}

func ExampleDuration_Milliseconds() {
	u, _ := time.ParseDuration("1s")
	fmt.Printf("One second is %d milliseconds.\n", u.Milliseconds())
	// Output:
	// One second is 1000 milliseconds.
}

func ExampleDuration_Minutes() {
	m, _ := time.ParseDuration("1h30m")
	fmt.Printf("The movie is %.0f minutes long.", m.Minutes())
	// Output: The movie is 90 minutes long.
}

func ExampleDuration_Nanoseconds() {
	u, _ := time.ParseDuration("1µs")
	fmt.Printf("One microsecond is %d nanoseconds.\n", u.Nanoseconds())
	// Output:
	// One microsecond is 1000 nanoseconds.
}

func ExampleDuration_Seconds() {
	m, _ := time.ParseDuration("1m30s")
	fmt.Printf("Take off in t-%.0f seconds.", m.Seconds())
	// Output: Take off in t-90 seconds.
}

var c chan int

func handle(int) {}

func ExampleAfter() {
	select {
	case m := <-c:
		handle(m)
	case <-time.After(10 * time.Second):
		fmt.Println("timed out")
	}
}

func ExampleSleep() {
	time.Sleep(100 * time.Millisecond)
}

func statusUpdate() string { return "" }

func ExampleTick() {
	c := time.Tick(5 * time.Second)
	for next := range c {
		fmt.Printf("%v %s\n", next, statusUpdate())
	}
}

func ExampleMonth() {
	_, month, day := time.Now().Date()
	if month == time.November && day == 10 {
		fmt.Println("Happy Go day!")
	}
}

func ExampleDate() {
	t := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Printf("Go launched at %s\n", t.Local())
	// Output: Go launched at 2009-11-10 15:00:00 -0800 PST
}

func ExampleNewTicker() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	done := make(chan bool)
	go func() {
		time.Sleep(10 * time.Second)
		done <- true
	}()
	for {
		select {
		case <-done:
			fmt.Println("Done!")
			return
		case t := <-ticker.C:
			fmt.Println("Current time: ", t)
		}
	}
}

func ExampleTime_Format() {
	// 从标准 Unix 格式的字符串中解析时间值。
	t, err := time.Parse(time.UnixDate, "Wed Feb 25 11:06:39 PST 2015")
	if err != nil { // 始终检查错误，即使它们不应该发生。
		panic(err)
	}

	tz, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { // 始终检查错误，即使它们不应该发生。
		panic(err)
	}

	// time.Time 的 Stringer 方法在没有任何格式的情况下很有用。
	fmt.Println("default format:", t)

	// 包中的预定义常量实现了常见的布局。
	fmt.Println("Unix format:", t.Format(time.UnixDate))

	// 附加到时间值的时区影响其输出。
	fmt.Println("Same, in UTC:", t.UTC().Format(time.UnixDate))

	fmt.Println("in Shanghai with seconds:", t.In(tz).Format("2006-01-02T15:04:05 -070000"))

	fmt.Println("in Shanghai with colon seconds:", t.In(tz).Format("2006-01-02T15:04:05 -07:00:00"))

	// 函数的其余部分演示了
	// 格式中使用的布局字符串的属性。

	// Parse 函数和 Format 方法使用的布局字符串
	// 通过示例显示如何表示参考时间。
	// 我们强调必须显示参考时间的格式方式，
	// 而不是用户选择的时间。因此，每个布局字符串都是
	// 时间戳的表示，
	//	Jan 2 15:04:05 2006 MST
	// 记住这个值的一个简单方法是，当按照
	// 这个顺序呈现时，它包含值（与上面的元素对齐）：
	//	  1 2  3  4  5    6  -7
	// 下面演示了一些问题。

	// Format 和 Parse 的大多数用途都使用常量布局字符串，例如
	// 本包中定义的那些，但接口很灵活，
	// 如这些示例所示。

	// 定义一个辅助函数来使示例的输出看起来很好。
	do := func(name, layout, want string) {
		got := t.Format(layout)
		if want != got {
			fmt.Printf("error: for %q got %q; expected %q\n", layout, got, want)
			return
		}
		fmt.Printf("%-16s %q gives %q\n", name, layout, got)
	}

	// 在我们的输出中打印标题。
	fmt.Printf("\nFormats:\n\n")

	// 简单的入门示例。
	do("Basic full date", "Mon Jan 2 15:04:05 MST 2006", "Wed Feb 25 11:06:39 PST 2015")
	do("Basic short date", "2006/01/02", "2015/02/25")

	// 参考时间的小时数是 15 或 3PM。布局可以以任何方式表达，
	// 由于我们的值是上午，我们应该将其视为
	// 上午时间。我们在一个格式字符串中显示两者。也是小写。
	do("AM/PM", "3PM==3pm==15h", "11AM==11am==11h")

	// 解析时，如果秒值后面跟着小数点
	// 和一些数字，那么即使
	// 布局字符串不表示小数秒，也会将其视为秒的分数。
	// 这里我们向上面使用的时间值添加小数秒。
	t, err = time.Parse(time.UnixDate, "Wed Feb 25 11:06:39.1234 PST 2015")
	if err != nil {
		panic(err)
	}
	// 如果布局字符串不包含
	// 小数秒的表示，它不会出现在输出中。
	do("No fraction", time.UnixDate, "Wed Feb 25 11:06:39 PST 2015")

	// 小数秒可以通过在
	// 布局字符串中秒值的小数点后添加一系列 0 或 9 来打印。
	// 如果布局数字是 0，则小数秒具有指定的
	// 宽度。请注意输出有尾部零。
	do("0s for fraction", "15:04:05.00000", "11:06:39.12340")

	// 如果布局中的分数是 9，则删除尾部零。
	do("9s for fraction", "15:04:05.99999999", "11:06:39.1234")

	// Output:
	// default format: 2015-02-25 11:06:39 -0800 PST
	// Unix format: Wed Feb 25 11:06:39 PST 2015
	// Same, in UTC: Wed Feb 25 19:06:39 UTC 2015
	// in Shanghai with seconds: 2015-02-26T03:06:39 +080000
	// in Shanghai with colon seconds: 2015-02-26T03:06:39 +08:00:00
	//
	// Formats:
	//
	// Basic full date  "Mon Jan 2 15:04:05 MST 2006" gives "Wed Feb 25 11:06:39 PST 2015"
	// Basic short date "2006/01/02" gives "2015/02/25"
	// AM/PM            "3PM==3pm==15h" gives "11AM==11am==11h"
	// No fraction      "Mon Jan _2 15:04:05 MST 2006" gives "Wed Feb 25 11:06:39 PST 2015"
	// 0s for fraction  "15:04:05.00000" gives "11:06:39.12340"
	// 9s for fraction  "15:04:05.99999999" gives "11:06:39.1234"

}

func ExampleTime_Format_pad() {
	// 从标准 Unix 格式的字符串中解析时间值。
	t, err := time.Parse(time.UnixDate, "Sat Mar 7 11:06:39 PST 2015")
	if err != nil { // 始终检查错误，即使它们不应该发生。
		panic(err)
	}

	// 定义一个辅助函数来使示例的输出看起来很好。
	do := func(name, layout, want string) {
		got := t.Format(layout)
		if want != got {
			fmt.Printf("error: for %q got %q; expected %q\n", layout, got, want)
			return
		}
		fmt.Printf("%-16s %q gives %q\n", name, layout, got)
	}

	// 预定义的常量 Unix 使用下划线来填充日期。
	do("Unix", time.UnixDate, "Sat Mar  7 11:06:39 PST 2015")

	// 对于固定宽度的值打印，例如可能是一个或
	// 两个字符（7 vs. 07）的日期，请在布局字符串中使用 _ 而不是空格。
	// 这里我们只打印日期，在我们的布局字符串中是 2，在我们的
	// 值中是 7。
	do("No pad", "<2>", "<7>")

	// 下划线表示空格填充，如果日期只有一位数字。
	do("Spaces", "<_2>", "< 7>")

	// "0" 表示单位数值的零填充。
	do("Zeros", "<02>", "<07>")

	// 如果值已经是正确的宽度，则不使用填充。
	// 例如，参考时间中的秒（05）在我们的值中是 39，
	// 所以它不需要填充，但分钟（04、06）需要。
	do("Suppressed pad", "04:05", "06:39")

	// Output:
	// Unix             "Mon Jan _2 15:04:05 MST 2006" gives "Sat Mar  7 11:06:39 PST 2015"
	// No pad           "<2>" gives "<7>"
	// Spaces           "<_2>" gives "< 7>"
	// Zeros            "<02>" gives "<07>"
	// Suppressed pad   "04:05" gives "06:39"

}

func ExampleTime_GoString() {
	t := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Println(t.GoString())
	t = t.Add(1 * time.Minute)
	fmt.Println(t.GoString())
	t = t.AddDate(0, 1, 0)
	fmt.Println(t.GoString())
	t, _ = time.Parse("Jan 2, 2006 at 3:04pm (MST)", "Feb 3, 2013 at 7:54pm (UTC)")
	fmt.Println(t.GoString())

	// Output:
	// time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	// time.Date(2009, time.November, 10, 23, 1, 0, 0, time.UTC)
	// time.Date(2009, time.December, 10, 23, 1, 0, 0, time.UTC)
	// time.Date(2013, time.February, 3, 19, 54, 0, 0, time.UTC)
}

func ExampleParse() {
	// 有关如何定义布局字符串以解析 time.Time 值的详细说明，
	// 请参阅 Time.Format 的示例；Parse 和
	// Format 使用相同的模型来描述它们的输入和输出。

	// longForm 通过示例显示参考时间在
	// 所需布局中的表示方式。
	const longForm = "Jan 2, 2006 at 3:04pm (MST)"
	t, _ := time.Parse(longForm, "Feb 3, 2013 at 7:54pm (PST)")
	fmt.Println(t)

	// shortForm 是参考时间在所需布局中的另一种表示方式；
	// 它没有时区。
	// 注意：不显式指定区域，则以 UTC 返回时间。
	const shortForm = "2006-Jan-02"
	t, _ = time.Parse(shortForm, "2013-Feb-03")
	fmt.Println(t)

	// 由于格式说明符，某些有效的布局是无效的时间值，
	// 例如用于空格填充的 _ 和用于区域信息的 Z。
	// 例如，RFC3339 布局 2006-01-02T15:04:05Z07:00
	// 同时包含 Z 和时区偏移，以便处理两个有效选项：
	// 2006-01-02T15:04:05Z
	// 2006-01-02T15:04:05+07:00
	t, _ = time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	fmt.Println(t)
	t, _ = time.Parse(time.RFC3339, "2006-01-02T15:04:05+07:00")
	fmt.Println(t)
	_, err := time.Parse(time.RFC3339, time.RFC3339)
	fmt.Println("error", err) // 返回错误，因为布局不是有效的时间值

	// Output:
	// 2013-02-03 19:54:00 -0800 PST
	// 2013-02-03 00:00:00 +0000 UTC
	// 2006-01-02 15:04:05 +0000 UTC
	// 2006-01-02 15:04:05 +0700 +0700
	// error parsing time "2006-01-02T15:04:05Z07:00": extra text: "07:00"
}

func ExampleParseInLocation() {
	loc, _ := time.LoadLocation("Europe/Berlin")

	// 这将在 Europe/Berlin 时区中查找名称 CEST。
	const longForm = "Jan 2, 2006 at 3:04pm (MST)"
	t, _ := time.ParseInLocation(longForm, "Jul 9, 2012 at 5:02am (CEST)", loc)
	fmt.Println(t)

	// 注意：不指定明确的区域，则以给定位置返回时间。
	const shortForm = "2006-Jan-02"
	t, _ = time.ParseInLocation(shortForm, "2012-Jul-09", loc)
	fmt.Println(t)

	// Output:
	// 2012-07-09 05:02:00 +0200 CEST
	// 2012-07-09 00:00:00 +0200 CEST
}

func ExampleUnix() {
	unixTime := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Println(unixTime.Unix())
	t := time.Unix(unixTime.Unix(), 0).UTC()
	fmt.Println(t)

	// Output:
	// 1257894000
	// 2009-11-10 23:00:00 +0000 UTC
}

func ExampleUnixMicro() {
	umt := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Println(umt.UnixMicro())
	t := time.UnixMicro(umt.UnixMicro()).UTC()
	fmt.Println(t)

	// Output:
	// 1257894000000000
	// 2009-11-10 23:00:00 +0000 UTC
}

func ExampleUnixMilli() {
	umt := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	fmt.Println(umt.UnixMilli())
	t := time.UnixMilli(umt.UnixMilli()).UTC()
	fmt.Println(t)

	// Output:
	// 1257894000000
	// 2009-11-10 23:00:00 +0000 UTC
}

func ExampleTime_Unix() {
	// Unix 的 10 亿秒，三种方式。
	fmt.Println(time.Unix(1e9, 0).UTC())     // 1e9 秒
	fmt.Println(time.Unix(0, 1e18).UTC())    // 1e18 纳秒
	fmt.Println(time.Unix(2e9, -1e18).UTC()) // 2e9 秒 - 1e18 纳秒

	t := time.Date(2001, time.September, 9, 1, 46, 40, 0, time.UTC)
	fmt.Println(t.Unix())     // 自 1970 年以来的秒数
	fmt.Println(t.UnixNano()) // 自 1970 年以来的纳秒数

	// Output:
	// 2001-09-09 01:46:40 +0000 UTC
	// 2001-09-09 01:46:40 +0000 UTC
	// 2001-09-09 01:46:40 +0000 UTC
	// 1000000000
	// 1000000000000000000
}

func ExampleTime_Round() {
	t := time.Date(0, 0, 0, 12, 15, 30, 918273645, time.UTC)
	round := []time.Duration{
		time.Nanosecond,
		time.Microsecond,
		time.Millisecond,
		time.Second,
		2 * time.Second,
		time.Minute,
		10 * time.Minute,
		time.Hour,
	}

	for _, d := range round {
		fmt.Printf("t.Round(%6s) = %s\n", d, t.Round(d).Format("15:04:05.999999999"))
	}
	// Output:
	// t.Round(   1ns) = 12:15:30.918273645
	// t.Round(   1µs) = 12:15:30.918274
	// t.Round(   1ms) = 12:15:30.918
	// t.Round(    1s) = 12:15:31
	// t.Round(    2s) = 12:15:30
	// t.Round(  1m0s) = 12:16:00
	// t.Round( 10m0s) = 12:20:00
	// t.Round(1h0m0s) = 12:00:00
}

func ExampleTime_Truncate() {
	t, _ := time.Parse("2006 Jan 02 15:04:05", "2012 Dec 07 12:15:30.918273645")
	trunc := []time.Duration{
		time.Nanosecond,
		time.Microsecond,
		time.Millisecond,
		time.Second,
		2 * time.Second,
		time.Minute,
		10 * time.Minute,
	}

	for _, d := range trunc {
		fmt.Printf("t.Truncate(%5s) = %s\n", d, t.Truncate(d).Format("15:04:05.999999999"))
	}
	// To round to the last midnight in the local timezone, create a new Date.
	midnight := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
	_ = midnight

	// Output:
	// t.Truncate(  1ns) = 12:15:30.918273645
	// t.Truncate(  1µs) = 12:15:30.918273
	// t.Truncate(  1ms) = 12:15:30.918
	// t.Truncate(   1s) = 12:15:30
	// t.Truncate(   2s) = 12:15:30
	// t.Truncate( 1m0s) = 12:15:00
	// t.Truncate(10m0s) = 12:10:00
}

func ExampleLoadLocation() {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		panic(err)
	}

	timeInUTC := time.Date(2018, 8, 30, 12, 0, 0, 0, time.UTC)
	fmt.Println(timeInUTC.In(location))
	// Output: 2018-08-30 05:00:00 -0700 PDT
}

func ExampleLocation() {
	// China doesn't have daylight saving. It uses a fixed 8 hour offset from UTC.
	secondsEastOfUTC := int((8 * time.Hour).Seconds())
	beijing := time.FixedZone("Beijing Time", secondsEastOfUTC)

	// If the system has a timezone database present, it's possible to load a location
	// from that, e.g.:
	//    newYork, err := time.LoadLocation("America/New_York")

	// Creating a time requires a location. Common locations are time.Local and time.UTC.
	timeInUTC := time.Date(2009, 1, 1, 12, 0, 0, 0, time.UTC)
	sameTimeInBeijing := time.Date(2009, 1, 1, 20, 0, 0, 0, beijing)

	// Although the UTC clock time is 1200 and the Beijing clock time is 2000, Beijing is
	// 8 hours ahead so the two dates actually represent the same instant.
	timesAreEqual := timeInUTC.Equal(sameTimeInBeijing)
	fmt.Println(timesAreEqual)

	// Output:
	// true
}

func ExampleTime_Add() {
	start := time.Date(2009, 1, 1, 12, 0, 0, 0, time.UTC)
	afterTenSeconds := start.Add(time.Second * 10)
	afterTenMinutes := start.Add(time.Minute * 10)
	afterTenHours := start.Add(time.Hour * 10)
	afterTenDays := start.Add(time.Hour * 24 * 10)

	fmt.Printf("start = %v\n", start)
	fmt.Printf("start.Add(time.Second * 10) = %v\n", afterTenSeconds)
	fmt.Printf("start.Add(time.Minute * 10) = %v\n", afterTenMinutes)
	fmt.Printf("start.Add(time.Hour * 10) = %v\n", afterTenHours)
	fmt.Printf("start.Add(time.Hour * 24 * 10) = %v\n", afterTenDays)

	// Output:
	// start = 2009-01-01 12:00:00 +0000 UTC
	// start.Add(time.Second * 10) = 2009-01-01 12:00:10 +0000 UTC
	// start.Add(time.Minute * 10) = 2009-01-01 12:10:00 +0000 UTC
	// start.Add(time.Hour * 10) = 2009-01-01 22:00:00 +0000 UTC
	// start.Add(time.Hour * 24 * 10) = 2009-01-11 12:00:00 +0000 UTC
}

func ExampleTime_AddDate() {
	start := time.Date(2023, 03, 25, 12, 0, 0, 0, time.UTC)
	oneDayLater := start.AddDate(0, 0, 1)
	dayDuration := oneDayLater.Sub(start)
	oneMonthLater := start.AddDate(0, 1, 0)
	oneYearLater := start.AddDate(1, 0, 0)

	zurich, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		panic(err)
	}
	// This was the day before a daylight saving time transition in Zürich.
	startZurich := time.Date(2023, 03, 25, 12, 0, 0, 0, zurich)
	oneDayLaterZurich := startZurich.AddDate(0, 0, 1)
	dayDurationZurich := oneDayLaterZurich.Sub(startZurich)

	fmt.Printf("oneDayLater: start.AddDate(0, 0, 1) = %v\n", oneDayLater)
	fmt.Printf("oneMonthLater: start.AddDate(0, 1, 0) = %v\n", oneMonthLater)
	fmt.Printf("oneYearLater: start.AddDate(1, 0, 0) = %v\n", oneYearLater)
	fmt.Printf("oneDayLaterZurich: startZurich.AddDate(0, 0, 1) = %v\n", oneDayLaterZurich)
	fmt.Printf("Day duration in UTC: %v | Day duration in Zürich: %v\n", dayDuration, dayDurationZurich)

	// Output:
	// oneDayLater: start.AddDate(0, 0, 1) = 2023-03-26 12:00:00 +0000 UTC
	// oneMonthLater: start.AddDate(0, 1, 0) = 2023-04-25 12:00:00 +0000 UTC
	// oneYearLater: start.AddDate(1, 0, 0) = 2024-03-25 12:00:00 +0000 UTC
	// oneDayLaterZurich: startZurich.AddDate(0, 0, 1) = 2023-03-26 12:00:00 +0200 CEST
	// Day duration in UTC: 24h0m0s | Day duration in Zürich: 23h0m0s
}

func ExampleTime_After() {
	year2000 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	year3000 := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)

	isYear3000AfterYear2000 := year3000.After(year2000) // True
	isYear2000AfterYear3000 := year2000.After(year3000) // False

	fmt.Printf("year3000.After(year2000) = %v\n", isYear3000AfterYear2000)
	fmt.Printf("year2000.After(year3000) = %v\n", isYear2000AfterYear3000)

	// Output:
	// year3000.After(year2000) = true
	// year2000.After(year3000) = false
}

func ExampleTime_Before() {
	year2000 := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	year3000 := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)

	isYear2000BeforeYear3000 := year2000.Before(year3000) // True
	isYear3000BeforeYear2000 := year3000.Before(year2000) // False

	fmt.Printf("year2000.Before(year3000) = %v\n", isYear2000BeforeYear3000)
	fmt.Printf("year3000.Before(year2000) = %v\n", isYear3000BeforeYear2000)

	// Output:
	// year2000.Before(year3000) = true
	// year3000.Before(year2000) = false
}

func ExampleTime_Date() {
	d := time.Date(2000, 2, 1, 12, 30, 0, 0, time.UTC)
	year, month, day := d.Date()

	fmt.Printf("year = %v\n", year)
	fmt.Printf("month = %v\n", month)
	fmt.Printf("day = %v\n", day)

	// Output:
	// year = 2000
	// month = February
	// day = 1
}

func ExampleTime_Day() {
	d := time.Date(2000, 2, 1, 12, 30, 0, 0, time.UTC)
	day := d.Day()

	fmt.Printf("day = %v\n", day)

	// Output:
	// day = 1
}

func ExampleTime_Equal() {
	secondsEastOfUTC := int((8 * time.Hour).Seconds())
	beijing := time.FixedZone("Beijing Time", secondsEastOfUTC)

	// Unlike the equal operator, Equal is aware that d1 and d2 are the
	// same instant but in different time zones.
	d1 := time.Date(2000, 2, 1, 12, 30, 0, 0, time.UTC)
	d2 := time.Date(2000, 2, 1, 20, 30, 0, 0, beijing)

	datesEqualUsingEqualOperator := d1 == d2
	datesEqualUsingFunction := d1.Equal(d2)

	fmt.Printf("datesEqualUsingEqualOperator = %v\n", datesEqualUsingEqualOperator)
	fmt.Printf("datesEqualUsingFunction = %v\n", datesEqualUsingFunction)

	// Output:
	// datesEqualUsingEqualOperator = false
	// datesEqualUsingFunction = true
}

func ExampleTime_String() {
	timeWithNanoseconds := time.Date(2000, 2, 1, 12, 13, 14, 15, time.UTC)
	withNanoseconds := timeWithNanoseconds.String()

	timeWithoutNanoseconds := time.Date(2000, 2, 1, 12, 13, 14, 0, time.UTC)
	withoutNanoseconds := timeWithoutNanoseconds.String()

	fmt.Printf("withNanoseconds = %v\n", withNanoseconds)
	fmt.Printf("withoutNanoseconds = %v\n", withoutNanoseconds)

	// Output:
	// withNanoseconds = 2000-02-01 12:13:14.000000015 +0000 UTC
	// withoutNanoseconds = 2000-02-01 12:13:14 +0000 UTC
}

func ExampleTime_Sub() {
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2000, 1, 1, 12, 0, 0, 0, time.UTC)

	difference := end.Sub(start)
	fmt.Printf("difference = %v\n", difference)

	// Output:
	// difference = 12h0m0s
}

func ExampleTime_AppendBinary() {
	t := time.Date(2025, 4, 1, 15, 30, 45, 123456789, time.UTC)

	var buffer []byte
	buffer, err := t.AppendBinary(buffer)
	if err != nil {
		panic(err)
	}

	var parseTime time.Time
	err = parseTime.UnmarshalBinary(buffer[:])
	if err != nil {
		panic(err)
	}

	fmt.Printf("t: %v\n", t)
	fmt.Printf("parseTime: %v\n", parseTime)
	fmt.Printf("equal: %v\n", parseTime.Equal(t))

	// Output:
	// t: 2025-04-01 15:30:45.123456789 +0000 UTC
	// parseTime: 2025-04-01 15:30:45.123456789 +0000 UTC
	// equal: true
}

func ExampleTime_AppendFormat() {
	t := time.Date(2017, time.November, 4, 11, 0, 0, 0, time.UTC)
	text := []byte("Time: ")

	text = t.AppendFormat(text, time.Kitchen)
	fmt.Println(string(text))

	// Output:
	// Time: 11:00AM
}

func ExampleTime_AppendText() {
	t := time.Date(2025, 4, 1, 15, 30, 45, 123456789, time.UTC)

	buffer := []byte("t: ")

	buffer, err := t.AppendText(buffer)
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s\n", buffer)

	// Output:
	// t: 2025-04-01T15:30:45.123456789Z
}
func ExampleFixedZone() {
	loc := time.FixedZone("UTC-8", -8*60*60)
	t := time.Date(2009, time.November, 10, 23, 0, 0, 0, loc)
	fmt.Println("The time is:", t.Format(time.RFC822))
	// Output: The time is: 10 Nov 09 23:00 UTC-8
}
