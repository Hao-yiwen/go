// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package driver 定义数据库驱动程序应实现的接口，如 sql 包所使用的。
//
// 大多数代码应使用 [database/sql] 包。
//
// 驱动程序接口随着时间的推移而演变。驱动程序应实现
// [Connector] 和 [DriverContext] 接口。
// Connector.Connect 和 Driver.Open 方法不应该返回 [ErrBadConn]。
// [ErrBadConn] 只应从 [Validator]、[SessionResetter] 或
// 查询方法返回，如果连接已经处于无效（例如已关闭）状态。
//
// 所有 [Conn] 实现应实现以下接口：
// [Pinger]、[SessionResetter] 和 [Validator]。
//
// 如果支持命名参数或上下文，驱动程序的 [Conn] 应实现：
// [ExecerContext]、[QueryerContext]、[ConnPrepareContext] 和 [ConnBeginTx]。
//
// 要支持自定义数据类型，实现 [NamedValueChecker]。[NamedValueChecker]
// 也允许查询通过返回 CheckNamedValue 中的
// [ErrRemoveArgument] 来接受每个查询选项作为参数。
//
// 如果支持多个结果集，[Rows] 应实现 [RowsNextResultSet]。
// 如果驱动程序知道如何描述返回结果中存在的类型，
// 它应实现以下接口：[RowsColumnTypeScanType]、
// [RowsColumnTypeDatabaseTypeName]、[RowsColumnTypeLength]、[RowsColumnTypeNullable]
// 和 [RowsColumnTypePrecisionScale]。给定的行值也可能返回 [Rows]
// 类型，其可能代表数据库游标值。
//
// 如果 [Conn] 实现 [Validator]，则在将连接返回到
// 连接池之前调用 IsValid 方法。如果连接池中的条目
// 实现 [SessionResetter]，则在重新使用连接进行另一个查询之前调用
// ResetSession。如果连接永远不会返回到连接池但立即重新使用，
// 则在重新使用之前调用 ResetSession，但不调用 IsValid。
package driver

import (
	"context"
	"errors"
	"reflect"
)

// Value 是驱动程序必须能够处理的值。
// 它要么是 nil，要么是由数据库驱动程序的 [NamedValueChecker]
// 接口处理的类型，要么是以下类型之一的实例：
//
//	int64
//	float64
//	bool
//	[]byte
//	string
//	time.Time
//
// 如果驱动程序支持游标，返回的 Value 也可能实现此包中的 [Rows] 接口。
// 例如，当用户选择游标时使用，如 "select cursor(select * from my_table) from dual"。
// 如果从 select 的 [Rows] 被关闭，游标 [Rows] 也将被关闭。
type Value any

// NamedValue 保存值名和值。
type NamedValue struct {
	// 如果 Name 不为空，则应将其用于参数标识符，
	// 而不是顺序位置。
	//
	// Name 将没有符号前缀。
	Name string

	// 参数的顺序位置从 1 开始，总是被设置。
	Ordinal int

	// Value 是参数值。
	Value Value
}

// Driver 是必须由数据库驱动程序实现的接口。
//
// 数据库驱动程序可能实现 [DriverContext] 以访问
// 上下文并只为连接池解析一次名称，
// 而不是每个连接一次。
type Driver interface {
	// Open 返回与数据库的新连接。
	// 名称是驱动程序特定格式的字符串。
	//
	// Open 可能返回缓存的连接（之前
	// 已关闭的连接），但这样做是不必要的；sql 包
	// 维护一个空闲连接池，以实现高效的重新使用。
	//
	// 返回的连接一次只被一个 goroutine 使用。
	Open(name string) (Conn, error)
}

// 如果 [Driver] 实现 DriverContext，则 [database/sql.DB] 将调用
// OpenConnector 以获得 [Connector]，然后调用
// 该 [Connector] 的 Connect 方法来获得每个所需的连接，
// 而不是为每个连接调用 [Driver] 的 Open 方法。
// 两步序列允许驱动程序只解析一次名称
// 并且还提供访问每个 [Conn] 上下文的权限。
type DriverContext interface {
	// OpenConnector 必须用 Driver.Open 解析的相同格式
	// 解析名称参数。
	OpenConnector(name string) (Connector, error)
}

