package model

import (
	"errors"
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"
)

var (
	ErrClassExists   = errors.New("Class already exists")
	ErrClassNotEmpty = errors.New("Class is not empty")
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
	Title       string `datastore: ",noindex"`
	Description string `datastore: ",noindex"`
	Teacher     string

	DayOfWeek     string
	StartTime     time.Time `datastore: ",noindex"`
	LengthMinutes int32     `datastore: ",noindex"`

	BeginDate time.Time
	EndDate   time.Time

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
	return datastore.NewKey(ctx, "Class", c.Title, 0, nil)
}

// A Scheduler is responsible for manipulating classes.
type Scheduler interface {
	AddNew(c *Class) error
	ListClasses(activeOnly bool) []*Class
	DeleteClass(class string) error
}

type scheduler struct {
	appengine.Context
}

func (s *scheduler) AddNew(c *Class) error {
	key := c.key(s)
	err := datastore.RunInTransaction(s, func(context appengine.Context) error {
		old := &Class{}
		if err := datastore.Get(context, key, old); err != datastore.ErrNoSuchEntity {
			return ErrClassExists
		}
		_, err := datastore.Put(context, key, c)
		return err
	}, nil)
	if err != nil && err != ErrClassExists {
		return fmt.Errorf("Erorr inserting class %s: %s", c.Title, err)
	}
	return err
}

func (s *scheduler) ListClasses(activeOnly bool) []*Class {
	classes := []*Class{}
	q := datastore.NewQuery("Class")
	_, err := q.GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing classes: %s", err)
		return nil
	}
	return classes
}

func (s *scheduler) DeleteClass(class string) error {
	key := datastore.NewKey(s, "Class", class, 0, nil)
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
		return fmt.Errorf("Error deleting %s: %s", class, err)
	}
	return err
}

func NewScheduler(c appengine.Context) Scheduler {
	return &scheduler{c}
}

// A Registration represents a reserved space in a class, either for the entire session or as a drop
// in.
type Registration struct {
	StudentID  string
	ClassTitle string

	// If this registration is a drop in, Date is the start time of the class to which it applies.
	Date time.Time
}

// A Registrar manages how students register for classes in the schedule.
type Registrar interface {
	LookupRegistration(class, studentID string) *Registration
}

func NewRegistrar(c appengine.Context) Registrar {
	return nil
}

type StudentRegistrar interface {
	ListRegistrations() []*Registration
	FilterRegisteredClasses(classes []*Class) ([]*Class, error)
	RegisterForClass(c *Class) (*Registration, error)
}

func NewStudentRegistrar(c appengine.Context, studentID string) StudentRegistrar {
	return nil
}
