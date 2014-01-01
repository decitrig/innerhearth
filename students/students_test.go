package students

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"appengine"
	"appengine/aetest"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/classes"
)

func TestStudents(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	accounts := []*account.Account{
		makeAccount(1, "a"),
		makeAccount(2, "b"),
		makeAccount(3, "c"),
	}
	classList := []*classes.Class{{}}
	for _, class := range classList {
		putClass(c, class)
	}
	rosters := []struct {
		class    *classes.Class
		accounts []int
	}{{
		class:    class(1, "class1", 1),
		accounts: []int{0},
	}, {
		class:    class(2, "class2", 5),
		accounts: []int{0, 1},
	}}
	for _, r := range rosters {
		putClass(c, r.class)
		for _, acct := range r.accounts {
			student := New(accounts[acct], r.class)
			if err := student.Add(c, time.Now()); err != nil {
				t.Fatalf("Failed to add student %s to class %d: %s", accounts[acct].ID, r.class.ID, err)
			}
			if got, err := WithIDInClass(c, student.ID, r.class, time.Now()); err != nil {
				t.Fatalf("Didn't find student %s in class %d: %s", student.ID, r.class.ID, err)
			} else if !studentsEqual(got, student) {
				t.Errorf("Wrong student; %v vs %v", got, student)
			}
		}
	}
	student := New(accounts[2], rosters[0].class)
	if err := student.Add(c, time.Now()); err != ErrClassIsFull {
		t.Errorf("Should have gotten class full error")
	}
	want := []*Student{
		New(accounts[0], rosters[0].class),
		New(accounts[0], rosters[1].class),
	}
	if got := WithID(c, accounts[0].ID); len(got) != len(want) {
		t.Errorf("Wrong number of students with id %s: %d vs %d", accounts[0].ID, len(got), len(want))
	}
	if got := WithEmail(c, accounts[0].Email); len(got) != len(want) {
		t.Errorf("Wrong number of students with email %s: %d vs %d", accounts[0].Email, len(got), len(want))
	}

	want = []*Student{
		New(accounts[0], rosters[1].class),
		New(accounts[1], rosters[1].class),
	}
	got, err := In(c, rosters[1].class, time.Now())
	if err != nil {
		t.Fatalf("Failed to get students in %d: %s", rosters[1].class.ID, err)
	}
	if len(got) != len(want) {
		t.Fatalf("Wrong number of students in %d: %d vs %d", rosters[1].class.ID, len(got), len(want))
	}
	sort.Sort(ByName(got))
	sort.Sort(ByName(want))
	for i, want := range want {
		if got := got[i]; !studentsEqual(got, want) {
			t.Errorf("Wrong student at %d: %v vs %v", i, got, want)
		}
	}
	dropIn := NewDropIn(accounts[2], rosters[1].class, time.Unix(1000, 0))
	if err := dropIn.Add(c, time.Unix(1000, 0)); err != nil {
		t.Fatalf("Error adding drop in: %s", err)
	}
	if _, err := WithIDInClass(c, dropIn.ID, rosters[1].class, time.Unix(1500, 0)); err == nil {
		t.Errorf("Shouldn't have found expired dropin")
	}
	got, err = In(c, rosters[1].class, time.Unix(500, 0))
	if err != nil {
		t.Fatalf("Failed to get students in %d: %s", rosters[1].class.ID, err)
	}
	if len(got) != 3 {
		t.Fatalf("Wrong number of students in %d: %d vs 3", rosters[1].class.ID, len(got))
	}
	got, err = In(c, rosters[1].class, time.Unix(1500, 0))
	if err != nil {
		t.Fatalf("Failed to get students in %d: %s", rosters[1].class.ID, err)
	}
	if len(got) != 2 {
		t.Fatalf("Wrong number of students in %d: %d vs 2", rosters[1].class.ID, len(got))
	}
}

func putClass(c appengine.Context, cls *classes.Class) {
	key := classes.NewClassKey(c, cls.ID)
	if _, err := datastore.Put(c, key, cls); err != nil {
		panic(err)
	}
}

func makeAccount(id int, name string) *account.Account {
	return &account.Account{
		ID:   fmt.Sprintf("%d", id),
		Info: account.Info{name, name, fmt.Sprintf("%s@%s.com", name, name), ""},
	}
}

func class(id int64, title string, capacity int32) *classes.Class {
	return &classes.Class{ID: id, Title: title, Capacity: capacity}
}

func studentsEqual(a, b *Student) bool {
	switch {
	case a == nil || b == nil:
		return a == b
	case a.ID != b.ID:
		return false
	case !reflect.DeepEqual(a.Info, b.Info):
		return false
	case a.ClassID != b.ClassID:
		return false
	case a.ClassType != b.ClassType:
		return false
	case !a.Date.Equal(b.Date):
		return false
	case a.DropIn != b.DropIn:
		return false
	}
	return true
}
