package model

import (
	"errors"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassFull         = errors.New("Class is full")
	ErrClassExists       = errors.New("Class already exists")
	ErrClassNotEmpty     = errors.New("Class is not empty")
	ErrAlreadyRegistered = errors.New("Student is already registered for class")
)

type ClassFullError struct {
	Class string
}

func (e *ClassFullError) Error() string {
	return fmt.Sprintf("Class %s is full", e.Class)
}

func classFullError(class string) error {
	return &ClassFullError{class}
}

type Class struct {
	ID          int64  `datastore: "-"`
	Title       string `datastore: ",noindex"`
	Description string `datastore: ",noindex"`
	Teacher     string

	DayOfWeek     string
	StartTime     time.Time `datastore: ",noindex"`
	LengthMinutes int32     `datastore: ",noindex"`

	BeginDate time.Time
	EndDate   time.Time
	Active    bool

	DropInOnly bool

	// The number of students who can register for the class.
	Capacity int32 `datastore: ",noindex"`
}

// NextClassTime returns the earliest start time of the class which starts strictly later than the
// given time.
func (c *Class) NextClassTime(after time.Time) time.Time {
	return time.Time{}
}

func (c *Class) key(ctx appengine.Context) *datastore.Key {
	return datastore.NewKey(ctx, "Class", "", c.ID, nil)
}

// A Scheduler is responsible for manipulating classes.
type Scheduler interface {
	AddNew(c *Class) error
	GetClass(id int64) *Class
	ListClasses(activeOnly bool) []*Class
	DeleteClass(c *Class) error
}

type scheduler struct {
	appengine.Context
}

func (s *scheduler) GetClass(id int64) *Class {
	key := datastore.NewKey(s, "Class", "", id, nil)
	class := &Class{}
	if err := datastore.Get(s, key, class); err != nil {
		s.Errorf("Error getting class %d: %s", id, err)
		return nil
	}
	class.ID = id
	return class
}

func (s *scheduler) AddNew(c *Class) error {
	key := datastore.NewIncompleteKey(s, "Class", nil)
	if _, err := datastore.Put(s, key, c); err != nil {
		return fmt.Errorf("Error writing class %s: %s", c.Title, err)
	}
	return nil
}

func (s *scheduler) ListClasses(activeOnly bool) []*Class {
	classes := []*Class{}
	q := datastore.NewQuery("Class")
	if activeOnly {
		q = q.Filter("Active =", true)
	}
	keys, err := q.GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing classes: %s", err)
		return nil
	}
	for idx, key := range keys {
		classes[idx].ID = key.IntID()
	}
	return classes
}

func (s *scheduler) DeleteClass(c *Class) error {
	key := c.key(s)
	err := datastore.RunInTransaction(s, func(context appengine.Context) error {
		q := datastore.NewQuery("Registration").
			Ancestor(key).
			KeysOnly().
			Limit(1)
		keys, err := q.GetAll(context, nil)
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			return ErrClassNotEmpty
		}
		if err := datastore.Delete(context, key); err != nil {
			return err
		}
		return nil
	}, nil)
	if err != nil && err != ErrClassNotEmpty {
		return fmt.Errorf("Error deleting class %d: %s", c.ID, err)
	}
	return err
}

func NewScheduler(c appengine.Context) Scheduler {
	return &scheduler{c}
}

// A Registration represents a reserved space in a class, either for the entire session or as a drop
// in.
type Registration struct {
	StudentID string
	ClassID   int64

	// If this registration is a drop in, Date is the start time of the class to which it applies.
	Date time.Time
}

func (r *Registration) key(c appengine.Context) *datastore.Key {
	classKey := datastore.NewKey(c, "Class", "", r.ClassID, nil)
	return datastore.NewKey(c, "Registration", r.StudentID, 0, classKey)
}

type Roster interface {
	LookupRegistration(studentID string) *Registration
	ListRegistrations() []*Registration
	AddStudent(studentID string) (*Registration, error)
	AddDropIn(studentID string, date time.Time) (*Registration, error)
}

type roster struct {
	appengine.Context
	class *Class
}

func NewRoster(c appengine.Context, class *Class) Roster {
	return &roster{c, class}
}

func (r *roster) ListRegistrations() []*Registration {
	q := datastore.NewQuery("Registration").
		Ancestor(r.class.key(r))
	rs := []*Registration{}
	if _, err := q.GetAll(r, &rs); err != nil {
		r.Errorf("Error looking up registrations for class %d: %s", r.class.ID, err)
		return nil
	}
	return rs
}

func (r *roster) LookupRegistration(studentID string) *Registration {
	key := datastore.NewKey(r, "Registration", studentID, 0, r.class.key(r))
	reg := &Registration{}
	if err := datastore.Get(r, key, reg); err != nil {
		r.Errorf("Error looking up registration for student %s in class %d: %s", studentID, r.class.ID, err)
		return nil
	}
	return reg
}

func (r *roster) AddStudent(studentID string) (*Registration, error) {
	reg := &Registration{
		ClassID:   r.class.ID,
		StudentID: studentID,
	}
	err := datastore.RunInTransaction(r, func(ctx appengine.Context) error {
		key := reg.key(ctx)
		old := &Registration{}
		if err := datastore.Get(ctx, key, old); err != datastore.ErrNoSuchEntity {
			if err != nil {
				return fmt.Errorf("Error looking up registration %+v: %s", reg, err)
			}
			return ErrAlreadyRegistered
		}
		if _, err := datastore.Put(ctx, key, reg); err != nil {
			return fmt.Errorf("Error writing registration %+v: %s", reg, err)
		}
		return nil
	}, nil)
	if err != nil {
		return nil, err
	}
	return reg, nil
}

func (r *roster) AddDropIn(studentID string, date time.Time) (*Registration, error) {
	return nil, fmt.Errorf("Not yet implemented")
}

type Registrar interface {
	ListRegistrations() []*Registration
	ListRegisteredClasses() []*Class
}

type registrar struct {
	appengine.Context
	studentID string
}

func NewRegistrar(c appengine.Context, studentID string) Registrar {
	return &registrar{c, studentID}
}

func (r *registrar) ListRegistrations() []*Registration {
	rs := []*Registration{}
	q := datastore.NewQuery("Registration").
		Filter("StudentID =", r.studentID)
	if _, err := q.GetAll(r, &rs); err != nil {
		return nil
	}
	return rs
}

func (r *registrar) ListRegisteredClasses() []*Class {
	q := datastore.NewQuery("Registration").
		Filter("StudentID =", r.studentID).
		KeysOnly()
	regKeys, err := q.GetAll(r, nil)
	if err != nil {
		r.Errorf("Error getting reg keys for student %s: %s", r.studentID, err)
		return nil
	}
	if len(regKeys) == 0 {
		return nil
	}
	classKeys := make([]*datastore.Key, len(regKeys))
	for i, k := range regKeys {
		classKeys[i] = k.Parent()
	}
	tmp := make([]Class, len(classKeys))
	if err := datastore.GetMulti(r, classKeys, tmp); err != nil {
		r.Errorf("Error getting registered classes for %s: %s", r.studentID, err)
		return nil
	}
	classes := make([]*Class, len(classKeys))
	for i, c := range tmp {
		c.ID = classKeys[i].IntID()
		classes[i] = &c
	}
	return classes
}