// A Connector 代表固定配置中的驱动程序
// 并可以创建任意数量的等效 Conns 供
// 多个 goroutine 使用。
//
// Connector 可以传递给 [database/sql.OpenDB] 以允许驱动程序
// 实现其自己的 [database/sql.DB] 构造函数，或由
// [DriverContext] 的 OpenConnector 方法返回，以允许驱动程序
// 访问上下文并避免重复解析驱动程序
// 配置。
//
// 如果 Connector 实现 [io.Closer]，[database/sql.DB.Close]
// 方法将调用 Close 方法并返回错误（如果有）。
type Connector interface {
	// Connect 返回与数据库的连接。
	// Connect 可能返回缓存的连接（之前
	// 已关闭的连接），但这样做是不必要的；sql 包
	// 维护一个空闲连接池，以实现高效的重新使用。
	//
	// 提供的 context.Context 仅用于拨号目的
	// （请参阅 net.DialContext），不应存储或用于
	// 其他目的。在拨号时应该仍然使用默认超时，
	// 因为连接池可能异步地调用 Connect
	// 到任何查询。
	//
	// 返回的连接一次只被一个 goroutine 使用。
	Connect(context.Context) (Conn, error)

	// Driver 返回 Connector 的底层 Driver，
	// 主要是为了保持与 sql.DB 上
	// Driver 方法的兼容性。
	Driver() Driver
}

// ErrSkip 可能由某些可选接口的方法返回，以
// 在运行时指示快速路径不可用，sql
// 包应该继续，就像没有实现可选接口一样。
// ErrSkip 仅在明确记录的地方支持。
var ErrSkip = errors.New("driver: skip fast-path; continue as if unimplemented")

// ErrBadConn 应由驱动程序返回以向 [database/sql]
// 包发出信号，表示 driver.[Conn] 处于坏状态（例如服务器
// 之前关闭了连接），[database/sql] 包应该
// 在新连接上重试。
//
// 为了防止重复操作，如果数据库服务器可能
// 已执行该操作，则不应返回 ErrBadConn。即使服务器发送回错误，
// 你也不应该返回 ErrBadConn。
//
// 将使用 [errors.Is] 检查错误。错误可能
// 包装 ErrBadConn 或实现 Is(error) bool 方法。
var ErrBadConn = errors.New("driver: bad connection")

// Pinger 是可选接口，可能由 [Conn] 实现。
//
// 如果 [Conn] 没有实现 Pinger，[database/sql.DB.Ping] 和
// [database/sql.DB.PingContext] 将检查是否至少有一个 [Conn] 可用。
//
// 如果 Conn.Ping 返回 [ErrBadConn]，[database/sql.DB.Ping] 和 [database/sql.DB.PingContext] 将
// 从池中删除 [Conn]。
type Pinger interface {
	Ping(ctx context.Context) error
}

// Execer 是可选接口，可能由 [Conn] 实现。
//
// 如果 [Conn] 既没有实现 [ExecerContext] 也没有实现 [Execer]，
// [database/sql.DB.Exec] 将首先准备查询、执行语句，
// 然后关闭语句。
//
// Exec 可能返回 [ErrSkip]。
//
// 已弃用：驱动程序应该改为实现 [ExecerContext]。
type Execer interface {
	Exec(query string, args []Value) (Result, error)
}

// ExecerContext 是可选接口，可能由 [Conn] 实现。
//
// 如果 [Conn] 没有实现 [ExecerContext]，[database/sql.DB.Exec]
// 将回退到 [Execer]；如果 Conn 也没有实现 Execer，
// [database/sql.DB.Exec] 将首先准备查询、执行语句，然后
// 关闭语句。
//
// ExecContext 可能返回 [ErrSkip]。
//
// ExecContext 必须尊重上下文超时并在上下文被取消时返回。
type ExecerContext interface {
	ExecContext(ctx context.Context, query string, args []NamedValue) (Result, error)
}

// Queryer 是可选接口，可能由 [Conn] 实现。
//
// 如果 [Conn] 既没有实现 [QueryerContext] 也没有实现 [Queryer]，
// [database/sql.DB.Query] 将首先准备查询、执行语句，
// 然后关闭语句。
//
// Query 可能返回 [ErrSkip]。
//
// 已弃用：驱动程序应该改为实现 [QueryerContext]。
type Queryer interface {
	Query(query string, args []Value) (Rows, error)
}

