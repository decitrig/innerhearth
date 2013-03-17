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
	"html/template"
	"net/http"

	"appengine"
	"appengine/datastore"

	"github.com/decitrig/innerhearth/model"
)

var (
	copyDescriptionPage = template.Must(template.ParseFiles("templates/base.html", "templates/fixups/copy-description.html"))
)

func init() {
	http.HandleFunc("/task/fixup/description", copyDescription)
}

func copyDescription(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if r.Method == "POST" {
		classes := []*model.Class{}
		keys, err := datastore.NewQuery("Class").
			Filter("LongDescription =", nil).
			Limit(10).
			GetAll(c, &classes)
		if err != nil {
			c.Errorf("Error looking up classes: %s", err)
			return
		}
		for i, class := range classes {
			class.LongDescription = []byte(class.Description)
			_, err := datastore.Put(c, keys[i], class)
			if err != nil {
				c.Warningf("Error writing class %d: %e", keys[i].IntID(), err)
			}
		}
		http.Redirect(w, r, "/task/fixup/description", http.StatusMovedPermanently)
	}
	count, err := datastore.NewQuery("Class").
		KeysOnly().
		Filter("LongDescription =", nil).
		Count(c)
	if err != nil {
		c.Errorf("Error counting classes: %s", err)
		http.Error(w, "Error counting classes.", http.StatusInternalServerError)
		return
	}
	if err := copyDescriptionPage.Execute(w, map[string]interface{}{
		"Count": count,
	}); err != nil {
		c.Errorf("Error rendering page: %s", err)
		http.Error(w, "An error ocurred", http.StatusInternalServerError)
	}
}
