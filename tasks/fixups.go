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
	"html/template"
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/taskqueue"

	"github.com/decitrig/innerhearth/model"
)

var (
	oldTeacherPage        = template.Must(template.ParseFiles("templates/base.html", "templates/fixups/old-teachers.html"))
	oldTeacherClassesPage = template.Must(template.ParseFiles("templates/base.html", "templates/fixups/old-teacher-classes.html"))
)

func init() {
	http.HandleFunc("/task/fixup/old-teachers", oldTeachers)
	http.HandleFunc("/task/fixup/old-teacher-classes", oldTeacherClasses)
	http.HandleFunc("/task/fixup/convert-registrations", convertRegistrations)
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
		q = q.Start(cursor)
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
			"cursor": {cursor.String()},
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
		q = q.Start(cursor)
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
			class.Length = time.Duration(class.LengthMinutes) * time.Minute
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

func oldTeachers(w http.ResponseWriter, r *http.Request) {
	c := appengine.NewContext(r)
	if r.Method == "POST" {
		email := r.FormValue("email")
		if email == "" {
			http.Error(w, "No email specified", http.StatusBadRequest)
			return
		}
		account := model.GetAccountByEmail(c, email)
		if account == nil {
			http.Error(w, fmt.Sprintf("Couldn't find account for %s", email), http.StatusBadRequest)
			return
		}
		if t := model.AddNewTeacher(c, account); t == nil {
			http.Error(w, "Couldn't add teacher", http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "OK")
		return
	}
	oldTeachers := []*model.UserAccount{}
	_, err := datastore.NewQuery("UserAccount").
		Filter("CanTeach = ", true).
		GetAll(c, &oldTeachers)
	if err != nil {
		c.Errorf("Error looking up old teachers: %s", err)
	}

	newTeachers := []*model.Teacher{}
	_, err = datastore.NewQuery("Teacher").GetAll(c, &newTeachers)
	if err != nil {
		c.Errorf("Error looking up new teachers: %s", err)
	}
	if err := oldTeacherPage.Execute(w, map[string]interface{}{
		"OldTeachers": oldTeachers,
		"NewTeachers": newTeachers,
	}); err != nil {
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
		return
	}
}

func oldTeacherClasses(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	if email == "" {
		http.Error(w, "Email not specified", http.StatusInternalServerError)
		return
	}
	c := appengine.NewContext(r)
	account := model.GetAccountByEmail(c, email)
	if account == nil {
		http.Error(w, fmt.Sprintf("Couldn't find account for %s", email), http.StatusBadRequest)
		return
	}
	accountKey := datastore.NewKey(c, "UserAccount", account.AccountID, 0, nil)
	tk := datastore.NewKey(c, "Teacher", account.AccountID, 0, nil)

	classes := []*model.Class{}
	keys, err := datastore.NewQuery("Class").Filter("Teacher =", accountKey).GetAll(c, &classes)
	if err != nil {
		c.Errorf("Error looking up classes for %s: %s", email, err)
		http.Error(w, "Error looking up classes", http.StatusInternalServerError)
		return
	}
	if r.Method == "POST" {
		for i, class := range classes {
			class.Teacher = tk
			if _, err := datastore.Put(c, keys[i], class); err != nil {
				c.Errorf("Error writing class %d: %s", keys[i].IntID(), err)
				http.Error(w, "Error writing class", http.StatusInternalServerError)
			}
		}
	}
	for i, _ := range classes {
		classes[i].ID = keys[i].IntID()
	}
	if err := oldTeacherClassesPage.Execute(w, map[string]interface{}{
		"Teacher": account,
		"Classes": classes,
	}); err != nil {
		c.Errorf("Error rendering template: %s", err)
		http.Error(w, "Error rendering template", http.StatusInternalServerError)
	}
}

func convertRegistrations(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	c := appengine.NewContext(r)
	regs := []*model.Registration{}
	_, err := datastore.NewQuery("Registration").GetAll(c, &regs)
	if err != nil {
		c.Errorf("Error looking up registrations: %s", err)
		fmt.Fprintf(w, "ERROR")
		return
	}

	accountKeys := make([]*datastore.Key, len(regs))
	accounts := make([]*model.UserAccount, len(regs))
	for i, _ := range accounts {
		accounts[i] = &model.UserAccount{}
		accountKeys[i] = datastore.NewKey(c, "UserAccount", regs[i].StudentID, 0, nil)
	}
	if err := datastore.GetMulti(c, accountKeys, accounts); err != nil {
		c.Errorf("Error getting registration accounts: %s", err)
		fmt.Fprintf(w, "ERROR")
		return
	}

	classKeys := make([]*datastore.Key, len(regs))
	classes := make([]*model.Class, len(regs))
	for i, _ := range classes {
		classes[i] = &model.Class{}
		classKeys[i] = datastore.NewKey(c, "Class", "", regs[i].ClassID, nil)
	}
	if err := datastore.GetMulti(c, classKeys, classes); err != nil {
		c.Errorf("Error getting registration classes: %s", err)
		fmt.Fprintf(w, "ERROR")
	}

	students := make([]*model.Student, len(regs))
	studentKeys := make([]*datastore.Key, len(regs))
	for i, _ := range students {
		studentKeys[i] = datastore.NewKey(c, "Student", accounts[i].AccountID, 0, classKeys[i])
		if regs[i].DropIn {
			students[i] = model.NewDropInStudent(classes[i], accounts[i], regs[i].Date)
		} else {
			students[i] = model.NewSessionStudent(classes[i], accounts[i])
		}
	}
	if _, err := datastore.PutMulti(c, studentKeys, students); err != nil {
		c.Errorf("Error writing students: %s", err)
		fmt.Fprintf(w, "ERROR")
	}
	fmt.Fprintf(w, "OK")
}
