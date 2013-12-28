package scheduling

import (
	"fmt"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/auth"
)

var (
	ErrUserIsNotStaff = fmt.Errorf("user is not staff")
)

// Staff is allowed to create/delete classes, add announcments, etc.
type Staff struct {
	AccountID string `datastore: "-"`
	auth.UserInfo
}

// StaffByName sorts Staff structs by first and then last name.
type StaffByName []*Staff

func (l StaffByName) Len() int      { return len(l) }
func (l StaffByName) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l StaffByName) Less(i, j int) bool {
	a, b := l[i], l[j]
	if a.FirstName != b.FirstName {
		return a.FirstName < b.FirstName
	}
	return a.LastName < b.LastName
}

// NewStaff creates a new Staff entity for the given user.
func NewStaff(user *auth.UserAccount) *Staff {
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

// LookupStaff looks up the Staff entity for the given user, if one exists.
func LookupStaff(c appengine.Context, user *auth.UserAccount) (*Staff, error) {
	return LookupStaffByID(c, user.AccountID)
}

// LookupStaff looks up the Staff entity for the user with the given ID, if one exists.
func LookupStaffByID(c appengine.Context, accountID string) (*Staff, error) {
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

// AllStaff returns a list of all the current staff members.
func AllStaff(c appengine.Context) ([]*Staff, error) {
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
