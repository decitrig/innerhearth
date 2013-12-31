// Package yogassage handles creation & manipulation of YinYogassage classes.
package yogassage

import (
	"time"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/classes"
)

// A YinYogassage entity represents a scheduled offering of a
// YinYogassage class.
type YinYogassage struct {
	ID         int64 `datastore: "-"`
	Date       time.Time
	SignupLink string
}

// New creates a YinYogassage class.
func New(date time.Time, signup string) *YinYogassage {
	return &YinYogassage{0, date, signup}
}

func key(c appengine.Context, id int64) *datastore.Key {
	return datastore.NewKey(c, "YinYogassage", "", id, nil)
}

// WithID returns the YinYogassage class with the given ID, if one exists.
func WithID(c appengine.Context, id int64) (*YinYogassage, error) {
	yin := &YinYogassage{}
	key := key(c, id)
	switch err := datastore.Get(c, key, yin); err {
	case nil:
		yin.ID = key.IntID()
		return yin, nil
	case datastore.ErrNoSuchEntity:
		return nil, classes.ErrClassNotFound
	default:
		return nil, err
	}
}

// Insert writes a new YinYogassage entity to the datastore; it will
// not overwrite any existing entities.
func (y *YinYogassage) Insert(c appengine.Context) error {
	iKey := datastore.NewIncompleteKey(c, "YinYogassage", nil)
	key, err := datastore.Put(c, iKey, y)
	if err != nil {
		return err
	}
	y.ID = key.IntID()
	return nil
}

// Delete removes a YinYogassage entity from the datastore.
func (y *YinYogassage) Delete(c appengine.Context) error {
	if err := datastore.Delete(c, key(c, y.ID)); err != nil {
		return err
	}
	return nil
}

// Classes returns a list of all YinYogassage classes which are after a specific time.
func Classes(c appengine.Context, after time.Time) []*YinYogassage {
	q := datastore.NewQuery("YinYogassage").
		Filter("Date >", after)
	yins := []*YinYogassage{}
	keys, err := q.GetAll(c, &yins)
	if err != nil {
		c.Errorf("Failed to look up yogassage classes: %s", err)
		return nil
	}
	for i, key := range keys {
		yins[i].ID = key.IntID()
	}
	return yins
}

// ByDate sorts YinYogassage classes by their date.
type ByDate []*YinYogassage

func (l ByDate) Len() int      { return len(l) }
func (l ByDate) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l ByDate) Less(i, j int) bool {
	return l[i].Date.Before(l[j].Date)
}
