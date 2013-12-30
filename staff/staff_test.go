package staff

import (
	"reflect"
	"sort"
	"testing"

	"appengine/aetest"

	"github.com/decitrig/innerhearth/auth"
)

func TestNewStaff(t *testing.T) {
	account := &auth.UserAccount{
		AccountID: "0xdeadbeef",
		UserInfo: auth.UserInfo{
			FirstName: "First",
			LastName:  "last",
			Email:     "foo@bar.com",
		},
	}
	expected := &Staff{
		AccountID: account.AccountID,
		UserInfo:  account.UserInfo,
	}
	if staff := NewStaff(account); !reflect.DeepEqual(expected, staff) {
		t.Errorf("Wrong staff created; %v vs %v", staff, expected)
	}
}

func usersToStaff(users []*auth.UserAccount) []*Staff {
	staff := make([]*Staff, len(users))
	for i, user := range users {
		staff[i] = NewStaff(user)
	}
	return staff
}

func TestStaff(t *testing.T) {
	users := []*auth.UserAccount{{
		AccountID: "0x1",
		UserInfo: auth.UserInfo{
			FirstName: "a",
			LastName:  "b",
			Email:     "a@example.com",
		}}, {
		AccountID: "0x2",
		UserInfo: auth.UserInfo{
			FirstName: "aa",
			LastName:  "bb",
			Email:     "aa@example.com",
		}}, {
		AccountID: "0x3",
		UserInfo: auth.UserInfo{
			FirstName: "aaa",
			LastName:  "bbb",
			Email:     "aaa@example.com",
		}}}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i, user := range users {
		if _, err := LookupStaff(c, user); err == nil {
			t.Errorf("Shouldn't have found staff for user %d", i)
		}
		staff := NewStaff(user)
		if err := staff.Store(c); err != nil {
			t.Fatalf("Failed to store staff %d: %s", i, err)
		}
		if found, err := LookupStaff(c, user); err != nil {
			t.Errorf("Didn't find staff %d: %s", i, err)
		} else if !reflect.DeepEqual(staff, found) {
			t.Errorf("Found wrong staff; %v vs %v", found, staff)
		}
		if found, err := LookupStaffByID(c, user.AccountID); err != nil {
			t.Errorf("Didn't find staff %d by ID: %s", i, err)
		} else if !reflect.DeepEqual(staff, found) {
			t.Errorf("Found wrong staff %d by ID: %v vs %v", found, staff)
		}
	}
	expected := usersToStaff(users)
	allStaff, err := AllStaff(c)
	if err != nil {
		t.Fatalf("Error reading all staff: %s", err)
	}
	if got, want := len(allStaff), len(expected); got != want {
		t.Fatalf("Wrong number of staff; %d vs %d", got, want)
	}
	sort.Sort(StaffByName(expected))
	sort.Sort(StaffByName(allStaff))
	for i, want := range expected {
		if got := allStaff[i]; !reflect.DeepEqual(got, want) {
			t.Errorf("Wrong staff at %d; %v vs %v", i, got, want)
		}
	}
}
