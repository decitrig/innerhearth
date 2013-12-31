package account

import (
	"reflect"
	"testing"
	"time"

	"appengine/aetest"
	"appengine/datastore"
	"appengine/user"
)

func TestNewUserAccount(t *testing.T) {
	info := Info{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	account, err := New(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	if account.ID == "" {
		t.Errorf("Account id not populated.")
	}
	if account.ID == u.FederatedIdentity {
		t.Errorf("Account ID %q should not match %q", account.ID, u.FederatedIdentity)
	}
	if !account.Confirmed.IsZero() {
		t.Errorf("Confirmation time is not zero: %s", account.Confirmed)
	}
	if len(account.ConfirmationCode) == 0 {
		t.Errorf("Confirmation code missing: %v", account)
	}
}

func usersEqual(u, v *Account) bool {
	switch {
	case u == nil || v == nil:
		return u == v
	case u.ID != v.ID:
		return false
	case !reflect.DeepEqual(u.Info, v.Info):
		return false
	case u.ConfirmationCode != v.ConfirmationCode:
		return false
	case !u.Confirmed.Equal(v.Confirmed):
		return false
	}
	return true
}

func TestStoreAndLookup(t *testing.T) {
	info := Info{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	account, err := New(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := account.Put(c); err != nil {
		t.Fatalf("Failed to store user: %s", err)
	}
	found, err := ForUser(c, u)
	if err != nil {
		t.Fatalf("Failed to find user for %v: %s", u, err)
	}
	if !usersEqual(account, found) {
		t.Errorf("Found wrong user; %v vs %v", found, account)
	}
	found, err = WithID(c, account.ID)
	if err != nil {
		t.Fatalf("Failed to find user for id %v: %s", account.ID, err)
	}
	if !usersEqual(account, found) {
		t.Errorf("Found wrong user for %q; %v vs %v", account.ID, found, account)
	}
	found, err = WithEmail(c, account.Email)
	if err != nil {
		t.Fatalf("Failed to find user for email %q: %s", account.Email, err)
	}
	if !usersEqual(account, found) {
		t.Errorf("Found wrong user for %q; %v vs %v", account.Email, found, account)
	}
}

func TestConvertOldUser(t *testing.T) {
	info := Info{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		ID:                "fooID",
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	account, err := New(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	old := &Account{}
	*old = *account
	old.ID = u.ID
	oldKey := datastore.NewKey(c, "UserAccount", u.ID, 0, nil)
	if _, err := datastore.Put(c, oldKey, old); err != nil {
		t.Fatalf("Failed to store user under old key %q: %s", oldKey.StringID(), err)
	}
	if _, err := ForUser(c, u); err != ErrUserNotFound {
		t.Errorf("Should not have found user under new key")
	} else if err != ErrUserNotFound {
		t.Fatalf("Error looking up user: %s", err)
	}
	if got, err := OldAccountForUser(c, u); err != nil {
		t.Fatalf("Error looking up user: %s", err)
	} else if !usersEqual(got, old) {
		t.Errorf("Wrong old user found: %v vs %v", got, account)
	}
	if err := old.RewriteID(c, u); err != nil {
		t.Fatalf("Failed to rewrite id: %s", err)
	}
	expected := &Account{}
	*expected = *account
	if got, err := ForUser(c, u); err != nil {
		t.Fatalf("Failed to find new user: %s", err)
	} else if !usersEqual(got, expected) {
		t.Errorf("Wrong user found; %v vs %v", got, expected)
	}
	if _, err := OldAccountForUser(c, u); err != ErrUserNotFound {
		t.Errorf("Should have deleted old user.")
	}
}

func TestConfirmation(t *testing.T) {
	info := Info{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	account, err := New(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := account.Put(c); err != nil {
		t.Fatalf("Failed to store user: %s", err)
	}
	if found, _ := WithID(c, account.ID); !found.Confirmed.IsZero() {
		t.Errorf("Unconfirmed user should not have confirmation time %s", found.Confirmed)
	}
	now := time.Unix(1234, 0)
	if err := account.Confirm(c, "wrongcode", now); err != ErrWrongConfirmationCode {
		if err == nil {
			t.Error("Should have failed to confirm")
		} else {
			t.Errorf("Wrong error code: %s vs %s", err, ErrWrongConfirmationCode)
		}
	}
	if err := account.Confirm(c, account.ConfirmationCode, now); err != nil {
		t.Fatalf("Failed to confirm: %s", err)
	}
	found, _ := WithID(c, account.ID)
	if confirmed := found.Confirmed; !confirmed.Equal(now) {
		t.Errorf("Wrong confirmation time; %s vs %s", confirmed, now)
	}
	if code := found.ConfirmationCode; code != "" {
		t.Errorf("Didn't clear confirmation code: %s", code)
	}
}

func TestClaimedEmail(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	claim := NewClaimedEmail(c, "0xdeadbeef", "test@example.com")
	if err := claim.Claim(c); err != nil {
		t.Fatalf("Failed to claim user email %q: %s", claim.Email, err)
	}
	if err := claim.Claim(c); err != ErrEmailAlreadyClaimed {
		t.Errorf("Expected error on claim; %q vs %q", err, ErrEmailAlreadyClaimed)
	}
}
