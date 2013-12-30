package classes_test

import (
	"reflect"
	"sort"
	"testing"

	"appengine/aetest"

	"github.com/decitrig/innerhearth/auth"
	. "github.com/decitrig/innerhearth/classes"
)

func TestTeacher(t *testing.T) {
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
		if _, err := LookupTeacher(c, user); err == nil {
			t.Errorf("Shouldn't have found teacher for user %d", i)
		}
		teacher := NewTeacher(user)
		if err := stafferSmith.PutTeacher(c, teacher); err != nil {
			t.Fatalf("Failed to store teacher %d: %s", i, err)
		}
		if found, err := LookupTeacher(c, user); err != nil {
			t.Errorf("Didn't find teacher %d: %s", i, err)
		} else if !reflect.DeepEqual(teacher, found) {
			t.Errorf("Found wrong teacher; %v vs %v", found, teacher)
		}
		if found, err := LookupTeacherByID(c, user.AccountID); err != nil {
			t.Errorf("Didn't find techer %d: %s", i, err)
		} else if !reflect.DeepEqual(teacher, found) {
			t.Errorf("Found wrong teacher by id; %v vs %v", found, teacher)
		}
	}
	expected := usersToTeachers(users)
	allTeachers := AllTeachers(c)
	if len(allTeachers) != len(expected) {
		t.Fatalf("Wrong number of teachers returned; %d vs %d", len(allTeachers), len(expected))
	}
	sort.Sort(TeachersByName(expected))
	sort.Sort(TeachersByName(allTeachers))
	for i, want := range expected {
		if got := allTeachers[i]; !reflect.DeepEqual(got, want) {
			t.Errorf("Wrong teacher at %d; %v vs %v", i, got, want)
		}
	}
}

func usersToTeachers(users []*auth.UserAccount) []*Teacher {
	teachers := make([]*Teacher, len(users))
	for i, user := range users {
		teachers[i] = NewTeacher(user)
	}
	return teachers
}
