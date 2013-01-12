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
	Active    bool

	DropInOnly bool

	Capacity   int32 `datastore: ",noindex"`
	SpacesLeft int32
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

func (c *Class) GetExpirationTime() time.Time {
	return c.EndDate.Add(
		c.StartTime.Add(time.Minute * time.Duration(c.LengthMinutes)).Sub(time.Time{}))
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
	q := datastore.NewQuery("Class").
		Filter("SpacesLeft >", 0)
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

	// Expiration date of this registration.
	Date time.Time
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
		Ancestor(r.class.key(r))
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
		Date:      r.class.GetExpirationTime(),
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
		if class.SpacesLeft == 0 {
			return ErrClassFull
		}
		class.SpacesLeft--
		if _, err := datastore.Put(ctx, classKey, class); err != nil {
			return fmt.Errorf("Error updating class %d: %s", class.ID, err)
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

func (r *registrar) listPaperRegistrations() []*Class {
	account, err := GetAccountByID(r, r.studentID)
	if err != nil {
		r.Errorf("Error looking up paper registrations for %s: %s", r.studentID, err)
		return nil
	}
	q := datastore.NewQuery("Registration").
		Filter("StudentID =", "PAPERREGISTRATION|"+account.Email).
		KeysOnly()
	regKeys, err := q.GetAll(r, nil)
	if err != nil {
		r.Errorf("Error getting paper reg keys for student %s: %s", r.studentID, err)
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
		classes[i] = &tmp[i]
	}
	return classes
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
	classKeys := make([]*datastore.Key, len(regKeys))
	classes := make([]*Class, len(classKeys))
	for i, k := range regKeys {
		classKeys[i] = k.Parent()
		classes[i] = &Class{}
	}
	if err := datastore.GetMulti(r, classKeys, classes); err != nil {
		r.Errorf("Error getting registered classes for %s: %s", r.studentID, err)
		return nil
	}
	classesByID := map[int64]*Class{}
	for _, class := range classes {
		classesByID[class.ID] = class
	}
	paperClasses := r.listPaperRegistrations()
	if paperClasses == nil {
		return classes
	}
	for _, class := range paperClasses {
		classesByID[class.ID] = class
	}
	classes = []*Class{}
	for _, class := range classesByID {
		classes = append(classes, class)
	}
	return classes
}
