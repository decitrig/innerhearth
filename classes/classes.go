package classes

import (
	"fmt"
	"sort"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrUserIsNotTeacher = fmt.Errorf("classes: user is not a teacher")
	ErrClassNotFound    = fmt.Errorf("classes: class not found")
	ErrSessionNotFound  = fmt.Errorf("classes: session not found")
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

// Sessions returns a list of all sessions whose end time is not in the past.
func Sessions(c appengine.Context, now time.Time) []*Session {
	q := datastore.NewQuery("Session").
		Filter("End >=", now)
	sessions := []*Session{}
	keys, err := q.GetAll(c, &sessions)
	if err != nil {
		c.Errorf("Failed to list sessions: %s", err)
		return nil
	}
	for i, key := range keys {
		sessions[i].ID = key.IntID()
	}
	return sessions
}

// Insert writes a new Session to the datastore. It will not overwrite
// any existing Sessions.
func (s *Session) Insert(c appengine.Context) error {
	iKey := datastore.NewIncompleteKey(c, "Session", nil)
	key, err := datastore.Put(c, iKey, s)
	if err != nil {
		return err
	}
	s.ID = key.IntID()
	return nil
}

// Classes returns a list of all the classes within the session.
func (s *Session) Classes(c appengine.Context) []*Class {
	q := datastore.NewQuery("Class").
		Filter("Session =", s.ID)
	classes := []*Class{}
	keys, err := q.GetAll(c, &classes)
	if err != nil && !isFieldMismatch(err) {
		c.Errorf("Failed to get classes for session %d: %s", s.ID, err)
		return nil
	}
	for i, key := range keys {
		classes[i].ID = key.IntID()
	}
	return classes
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
		break
	case datastore.ErrNoSuchEntity:
		return nil, ErrClassNotFound
	default:
		if isFieldMismatch(err) {
			break
		}
		return nil, err
	}
	class.ID = id
	return class, nil
}

// ClassesWithIDs returns a list of classes which correspond to the given IDs.
func ClassesWithIDs(c appengine.Context, ids []int64) []*Class {
	keys := make([]*datastore.Key, len(ids))
	classes := make([]*Class, len(ids))
	for i, id := range ids {
		classes[i] = &Class{}
		keys[i] = classKeyFromID(c, id)
	}
	switch err := datastore.GetMulti(c, keys, classes); err {
	case nil:
		break
	default:
		if isFieldMismatch(err) {
			break
		}
		c.Errorf("Failed to get classes with list of IDs: %s", err)
		return nil
	}
	for i, key := range keys {
		classes[i].ID = key.IntID()
	}
	return classes
}

func (c *Class) Description() string {
	return string(c.LongDescription)
}

func (cls *Class) TeacherEntity(c appengine.Context) *Teacher {
	if cls.Teacher == nil {
		return nil
	}
	teacher, err := teacherByKey(c, cls.Teacher)
	if err != nil && !isFieldMismatch(err) {
		c.Errorf("Failed to find teacher for class %d: %s", cls.ID, err)
		return nil
	}
	return teacher
}

// Insert adds a new Class to the datastore; it will not overwrite an existing Class.
func (cls *Class) Insert(c appengine.Context) error {
	iKey := datastore.NewIncompleteKey(c, "Class", nil)
	key, err := datastore.Put(c, iKey, cls)
	if err != nil {
		return err
	}
	cls.ID = key.IntID()
	return nil
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

func (cls *Class) Delete(c appengine.Context) error {
	// TODO(rwsims): This should defer a task to delete all students of the class. And maybe notify them?
	if err := datastore.Delete(c, cls.Key(c)); err != nil {
		return err
	}
	return nil
}

// TeachersByClass returns a map from Class ID to Teacher entity (or nil if the class has no teacher).
func TeachersByClass(c appengine.Context, classList []*Class) map[int64]*Teacher {
	teachers := make(map[int64]*Teacher)
	for _, class := range classList {
		key := class.Teacher
		if key == nil {
			continue
		}
		teacher, err := teacherByKey(c, key)
		if err != nil && !isFieldMismatch(err) {
			c.Errorf("Failed to find teacher for class %d: %s", class.ID, err)
			continue
		}
		teachers[class.ID] = teacher
	}
	return teachers
}

// GroupedByDay returns a map from weekday to a list of classes on that day.
func GroupedByDay(classList []*Class) map[time.Weekday][]*Class {
	byDay := make(map[time.Weekday][]*Class)
	for _, class := range classList {
		day := byDay[class.Weekday]
		day = append(day, class)
		byDay[class.Weekday] = day
	}
	return byDay
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
