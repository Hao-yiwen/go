// 版权所有 2011 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

package time

import (
	"errors"
	"sync"
	"syscall"
)

//go:generate env ZONEINFO=$GOROOT/lib/time/zoneinfo.zip go run genzabbrs.go -output zoneinfo_abbrs_windows.go

// Location 将时间瞬间映射到当时使用的时区。
// 通常，Location 表示某个地理区域使用的时间偏移量集合。
// 对于许多 Location，时间偏移量取决于在该时间瞬间是否使用夏令时。
//
// Location 用于在打印的 Time 值中提供时区，
// 以及涉及可能跨越夏令时边界的时间间隔的计算。
type Location struct {
	name string
	zone []zone
	tx   []zoneTrans

	// tzdata 信息后面可以跟一个字符串，描述如何处理
	// 未记录在 zoneTrans 中的夏令时转换。
	// 格式是不带冒号的 TZ 环境变量；参见
	// https://pubs.opengroup.org/onlinepubs/9699919799/basedefs/V1_chap08.html。
	// 示例字符串，用于 America/Los_Angeles：PST8PDT,M3.2.0,M11.1.0
	extend string

	// 大多数查找是针对当前时间的。
	// 为了避免在 tx 中进行二分查找，保持一个
	// 静态的单元素缓存，它给出创建 Location 时
	// 正确的时区。
	// 如果 cacheStart <= t < cacheEnd，
	// lookup 可以返回 cacheZone。
	// cacheStart 和 cacheEnd 的单位是自 1970 年 1 月 1 日 UTC
	// 以来的秒数，以匹配 lookup 的参数。
	cacheStart int64
	cacheEnd   int64
	cacheZone  *zone
}

// zone 表示单个时区，例如 CET。
type zone struct {
	name   string // 缩写名称，如 "CET"
	offset int    // UTC 以东的秒数
	isDST  bool   // 这个时区是夏令时吗？
}

// zoneTrans 表示单个时区转换。
type zoneTrans struct {
	when         int64 // 转换时间，自 1970 年 GMT 以来的秒数
	index        uint8 // 在该时间生效的时区索引
	isstd, isutc bool  // 忽略 - 不知道这些是什么意思
}

// alpha 和 omega 是时区转换的时间起点和终点。
const (
	alpha = -1 << 63  // math.MinInt64
	omega = 1<<63 - 1 // math.MaxInt64
)

// UTC 表示协调世界时（UTC）。
var UTC *Location = &utcLoc

// utcLoc 是单独的，这样 get 可以引用 &utcLoc
// 并确保它永远不会返回 nil *Location，
// 即使行为不当的客户端已经更改了 UTC。
var utcLoc = Location{name: "UTC"}

// Local 表示系统的本地时区。
// 在 Unix 系统上，Local 查询 TZ 环境变量
// 来查找要使用的时区。没有 TZ 意味着
// 使用系统默认的 /etc/localtime。
// TZ="" 意味着使用 UTC。
// TZ="foo" 意味着使用系统时区目录中的 foo 文件。
var Local *Location = &localLoc

// localLoc 是单独的，这样即使客户端已经更改了 Local，
// initLocal 也可以初始化它。
var localLoc Location
var localOnce sync.Once

func (l *Location) get() *Location {
	if l == nil {
		return &utcLoc
	}
	if l == &localLoc {
		localOnce.Do(initLocal)
	}
	return l
}

// String 返回时区信息的描述性名称，
// 对应于 [LoadLocation] 或 [FixedZone] 的 name 参数。
func (l *Location) String() string {
	return l.get().name
}

var unnamedFixedZones []*Location
var unnamedFixedZonesOnce sync.Once

// FixedZone 返回一个始终使用给定时区名称和偏移量
// （UTC 以东的秒数）的 [Location]。
func FixedZone(name string, offset int) *Location {
	// 大多数对 FixedZone 的调用使用无名时区和按小时的偏移量。
	// 针对这种情况进行优化，为给定的小时返回相同的 *Location。
	const hoursBeforeUTC = 12
	const hoursAfterUTC = 14
	hour := offset / 60 / 60
	if name == "" && -hoursBeforeUTC <= hour && hour <= +hoursAfterUTC && hour*60*60 == offset {
		unnamedFixedZonesOnce.Do(func() {
			unnamedFixedZones = make([]*Location, hoursBeforeUTC+1+hoursAfterUTC)
			for hr := -hoursBeforeUTC; hr <= +hoursAfterUTC; hr++ {
				unnamedFixedZones[hr+hoursBeforeUTC] = fixedZone("", hr*60*60)
			}
		})
		return unnamedFixedZones[hour+hoursBeforeUTC]
	}
	return fixedZone(name, offset)
}

