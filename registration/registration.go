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
package registration

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"appengine/taskqueue"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	classFullPage         = template.Must(template.ParseFiles("templates/base.html", "templates/registration/class-full.html"))
	alreadyRegisteredPage = template.Must(template.ParseFiles("templates/base.html", "templates/registration/already-registered.html"))
)

func classTeacherOrStaffOnly(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		u := webapp.GetCurrentUser(r)
		c := appengine.NewContext(r)
		classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
		if err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't parse class ID %s: %s", r.FormValue("class"), err))
		}
		scheduler := model.NewScheduler(c)
		class := scheduler.GetClass(classID)
		if class == nil {
			return webapp.InternalError(fmt.Errorf("No class with ID %d", classID))
		}

		// Staff can access any class page.
		if staff := model.GetStaff(c, u); staff != nil {
			return handler.Serve(w, r)
		}

		teacher := model.GetTeacher(c, u)
		if teacher == nil {
			return webapp.UnauthorizedError(fmt.Errorf("No Teacher found for account %s", u.AccountID))
		}
		if class.Teacher.StringID() != teacher.AccountID {
			return webapp.UnauthorizedError(fmt.Errorf("Teacher %s does not teach class %d", teacher.AccountID, class.ID))
		}
		return handler.Serve(w, r)
	}
}

func init() {
	webapp.HandleFunc("/registration/session", sessionRegistration)
	webapp.HandleFunc("/registration/oneday", oneDayRegistration)
	webapp.HandleFunc("/registration/paper", classTeacherOrStaffOnly(webapp.HandlerFunc(paperRegistration)))
}

func classFull(class *model.Class, w http.ResponseWriter, r *http.Request) *webapp.Error {
	if err := classFullPage.Execute(w, class); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func alreadyRegistered(class *model.Class, w http.ResponseWriter, r *http.Request) *webapp.Error {
	if err := alreadyRegisteredPage.Execute(w, class); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func sessionRegistration(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return nil
	}
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't parse class id %s: %s", r.FormValue("class"), err))
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	roster := model.NewRoster(c, class)
	u := webapp.GetCurrentUser(r)
	_, err = roster.AddStudent(u.AccountID)
	switch {
	case err == model.ErrClassFull:
		return classFull(class, w, r)

	case err == model.ErrAlreadyRegistered:
		return alreadyRegistered(class, w, r)

	case err != nil:
		return webapp.InternalError(fmt.Errorf("Error registering student: %s", err))
	}
	t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
		"account": {u.AccountID},
		"class":   {fmt.Sprintf("%d", class.ID)},
	})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		return webapp.InternalError(fmt.Errorf("Error adding email confirmation task: %s", err))
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func oneDayRegistration(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		http.Error(w, "Bad Request", http.StatusBadRequest)
	}
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't parse class id %s: %s", r.FormValue("class"), err))
	}
	date, err := time.Parse("2006-01-02", r.FormValue("date"))
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't parse date %s: %s", r.FormValue("date"), err))
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	roster := model.NewRoster(c, class)
	u := webapp.GetCurrentUser(r)
	_, err = roster.AddDropIn(u.AccountID, date)
	switch {
	case err == model.ErrClassFull:
		return classFull(class, w, r)

	case err == model.ErrAlreadyRegistered:
		return alreadyRegistered(class, w, r)

	case err != nil:
		return webapp.InternalError(fmt.Errorf("Error registering student: %s", err))
	}
	t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
		"account": {u.AccountID},
		"class":   {fmt.Sprintf("%d", class.ID)},
	})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		return webapp.InternalError(fmt.Errorf("Error adding email confirmation task: %s", err))
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}

func paperRegistration(w http.ResponseWriter, r *http.Request) *webapp.Error {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return nil
	}
	fields, err := webapp.ParseRequiredValues(r, "xsrf_token", "class", "firstname", "lastname", "email", "type")
	if err != nil {
		return webapp.InternalError(err)
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classID, err := strconv.ParseInt(fields["class"], 10, 64)
	if err != nil {
		return webapp.InternalError(err)
	}
	class := scheduler.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	account := model.GetAccountByEmail(c, fields["email"])
	if account == nil {
		account = &model.UserAccount{
			FirstName: fields["firstname"],
			LastName:  fields["lastname"],
			Email:     fields["email"],
		}
		if p := r.FormValue("phone"); p != "" {
			account.Phone = p
		}
		account.AccountID = "PAPERREGISTRATION|" + fields["email"]
		if err := model.StoreAccount(c, nil, account); err != nil {
			return webapp.InternalError(err)
		}
	}
	roster := model.NewRoster(c, class)
	switch t := fields["type"]; t {
	case "dropin":
		day, err := time.Parse("2006-01-02", r.FormValue("date"))
		if err != nil {
			return webapp.InternalError(err)
		}
		if _, err = roster.AddDropIn(account.AccountID, day); err != nil {
			if err == model.ErrAlreadyRegistered {
				return alreadyRegistered(class, w, r)
			}
			return webapp.InternalError(err)
		}

	case "session":
		if _, err = roster.AddStudent(account.AccountID); err != nil {
			if err == model.ErrAlreadyRegistered {
				return alreadyRegistered(class, w, r)
			}
			return webapp.InternalError(err)
		}

	default:
		return webapp.InternalError(fmt.Errorf("Invalid registration type '%s'", t))
	}
	http.Redirect(w, r, fmt.Sprintf("/teacher/roster?class=%d", class.ID), http.StatusSeeOther)
	return nil
}