// QueryerContext 是可选接口，可能由 [Conn] 实现。
//
// 如果 [Conn] 没有实现 QueryerContext，[database/sql.DB.Query]
// 将回退到 [Queryer]；如果 [Conn] 也没有实现 [Queryer]，
// [database/sql.DB.Query] 将首先准备查询、执行语句，然后
// 关闭语句。
//
// QueryContext 可能返回 [ErrSkip]。
//
// QueryContext 必须尊重上下文超时并在上下文被取消时返回。
type QueryerContext interface {
	QueryContext(ctx context.Context, query string, args []NamedValue) (Rows, error)
}

// Conn 是与数据库的连接。它不会
// 被多个 goroutine 并发使用。
//
// Conn 被假定为有状态。
type Conn interface {
	// Prepare 返回准备的语句，绑定到此连接。
	Prepare(query string) (Stmt, error)

	// Close 使其无效并可能停止任何当前
	// 准备的语句和事务，将其标记为
	// 不再使用。
	//
	// 因为 sql 包维护一个免费的
	// 连接池，仅在有过量的
	// 空闲连接时调用 Close，驱动程序不应该
	// 需要自己进行连接缓存。
	//
	// 驱动程序必须确保 Close 进行的所有网络调用
	// 不会无限期地阻止（例如应用超时）。
	Close() error

	// Begin 启动并返回新事务。
	//
	// 已弃用：驱动程序应该改为实现 ConnBeginTx（或另外）。
	Begin() (Tx, error)
}

// ConnPrepareContext 用上下文增强 [Conn] 接口。
type ConnPrepareContext interface {
	// PrepareContext 返回准备的语句，绑定到此连接。
	// context 用于准备语句，
	// 它不能将上下文存储在语句本身中。
	PrepareContext(ctx context.Context, query string) (Stmt, error)
}

// IsolationLevel 是存储在 [TxOptions] 中的事务隔离级别。
//
// 此类型应被视为与 [database/sql.IsolationLevel] 相同，
// 以及在其上定义的任何值。
type IsolationLevel int

// TxOptions 保存事务选项。
//
// 此类型应被视为与 [database/sql.TxOptions] 相同。
type TxOptions struct {
	Isolation IsolationLevel
	ReadOnly  bool
}

// ConnBeginTx 用上下文和 [TxOptions] 增强 [Conn] 接口。
type ConnBeginTx interface {
	// BeginTx 启动并返回新事务。
	// 如果用户取消上下文，sql 包将
	// 在丢弃和关闭连接之前调用 Tx.Rollback。
	//
	// 这必须检查 opts.Isolation 以确定是否有一组
	// 隔离级别。如果驱动程序不支持非默认
	// 级别且已设置或存在不支持的非默认隔离级别，
	// 必须返回错误。
	//
	// 这也必须检查 opts.ReadOnly 以确定只读
	// 值是否为 true，以便设置只读事务属性（如果支持）
	// 或在不支持时返回错误。
	BeginTx(ctx context.Context, opts TxOptions) (Tx, error)
}

// SessionResetter 可能由 [Conn] 实现，以允许驱动程序重置
// 与连接关联的会话状态并发出坏连接的信号。
type SessionResetter interface {
	// ResetSession 在执行连接上的查询之前调用
	// 如果连接之前已使用过。如果驱动程序返回 ErrBadConn，
	// 连接被丢弃。
	ResetSession(ctx context.Context) error
}

// Validator 可能由 [Conn] 实现，以允许驱动程序
// 信号连接是否有效或是否应该被丢弃。
//
// 如果实现，驱动程序可能返回来自查询的基础错误，
// 即使连接应该被连接池丢弃。
type Validator interface {
	// IsValid 在将连接放入
	// 连接池之前调用。如果返回 false，连接将被丢弃。
	IsValid() bool
}

// Result 是查询执行的结果。
type Result interface {
	// LastInsertId 返回数据库的自动生成 ID
	// 之后，例如，对具有主
	// 键的表的 INSERT。
	LastInsertId() (int64, error)

	// RowsAffected 返回受查询影响的行数。
	RowsAffected() (int64, error)
}