func fixedZone(name string, offset int) *Location {
	l := &Location{
		name:       name,
		zone:       []zone{{name, offset, false}},
		tx:         []zoneTrans{{alpha, 0, false, false}},
		cacheStart: alpha,
		cacheEnd:   omega,
	}
	l.cacheZone = &l.zone[0]
	return l
}

// lookup 返回以自 1970 年 1 月 1 日 00:00:00 UTC 以来的秒数
// 表示的时间瞬间所使用的时区信息。
//
// 返回的信息给出时区名称（如 "CET"）、
// 当该时区生效时包含 sec 的开始和结束时间、
// UTC 以东的偏移量（如 -5*60*60），以及
// 在该时间是否正在使用夏令时。
func (l *Location) lookup(sec int64) (name string, offset int, start, end int64, isDST bool) {
	l = l.get()

	if len(l.zone) == 0 {
		name = "UTC"
		offset = 0
		start = alpha
		end = omega
		isDST = false
		return
	}

	if zone := l.cacheZone; zone != nil && l.cacheStart <= sec && sec < l.cacheEnd {
		name = zone.name
		offset = zone.offset
		start = l.cacheStart
		end = l.cacheEnd
		isDST = zone.isDST
		return
	}

	if len(l.tx) == 0 || sec < l.tx[0].when {
		zone := &l.zone[l.lookupFirstZone()]
		name = zone.name
		offset = zone.offset
		start = alpha
		if len(l.tx) > 0 {
			end = l.tx[0].when
		} else {
			end = omega
		}
		isDST = zone.isDST
		return
	}

	// 二分查找时间最大且 <= sec 的条目。
	// 不使用 sort.Search 以避免依赖。
	tx := l.tx
	end = omega
	lo := 0
	hi := len(tx)
	for hi-lo > 1 {
		m := int(uint(lo+hi) >> 1)
		lim := tx[m].when
		if sec < lim {
			end = lim
			hi = m
		} else {
			lo = m
		}
	}
	zone := &l.zone[tx[lo].index]
	name = zone.name
	offset = zone.offset
	start = tx[lo].when
	// end = 在搜索过程中维护
	isDST = zone.isDST

	// 如果我们在已知时区转换的末尾，
	// 尝试 extend 字符串。
	if lo == len(tx)-1 && l.extend != "" {
		if ename, eoffset, estart, eend, eisDST, ok := tzset(l.extend, start, sec); ok {
			return ename, eoffset, estart, eend, eisDST
		}
	}

	return
}

// lookupFirstZone 返回在第一个转换时间之前或
// 没有转换时间时要使用的时区索引。
//
// 来自 https://www.iana.org/time-zones/repository/releases/tzcode2013g.tar.gz
// 的 localtime.c 中的参考实现对这些情况实现了以下算法：
//  1. 如果第一个时区未被转换使用，使用它。
//  2. 否则，如果有转换时间，并且第一个转换是到夏令时时区，
//     找到第一个转换时区之前最近的非夏令时时区。
//  3. 否则，使用第一个非夏令时时区（如果有的话）。
//  4. 否则，使用第一个时区。
func (l *Location) lookupFirstZone() int {
	// 情况 1。
	if !l.firstZoneUsed() {
		return 0
	}

	// 情况 2。
	if len(l.tx) > 0 && l.zone[l.tx[0].index].isDST {
		for zi := int(l.tx[0].index) - 1; zi >= 0; zi-- {
			if !l.zone[zi].isDST {
				return zi
			}
		}
	}

	// 情况 3。
	for zi := range l.zone {
		if !l.zone[zi].isDST {
			return zi
		}
	}

	// 情况 4。
	return 0
}

// firstZoneUsed 报告第一个时区是否被某个转换使用。
func (l *Location) firstZoneUsed() bool {
	for _, tx := range l.tx {
		if tx.index == 0 {
			return true
		}
	}
	return false
}

