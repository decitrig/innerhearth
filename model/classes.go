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
	ErrInvalidDropInDate = errors.New("Date is not in class's session")
)

type Class struct {
	ID          int64  `datastore: "-"`
	Title       string `datastore: ",noindex"`
	Description string `datastore: ",noindex"`
	Teacher     *datastore.Key

	DayOfWeek     string
	StartTime     time.Time `datastore: ",noindex"`
	LengthMinutes int32     `datastore: ",noindex"`

	BeginDate time.Time
	EndDate   time.Time

	DropInOnly bool
	Capacity   int32 `datastore: ",noindex"`

	// The following fields are deprecated, but exist in legacy data.
	SpacesLeft int32
	Active     bool
}

// NextClassTime returns the earliest start time of the class which starts strictly later than the
// given time.
func (c *Class) NextClassTime(after time.Time) time.Time {
	return time.Time{}
}

func (c *Class) key(ctx appengine.Context) *datastore.Key {
	return datastore.NewKey(ctx, "Class", "", c.ID, nil)
}

func (c *Class) Registrations() int32 {
	return c.Capacity - c.SpacesLeft
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(),
		0, 0, 0, 0, t.Location())
}

func dateTime(d, t time.Time) time.Time {
	return time.Date(
		d.Year(), d.Month(), d.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), t.Location(),
	)
}

func (c *Class) GetExpirationTime() time.Time {
	return c.GetEndingTime(c.EndDate)
}

func (c *Class) GetEndingTime(date time.Time) time.Time {
	t := c.StartTime.Add(time.Minute * time.Duration(c.LengthMinutes))
	return dateTime(date, t)
}

func (c *Class) ValidDate(date time.Time) bool {
	if c.DayOfWeek != date.Weekday().String() {
		return false
	}
	if c.DropInOnly {
		return true
	}
	day := dateOnly(date)
	if day.Before(c.BeginDate) || day.After(c.EndDate) {
		return false
	}
	return true
}

// A Scheduler is responsible for manipulating classes.
type Scheduler interface {
	AddNew(c *Class) error
	GetClass(id int64) *Class
	ListClasses(activeOnly bool) []*Class
	DeleteClass(c *Class) error
	ListOpenClasses(activeOnly bool) []*Class
	GetTeacherNames(classes []*Class) map[int64]string
	GetTeacher(class *Class) *UserAccount
	GetClassesForTeacher(teacher *UserAccount) []*Class
	WriteClass(class *Class) error
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

func (s *scheduler) ListOpenClasses(activeOnly bool) []*Class {
	classes := []*Class{}
	q := datastore.NewQuery("Class")
	keys, err := q.GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing classes: %s", err)
		return nil
	}
	openClasses := []*Class{}
	for i, class := range classes {
		class.ID = keys[i].IntID()
		if !class.DropInOnly {
			today := dateOnly(time.Now())
			if today.After(class.EndDate) {
				continue
			}
		}
		openClasses = append(openClasses, class)
	}
	return openClasses
}

func (s *scheduler) GetClassesForTeacher(t *UserAccount) []*Class {
	q := datastore.NewQuery("Class").
		Filter("Teacher =", t.key(s))
	classes := []*Class{}
	keys, err := q.GetAll(s, &classes)
	if err != nil {
		s.Errorf("Error listing classes for teacher %s: %s", t.AccountID, err)
		return nil
	}
	for idx, key := range keys {
		classes[idx].ID = key.IntID()
	}
	return classes
}

func (s *scheduler) GetTeacherNames(classes []*Class) map[int64]string {
	keys := make([]*datastore.Key, len(classes))
	teachers := make([]*UserAccount, len(classes))
	for idx, class := range classes {
		keys[idx] = class.Teacher
		teachers[idx] = &UserAccount{}
	}
	if err := datastore.GetMulti(s, keys, teachers); err != nil {
		s.Errorf("Error looking up teacher names: %s", err)
		return nil
	}
	names := map[int64]string{}
	for idx, class := range classes {
		names[class.ID] = teachers[idx].FirstName
	}
	return names
}

