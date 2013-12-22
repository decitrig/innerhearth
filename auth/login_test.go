package auth

import (
	"reflect"
	"testing"
	"time"

	"appengine/aetest"
	"appengine/user"
)

func TestNewInnerHearthUser(t *testing.T) {
	info := UserInfo{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	ihu, err := NewInnerHearthUser(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	if ihu.AccountID == "" {
		t.Errorf("Account id not populated.")
	}
	if ihu.AccountID == u.FederatedIdentity {
		t.Errorf("Account ID %q should not match %q", ihu.AccountID, u.FederatedIdentity)
	}
	if !ihu.Confirmed.IsZero() {
		t.Errorf("Confirmation time is not zero: %s", ihu.Confirmed)
	}
	if len(ihu.ConfirmationCode) == 0 {
		t.Errorf("Confirmation code missing: %v", ihu)
	}
}

func usersEqual(u, v *InnerHearthUser) bool {
	if u.AccountID != v.AccountID {
		return false
	}
	if !reflect.DeepEqual(u.UserInfo, v.UserInfo) {
		return false
	}
	if u.ConfirmationCode != v.ConfirmationCode {
		return false
	}
	if !u.Confirmed.Equal(v.Confirmed) {
		return false
	}
	return true
}

func TestStoreAndLookup(t *testing.T) {
	info := UserInfo{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	ihu, err := NewInnerHearthUser(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := ihu.Store(c); err != nil {
		t.Fatalf("Failed to store user: %s", err)
	}
	found, err := LookupUser(c, u)
	if err != nil {
		t.Fatalf("Failed to find user for %v: %s", u, err)
	}
	if !usersEqual(ihu, found) {
		t.Errorf("Found wrong user; %v vs %v", found, ihu)
	}
	found, err = LookupUserByID(c, ihu.AccountID)
	if err != nil {
		t.Fatalf("Failed to find user for id %v: %s", ihu.AccountID, err)
	}
	if !usersEqual(ihu, found) {
		t.Errorf("Found wrong user for %q; %v vs %v", ihu.AccountID, found, ihu)
	}
	found, err = LookupUserByEmail(c, ihu.Email)
	if err != nil {
		t.Fatalf("Failed to find user for email %q: %s", ihu.Email, err)
	}
	if !usersEqual(ihu, found) {
		t.Errorf("Found wrong user for %q; %v vs %v", ihu.Email, found, ihu)
	}
}

func TestConfirmation(t *testing.T) {
	info := UserInfo{"First", "Last", "foo@foo.com", "5551212"}
	u := &user.User{
		Email:             info.Email,
		FederatedIdentity: "0xdeadbeef",
	}
	ihu, err := NewInnerHearthUser(u, info)
	if err != nil {
		t.Fatalf("Failed to create user: %s", err)
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := ihu.Store(c); err != nil {
		t.Fatalf("Failed to store user: %s", err)
	}
	if found, _ := LookupUserByID(c, ihu.AccountID); !found.Confirmed.IsZero() {
		t.Errorf("Unconfirmed user should not have confirmation time %s", found.Confirmed)
	}
	now := time.Unix(1234, 0)
	if err := ihu.Confirm(c, "wrongcode", now); err != ErrWrongConfirmationCode {
		if err == nil {
			t.Error("Should have failed to confirm")
		} else {
			t.Errorf("Wrong error code: %s vs %s", err, ErrWrongConfirmationCode)
		}
	}
	if err := ihu.Confirm(c, ihu.ConfirmationCode, now); err != nil {
		t.Fatalf("Failed to confirm: %s", err)
	}
	found, _ := LookupUserByID(c, ihu.AccountID)
	if confirmed := found.Confirmed; !confirmed.Equal(now) {
		t.Errorf("Wrong confirmation time; %s vs %s", confirmed, now)
	}
	if code := found.ConfirmationCode; code != "" {
		t.Errorf("Didn't clear confirmation code: %s", code)
	}
}

func TestUserEmail(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	u := &user.User{
		FederatedIdentity: "0xdeadbeef",
	}
	ue := NewUserEmail(c, u, "test@example.com")
	if err := ue.Claim(c); err != nil {
		t.Fatalf("Failed to claim user email %q: %s", ue.Email, err)
	}
	if err := ue.Claim(c); err != ErrEmailAlreadyClaimed {
		t.Errorf("Expected error on claim; %q vs %q", err, ErrEmailAlreadyClaimed)
	}
}
