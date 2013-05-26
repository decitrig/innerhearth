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
	"net/url"
	"strconv"
	"time"

	"appengine"
	"appengine/user"

	"github.com/decitrig/innerhearth/model"
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
	http.Handle("/", webapp.Router)
	webapp.HandleFunc("/", index)
	webapp.HandleFunc("/login", login)
	webapp.HandleFunc("/_ah/login_required", login)
	webapp.HandleFunc("/class", class)

	webapp.HandleFunc("/about", staticTemplate("templates/about.html"))
	webapp.HandleFunc("/pricing", staticTemplate("templates/pricing.html"))
	webapp.HandleFunc("/teachers", staticTemplate("templates/teachers.html"))
}

func groupByDay(data []*model.ClassCalendarData) [][]*model.ClassCalendarData {
	days := make([][]*model.ClassCalendarData, 7)
	for _, d := range data {
		idx := int(d.Weekday) - 1
		if idx < 0 {
			idx = 6
		}
		days[idx] = append(days[idx], d)
	}
	return days
}

type session struct {
	*model.Session
	Classes [][]*model.ClassCalendarData
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	sessions := []session{}
	for _, s := range model.ListSessions(c, time.Now()) {
		classes := s.ListClasses(c)
		if len(classes) == 0 {
			c.Infof("session %q has no classes", s.Name)
			continue
		}
		sessions = append(sessions, session{s, groupByDay(classes)})
	}
	data := map[string]interface{}{
		"Announcements": model.ListAnnouncements(c),
		"Sessions":      sessions,
	}
	if u := webapp.GetCurrentUser(r); u != nil {
		data["LoggedIn"] = true
		data["User"] = u
		if url, err := user.LogoutURL(c, "/"); err != nil {
			return webapp.InternalError(fmt.Errorf("Error creating logout url: %s", err))
		} else {
			data["LogoutURL"] = url
		}
		data["Staff"] = model.GetStaff(c, u) != nil
		data["Teacher"] = model.GetTeacher(c, u) != nil
		data["Admin"] = user.IsAdmin(c)

		registrar := model.NewRegistrar(c, u)
		data["Registrations"] = registrar.ListRegisteredClasses(registrar.ListRegistrations())
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

type directProvider struct {
	Name       string
	Identifier string
}

type directProviderLink struct {
	Name string
	URL  string
}

var (
	directProviders = []directProvider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "yahoo.com"},
		{"AOL", "aol.com"},
		{"MyOpenID", "myopenid.com"},
	}
)

func login(w http.ResponseWriter, r *http.Request) *webapp.Error {
	redirect, err := url.Parse("/login/account")
	if err != nil {
		return webapp.InternalError(err)
	}
	q := redirect.Query()
	q.Set("continue", webapp.PathOrRoot(r.FormValue("continue")))
	redirect.RawQuery = q.Encode()
	c := appengine.NewContext(r)
	directProviderLinks := []*directProviderLink{}
	for _, provider := range directProviders {
		url, err := user.LoginURLFederated(c, redirect.String(), provider.Identifier)
		if err != nil {
			c.Errorf("Error creating URL for %s: %s", provider.Name, err)
			continue
		}
		directProviderLinks = append(directProviderLinks, &directProviderLink{
			Name: provider.Name,
			URL:  url,
		})
	}
	if err := loginPage.Execute(w, map[string]interface{}{
		"DirectProviders": directProviderLinks,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func class(w http.ResponseWriter, r *http.Request) *webapp.Error {
	classID, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		return webapp.InternalError(err)
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	user := webapp.GetCurrentUser(r)
	data := map[string]interface{}{
		"Class": scheduler.GetCalendarData(class),
		"User":  user,
		"Token": webapp.GetXSRFToken(r),
	}
	if user != nil {
		roster := model.NewRoster(c, class)
		data["Student"] = roster.LookupStudent(user.Email)
	}
	if err := classPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
