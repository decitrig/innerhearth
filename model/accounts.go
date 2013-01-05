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

type UserAccount struct {
	AccountID        string `datastore: "-"`
	FirstName        string `datastore: ",noindex"`
	LastName         string
	Email            string
	ConfirmationCode string    `datastore: ",noindex"`
	Confirmed        time.Time `datastore: ",noindex"`
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
