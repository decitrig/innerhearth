package classes

import (
	"fmt"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/account"
)

// A Teacher can be assigned to one or more classes or workshops, and
// is allowed to view rosters for classes & workshops they teach.
type Teacher struct {
	// The Account with which this teacher is associated.
	ID string `datastore: "-"`

	// Contact information for the teacher. This is identical to the
	// information in the teachers' InnerHearthUser account.
	account.Info
}

// Creates a new Teacher associated with the given user.
func NewTeacher(user *account.Account) *Teacher {
	return &Teacher{
		ID:   user.ID,
		Info: user.Info,
	}
}

// TeachersByName sorts teachers in alphabetical order by first and then last name.
type TeachersByName []*Teacher

func (l TeachersByName) Len() int      { return len(l) }
func (l TeachersByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l TeachersByName) Less(i, j int) bool {
	if l[i].FirstName != l[j].FirstName {
		return l[i].FirstName < l[j].FirstName
	}
	return l[i].LastName < l[j].LastName
}

func teacherKeyFromID(c appengine.Context, id string) *datastore.Key {
	return datastore.NewKey(c, "Teacher", id, 0, nil)
}

func (t *Teacher) Key(c appengine.Context) *datastore.Key {
	return teacherKeyFromID(c, t.ID)
}

func isFieldMismatch(err error) bool {
	_, ok := err.(*datastore.ErrFieldMismatch)
	return ok
}

func teacherByKey(c appengine.Context, key *datastore.Key) (*Teacher, error) {
	teacher := &Teacher{}
	switch err := datastore.Get(c, key, teacher); err {
	case nil:
		break
	case datastore.ErrNoSuchEntity:
		return nil, ErrUserIsNotTeacher
	default:
		if isFieldMismatch(err) {
			c.Errorf("Teacher field mismatch: %s", err)
			break
		}
		return nil, err
	}
	teacher.ID = key.StringID()
	return teacher, nil
}

// TeacherForUser returns the Teacher associated with a specific user Account.
func TeacherForUser(c appengine.Context, user *account.Account) (*Teacher, error) {
	return teacherByKey(c, teacherKeyFromID(c, user.ID))
}

// TeacherWithID returns the Teacher with the given ID, if one exists.
func TeacherWithID(c appengine.Context, id string) (*Teacher, error) {
	return teacherByKey(c, teacherKeyFromID(c, id))
}

// TeacherWithID returns the Teacher with the give email, if one exists.
func TeacherWithEmail(c appengine.Context, email string) (*Teacher, error) {
	q := datastore.NewQuery("Teacher").
		Filter("Email =", email).
		Limit(1)
	teachers := []*Teacher{}
	keys, err := q.GetAll(c, &teachers)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, ErrUserIsNotTeacher
	}
	teacher := teachers[0]
	teacher.ID = keys[0].StringID()
	return teacher, nil
}

func (t *Teacher) Put(c appengine.Context) error {
	if _, err := datastore.Put(c, t.Key(c), t); err != nil {
		return err
	}
	return nil
}

func (t *Teacher) Delete(c appengine.Context) error {
	if err := datastore.Delete(c, t.Key(c)); err != nil {
		return err
	}
	return nil
}

func (t *Teacher) DisplayName() string {
	if t == nil {
		return "Inner Hearth Staff"
	}
	return fmt.Sprintf("%s %s", t.FirstName, t.LastName)
}

// Teachers returns a list of all the Teachers which currently exist.
func Teachers(c appengine.Context) []*Teacher {
	q := datastore.NewQuery("Teacher").
		Limit(100)
	teachers := []*Teacher{}
	keys, err := q.GetAll(c, &teachers)
	if err != nil {
		c.Errorf("Failed to look up teachers: %s", err)
		return nil
	}
	for i, key := range keys {
		teachers[i].ID = key.StringID()
	}
	return teachers
}