func (s *scheduler) GetTeacher(class *Class) *UserAccount {
	t := &UserAccount{}
	if err := datastore.Get(s, class.Teacher, t); err != nil {
		s.Criticalf("Couldn't get teacher for class %d: %s", class.ID, err)
		return nil
	}
	return t
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

func (s *scheduler) WriteClass(c *Class) error {
	key := c.key(s)
	if _, err := datastore.Put(s, key, c); err != nil {
		return fmt.Errorf("Error writing class %d: %s", c.ID, err)
	}
	return nil
}

func NewScheduler(c appengine.Context) Scheduler {
	return &scheduler{c}
}

// A Registration represents a reserved space in a class, either for the entire session or as a drop
// in.
type Registration struct {
	StudentID string
	ClassID   int64

	// The last date on which this registration is still valid.
	Date   time.Time
	DropIn bool
}

func (r *Registration) key(c appengine.Context) *datastore.Key {
	classKey := datastore.NewKey(c, "Class", "", r.ClassID, nil)
	return datastore.NewKey(c, "Registration", r.StudentID, 0, classKey)
}

type Roster interface {
	LookupRegistration(studentID string) *Registration
	ListRegistrations() []*Registration
	GetStudents(registrations []*Registration) []*UserAccount
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
		Ancestor(r.class.key(r)).
		Filter("Date >=", dateOnly(time.Now()))
	rs := []*Registration{}
	if _, err := q.GetAll(r, &rs); err != nil {
		r.Errorf("Error looking up registrations for class %d: %s", r.class.ID, err)
		return nil
	}
	return rs
}

func (r *roster) GetStudents(rs []*Registration) []*UserAccount {
	keys := make([]*datastore.Key, len(rs))
	students := make([]*UserAccount, len(rs))
	for idx, reg := range rs {
		keys[idx] = datastore.NewKey(r, "UserAccount", reg.StudentID, 0, nil)
		students[idx] = &UserAccount{}
	}
	if err := datastore.GetMulti(r, keys, students); err != nil {
		r.Errorf("Error looking up students from registrations: %s", err)
		return nil
	}
	return students
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
		Date:      r.class.EndDate,
		DropIn:    false,
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

		classKey := datastore.NewKey(ctx, "Class", "", reg.ClassID, nil)
		class := &Class{}
		if err := datastore.Get(ctx, classKey, class); err != nil {
			return fmt.Errorf("Error looking up class %d for registration %+v: %s",
				reg.ClassID, reg, err)
		}
		class.ID = classKey.IntID()
		q := datastore.NewQuery("Registration").
			Ancestor(classKey).
			KeysOnly().
			Filter("Date >=", dateOnly(time.Now()))
		regs, err := q.Count(r)
		if err != nil {
			return fmt.Errorf("Error counting registration: %s", err)
		}
		if int32(regs) >= class.Capacity {
			return ErrClassFull
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

func (r *roster) getClass(ctx appengine.Context) *Class {
	class := &Class{}
	if err := datastore.Get(ctx, r.class.key(ctx), class); err != nil {
		ctx.Errorf("Couldnt' find class %d: %s", r.class.ID, err)
		return nil
	}
	class.ID = r.class.ID
	return class
}

func (r *roster) DropStudent(studentID string) error {
	err := datastore.RunInTransaction(r, func(ctx appengine.Context) error {
		reg := r.LookupRegistration(studentID)
		if reg == nil {
			return fmt.Errorf("Student %s not registered for class %d", studentID, r.class.ID)
		}
		class := r.getClass(ctx)
		if class == nil {
			return fmt.Errorf("No such class class %d", r.class.ID)
		}
		class.SpacesLeft++
		if class.SpacesLeft > class.Capacity {
			r.Warningf("Tried to increase space for class %d beyond capacity", class.ID)
			class.SpacesLeft = class.Capacity
		}
		if _, err := datastore.Put(ctx, class.key(ctx), class); err != nil {
			return fmt.Errorf("Error updating class %d: %s", class.ID, err)
		}
		if err := datastore.Delete(ctx, reg.key(ctx)); err != nil {
			return fmt.Errorf("Error deleting registration %+v: %s", reg, err)
		}
		return nil
	}, nil)
	return err
}

func (r *roster) AddDropIn(studentID string, date time.Time) (*Registration, error) {
	if !r.class.ValidDate(dateOnly(date)) {
		return nil, ErrInvalidDropInDate
	}
	reg := &Registration{
		ClassID:   r.class.ID,
		StudentID: studentID,
		Date:      dateOnly(date),
		DropIn:    true,
	}
	err := datastore.RunInTransaction(r, func(ctx appengine.Context) error {
		key := reg.key(ctx)
		old := &Registration{}
		if err := datastore.Get(ctx, key, old); err != datastore.ErrNoSuchEntity {
			if err != nil {
				return fmt.Errorf("Error looking up registration %+v: %s", reg, err)
			}
			if !old.Date.Before(time.Now()) {
				return ErrAlreadyRegistered
			}
		}
		classKey := datastore.NewKey(ctx, "Class", "", reg.ClassID, nil)
		class := &Class{}
		if err := datastore.Get(ctx, classKey, class); err != nil {
			return fmt.Errorf("Error looking up class %d for registration %+v: %s",
				reg.ClassID, reg, err)
		}
		class.ID = classKey.IntID()
		q := datastore.NewQuery("Registration").
			Ancestor(classKey).
			KeysOnly().
			Filter("Date >=", reg.Date)
		regs, err := q.Count(r)
		if err != nil {
			return fmt.Errorf("Error counting registration: %s", err)
		}
		if int32(regs) >= class.Capacity {
			return ErrClassFull
		}
		r.Infof("reg %+v", reg)
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

type Registrar interface {
	ListRegistrations() []*Registration
	ListRegisteredClasses([]*Registration) []*Class
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
		Filter("StudentID =", r.studentID).
		Filter("Date >=", dateOnly(time.Now()))
	if _, err := q.GetAll(r, &rs); err != nil {
		return nil
	}
	account, err := GetAccountByID(r, r.studentID)
	if err != nil {
		r.Errorf("Error looking up paper registrations for %s: %s", r.studentID, err)
		return nil
	}
	papers := []*Registration{}
	if _, err := datastore.NewQuery("Registration").
		Filter("StudentID = ", "PAPERREGISTRATION|"+account.Email).
		GetAll(r, &papers); err != nil {
		r.Errorf("Error getting paper registrations for %s: %s", account.Email, err)
		return nil
	}
	return append(rs, papers...)
}

func (r *registrar) ListRegisteredClasses(regs []*Registration) []*Class {
	classKeys := make([]*datastore.Key, len(regs))
	classes := make([]*Class, len(classKeys))
	for i, reg := range regs {
		classKeys[i] = datastore.NewKey(r, "Class", "", reg.ClassID, nil)
		classes[i] = &Class{}
	}
	if err := datastore.GetMulti(r, classKeys, classes); err != nil {
		r.Errorf("Error getting registered classes for %s: %s", r.studentID, err)
		return nil
	}
	for i, _ := range classes {
		classes[i].ID = classKeys[i].IntID()
	}
	return classes
}
