package scheduling

import (
	"time"

	"appengine"
	"appengine/datastore"
)

// Announcements are short snippets of text that display for a set
// time at the top of the homepage.
type Announcement struct {
	ID int64

	Text       []byte
	Expiration time.Time
}

// NewAnnouncement creates a new Announcement entity with the given
// text and expiring at the given time.
func NewAnnouncement(text string, expiration time.Time) *Announcement {
	return &Announcement{
		Text:       []byte(text),
		Expiration: expiration,
	}
}

func announcementKeyFromID(c appengine.Context, id int64) *datastore.Key {
	return datastore.NewKey(c, "Announcement", "", id, nil)
}

// AnnouncementByID returns the Announcement entity with the given ID,
// if one exists.
func AnnouncementByID(c appengine.Context, id int64) (*Announcement, error) {
	a := &Announcement{}
	if err := datastore.Get(c, announcementKeyFromID(c, id), a); err != nil {
		return nil, err
	}
	a.ID = id
	return a, nil
}

// CurrentAnnouncements returns a list of all Announcements whose
// expiration time is not in the past.
func CurrentAnnouncements(c appengine.Context, now time.Time) ([]*Announcement, error) {
	q := datastore.NewQuery("Announcement").
		Filter("Expiration >=", now)
	current := []*Announcement{}
	keys, err := q.GetAll(c, &current)
	if err != nil {
		return nil, err
	}
	for i, key := range keys {
		current[i].ID = key.IntID()
	}
	return current, nil
}

// Delete removes an announcement from the datastore.
func (a *Announcement) Delete(c appengine.Context) error {
	if err := datastore.Delete(c, announcementKeyFromID(c, a.ID)); err != nil {
		return err
	}
	return nil
}

// AnnouncementsByExpiration sorts Announcements by their expiration
// time, earliest first.
type AnnouncementsByExpiration []*Announcement

func (l AnnouncementsByExpiration) Len() int      { return len(l) }
func (l AnnouncementsByExpiration) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l AnnouncementsByExpiration) Less(i, j int) bool {
	return l[i].Expiration.Before(l[j].Expiration)
}
