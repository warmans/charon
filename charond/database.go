package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"strings"

	"github.com/piotrkowalczuk/pqcomp"
)

type repositories struct {
	user            UserRepository
	userGroups      UserGroupsRepository
	userPermissions UserPermissionsRepository
	permission      PermissionRepository
	group           GroupRepository
	groupPermissions           GroupPermissionsRepository
}

func newRepositories(db *sql.DB) repositories {
	return repositories{
		user :newUserRepository(db),
		userGroups : newUserGroupsRepository(db),
		userPermissions: newUserPermissionsRepository(db),
		permission: newPermissionRepository(db),
		group : newGroupRepository(db),
		groupPermissions : newGroupPermissionsRepository(db),
	}
}

func execQueries(db *sql.DB, queries ...string) (err error) {
	exec := func(query string) {
		if err != nil {
			return
		}

		_, err = db.Exec(query)
	}

	for _, q := range queries {
		exec(q)
	}

	return
}

func setupDatabase(db *sql.DB) error {
	return execQueries(
		db,
		schemaSQL,
	)
}

func tearDownDatabase(db *sql.DB) error {
	return execQueries(
		db,
		`DROP SCHEMA IF EXISTS charon CASCADE`,
	)
}

func columns(names []string, prefix string) string {
	b := bytes.NewBuffer(nil)
	for i, n := range names {
		if i != 0 {
			b.WriteRune(',')
		}
		b.WriteString(prefix)
		b.WriteRune('.')
		b.WriteString(n)
	}

	return b.String()
}

func findQueryComp(db *sql.DB, table string, root, where *pqcomp.Composer, sort map[string]bool, columns []string) (*sql.Rows, error) {
	b := bytes.NewBufferString(`SELECT ` + strings.Join(columns, ", ") + ` FROM ` + table)

	if where.Len() != 0 {
		b.WriteString(` WHERE `)
		for where.Next() {
			if !where.First() {
				b.WriteString(" AND ")
			}

			fmt.Fprintf(b, "%s %s %s", where.Key(), where.Oper(), where.PlaceHolder())
		}
	}

	i := 0
SortLoop:
	for column, asc := range sort {
		if i != 0 {
			b.WriteString(", ")
		} else {
			b.WriteString(" ORDER BY ")
		}
		i++
		if asc {
			fmt.Fprintf(b, "%s ASC", column)
			continue SortLoop
		}

		fmt.Fprintf(b, "%s DESC ", column)
	}
	b.WriteString(" OFFSET $1 LIMIT $2")

	return db.Query(b.String(), root.Args()...)

}

func insertQueryComp(db *sql.DB, table string, insert *pqcomp.Composer, col []string) *sql.Row {
	b := bytes.NewBufferString(`INSERT INTO ` + table)

	if insert.Len() != 0 {
		b.WriteString(` (`)
		for insert.Next() {
			if !insert.First() {
				b.WriteString(", ")
			}

			fmt.Fprintf(b, "%s", insert.Key())
		}
		insert.Reset()
		b.WriteString(`) VALUES (`)
		for insert.Next() {
			if !insert.First() {
				b.WriteString(", ")
			}

			fmt.Fprintf(b, "%s", insert.PlaceHolder())
		}
		b.WriteString(`)`)
		if len(col) > 0 {
			b.WriteString(" RETURNING ")
			b.WriteString(strings.Join(col, ","))
		}
	}

	return db.QueryRow(b.String(), insert.Args()...)
}


func existsManyToManyQuery(table, column1, column2 string) string {
	return `
		SELECT EXISTS(
			SELECT 1 FROM  ` + table + ` AS ug
			WHERE ug.` + column1+ ` = $1
				AND ug.` + column2+ `= $2
		)
	`
}

func setManyToMany(db *sql.DB, table, column1, column2 string, id int64, ids []int64) (int64, int64, error) {
	var (
		err                    error
		aff, inserted, deleted int64
		tx                     *sql.Tx
		insert, exists         *sql.Stmt
		res                    sql.Result
		in                     []int64
		granted                bool
	)

	tx, err = db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		} else {
			tx.Commit()
		}
	}()

	if len(ids) > 0 {
		insert, err = tx.Prepare(`INSERT INTO ` + table + ` (` + column1 + `, ` + column2 + `) VALUES ($1, $2)`)
		if err != nil {
			return 0, 0, err
		}
		exists, err = tx.Prepare(existsManyToManyQuery(table, column1,column2))
		if err != nil {
			return 0, 0, err
		}

		in = make([]int64, 0, len(ids))
		InsertLoop:
		for _, idd := range ids {
			if err = exists.QueryRow(id, idd).Scan(&granted); err != nil {
				return 0, 0, err
			}
			// Given combination already exists, ignore.
			if granted {
				in = append(in, idd)
				granted = false
				continue InsertLoop
			}
			res, err = insert.Exec(id, idd)
			if err != nil {
				return 0, 0, err
			}

			aff, err = res.RowsAffected()
			if err != nil {
				return 0, 0, err
			}
			inserted += aff

			in = append(in, idd)
		}
	}

	delete := pqcomp.New(1, len(in))
	delete.AddArg(id)
	for _, id := range in {
		delete.AddExpr(column2, "IN", id)
	}

	query := bytes.NewBufferString(`DELETE FROM ` + table + ` WHERE ` + column1 + ` = $1`)
	for delete.Next() {
		if delete.First() {
			fmt.Fprint(query, " AND "+column2+" NOT IN (")
		} else {
			fmt.Fprint(query, ", ")

		}
		fmt.Fprintf(query, "%s", delete.PlaceHolder())
	}
	if len(in) > 0 {
		fmt.Fprint(query, ")")
	}

	res, err = tx.Exec(query.String(), delete.Args()...)
	if err != nil {
		return 0, 0, err
	}
	deleted, err = res.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	return inserted, deleted, nil
}