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
}

func longDescription(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c := appengine.NewContext(r)
	var taskNum int64 = 0
	if numString := r.FormValue("tasknum"); numString != "" {
		var err error
		taskNum, err = strconv.ParseInt(numString, 10, 0)
		if err != nil {
			c.Errorf("Couldn't parse task number %s: %s", numString, err)
			return
		}
	}
	c.Infof("Starting LongDescription fixup #%d", taskNum)
	q := datastore.NewQuery("Class").
		Limit(10)
	if cursorString := r.FormValue("cursor"); cursorString != "" {
		cursor, err := datastore.DecodeCursor(cursorString)
		if err != nil {
			c.Errorf("Error decoding cursor: %s", err)
			return
		}
		q.Start(cursor)
		c.Infof("Starting LongDescription fixup iterator at %q", cursor)
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
		if class.LongDescription != nil {
			continue
		}
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
		t := taskqueue.NewPOSTTask("/task/fixup/description", map[string][]string{
			"cursor":  {cursor.String()},
			"tasknum": {fmt.Sprintf("%d", taskNum+1)},
		})
		t.Name = fmt.Sprintf("long-description-fixup-%d", taskNum+1)
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			c.Errorf("Error adding next task: %s", err)
		}
	}
	if taskNum == 0 {
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
	}
}