// tzset 接受一个类似 TZ 环境变量中的时区字符串、
// 以自 1970 年 1 月 1 日 00:00:00 UTC 以来的秒数表示的
// 最后一次时区转换时间，以及以相同方式表示的时间。
// 我们称其为 tzset 字符串，因为在 C 中函数 tzset 读取 TZ。
// 返回值与 lookup 相同，加上 ok 报告解析是否成功。
func tzset(s string, lastTxSec, sec int64) (name string, offset int, start, end int64, isDST, ok bool) {
	var (
		stdName, dstName     string
		stdOffset, dstOffset int
	)

	stdName, s, ok = tzsetName(s)
	if ok {
		stdOffset, s, ok = tzsetOffset(s)
	}
	if !ok {
		return "", 0, 0, 0, false, false
	}

	// tzset 字符串中的数字加到本地时间得到 UTC，
	// 但我们的偏移量是加到 UTC 得到本地时间，
	// 所以我们对这里看到的数字取反。
	stdOffset = -stdOffset

	if len(s) == 0 || s[0] == ',' {
		// 没有夏令时。
		return stdName, stdOffset, lastTxSec, omega, false, true
	}

	dstName, s, ok = tzsetName(s)
	if ok {
		if len(s) == 0 || s[0] == ',' {
			dstOffset = stdOffset + secondsPerHour
		} else {
			dstOffset, s, ok = tzsetOffset(s)
			dstOffset = -dstOffset // 与上面的 stdOffset 一样
		}
	}
	if !ok {
		return "", 0, 0, 0, false, false
	}

	if len(s) == 0 {
		// 根据 tzcode 的默认 DST 规则。
		s = ",M3.2.0,M11.1.0"
	}
	// TZ 定义在这里没有提到 ';'，但 tzcode 接受它。
	if s[0] != ',' && s[0] != ';' {
		return "", 0, 0, 0, false, false
	}
	s = s[1:]

	var startRule, endRule rule
	startRule, s, ok = tzsetRule(s)
	if !ok || len(s) == 0 || s[0] != ',' {
		return "", 0, 0, 0, false, false
	}
	s = s[1:]
	endRule, s, ok = tzsetRule(s)
	if !ok || len(s) > 0 {
		return "", 0, 0, 0, false, false
	}

	// 计算以 Unix 纪元以来秒数表示的年初，
	// 以及从那时起到 sec 的秒数。
	year, yday := absSeconds(sec + unixToInternal + internalToAbsolute).days().yearYday()
	ysec := int64((yday-1)*secondsPerDay) + sec%secondsPerDay
	ystart := sec - ysec

	startSec := int64(tzruleTime(year, startRule, stdOffset))
	endSec := int64(tzruleTime(year, endRule, dstOffset))
	dstIsDST, stdIsDST := true, false
	// 注意：这是在保留标签的同时翻转 "DST" 和 "STD"。
	// 这发生在南半球。因此这里的标签与目标有些不一致。
	if endSec < startSec {
		startSec, endSec = endSec, startSec
		stdName, dstName = dstName, stdName
		stdOffset, dstOffset = dstOffset, stdOffset
		stdIsDST, dstIsDST = dstIsDST, stdIsDST
	}

	// 我们返回的 start 和 end 值在接近夏令时转换时是准确的，
	// 但在其他情况下只是年初和年末。这对于唯一关心的
	// 调用者 Date 来说已经足够了。
	if ysec < startSec {
		return stdName, stdOffset, ystart, startSec + ystart, stdIsDST, true
	} else if ysec >= endSec {
		return stdName, stdOffset, endSec + ystart, ystart + 365*secondsPerDay, stdIsDST, true
	} else {
		return dstName, dstOffset, startSec + ystart, endSec + ystart, dstIsDST, true
	}
}

// tzsetName 返回 tzset 字符串 s 开头的时区名称、
// s 的剩余部分，并报告解析是否正确。
func tzsetName(s string) (string, string, bool) {
	if len(s) == 0 {
		return "", "", false
	}
	if s[0] != '<' {
		for i, r := range s {
			switch r {
			case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9', ',', '-', '+':
				if i < 3 {
					return "", "", false
				}
				return s[:i], s[i:], true
			}
		}
		if len(s) < 3 {
			return "", "", false
		}
		return s, "", true
	} else {
		for i, r := range s {
			if r == '>' {
				return s[1:i], s[i+1:], true
			}
		}
		return "", "", false
	}
}

