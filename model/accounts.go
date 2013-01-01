package model

import (
	"fmt"

	"appengine"
	"appengine/datastore"
	"appengine/user"
)

type UserAccount struct {
	AccountID string `datastore: "-"`
	FirstName string `datastore: ",noindex"`
	LastName  string
	Email     string
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
	return account, nil
}

func StoreAccount(c appengine.Context, u *user.User, account *UserAccount) error {
	key := datastore.NewKey(c, "UserAccount", u.ID, 0, nil)
	if _, err := datastore.Put(c, key, account); err != nil {
		return fmt.Errorf("Error storing account: %s", err)
	}
	return nil
}
