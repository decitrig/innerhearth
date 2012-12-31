package model

import (
	"fmt"

	"appengine"
	"appengine/datastore"
)

type UserAccount struct {
	ID        string `datastore: "-"`
	FirstName string `datastore: ",noindex"`
	LastName  string
	Email     string
	Fresh     bool `datastore: ",noindex"`
}

func GetOrCreateAccount(c appengine.Context, id string) (*UserAccount, error) {
	key := datastore.NewKey(c, "UserAccount", id, 0, nil)
	account := &UserAccount{
		ID: id,
	}
	err := datastore.RunInTransaction(c, func(c appengine.Context) error {
		if err := datastore.Get(c, key, account); err != nil {
			if err != datastore.ErrNoSuchEntity {
				return fmt.Errorf("Error looking up account %s: %s", id, err)
			}
			account.Fresh = true
			if _, err := datastore.Put(c, key, account); err != nil {
				return fmt.Errorf("Error writing account %s: %s", id, err)
			}
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return account, nil
}

func GetAccount(c appengine.Context, id string) (*UserAccount, error) {
	key := datastore.NewKey(c, "UserAccount", id, 0, nil)
	account := &UserAccount{
		ID: id,
	}
	if err := datastore.Get(c, key, account); err != nil {
		return nil, fmt.Errorf("Error getting account %s: %s", id, err)
	}
	return account, nil
}
