package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ihexxa/quickshare/src/db"
)

type SQLiteStore struct {
	db  db.IDB
	mtx *sync.RWMutex
}

func (st *SQLiteStore) setUser(ctx context.Context, user *db.User) error {
	var err error
	if err = db.CheckUser(user, false); err != nil {
		return err
	}

	quotaStr, err := json.Marshal(user.Quota)
	if err != nil {
		return err
	}
	preferencesStr, err := json.Marshal(user.Preferences)
	if err != nil {
		return err
	}
	_, err = st.db.ExecContext(
		ctx,
		`update t_user
		set name=?, pwd=?, role=?, used_space=?, quota=?, preference=?
		where id=?`,
		user.Name,
		user.Pwd,
		user.Role,
		user.UsedSpace,
		quotaStr,
		preferencesStr,
		user.ID,
	)
	return err
}

func (st *SQLiteStore) getUser(ctx context.Context, id uint64) (*db.User, error) {
	user := &db.User{}
	var quotaStr, preferenceStr string
	err := st.db.QueryRowContext(
		ctx,
		`select id, name, pwd, role, used_space, quota, preference
		from t_user
		where id=?`,
		id,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Pwd,
		&user.Role,
		&user.UsedSpace,
		&quotaStr,
		&preferenceStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrUserNotFound
		}
		return nil, err
	}

	err = json.Unmarshal([]byte(quotaStr), &user.Quota)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(preferenceStr), &user.Preferences)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (st *SQLiteStore) AddUser(ctx context.Context, user *db.User) error {
	st.Lock()
	defer st.Unlock()

	quotaStr, err := json.Marshal(user.Quota)
	if err != nil {
		return err
	}
	preferenceStr, err := json.Marshal(user.Preferences)
	if err != nil {
		return err
	}
	_, err = st.db.ExecContext(
		ctx,
		`insert into t_user (id, name, pwd, role, used_space, quota, preference) values (?, ?, ?, ?, ?, ?, ?)`,
		user.ID,
		user.Name,
		user.Pwd,
		user.Role,
		user.UsedSpace,
		quotaStr,
		preferenceStr,
	)
	return err
}

func (st *SQLiteStore) DelUser(ctx context.Context, id uint64) error {
	st.Lock()
	defer st.Unlock()

	_, err := st.db.ExecContext(
		ctx,
		`delete from t_user where id=?`,
		id,
	)
	return err
}

func (st *SQLiteStore) GetUser(ctx context.Context, id uint64) (*db.User, error) {
	st.RLock()
	defer st.RUnlock()

	user, err := st.getUser(ctx, id)
	if err != nil {
		return nil, err
	}

	return user, err
}

func (st *SQLiteStore) GetUserByName(ctx context.Context, name string) (*db.User, error) {
	st.RLock()
	defer st.RUnlock()

	user := &db.User{}
	var quotaStr, preferenceStr string
	err := st.db.QueryRowContext(
		ctx,
		`select id, name, pwd, role, used_space, quota, preference
		from t_user
		where name=?`,
		name,
	).Scan(
		&user.ID,
		&user.Name,
		&user.Pwd,
		&user.Role,
		&user.UsedSpace,
		&quotaStr,
		&preferenceStr,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrUserNotFound
		}
		return nil, err
	}

	err = json.Unmarshal([]byte(quotaStr), &user.Quota)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal([]byte(preferenceStr), &user.Preferences)
	if err != nil {
		return nil, err
	}
	return user, nil
}

func (st *SQLiteStore) SetPwd(ctx context.Context, id uint64, pwd string) error {
	st.Lock()
	defer st.Unlock()

	_, err := st.db.ExecContext(
		ctx,
		`update t_user
		set pwd=?
		where id=?`,
		pwd,
		id,
	)
	return err
}

