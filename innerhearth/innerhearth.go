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

package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"time"

	"appengine"
	"appengine/user"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/classes"
	"github.com/decitrig/innerhearth/staff"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	indexPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": func(i int) time.Weekday { return time.Weekday((i + 1) % 7) },
	}).ParseFiles("templates/base.html", "templates/index.html"))
	loginPage = template.Must(template.ParseFiles("templates/base.html", "templates/login.html"))
	classPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"weekdayAsIndex": func(w time.Weekday) int { return int(w) },
	}).ParseFiles("templates/base.html", "templates/class.html"))
)

const (
	dateFormat = "01/02/2006"
	timeFormat = "3:04pm"
)

var (
	local *time.Location
)

func parseLocalTime(s string) (time.Time, error) {
	t, err := time.ParseInLocation(timeFormat, s, local)
	if err != nil {
		return t, err
	}
	return t, nil
}

func parseLocalDate(s string) (time.Time, error) {
	t, err := time.ParseInLocation(dateFormat, s, local)
	if err != nil {
		return t, err
	}
	return t, nil
}

func staticTemplate(file string) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		t, err := template.ParseFiles("templates/base.html", file)
		if err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing template %s: %s", file, err))
		}
		if err := t.Execute(w, nil); err != nil {
			return webapp.InternalError(fmt.Errorf("Error rendering template %s: %s", file, err))
		}
		return nil
	}
}

func init() {
	var err error
	local, err = time.LoadLocation("America/New_York")
	if err != nil {
		panic(err)
	}
	http.Handle("/", webapp.Router)
	webapp.HandleFunc("/", index)
	webapp.HandleFunc("/class", class)
	if appengine.IsDevAppServer() {
		webapp.HandleFunc("/error", throwError)
	}

	for url, template := range map[string]string{
		"/about":           "templates/about.html",
		"/pricing":         "templates/pricing.html",
		"/privates-groups": "templates/privates-groups.html",
		"/teachers":        "templates/teachers.html",
		"/workshops":       "templates/workshops.html",
		"/mailinglist":     "templates/mailinglist.html",
	} {
		webapp.HandleFunc(url, staticTemplate(template))
	}
}

func maybeOldAccount(c appengine.Context, u *user.User) (*account.Account, error) {
	switch acct, err := account.ForUser(c, u); err {
	case nil:
		return acct, nil
	case account.ErrUserNotFound:
		break
	default:
		return nil, err
	}
	old, err := account.WithID(c, u.ID)
	if err != nil {
		return nil, err
	}
	c.Warningf("Found user account under old ID %q", u.ID)
	if err := old.RewriteID(c, u); err != nil {
		return nil, fmt.Errorf("failed to rewrite user ID: %s", err)
	}
	return old, err
}

func maybeOldStaff(c appengine.Context, a *account.Account, u *user.User) (*staff.Staff, error) {
	switch staffer, err := staff.WithID(c, a.ID); err {
	case nil:
		return staffer, nil
	case staff.ErrUserIsNotStaff:
		break
	default:
		return nil, err
	}
	staffer, err := staff.WithID(c, u.ID)
	if err != nil {
		return nil, err
	}
	c.Warningf("Found staff account under old ID %q", u.ID)
	if err := staffer.Delete(c); err != nil {
		return nil, fmt.Errorf("failed to delete old staff %q: %s", staffer.ID, err)
	}
	staffer.ID = a.ID
	if err := staffer.Store(c); err != nil {
		return nil, fmt.Errorf("failed to store new staff for %s: %s", a.Email, err)
	}
	return staffer, nil
}

func maybeOldTeacher(c appengine.Context, a *account.Account, u *user.User) (*classes.Teacher, error) {
	switch teacher, err := classes.TeacherWithID(c, a.ID); err {
	case nil:
		return teacher, nil
	case classes.ErrUserIsNotTeacher:
		break
	default:
		return nil, err
	}
	teacher, err := classes.TeacherWithID(c, u.ID)
	if err != nil {
		return nil, err
	}
	c.Warningf("Found teacher under old ID %q", u.ID)
	if err := teacher.Delete(c); err != nil {
		return nil, fmt.Errorf("failed to delete old teacher %q: %s", teacher.ID, err)
	}
	teacher.ID = a.ID
	if err := teacher.Put(c); err != nil {
		return nil, fmt.Errorf("failed to store new teacher for %s: %s", a.Email, err)
	}
	return teacher, nil
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	announcements := staff.CurrentAnnouncements(c, time.Now())
	sort.Sort(staff.AnnouncementsByExpiration(announcements))
	data := map[string]interface{}{
		"Announcements": announcements,
	}
	if u := user.Current(c); u != nil {
		acct, err := maybeOldAccount(c, u)
		switch err {
		case nil:
			break
		case account.ErrUserNotFound:
			http.Redirect(w, r, "/login/new", http.StatusSeeOther)
			return nil
		default:
			return webapp.InternalError(fmt.Errorf("failed to find user: %s", err))
		}
		data["LoggedIn"] = true
		data["User"] = acct
		if url, err := user.LogoutURL(c, "/"); err != nil {
			return webapp.InternalError(fmt.Errorf("Error creating logout url: %s", err))
		} else {
			data["LogoutURL"] = url
		}
		switch staffer, err := maybeOldStaff(c, acct, u); err {
		case nil:
			data["Staff"] = staffer
		case staff.ErrUserIsNotStaff:
			break
		default:
			return webapp.InternalError(err)
		}
		switch teacher, err := maybeOldTeacher(c, acct, u); err {
		case nil:
			data["Teacher"] = teacher
		case classes.ErrUserIsNotTeacher:
			break
		default:
			return webapp.InternalError(err)
		}
		data["Admin"] = user.IsAdmin(c)

		/*
			registrar := model.NewRegistrar(c, u)
			data["Registrations"] = registrar.ListRegisteredClasses(registrar.ListRegistrations())
		*/
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func class(w http.ResponseWriter, r *http.Request) *webapp.Error {
	_, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{}
	if err := classPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func throwError(w http.ResponseWriter, r *http.Request) *webapp.Error {
	return webapp.InternalError(fmt.Errorf("this is an intentional error"))
}
