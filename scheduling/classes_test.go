package scheduling

import (
	"sort"
	"testing"
	"time"

	"appengine/aetest"
)

var week = 7 * 24 * time.Hour

func sessionsEqual(s1, s2 *Session) bool {
	switch {
	case s1.ID != s2.ID:
		return false
	case s1.Name != s2.Name:
		return false
	case !s1.Start.Equal(s2.Start):
		return false
	case !s1.End.Equal(s2.End):
		return false
	}
	return true
}

func TestSessions(t *testing.T) {
	sessions := []*Session{
		NewSession("foo", unix(1000), unix(10000)),
		NewSession("bar", unix(1500), unix(10000)),
		NewSession("baz", unix(2000), unix(10000)),
		NewSession("bat", unix(100), unix(5000)),
	}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i, s := range sessions {
		if err := s.Insert(c); err != nil {
			t.Fatalf("Failed to store session %d: %s", i, err)
		}
		if s.ID <= 0 {
			t.Fatalf("Session %d got invalid ID %d", i, s.ID)
		}
		if got, err := SessionByID(c, s.ID); err != nil {
			t.Errorf("Failed to lookup session: %s", err)
		} else if !sessionsEqual(got, s) {
			t.Errorf("Found wrong session for %d: %v vs %v", got, s)
		}
	}
	expected := sessions[:len(sessions)-1]
	got, err := ActiveSessions(c, unix(6000))
	if err != nil {
		t.Fatalf("Failed to list active sessions: %s", err)
	}
	if len(got) != len(expected) {
		t.Fatalf("Wrong number of active sessions, %d vs %d", len(got), len(expected))
	}
	sort.Sort(SessionsByStartDate(expected))
	sort.Sort(SessionsByStartDate(got))
	for i, want := range expected {
		if !sessionsEqual(got[i], want) {
			t.Errorf("Wrong session at %d; %v vs %v", i, got[i], want)
		}
	}
}

func TestSessionClasses(t *testing.T) {
	session1 := &Session{1, "session1", unix(10), unix(100)}
	session2 := &Session{2, "session2", unix(10), unix(100)}
	classes := []*Class{{
		Title:   "class1",
		Weekday: time.Monday,
		Session: session1.ID,
	}, {
		Title:   "class2",
		Weekday: time.Tuesday,
		Session: session1.ID,
	}, {
		Title:   "class3",
		Weekday: time.Wednesday,
		Session: session2.ID,
	}}
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for i, class := range classes {
		if err := class.Insert(c); err != nil {
			t.Fatalf("Failed to store class %d: %s", i, err)
		}
		if class.ID <= 0 {
			t.Fatalf("Invalid id given to class %d: %d", i, class.ID)
		}
		if got, err := ClassByID(c, class.ID); err != nil {
			t.Fatalf("Couldn't find class %d by %d: %s", i, class.ID, err)
		} else if got.Title != class.Title {
			t.Errorf("Wrong class %d found for %d: %v vs %v", i, class.ID, got, class)
		}
	}
	expected := classes[0:2]
	sClasses, err := session1.Classes(c)
	if err != nil {
		t.Fatalf("Failed to look up session1 classes")
	}
	if len(sClasses) != len(expected) {
		t.Fatalf("Wrong number of classes for session 1: %d vs %d", len(sClasses), len(expected))
	}
	sort.Sort(ClassesByTitle{sClasses})
	sort.Sort(ClassesByTitle{expected})
	for i, want := range expected {
		if got := sClasses[i]; got.Title != want.Title {
			t.Errorf("Wrong class at %d: %v vs %v", got, want)
		}
	}
	class := classes[0]
	class.Title = "new title"
	if err := class.Update(c); err != nil {
		t.Fatal(err)
	}
	if got, err := ClassByID(c, class.ID); err != nil {
		t.Errorf("Failed to get updated class %d: %s", class.ID, err)
	} else if got.Title != class.Title {
		t.Errorf("Didn't get expected class %d; %v vs %v", class.ID, got, class)
	}
}
