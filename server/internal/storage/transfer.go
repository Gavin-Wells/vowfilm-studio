package storage

import (
	"errors"
	"fmt"
	"strings"
)

// TransferTo copies a stopped instance into an empty database in a single target transaction.
// It deliberately refuses to merge tenants or balances into an existing installation.
func (r *SQLRepository) TransferTo(target *SQLRepository) error {
	if r == target {
		return errors.New("源数据库和目标数据库不能相同")
	}
	tables := []struct{ name, columns string }{
		{"users", "id,email,name,password,recovery,role,disabled,balance,held,created_at"},
		{"sessions", "hash,user_id,created_at,expires,agent"},
		{"pricing", "version,body,actor,created_at"},
		{"quotes", "id,user_id,body,expires"},
		{"charges", "id,user_id,quote_id,request_key,project_id,amount,state,created_at,updated_at"},
		{"ledger", "id,user_id,kind,delta,held_delta,balance_after,held_after,reference,note,actor,created_at"},
		{"audit", "id,actor,action,target,detail,created_at"},
		{"projects", "id,body"},
	}
	t, err := target.begin()
	if err != nil {
		return err
	}
	defer t.tx.Rollback()
	for _, table := range tables {
		var count int
		if err = t.row("SELECT COUNT(*) FROM " + table.name).Scan(&count); err != nil {
			return err
		}
		if table.name == "pricing" {
			if count != 1 {
				return errors.New("目标价格表已有历史，拒绝覆盖")
			}
			var actor string
			if err = t.row("SELECT actor FROM pricing WHERE version=1").Scan(&actor); err != nil || actor != "system" {
				return errors.New("目标价格表不是初始状态")
			}
		} else if count != 0 {
			return fmt.Errorf("目标数据库的 %s 非空，拒绝覆盖", table.name)
		}
	}
	if _, err = t.exec("DELETE FROM pricing"); err != nil {
		return err
	}
	for _, table := range tables {
		rows, err := r.query("SELECT " + table.columns + " FROM " + table.name)
		if err != nil {
			return err
		}
		count := len(strings.Split(table.columns, ","))
		marks := strings.TrimSuffix(strings.Repeat("?,", count), ",")
		for rows.Next() {
			values := make([]any, count)
			ptrs := make([]any, count)
			for i := range values {
				ptrs[i] = &values[i]
			}
			if err = rows.Scan(ptrs...); err != nil {
				_ = rows.Close()
				return err
			}
			if _, err = t.exec("INSERT INTO "+table.name+"("+table.columns+") VALUES("+marks+")", values...); err != nil {
				_ = rows.Close()
				return err
			}
		}
		err = rows.Err()
		_ = rows.Close()
		if err != nil {
			return err
		}
	}
	return t.tx.Commit()
}