// tzsetOffset 返回 tzset 字符串 s 开头的时区偏移量、
// s 的剩余部分，并报告解析是否正确。
// 时区偏移量以秒数返回。
func tzsetOffset(s string) (offset int, rest string, ok bool) {
	if len(s) == 0 {
		return 0, "", false
	}
	neg := false
	if s[0] == '+' {
		s = s[1:]
	} else if s[0] == '-' {
		s = s[1:]
		neg = true
	}

	// tzdata 代码允许这里的值最大到 24 * 7，
	// 尽管 POSIX 不允许。
	var hours int
	hours, s, ok = tzsetNum(s, 0, 24*7)
	if !ok {
		return 0, "", false
	}
	off := hours * secondsPerHour
	if len(s) == 0 || s[0] != ':' {
		if neg {
			off = -off
		}
		return off, s, true
	}

	var mins int
	mins, s, ok = tzsetNum(s[1:], 0, 59)
	if !ok {
		return 0, "", false
	}
	off += mins * secondsPerMinute
	if len(s) == 0 || s[0] != ':' {
		if neg {
			off = -off
		}
		return off, s, true
	}

	var secs int
	secs, s, ok = tzsetNum(s[1:], 0, 59)
	if !ok {
		return 0, "", false
	}
	off += secs

	if neg {
		off = -off
	}
	return off, s, true
}

// ruleKind 是在 tzset 字符串中可以看到的规则类型。
type ruleKind int

const (
	ruleJulian ruleKind = iota
	ruleDOY
	ruleMonthWeekDay
)

// rule 是从 tzset 字符串读取的规则。
type rule struct {
	kind ruleKind
	day  int
	week int
	mon  int
	time int // 转换时间
}

// tzsetRule 从 tzset 字符串解析规则。
// 它返回规则、字符串的剩余部分，并报告是否成功。
func tzsetRule(s string) (rule, string, bool) {
	var r rule
	if len(s) == 0 {
		return rule{}, "", false
	}
	ok := false
	if s[0] == 'J' {
		var jday int
		jday, s, ok = tzsetNum(s[1:], 1, 365)
		if !ok {
			return rule{}, "", false
		}
		r.kind = ruleJulian
		r.day = jday
	} else if s[0] == 'M' {
		var mon int
		mon, s, ok = tzsetNum(s[1:], 1, 12)
		if !ok || len(s) == 0 || s[0] != '.' {
			return rule{}, "", false

		}
		var week int
		week, s, ok = tzsetNum(s[1:], 1, 5)
		if !ok || len(s) == 0 || s[0] != '.' {
			return rule{}, "", false
		}
		var day int
		day, s, ok = tzsetNum(s[1:], 0, 6)
		if !ok {
			return rule{}, "", false
		}
		r.kind = ruleMonthWeekDay
		r.day = day
		r.week = week
		r.mon = mon
	} else {
		var day int
		day, s, ok = tzsetNum(s, 0, 365)
		if !ok {
			return rule{}, "", false
		}
		r.kind = ruleDOY
		r.day = day
	}

	if len(s) == 0 || s[0] != '/' {
		r.time = 2 * secondsPerHour // 默认是凌晨 2 点
		return r, s, true
	}

	offset, s, ok := tzsetOffset(s[1:])
	if !ok {
		return rule{}, "", false
	}
	r.time = offset

	return r, s, true
}

// tzsetNum 从 tzset 字符串解析数字。
// 它返回数字、字符串的剩余部分，并报告是否成功。
// 数字必须在 min 和 max 之间。
func tzsetNum(s string, min, max int) (num int, rest string, ok bool) {
	if len(s) == 0 {
		return 0, "", false
	}
	num = 0
	for i, r := range s {
		if r < '0' || r > '9' {
			if i == 0 || num < min {
				return 0, "", false
			}
			return num, s[i:], true
		}
		num *= 10
		num += int(r) - '0'
		if num > max {
			return 0, "", false
		}
	}
	if num < min {
		return 0, "", false
	}
	return num, "", true
}

