package scheduling

import (
	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/auth"
)

// A Teacher can be assigned to one or more classes or workshops, and
// is allowed to view rosters for classes & workshops they teach.
type Teacher struct {
	// The InnerHearthUser account with which this teacher is associated.
	AccountID string `datastore: "-"`

	// Contact information for the teacher. This is identical to the
	// information in the teachers' InnerHearthUser account.
	auth.UserInfo
}

// Creates a new Teacher associated with the given user.
func NewTeacher(user *auth.UserAccount) *Teacher {
	return &Teacher{
		AccountID: user.AccountID,
		UserInfo:  user.UserInfo,
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

func (t *Teacher) key(c appengine.Context) *datastore.Key {
	return teacherKeyFromID(c, t.AccountID)
}

func lookupTeacher(c appengine.Context, key *datastore.Key) (*Teacher, error) {
	teacher := &Teacher{}
	if err := datastore.Get(c, key, teacher); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("Error looking up teacher %q: %s", key.StringID(), err)
		}
		return nil, auth.ErrUserNotFound
	}
	return teacher, nil
}

// LookupTeacher returns the Teacher associated with the given user, if one exists.
func LookupTeacher(c appengine.Context, user *auth.UserAccount) (*Teacher, error) {
	return lookupTeacher(c, teacherKeyFromID(c, user.AccountID))
}

// LookupTeacher returns the Teacher with thte given ID, if one exists.
func LookupTeacherByID(c appengine.Context, id string) (*Teacher, error) {
	return lookupTeacher(c, teacherKeyFromID(c, id))
}

// Store persists the Teacher to the datastore.
func (t *Teacher) Store(c appengine.Context) error {
	key := t.key(c)
	if _, err := datastore.Put(c, key, t); err != nil {
		return err
	}
	return nil
}

// AllTeachers returns a list of all the Teachers which currently exist.
func AllTeachers(c appengine.Context) []*Teacher {
	q := datastore.NewQuery("Teacher").
		Limit(100)
	teachers := []*Teacher{}
	keys, err := q.GetAll(c, &teachers)
	if err != nil {
		c.Errorf("Failed to look up teachers: %s", err)
		return nil
	}
	for i, key := range keys {
		teachers[i].AccountID = key.StringID()
	}
	return teachers
}
