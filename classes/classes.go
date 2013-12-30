package classes

import (
	"fmt"
	"sort"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassNotFound   = fmt.Errorf("classes: class not found")
	ErrSessionNotFound = fmt.Errorf("classes: session not found")
)

// A Session is a contiguous block of time which contains classes.
type Session struct {
	ID int64 `datastore: "-"`

	Name  string
	Start time.Time
	End   time.Time
}

// NewSession creates a new Session with the given name and start and
// end times.
func NewSession(name string, start, end time.Time) *Session {
	return &Session{Name: name, Start: start, End: end}
}

func sessionKeyFromID(c appengine.Context, id int64) *datastore.Key {
	return datastore.NewKey(c, "Session", "", id, nil)
}

// SessionWithID returns the Session entity with the given ID, if one exists.
func SessionWithID(c appengine.Context, id int64) (*Session, error) {
	key := sessionKeyFromID(c, id)
	session := &Session{}
	switch err := datastore.Get(c, key, session); err {
	case nil:
		session.ID = id
		return session, nil
	case datastore.ErrNoSuchEntity:
		return nil, ErrSessionNotFound
	default:
		return nil, err
	}
}

// ActiveSessions returns a list of all sessions whose end time is not in the past.
func ActiveSessions(c appengine.Context, now time.Time) ([]*Session, error) {
	q := datastore.NewQuery("Session").
		Filter("End >=", now)
	sessions := []*Session{}
	keys, err := q.GetAll(c, &sessions)
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		sessions[i].ID = key.IntID()
	}
	return sessions, nil
}

// Classes returns a list of all the classes within the session.
func (s *Session) Classes(c appengine.Context) ([]*Class, error) {
	q := datastore.NewQuery("Class").
		Filter("Session =", s.ID)
	classes := []*Class{}
	keys, err := q.GetAll(c, &classes)
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		classes[i].ID = key.IntID()
	}
	return classes, nil
}

func (s *Session) NewIncompleteKey(c appengine.Context) *datastore.Key {
	return datastore.NewIncompleteKey(c, "Session", nil)
}

// SessionsByStartDate sorts sessions by their start date.
type SessionsByStartDate []*Session

func (l SessionsByStartDate) Len() int      { return len(l) }
func (l SessionsByStartDate) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l SessionsByStartDate) Less(i, j int) bool {
	return l[i].Start.Before(l[j].Start)
}

type Type int

const (
	Regular Type = iota
	Workshop
	YinYogassage
)

// A Class is a yoga class for which students can register.
type Class struct {
	ID int64 `datastore: "-"`

	Title           string `datastore: ",noindex"`
	LongDescription []byte `datastore: ",noindex"`
	Teacher         *datastore.Key

	Weekday   time.Weekday
	StartTime time.Time     `datastore: ",noindex"`
	Length    time.Duration `datastore: ",noindex"`
	Session   int64

	DropInOnly bool
	Capacity   int32 `datastore: ",noindex"`
}

func classKeyFromID(c appengine.Context, id int64) *datastore.Key {
	return datastore.NewKey(c, "Class", "", id, nil)
}

// ClassWithID returns the class with the given ID, if one exists.
func ClassWithID(c appengine.Context, id int64) (*Class, error) {
	class := &Class{}
	switch err := datastore.Get(c, classKeyFromID(c, id), class); err {
	case nil:
		class.ID = id
		return class, nil
	case datastore.ErrNoSuchEntity:
		return nil, ErrClassNotFound
	default:
		return nil, err
	}
}

func (cls *Class) NewIncompleteKey(c appengine.Context) *datastore.Key {
	return datastore.NewIncompleteKey(c, "Class", nil)
}

func NewClassKey(c appengine.Context, id int64) *datastore.Key {
	return datastore.NewKey(c, "Class", "", id, nil)
}

func (cls *Class) Key(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, "Class", "", cls.ID, nil)
}

func (cls *Class) Update(c appengine.Context) error {
	if _, err := datastore.Put(c, cls.Key(c), cls); err != nil {
		return err
	}
	return nil
}

// ClassesByTitle returns a sort.Interface which sorts classes
// alphabetically by title.
func ClassesByTitle(cs []*Class) sort.Interface {
	return classesByTitle{classes(cs)}
}

// ClassesByStartTime returns a sort.Interface which sorts classes by
// their start times, earliest first.
func ClassesByStartTime(cs []*Class) sort.Interface {
	return classesByStartTime{classes(cs)}
}

type classes []*Class

func (cs classes) Len() int      { return len(cs) }
func (cs classes) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }

type classesByTitle struct {
	classes
}

func (cs classesByTitle) Less(i, j int) bool {
	return cs.classes[i].Title < cs.classes[j].Title
}

type classesByStartTime struct {
	classes
}

func (cs classesByStartTime) Less(i, j int) bool {
	return cs.classes[i].StartTime.Before(cs.classes[j].StartTime)
}