// tzruleTime 接受年份、规则和时区偏移量，
// 返回规则生效时自年初以来的秒数。
func tzruleTime(year int, r rule, off int) int {
	var s int
	switch r.kind {
	case ruleJulian:
		s = (r.day - 1) * secondsPerDay
		if isLeap(year) && r.day >= 60 {
			s += secondsPerDay
		}
	case ruleDOY:
		s = r.day * secondsPerDay
	case ruleMonthWeekDay:
		// 蔡勒公式。
		m1 := (r.mon+9)%12 + 1
		yy0 := year
		if r.mon <= 2 {
			yy0--
		}
		yy1 := yy0 / 100
		yy2 := yy0 % 100
		dow := ((26*m1-2)/10 + 1 + yy2 + yy2/4 + yy1/4 - 2*yy1) % 7
		if dow < 0 {
			dow += 7
		}
		// 现在 dow 是 r.mon 月第一天的星期几。
		// 获取第一个 "dow" 日的月份日期。
		d := r.day - dow
		if d < 0 {
			d += 7
		}
		for i := 1; i < r.week; i++ {
			if d+7 >= daysIn(Month(r.mon), year) {
				break
			}
			d += 7
		}
		d += daysBefore(Month(r.mon))
		if isLeap(year) && r.mon > 2 {
			d++
		}
		s = d * secondsPerDay
	}

	return s + r.time - off
}

// lookupName 返回在给定伪 Unix 时间（即给定时刻在 UTC 中的时间）
// 具有给定名称（如 "EST"）的时区信息。
func (l *Location) lookupName(name string, unix int64) (offset int, ok bool) {
	l = l.get()

	// 首先尝试查找在给定时间实际生效的具有正确名称的时区。
	// （在澳大利亚悉尼，标准时间和夏令时都缩写为 "EST"。
	// 使用偏移量帮助我们为给定时间选择正确的时区。
	// 它不完美：在向后转换期间，我们可能会选择任意一个。）
	for i := range l.zone {
		zone := &l.zone[i]
		if zone.name == name {
			nam, offset, _, _, _ := l.lookup(unix - int64(zone.offset))
			if nam == zone.name {
				return offset, true
			}
		}
	}

	// 否则回退到普通的名称匹配。
	for i := range l.zone {
		zone := &l.zone[i]
		if zone.name == name {
			return zone.offset, true
		}
	}

	// 否则，放弃。
	return
}

// 注意(rsc)：最终我们需要接受 POSIX TZ 环境变量语法，
// 但我今天不想实现它。

var errLocation = errors.New("time: invalid location name")

var zoneinfo *string
var zoneinfoOnce sync.Once

// LoadLocation 返回具有给定名称的 Location。
//
// 如果名称是 "" 或 "UTC"，LoadLocation 返回 UTC。
// 如果名称是 "Local"，LoadLocation 返回 Local。
//
// 否则，名称被认为是对应于 IANA 时区数据库中
// 文件的位置名称，例如 "America/New_York"。
//
// LoadLocation 按以下顺序在以下位置查找 IANA 时区数据库：
//
//   - ZONEINFO 环境变量指定的目录或未压缩的 zip 文件
//   - 在 Unix 系统上，系统标准安装位置
//   - $GOROOT/lib/time/zoneinfo.zip
//   - time/tzdata 包，如果它被导入的话
func LoadLocation(name string) (*Location, error) {
	if name == "" || name == "UTC" {
		return UTC, nil
	}
	if name == "Local" {
		return Local, nil
	}
	if containsDotDot(name) || name[0] == '/' || name[0] == '\\' {
		// 有效的 IANA 时区名称不包含单个点，
		// 更不用说双点了。同样，没有以斜杠开头的。
		return nil, errLocation
	}
	zoneinfoOnce.Do(func() {
		env, _ := syscall.Getenv("ZONEINFO")
		zoneinfo = &env
	})
	var firstErr error
	if *zoneinfo != "" {
		if zoneData, err := loadTzinfoFromDirOrZip(*zoneinfo, name); err == nil {
			if z, err := LoadLocationFromTZData(name, zoneData); err == nil {
				return z, nil
			}
			firstErr = err
		} else if err != syscall.ENOENT {
			firstErr = err
		}
	}
	if z, err := loadLocation(name, platformZoneSources); err == nil {
		return z, nil
	} else if firstErr == nil {
		firstErr = err
	}
	return nil, firstErr
}

// containsDotDot 报告 s 是否包含 ".."。
func containsDotDot(s string) bool {
	if len(s) < 2 {
		return false
	}
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}
