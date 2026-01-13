// 版权所有 2012 The Go Authors。保留所有权利。
// 本源代码的使用受 BSD 风格许可证约束，
// 该许可证可在 LICENSE 文件中找到。

// 代码由 mksyntaxgo 从 RE2 发行版生成。请勿编辑。

/*
Package syntax 将正则表达式解析为解析树，并将解析树编译为程序。
大多数正则表达式的客户端将使用 [regexp] 包的功能（如 [regexp.Compile] 和 [regexp.Match]）
而不是此包。

# 语法

使用 [Perl] 标志解析时，此包理解的正则表达式语法如下。
可以通过向 [Parse] 传递替代标志来禁用部分语法。

单个字符：

	.              任意字符，可能包括换行符（标志 s=true）
	[xyz]          字符类
	[^xyz]         否定字符类
	\d             Perl 字符类
	\D             否定 Perl 字符类
	[[:alpha:]]    ASCII 字符类
	[[:^alpha:]]   否定 ASCII 字符类
	\pN            Unicode 字符类（单字母名称）
	\p{Greek}      Unicode 字符类
	\PN            否定 Unicode 字符类（单字母名称）
	\P{Greek}      否定 Unicode 字符类

组合：

	xy             x 后跟 y
	x|y            x 或 y（优先 x）

重复：

	x*             零个或多个 x，优先更多
	x+             一个或多个 x，优先更多
	x?             零个或一个 x，优先一个
	x{n,m}         n 或 n+1 或 ... 或 m 个 x，优先更多
	x{n,}          n 个或更多 x，优先更多
	x{n}           恰好 n 个 x
	x*?            零个或多个 x，优先更少
	x+?            一个或多个 x，优先更少
	x??            零个或一个 x，优先零个
	x{n,m}?        n 或 n+1 或 ... 或 m 个 x，优先更少
	x{n,}?         n 个或更多 x，优先更少
	x{n}?          恰好 n 个 x

实现限制：计数形式 x{n,m}、x{n,} 和 x{n}
拒绝创建最小或最大重复计数超过 1000 的形式。
无限重复不受此限制。

分组：

	(re)           编号捕获组（子匹配）
	(?P<name>re)   命名和编号捕获组（子匹配）
	(?<name>re)    命名和编号捕获组（子匹配）
	(?:re)         非捕获组
	(?flags)       在当前组内设置标志；非捕获
	(?flags:re)    在 re 期间设置标志；非捕获

	标志语法是 xyz（设置）或 -xyz（清除）或 xy-z（设置 xy，清除 z）。标志有：

	i              大小写不敏感（默认 false）
	m              多行模式：^ 和 $ 除了匹配文本开头/结尾外还匹配行开头/结尾（默认 false）
	s              让 . 匹配 \n（默认 false）
	U              非贪婪：交换 x* 和 x*?、x+ 和 x+? 等的含义（默认 false）

空字符串：

	^              在文本或行的开头（标志 m=true）
	$              在文本结尾（像 \z 而不是 \Z）或行结尾（标志 m=true）
	\A             在文本开头
	\b             在 ASCII 单词边界（一侧是 \w，另一侧是 \W、\A 或 \z）
	\B             不在 ASCII 单词边界
	\z             在文本结尾

转义序列：

	\a             响铃（== \007）
	\f             换页（== \014）
	\t             水平制表符（== \011）
	\n             换行符（== \012）
	\r             回车符（== \015）
	\v             垂直制表符（== \013）
	\*             字面 *，对于任何标点字符 *
	\123           八进制字符代码（最多三位数字）
	\x7F           十六进制字符代码（恰好两位数字）
	\x{10FFFF}     十六进制字符代码
	\Q...\E        字面文本 ... 即使 ... 包含标点符号

字符类元素：

	x              单个字符
	A-Z            字符范围（包含）
	\d             Perl 字符类
	[:foo:]        ASCII 字符类 foo
	\p{Foo}        Unicode 字符类 Foo
	\pF            Unicode 字符类 F（单字母名称）

作为字符类元素的命名字符类：

	[\d]           数字（== \d）
	[^\d]          非数字（== \D）
	[\D]           非数字（== \D）
	[^\D]          非非数字（== \d）
	[[:name:]]     字符类内的命名 ASCII 类（== [:name:]）
	[^[:name:]]    否定字符类内的命名 ASCII 类（== [:^name:]）
	[\p{Name}]     字符类内的命名 Unicode 属性（== \p{Name}）
	[^\p{Name}]    否定字符类内的命名 Unicode 属性（== \P{Name}）

Perl 字符类（全部仅限 ASCII）：

	\d             数字（== [0-9]）
	\D             非数字（== [^0-9]）
	\s             空白（== [\t\n\f\r ]）
	\S             非空白（== [^\t\n\f\r ]）
	\w             单词字符（== [0-9A-Za-z_]）
	\W             非单词字符（== [^0-9A-Za-z_]）

ASCII 字符类：

	[[:alnum:]]    字母数字（== [0-9A-Za-z]）
	[[:alpha:]]    字母（== [A-Za-z]）
	[[:ascii:]]    ASCII（== [\x00-\x7F]）
	[[:blank:]]    空格（== [\t ]）
	[[:cntrl:]]    控制（== [\x00-\x1F\x7F]）
	[[:digit:]]    数字（== [0-9]）
	[[:graph:]]    图形（== [!-~] == [A-Za-z0-9!"#$%&'()*+,\-./:;<=>?@[\\\]^_`{|}~]）
	[[:lower:]]    小写（== [a-z]）
	[[:print:]]    可打印（== [ -~] == [ [:graph:]]）
	[[:punct:]]    标点（== [!-/:-@[-`{-~]）
	[[:space:]]    空白（== [\t\n\v\f\r ]）
	[[:upper:]]    大写（== [A-Z]）
	[[:word:]]     单词字符（== [0-9A-Za-z_]）
	[[:xdigit:]]   十六进制数字（== [0-9A-Fa-f]）

Unicode 字符类是 [unicode.Categories]、[unicode.CategoryAliases]
和 [unicode.Scripts] 中的那些。
*/
package syntax
