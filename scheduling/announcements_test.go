package scheduling

import (
	"bytes"
	"sort"
	"testing"

	"appengine/aetest"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/auth"
)

func TestAnnouncements(t *testing.T) {
	c, err := aetest.NewContext(nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	announcements := []*Announcement{
		NewAnnouncement("a1", unix(1000)),
		NewAnnouncement("a2", unix(2000)),
		NewAnnouncement("a3", unix(3000)),
		NewAnnouncement("a4", unix(500)),
	}
	staffer := &Staff{"0x1", auth.UserInfo{"staffer", "smith", "foo@foo.com", ""}}
	for i, a := range announcements {
		if err := staffer.AddAnnouncement(c, a); err != nil {
			t.Fatalf("Failed to add announcement %d: %s", i, err)
		}
		if a.ID <= 0 {
			t.Errorf("Invalid id for announcement %d: %d", i, a.ID)
		}
		if got, err := AnnouncementByID(c, a.ID); err != nil {
			t.Errorf("Failed to lookup announcement %d by %d: %s", i, a.ID, err)
		} else if !announcementsEqual(got, a) {
			t.Errorf("Wrong announcement %d; %v vs %v", i, got, a)
		}
	}
	expected := announcements[:len(announcements)-1]
	current, err := CurrentAnnouncements(c, unix(900))
	if len(current) != len(expected) {
		t.Fatalf("Wrong number of current announcements; %d vs %d", len(current), len(expected))
	}
	sort.Sort(AnnouncementsByExpiration(expected))
	sort.Sort(AnnouncementsByExpiration(current))
	for i, want := range expected {
		if got := current[i]; !announcementsEqual(got, want) {
			t.Errorf("Wrong announcement at %d: %v vs %v", got, want)
		}
	}
	a := announcements[0]
	if err := a.Delete(c); err != nil {
		t.Fatalf("Failed to delete announcement %d: %s", a.ID, err)
	}
	if _, err := AnnouncementByID(c, a.ID); err != datastore.ErrNoSuchEntity {
		t.Errorf("Should not have found announcement %d", a.ID)
	}
}

func announcementsEqual(a, b *Announcement) bool {
	switch {
	case a == nil || b == nil:
		return a == b
	case a.ID != b.ID:
		return false
	case !bytes.Equal(a.Text, b.Text):
		return false
	case !a.Expiration.Equal(b.Expiration):
		return false
	}
	return true
}
