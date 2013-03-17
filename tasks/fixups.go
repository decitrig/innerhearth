/*  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
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
package tasks

import (
	"fmt"
	"net/http"
	"strconv"

	"appengine"
	"appengine/datastore"
	"appengine/taskqueue"

	"github.com/decitrig/innerhearth/model"
)

func init() {
	http.HandleFunc("/task/fixup/long-description", longDescription)
	http.HandleFunc("/task/fixup/calendar-data", calendarData)
}

func longDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c := appengine.NewContext(r)
	q := datastore.NewQuery("Class").
		Limit(10)
	if cursorString := r.FormValue("cursor"); cursorString != "" {
		cursor, err := datastore.DecodeCursor(cursorString)
		if err != nil {
			c.Errorf("Error decoding cursor: %s", err)
			return
		}
		q.Start(cursor)
	}
	classes := q.Run(c)
	found := 0
	for {
		class := &model.Class{}
		key, err := classes.Next(class)
		if err == datastore.Done {
			break
		}
		found++
		if err != nil {
			c.Errorf("Error reading class from iterator: %s", err)
			continue
		}
		class.Length = class.LengthMinutes * time.Minute
		class.LongDescription = []byte(class.Description)
		_, err = datastore.Put(c, key, class)
		if err != nil {
			c.Warningf("Error writing class %d: %e", key.IntID(), err)
		}
	}
	if found == 10 {
		cursor, err := classes.Cursor()
		if err != nil {
			c.Errorf("Error getting cursor: %s", err)
			return
		}
		t := taskqueue.NewPOSTTask("/task/fixup/long-description", map[string][]string{
			"cursor":  {cursor.String()},
			"tasknum": {fmt.Sprintf("%d", taskNum+1)},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			c.Errorf("Error adding next task: %s", err)
		}
	}
}

var weekdays = []string{
	"Sunday",
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
}

func stringToWeekday(s string) (time.Weekday, error) {
	for i, name := range weekdays {
		if name == s {
			return time.Weekday(i), nil
		}
	}
	return time.Weekday(0), fmt.Errorf("Couldn't find weekday of %q", s)
}

func calendarData(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c := appengine.NewContext(r)
	q := datastore.NewQuery("Class").
		Limit(10)
	if cursorString := r.FormValue("cursor"); cursorString != "" {
		cursor, err := datastore.DecodeCursor(cursorString)
		if err != nil {
			c.Errorf("Error decoding cursor: %s", err)
			return
		}
		q.Start(cursor)
	}
	classes := q.Run(c)
	found := 0
	for {
		class := &model.Class{}
		key, err := classes.Next(class)
		if err == datastore.Done {
			break
		}
		found++
		if err != nil {
			c.Errorf("Error reading class from iterator: %s", err)
			continue
		}
		if class.Length == 0 {
			class.Length = class.LengthMinutes * time.Minute
		}
		if class.Weekday == 0 {
			if w, err := stringToWeekday(class.DayOfWeek); err != nil {
				c.Errorf("Error parsing weekday from %d: %s", key.IntID(), err)
			} else {
				class.Weekday = w
			}
		}
		_, err = datastore.Put(c, key, class)
		if err != nil {
			c.Warningf("Error writing class %d: %e", key.IntID(), err)
		}
	}
	if found == 10 {
		cursor, err := classes.Cursor()
		if err != nil {
			c.Errorf("Error getting cursor: %s", err)
			return
		}
		t := taskqueue.NewPOSTTask("/task/fixup/calendar-data", map[string][]string{
			"cursor": {cursor.String()},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			c.Errorf("Error adding next task: %s", err)
		}
	}
}
