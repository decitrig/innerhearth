package students

import (
	"fmt"
	"time"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/classes"
)

var (
	ErrStudentNotFound = fmt.Errorf("students: student not found")
	ErrClassIsFull     = fmt.Errorf("students: class is full")
)

// A Student is a single registration in a single class. A UserAccount
// may have multiple Students associated with it.
type Student struct {
	ID string
	account.Info

	ClassID   int64
	ClassType classes.Type

	Date   time.Time
	DropIn bool
}

// New creates a new session Student registration for a user in a class.
func New(user *account.Account, class *classes.Class) *Student {
	return &Student{
		ID:        user.ID,
		Info:      user.Info,
		ClassID:   class.ID,
		ClassType: classes.Regular,
	}
}

// NewDropIn creates a new drop-in Student registration for a user in
// a class on a specific date.
func NewDropIn(user *account.Account, class *classes.Class, date time.Time) *Student {
	return &Student{
		ID:        user.ID,
		Info:      user.Info,
		ClassID:   class.ID,
		ClassType: classes.Regular,
		DropIn:    true,
		Date:      date,
	}
}

// WithID returns a list of all Students with an account ID.
func WithID(c appengine.Context, id string) []*Student {
	q := datastore.NewQuery("Student").
		Filter("ID =", id)
	students := []*Student{}
	_, err := q.GetAll(c, &students)
	if err != nil {
		c.Errorf("Failed to look up students for %q: %s", id, err)
		return nil
	}
	return students
}

// WithEmail returns a list of all Students with an email.
func WithEmail(c appengine.Context, email string) []*Student {
	q := datastore.NewQuery("Student").
		Filter("Email =", email)
	students := []*Student{}
	_, err := q.GetAll(c, &students)
	if err != nil {
		c.Errorf("Failed to look up students for %q: %s", email, err)
		return nil
	}
	return students
}

// In returns a list of all Students registered for a class. The list
// will include only those drop-in Students whose date is not in the
// past.
func In(c appengine.Context, class *classes.Class, now time.Time) ([]*Student, error) {
	q := datastore.NewQuery("Student").
		Ancestor(class.Key(c))
	students := []*Student{}
	_, err := q.GetAll(c, &students)
	if err != nil {
		return nil, err
	}
	filtered := []*Student{}
	for _, student := range students {
		if student.DropIn && student.Date.Before(now) {
			continue
		}
		filtered = append(filtered, student)
	}
	return filtered, nil
}

// Add attempts to write a new Student entity; it will not overwrite
// any existing Students. Returns ErrClassFull if the class is full as
// of the given date. The number of students "currently registered"
// for a class is the number of session-registered students plus any
// future drop ins. This may be smaller than the number of students
// registered on a particular day, and so may prevent drop-ins which
// may otherwise have succeeded. In other words, a student can only
// drop in if we can prove that there is room for them to register for
// the rest of the session.
func (s *Student) Add(c appengine.Context, asOf time.Time) error {
	key := s.key(c)
	var txnErr error
	for i := 0; i < 25; i++ {
		txnErr = datastore.RunInTransaction(c, func(c appengine.Context) error {
			old := &Student{}
			switch err := datastore.Get(c, key, old); err {
			case datastore.ErrNoSuchEntity:
				break
			case nil:
				if old.DropIn && old.Date.Before(asOf) {
					// Old registration is an expired drop-in. Allow re-registering.
					break
				}
				// Old registration is still active; do nothing.
				c.Warningf("Attempted duplicate registration of %q in %d", s.ID, s.ClassID)
				return nil
			default:
				return fmt.Errorf("students: failed to look up existing student: %s", err)
			}
			class, err := classes.ClassWithID(c, s.ClassID)
			if err != nil {
				return err
			}
			in, err := In(c, class, asOf)
			if err != nil {
				return fmt.Errorf("students: failed to look up registered students")
			}
			if int32(len(in)) >= class.Capacity {
				return ErrClassIsFull
			}
			if err := class.Update(c); err != nil {
				return fmt.Errorf("students: failed to update class: %s", err)
			}
			if _, err := datastore.Put(c, key, s); err != nil {
				return fmt.Errorf("students: failed to write student: %s", err)
			}
			return nil
		}, nil)
		if txnErr != datastore.ErrConcurrentTransaction {
			break
		}
	}
	switch txnErr {
	case nil:
		return nil
	case datastore.ErrConcurrentTransaction:
		return fmt.Errorf("students: too many concurrent updates to class %d", s.ClassID)
	default:
		return txnErr
	}
}

func (s *Student) key(c appengine.Context) *datastore.Key {
	return datastore.NewKey(c, "Student", s.ID, 0, classes.NewClassKey(c, s.ClassID))
}

// ByName sorts Students in alphabetial order by first and then last name.
type ByName []*Student

func (l ByName) Len() int      { return len(l) }
func (l ByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l ByName) Less(i, j int) bool {
	switch a, b := l[i], l[j]; {
	case a.FirstName != b.FirstName:
		return a.FirstName < b.FirstName
	case a.LastName != b.LastName:
		return a.LastName < b.LastName
	default:
		return false
	}
}

// ClassAndTeacher bundles together a Class and its related Teacher.
type ClassAndTeacher struct {
	Class   *classes.Class
	Teacher *classes.Teacher
}

func ClassesAndTeachers(studentList []*Student) []*ClassAndTeacher {
	classAndTeachers := make([]*ClassAndTeacher, len(studentList))
}
