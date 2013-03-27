/*
 *  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
package model

import (
	"sort"
	"time"

	"appengine"
	"appengine/datastore"
)

type Announcement struct {
	ID int64

	Text       []byte
	Expiration time.Time
}

func (a *Announcement) GetText() string {
	return string(a.Text)
}

func NewAnnouncement(c appengine.Context, text string, expiration time.Time) *Announcement {
	a := &Announcement{Text: []byte(text), Expiration: dateOnly(expiration)}
	key := datastore.NewIncompleteKey(c, "Announcement", nil)
	key, err := datastore.Put(c, key, a)
	if err != nil {
		c.Errorf("Couldn't write announcement: %s", err)
		return nil
	}
	a.ID = key.IntID()
	return a
}

func GetAnnouncement(c appengine.Context, id int64) *Announcement {
	key := datastore.NewKey(c, "Announcement", "", id, nil)
	a := &Announcement{}
	if err := datastore.Get(c, key, a); err != nil {
		c.Errorf("Error looking up announcement %d: %s", id, err)
		return nil
	}
	a.ID = key.IntID()
	return a
}

type announcementList []*Announcement

func (l announcementList) Len() int      { return len(l) }
func (l announcementList) Swap(i, j int) { l[j], l[i] = l[i], l[j] }

func (l announcementList) Less(i, j int) bool {
	return l[i].Expiration.Before(l[j].Expiration)
}

func ListAnnouncements(c appengine.Context) []*Announcement {
	q := datastore.NewQuery("Announcement").
		Filter("Expiration >=", dateOnly(time.Now()))
	result := []*Announcement{}
	keys, err := q.GetAll(c, &result)
	if err != nil {
		c.Errorf("Error looking up announcements: %s", err)
		return nil
	}
	for i, key := range keys {
		result[i].ID = key.IntID()
	}
	sort.Sort(announcementList(result))
	return result
}

func DeleteAnnouncement(c appengine.Context, id int64) {
	key := datastore.NewKey(c, "Announcement", "", id, nil)
	if err := datastore.Delete(c, key); err != nil {
		c.Errorf("Error deleting announcement %d: %s", id, err)
	}
}
