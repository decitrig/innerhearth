package staff

import (
	"fmt"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/classes"
)

var (
	ErrUserIsNotStaff = fmt.Errorf("user is not staff")
)

// Staff is allowed to create/delete classes, add announcments, etc.
type Staff struct {
	AccountID string `datastore: "-"`
	auth.UserInfo
}

// ByName sorts Staff structs by first and then last name.
type ByName []*Staff

func (l ByName) Len() int      { return len(l) }
func (l ByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l ByName) Less(i, j int) bool {
	a, b := l[i], l[j]
	if a.FirstName != b.FirstName {
		return a.FirstName < b.FirstName
	}
	return a.LastName < b.LastName
}

// New creates a new Staff entity for the given user.
func New(user *auth.UserAccount) *Staff {
	return &Staff{
		AccountID: user.AccountID,
		UserInfo:  user.UserInfo,
	}
}

func staffKeyFromID(c appengine.Context, id string) *datastore.Key {
	return datastore.NewKey(c, "Staff", id, 0, nil)
}

func (s *Staff) key(c appengine.Context) *datastore.Key {
	return staffKeyFromID(c, s.AccountID)
}

// ForUserAccount returns the Staff entity for the given user, if one exists.
func ForUserAccount(c appengine.Context, user *auth.UserAccount) (*Staff, error) {
	return WithID(c, user.AccountID)
}

// WithID returns the Staff entity for the user with the given ID, if one exists.
func WithID(c appengine.Context, accountID string) (*Staff, error) {
	key := staffKeyFromID(c, accountID)
	staff := &Staff{}
	if err := datastore.Get(c, key, staff); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("Failed to look up staff %q: %s", accountID, err)
		}
		return nil, ErrUserIsNotStaff
	}
	staff.AccountID = key.StringID()
	return staff, nil
}

// Store persists the Staff entity to the datastore.
func (s *Staff) Store(c appengine.Context) error {
	key := s.key(c)
	if _, err := datastore.Put(c, key, s); err != nil {
		return err
	}
	return nil
}

// PutTeacher persists a Teacher entity to the datastore.
func (s *Staff) PutTeacher(c appengine.Context, teacher *classes.Teacher) error {
	if _, err := datastore.Put(c, teacher.Key(c), teacher); err != nil {
		return err
	}
	return nil
}

// AddSession writes a new Session to the datastore. It will not
// overwrite any existing Session.
func (s *Staff) AddSession(c appengine.Context, session *classes.Session) error {
	incompleteKey := session.NewIncompleteKey(c)
	key, err := datastore.Put(c, incompleteKey, session)
	if err != nil {
		return err
	}
	session.ID = key.IntID()
	return nil
}

// AddClass inserts a new Class into the datastore. It will not overwrite any existing Class.
func (s *Staff) AddClass(c appengine.Context, class *classes.Class) error {
	incompleteKey := class.NewIncompleteKey(c)
	key, err := datastore.Put(c, incompleteKey, class)
	if err != nil {
		return err
	}
	class.ID = key.IntID()
	return nil
}

// UpdateClass overwrites an existing Class entity with new data.
func (s *Staff) UpdateClass(c appengine.Context, class *classes.Class) error {
	if _, err := datastore.Put(c, class.Key(c), class); err != nil {
		return err
	}
	return nil
}

// AddAnnouncement persists an Announcement entity to the datastore.
func (s *Staff) AddAnnouncement(c appengine.Context, announcement *Announcement) error {
	iKey := datastore.NewIncompleteKey(c, "Announcement", nil)
	key, err := datastore.Put(c, iKey, announcement)
	if err != nil {
		return err
	}
	announcement.ID = key.IntID()
	return nil
}

// All returns a list of all current staff members.
func All(c appengine.Context) ([]*Staff, error) {
	q := datastore.NewQuery("Staff").
		Limit(100)
	allStaff := []*Staff{}
	keys, err := q.GetAll(c, &allStaff)
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		allStaff[i].AccountID = key.StringID()
	}
	return allStaff, nil
}
