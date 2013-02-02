/*
 *  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
package model

import (
	"errors"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"
)

var (
	ErrEmailInUse = errors.New("The email is already in use")
)

type ClaimedEmail struct {
	ClaimedBy *datastore.Key
}

func checkClaimAvailable(c appengine.Context, email string) error {
	key := datastore.NewKey(c, "ClaimedEmail", email, 0, nil)
	claim := &ClaimedEmail{}
	if err := datastore.Get(c, key, claim); err != datastore.ErrNoSuchEntity {
		if err != nil {
			return fmt.Errorf("Error looking up claim on %s: %s", email, err)
		}
		return ErrEmailInUse
	}
	return nil
}

func ClaimEmail(c appengine.Context, accountID, email string) error {
	if err := checkClaimAvailable(c, email); err != nil {
		return err
	}
	err := datastore.RunInTransaction(c, func(ctx appengine.Context) error {
		accountKey := datastore.NewKey(ctx, "UserAccount", accountID, 0, nil)
		if err := checkClaimAvailable(ctx, email); err != nil {
			return err
		}
		claim := &ClaimedEmail{accountKey}
		key := datastore.NewKey(ctx, "ClaimedEmail", email, 0, nil)
		if _, err := datastore.Put(ctx, key, claim); err != nil {
			return err
		}
		return nil
	}, nil)
	return err
}

type UserRole string

const (
	RoleStudent = ""
	RoleTeacher = "TEACHER"
	RoleStaff   = "STAFF"
	RoleAdmin   = "ADMIN"
)

func (r UserRole) CanTeach() bool {
	return r.IsStaff() || r == RoleTeacher
}

func (r UserRole) IsStaff() bool {
	return r == RoleStaff || r == RoleAdmin
}

func ParseRole(r string) UserRole {
	switch r {
	case "TEACHER":
		return RoleTeacher
	case "STAFF":
		return RoleStaff
	case "ADMIN":
		return RoleAdmin
	}
	return RoleStudent
}

type UserAccount struct {
	AccountID string `datastore: "-"`

	FirstName string `datastore: ",noindex"`
	LastName  string
	Email     string
	Phone     string

	ConfirmationCode string    `datastore: ",noindex"`
	Confirmed        time.Time `datastore: ",noindex"`

	Role     UserRole
	CanTeach bool
}

func (a *UserAccount) SetRole(role UserRole) {
	a.Role = role
	a.CanTeach = role.CanTeach()
}

func (a *UserAccount) key(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, "UserAccount", a.AccountID, 0, nil)
}

func HasAccount(c appengine.Context, u *user.User) bool {
	_, err := GetAccount(c, u)
	return err == nil
}

func GetCurrentUserAccount(c appengine.Context) (*UserAccount, error) {
	u := user.Current(c)
	if u == nil {
		return nil, fmt.Errorf("No logged-in user")
	}
	return GetAccount(c, u)
}

func GetAccount(c appengine.Context, u *user.User) (*UserAccount, error) {
	return GetAccountByID(c, u.ID)
}

func GetAccountByID(c appengine.Context, id string) (*UserAccount, error) {
	key := datastore.NewKey(c, "UserAccount", id, 0, nil)
	account := &UserAccount{}
	if err := datastore.Get(c, key, account); err != nil {
		return nil, fmt.Errorf("Error looking up account: %s", err)
	}
	account.AccountID = id
	if user.IsAdmin(c) {
		account.Role = RoleAdmin
	}
	return account, nil
}

func MaybeGetCurrentUser(c appengine.Context) *UserAccount {
	u := user.Current(c)
	if u == nil {
		return nil
	}
	key := datastore.NewKey(c, "UserAccount", u.ID, 0, nil)
	account := &UserAccount{}
	if err := datastore.Get(c, key, account); err != nil {
		return nil
	}
	account.AccountID = key.StringID()
	if user.IsAdmin(c) {
		account.SetRole(RoleAdmin)
	}
	return account
}

func GetAccountByEmail(c appengine.Context, email string) *UserAccount {
	q := datastore.NewQuery("UserAccount").
		Filter("Email =", email).
		Limit(2)
	accounts := []*UserAccount{}
	keys, err := q.GetAll(c, &accounts)
	if err != nil {
		c.Errorf("Error looking up user %s", email)
		return nil
	}
	if len(accounts) > 1 {
		c.Criticalf("More than 1 account for email %s: %v", accounts)
		return nil
	}
	if len(accounts) == 0 {
		return nil
	}
	account := accounts[0]
	account.AccountID = keys[0].StringID()
	return account
}

func ListRoleAccounts(c appengine.Context, role UserRole) []*UserAccount {
	q := datastore.NewQuery("UserAccount").
		Filter("Role =", role)
	accounts := []*UserAccount{}
	_, err := q.GetAll(c, &accounts)
	if err != nil {
		c.Errorf("Error getting %s accounts: %s", role, err)
		return nil
	}
	return accounts
}

func ListTeachers(c appengine.Context) []*UserAccount {
	q := datastore.NewQuery("UserAccount").
		Filter("CanTeach =", true)
	accounts := []*UserAccount{}
	_, err := q.GetAll(c, &accounts)
	if err != nil {
		c.Errorf("Error listing teachers: %s", err)
		return nil
	}
	return accounts
}

func StoreAccount(c appengine.Context, u *user.User, account *UserAccount) error {
	var id string
	if u == nil {
		id = account.AccountID
	} else {
		id = u.ID
	}
	key := datastore.NewKey(c, "UserAccount", id, 0, nil)
	if _, err := datastore.Put(c, key, account); err != nil {
		return fmt.Errorf("Error storing account: %s", err)
	}
	return nil
}

func ConfirmAccount(c appengine.Context, code string, account *UserAccount) error {
	if code != account.ConfirmationCode {
		return fmt.Errorf("Incorrect code")
	}
	if !account.Confirmed.IsZero() {
		return nil
	}
	account.Confirmed = time.Now()
	key := datastore.NewKey(c, "UserAccount", account.AccountID, 0, nil)
	if _, err := datastore.Put(c, key, account); err != nil {
		return fmt.Errorf("Error storing account: %s", err)
	}
	return nil
}