// role + quota
func (st *SQLiteStore) SetInfo(ctx context.Context, id uint64, user *db.User) error {
	st.Lock()
	defer st.Unlock()

	quotaStr, err := json.Marshal(user.Quota)
	if err != nil {
		return err
	}

	_, err = st.db.ExecContext(
		ctx,
		`update t_user
		set role=?, quota=?
		where id=?`,
		user.Role, quotaStr,
		id,
	)
	return err
}

func (st *SQLiteStore) SetPreferences(ctx context.Context, id uint64, prefers *db.Preferences) error {
	st.Lock()
	defer st.Unlock()

	preferenceStr, err := json.Marshal(prefers)
	if err != nil {
		return err
	}

	_, err = st.db.ExecContext(
		ctx,
		`update t_user
		set preference=?
		where id=?`,
		preferenceStr,
		id,
	)
	return err
}

func (st *SQLiteStore) SetUsed(ctx context.Context, id uint64, incr bool, capacity int64) error {
	st.Lock()
	defer st.Unlock()
	return st.setUsed(ctx, id, incr, capacity)
}

func (st *SQLiteStore) setUsed(ctx context.Context, id uint64, incr bool, capacity int64) error {
	gotUser, err := st.getUser(ctx, id)
	if err != nil {
		return err
	}

	if incr && gotUser.UsedSpace+capacity > int64(gotUser.Quota.SpaceLimit) {
		return db.ErrReachedLimit
	}

	if incr {
		gotUser.UsedSpace = gotUser.UsedSpace + capacity
	} else {
		if gotUser.UsedSpace-capacity < 0 {
			return db.ErrNegtiveUsedSpace
		}
		gotUser.UsedSpace = gotUser.UsedSpace - capacity
	}

	_, err = st.db.ExecContext(
		ctx,
		`update t_user
		set used_space=?
		where id=?`,
		gotUser.UsedSpace,
		gotUser.ID,
	)
	if err != nil {
		return err
	}

	return nil
}

func (st *SQLiteStore) ResetUsed(ctx context.Context, id uint64, used int64) error {
	st.Lock()
	defer st.Unlock()

	_, err := st.db.ExecContext(
		ctx,
		`update t_user
		set used_space=?
		where id=?`,
		used,
		id,
	)
	return err
}

func (st *SQLiteStore) ListUsers(ctx context.Context) ([]*db.User, error) {
	st.RLock()
	defer st.RUnlock()

	// TODO: support pagination
	rows, err := st.db.QueryContext(
		ctx,
		`select id, name, role, used_space, quota, preference
		from t_user`,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, db.ErrUserNotFound
		}
		return nil, err
	}
	defer rows.Close() // TODO: check error

	users := []*db.User{}
	for rows.Next() {
		user := &db.User{}
		var quotaStr, preferenceStr string
		err = rows.Scan(
			&user.ID,
			&user.Name,
			&user.Role,
			&user.UsedSpace,
			&quotaStr,
			&preferenceStr,
		)
		err = json.Unmarshal([]byte(quotaStr), &user.Quota)
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal([]byte(preferenceStr), &user.Preferences)
		if err != nil {
			return nil, err
		}

		users = append(users, user)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	return users, nil
}

func (st *SQLiteStore) ListUserIDs(ctx context.Context) (map[string]string, error) {
	st.RLock()
	defer st.RUnlock()

	users, err := st.ListUsers(ctx)
	if err != nil {
		return nil, err
	}

	nameToId := map[string]string{}
	for _, user := range users {
		nameToId[user.Name] = fmt.Sprint(user.ID)
	}
	return nameToId, nil
}

func (st *SQLiteStore) AddRole(role string) error {
	// TODO: implement this after adding grant/revoke
	panic("not implemented")
}

func (st *SQLiteStore) DelRole(role string) error {
	// TODO: implement this after adding grant/revoke
	panic("not implemented")
}

func (st *SQLiteStore) ListRoles() (map[string]bool, error) {
	// TODO: implement this after adding grant/revoke
	panic("not implemented")
}
