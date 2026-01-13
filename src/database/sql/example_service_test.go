// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sql_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func Example_openDBService() {
	// 打开驱动程序通常不会尝试连接到数据库。
	db, err := sql.Open("driver-name", "database=test1")
	if err != nil {
		// 这不会是连接错误，而是 DSN 解析错误或
		// 另一个初始化错误。
		log.Fatal(err)
	}
	db.SetConnMaxLifetime(0)
	db.SetMaxIdleConns(50)
	db.SetMaxOpenConns(50)

	s := &Service{db: db}

	http.ListenAndServe(":8080", s)
}

type Service struct {
	db *sql.DB
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	db := s.db
	switch r.URL.Path {
	default:
		http.Error(w, "not found", http.StatusNotFound)
		return
	case "/healthz":
		ctx, cancel := context.WithTimeout(r.Context(), 1*time.Second)
		defer cancel()

		err := s.db.PingContext(ctx)
		if err != nil {
			http.Error(w, fmt.Sprintf("db down: %v", err), http.StatusFailedDependency)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	case "/quick-action":
		// 这是一个简短的 SELECT。使用请求上下文作为
		// 上下文超时的基础。
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()

		id := 5
		org := 10
		var name string
		err := db.QueryRowContext(ctx, `
select
	p.name
from
	people as p
	join organization as o on p.organization = o.id
where
	p.id = :id
	and o.id = :org
;`,
			sql.Named("id", id),
			sql.Named("org", org),
		).Scan(&name)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		io.WriteString(w, name)
		return
	case "/long-action":
		// 这是一个长 SELECT。使用请求上下文作为
		// 上下文超时的基础，但给它一些时间完成。如果
		// 客户端在查询完成前取消，查询也将被取消。
		ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
		defer cancel()

		var names []string
		rows, err := db.QueryContext(ctx, "select p.name from people as p where p.active = true;")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		for rows.Next() {
			var name string
			err = rows.Scan(&name)
			if err != nil {
				break
			}
			names = append(names, name)
		}
		// 检查行 "Close" 期间的错误。
		// 如果多个语句在单个批处理中执行且
		// 行被写入和读取，这可能更重要。
		if closeErr := rows.Close(); closeErr != nil {
			http.Error(w, closeErr.Error(), http.StatusInternalServerError)
			return
		}

		// 检查行扫描错误。
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// 检查行迭代期间的错误。
		if err = rows.Err(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(names)
		return
	case "/async-action":
		// 此操作具有我们想要保留的副作用，
		// 即使客户端中途取消 HTTP 请求。
		// 为此，我们不使用 http 请求上下文作为
		// 超时的基础。
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		var orderRef = "ABC123"
		tx, err := db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, err = tx.ExecContext(ctx, "stored_proc_name", orderRef)

		if err != nil {
			tx.Rollback()
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		err = tx.Commit()
		if err != nil {
			http.Error(w, "action in unknown state, check state before attempting again", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		return
	}
}