// Stmt 是一个准备的语句。它绑定到 [Conn]，不会
// 被多个 goroutine 并发使用。
type Stmt interface {
	// Close 关闭语句。
	//
	// 从 Go 1.1 开始，如果 Stmt 在任何查询中使用，
	// 它将不会被关闭。
	//
	// 驱动程序必须确保 Close 进行的所有网络调用
	// 不会无限期地阻止（例如应用超时）。
	Close() error

	// NumInput 返回占位符参数的数量。
	//
	// 如果 NumInput 返回 >= 0，sql 包将理智检查
	// 调用者的参数计数，并在语句的
	// Exec 或 Query 方法被调用之前将错误返回给调用者。
	//
	// NumInput 也可能返回 -1，如果驱动程序不知道
	// 其占位符的数量。在这种情况下，sql 包
	// 不会理智检查 Exec 或 Query 参数计数。
	NumInput() int

	// Exec 执行不返回行的查询，例如
	// INSERT 或 UPDATE。
	//
	// 已弃用：驱动程序应该改为实现 StmtExecContext（或另外）。
	Exec(args []Value) (Result, error)

	// Query 执行可能返回行的查询，例如
	// SELECT。
	//
	// 已弃用：驱动程序应该改为实现 StmtQueryContext（或另外）。
	Query(args []Value) (Rows, error)
}

// StmtExecContext 通过使用上下文提供 Exec 来增强 [Stmt] 接口。
type StmtExecContext interface {
	// ExecContext 执行不返回行的查询，例如
	// INSERT 或 UPDATE。
	//
	// ExecContext 必须尊重上下文超时并在其被取消时返回。
	ExecContext(ctx context.Context, args []NamedValue) (Result, error)
}

// StmtQueryContext 通过使用上下文提供 Query 来增强 [Stmt] 接口。
type StmtQueryContext interface {
	// QueryContext 执行可能返回行的查询，例如
	// SELECT。
	//
	// QueryContext 必须尊重上下文超时并在其被取消时返回。
	QueryContext(ctx context.Context, args []NamedValue) (Rows, error)
}

// ErrRemoveArgument 可能从 [NamedValueChecker] 返回，以指示
// [database/sql] 包不将参数传递给驱动程序查询接口。
// 在接受查询特定选项或不是
// SQL 查询参数的结构时返回。
var ErrRemoveArgument = errors.New("driver: remove argument from query")

// NamedValueChecker 可能由 [Conn] 或 [Stmt] 可选实现。它为
// 驱动程序提供更多控制以处理超出允许的默认
// [Value] 类型的 Go 和数据库类型。
//
// [database/sql] 包按以下顺序检查值检查器，
// 在找到第一个匹配项时停止：Stmt.NamedValueChecker、Conn.NamedValueChecker、
// Stmt.ColumnConverter、[DefaultParameterConverter]。
//
// 如果 CheckNamedValue 返回 [ErrRemoveArgument]，[NamedValue] 将不包含在
// 最终查询参数中。这可用于向
// 查询本身传递特殊选项。
//
// 如果返回 [ErrSkip]，将使用列转换器错误检查
// 路径处理参数。驱动程序可能希望在
// 用尽其自己的特殊情况后返回 [ErrSkip]。
type NamedValueChecker interface {
	// CheckNamedValue 在将参数传递给驱动程序之前调用
	// 并代替任何 ColumnConverter 调用。CheckNamedValue 必须进行
	// 类型验证和转换，适合驱动程序。
	CheckNamedValue(*NamedValue) error
}

// ColumnConverter 可能由 [Stmt] 可选实现，如果
// 语句知道其自己的列类型并可以从
// 任何类型转换为驱动程序 [Value]。
//
// 已弃用：驱动程序应该实现 [NamedValueChecker]。
type ColumnConverter interface {
	// ColumnConverter 为提供的
	// 列索引返回 ValueConverter。如果特定列的类型未知
	// 或不应特殊处理，[DefaultParameterConverter]
	// 可以返回。
	ColumnConverter(idx int) ValueConverter
}

// Rows 是已执行查询结果的迭代器。
type Rows interface {
	// Columns 返回列的名称。结果的
	// 列数从切片的长度推断出来。
	// 如果特定列名未知，应为该条目
	// 返回空字符串。
	Columns() []string

	// Close 关闭行迭代器。
	Close() error

	// Next 被调用以将下一行数据填充到
	// 提供的切片中。提供的切片将
	// 与 Columns() 的宽度相同。
	//
	// 当没有更多行时，Next 应该返回 io.EOF。
	//
	// dest 不应在 Next 外部被写入。在关闭 Rows 时应小心
	// 不要修改
	// dest 中保存的缓冲区。
	Next(dest []Value) error
}

