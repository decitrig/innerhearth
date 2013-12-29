package scheduling

import (
	"time"

	"appengine"
	"appengine/datastore"
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

// SessionByID returns the Session entity with the given ID, if one exists.
func SessionByID(c appengine.Context, id int64) (*Session, error) {
	key := sessionKeyFromID(c, id)
	session := &Session{}
	if err := datastore.Get(c, key, session); err != nil {
		return nil, err
	}
	session.ID = id
	return session, nil
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

// SessionsByStartDate sorts sessions by their start date.
type SessionsByStartDate []*Session

func (l SessionsByStartDate) Len() int      { return len(l) }
func (l SessionsByStartDate) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l SessionsByStartDate) Less(i, j int) bool {
	return l[i].Start.Before(l[j].Start)
}

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

func ClassByID(c appengine.Context, id int64) (*Class, error) {
	class := &Class{}
	if err := datastore.Get(c, classKeyFromID(c, id), class); err != nil {
		return nil, err
	}
	class.ID = id
	return class, nil
}

// Classes wraps a list of Class entities for sorting.
type Classes []*Class

func (cs Classes) Len() int      { return len(cs) }
func (cs Classes) Swap(i, j int) { cs[i], cs[j] = cs[j], cs[i] }

// ClassesByTitle sorts classes in alphabetical order by title.
type ClassesByTitle struct {
	Classes
}

func (cs ClassesByTitle) Less(i, j int) bool {
	return cs.Classes[i].Title < cs.Classes[j].Title
}

// ClassesByStartTime sorts classes by start time, earliest first.
type ClassesByStartTime struct {
	Classes
}

func (cs ClassesByStartTime) Less(i, j int) bool {
	return cs.Classes[i].StartTime.Before(cs.Classes[j].StartTime)
}