// RowsNextResultSet 通过提供一种方式信号驱动程序
// 前进到下一个结果集来扩展 [Rows] 接口。
type RowsNextResultSet interface {
	Rows

	// HasNextResultSet 在当前结果集的末尾被调用，
	// 并报告当前结果集之后是否还有另一个结果集。
	HasNextResultSet() bool

	// NextResultSet 推进驱动程序到下一个结果集，即使
	// 当前结果集中还有剩余行。
	//
	// 当没有更多结果集时，NextResultSet 应返回 io.EOF。
	NextResultSet() error
}

// RowsColumnTypeScanType 可能由 [Rows] 实现。它应该返回
// 可用于扫描类型到的值类型。例如，数据库
// 列类型 "bigint" 应返回 "[reflect.TypeOf](int64(0))"。
type RowsColumnTypeScanType interface {
	Rows
	ColumnTypeScanType(index int) reflect.Type
}

// RowsColumnTypeDatabaseTypeName 可能由 [Rows] 实现。它应该返回
// 数据库系统类型名称，不带长度。类型名称应该是大写的。
// 返回的类型示例："VARCHAR"、"NVARCHAR"、"VARCHAR2"、"CHAR"、"TEXT"、
// "DECIMAL"、"SMALLINT"、"INT"、"BIGINT"、"BOOL"、"[]BIGINT"、"JSONB"、"XML"、
// "TIMESTAMP"。
type RowsColumnTypeDatabaseTypeName interface {
	Rows
	ColumnTypeDatabaseTypeName(index int) string
}

// RowsColumnTypeLength 可能由 [Rows] 实现。它应该返回列类型的长度
// 如果列是可变长度类型。如果列
// 不是可变长度类型，ok 应该返回 false。
// 如果长度不受系统限制之外的限制，它应该返回 [math.MaxInt64]。
// 以下是各种类型返回值的示例：
//
//	TEXT          (math.MaxInt64, true)
//	varchar(10)   (10, true)
//	nvarchar(10)  (10, true)
//	decimal       (0, false)
//	int           (0, false)
//	bytea(30)     (30, true)
type RowsColumnTypeLength interface {
	Rows
	ColumnTypeLength(index int) (length int64, ok bool)
}

// RowsColumnTypeNullable 可能由 [Rows] 实现。如果已知
// 列可能为 null，nullable 值应为 true，或者如果已知
// 列不可为 null，则为 false。
// 如果列的可空性未知，ok 应为 false。
type RowsColumnTypeNullable interface {
	Rows
	ColumnTypeNullable(index int) (nullable, ok bool)
}

// RowsColumnTypePrecisionScale 可能由 [Rows] 实现。它应该返回
// 十进制类型的精度和小数位数。如果不适用，ok 应为 false。
// 以下是各种类型返回值的示例：
//
//	decimal(38, 4)    (38, 4, true)
//	int               (0, 0, false)
//	decimal           (math.MaxInt64, math.MaxInt64, true)
type RowsColumnTypePrecisionScale interface {
	Rows
	ColumnTypePrecisionScale(index int) (precision, scale int64, ok bool)
}

// Tx 是事务。
type Tx interface {
	Commit() error
	Rollback() error
}

// RowsAffected 为 INSERT 或 UPDATE 操作实现 [Result]，
// 它改变了许多行。
type RowsAffected int64

var _ Result = RowsAffected(0)

func (RowsAffected) LastInsertId() (int64, error) {
	return 0, errors.New("LastInsertId is not supported by this driver")
}

func (v RowsAffected) RowsAffected() (int64, error) {
	return int64(v), nil
}

// ResultNoRows 是预定义的 [Result] 供驱动程序在 DDL 时返回
// 命令（如 CREATE TABLE）成功。它为两者都返回错误
// LastInsertId 和 [RowsAffected]。
var ResultNoRows noRows

type noRows struct{}

var _ Result = noRows{}

func (noRows) LastInsertId() (int64, error) {
	return 0, errors.New("no LastInsertId available after DDL statement")
}

func (noRows) RowsAffected() (int64, error) {
	return 0, errors.New("no RowsAffected available after DDL statement")
}
