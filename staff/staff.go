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

package staff

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"appengine"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	staffPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": func(i int) time.Weekday { return time.Weekday((i + 1) % 7) },
	}).ParseFiles("templates/base.html", "templates/staff/index.html"))
	addTeacherPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-teacher.html"))
	addClassPage        = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-class.html"))
	addSessionPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-session.html"))
	deleteClassPage     = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-class.html"))
	editClassPage       = template.Must(template.ParseFiles("templates/base.html", "templates/staff/edit-class.html"))
	rescheduleClassPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"weekdayEquals": func(a, b time.Weekday) bool { return a == b },
	}).ParseFiles("templates/base.html", "templates/staff/reschedule-class.html"))
	sessionPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": func(i int) time.Weekday { return time.Weekday((i + 1) % 7) },
	}).ParseFiles("templates/base.html", "templates/staff/session.html"))
	addAnnouncementPage    = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-announcement.html"))
	deleteAnnouncementPage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-announcement.html"))
)

const (
	dateFormat = "01/02/2006"
	timeFormat = "3:04pm"
)

func staffOnly(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		u := webapp.GetCurrentUser(r)
		if u == nil {
			webapp.RedirectToLogin(w, r, r.URL.Path)
			return nil
		}
		c := appengine.NewContext(r)
		if model.GetStaff(c, u) == nil {
			return webapp.UnauthorizedError(fmt.Errorf("%s is not staff", u.Email))
		}
		return handler.Serve(w, r)
	}
}

func handle(path string, handler webapp.Handler) {
	webapp.HandleFunc(path, staffOnly(handler))
}

func handleFunc(path string, fn webapp.HandlerFunc) {
	webapp.HandleFunc(path, staffOnly(fn))
}

func init() {
	handleFunc("/staff", staff)
	handleFunc("/staff/add-teacher", addTeacher)
	handleFunc("/staff/add-class", addClass)
	handleFunc("/staff/add-session", addSession)
	handleFunc("/staff/delete-class", deleteClass)
	handleFunc("/staff/edit-class", editClass)
	handleFunc("/staff/reschedule-class", rescheduleClass)
	handleFunc("/staff/session", session)
	handleFunc("/staff/add-announcement", addAnnouncement)
	handleFunc("/staff/delete-announcement", deleteAnnouncement)
}

func groupByDay(data []*model.ClassCalendarData) [][]*model.ClassCalendarData {
	days := make([][]*model.ClassCalendarData, 7)
	for _, d := range data {
		idx := d.Weekday - 1
		if idx < 0 {
			idx = 6
		}
		days[idx] = append(days[idx], d)
	}
	return days
}

func groupByDay2(data []*model.Class) [][]*model.Class {
	days := make([][]*model.Class, 7)
	for _, d := range data {
		idx := d.Weekday - 1
		if idx < 0 {
			idx = 6
		}
		days[idx] = append(days[idx], d)
	}
	return days
}

func staff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListAllClasses()
	classesByDay := groupByDay(scheduler.ListCalendarData(classes))
	sessions := model.ListSessions(c, time.Now())
	data := map[string]interface{}{
		"Teachers":      model.ListTeachers(c),
		"Classes":       classesByDay,
		"Announcements": model.ListAnnouncements(c),
		"Sessions":      sessions,
	}
	if err := staffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addTeacher(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	vals, err := webapp.ParseRequiredValues(r, "email")
	if err != nil {
		return webapp.InternalError(err)
	}
	account := model.GetAccountByEmail(c, vals["email"])
	if account == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find account for '%s'", vals["email"]))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.UnauthorizedError(fmt.Errorf("Unauthorized XSRF token given: %s", t))
		}
		if teacher := model.AddNewTeacher(c, account); teacher == nil {
			return webapp.InternalError(fmt.Errorf("Couldn't create teacher for %s", account.Email))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	data := map[string]interface{}{
		"Token": token.Token,
		"User":  account,
	}
	if err := addTeacherPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func mustParseInt(s string, size int) int64 {
	n, err := strconv.ParseInt(s, 10, size)
	if err != nil {
		panic(err)
	}
	return n
}

func mustParseTime(layout, value string) time.Time {
	t, err := time.Parse(layout, value)
	if err != nil {
		panic(err)
	}
	return t
}

func addClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	sessionID, err := strconv.ParseInt(r.FormValue("session"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("couldn't parse session id %q", r.FormValue("session")))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}

		teacher := model.GetAccountByEmail(c, r.FormValue("teacher"))
		if teacher == nil {
			return webapp.InternalError(fmt.Errorf("No such teacher %s", r.FormValue("teacher")))
		}

		fields, err := webapp.ParseRequiredValues(r, "name", "description", "teacher", "maxstudents",
			"dayofweek", "starttime", "length")
		class := &model.Class{
			Title:           fields["name"],
			LongDescription: []byte(fields["description"]),
			Session:         sessionID,
			Teacher:         model.MakeTeacherKey(c, teacher),
		}
		if cap, err := strconv.ParseInt(fields["maxstudents"], 10, 32); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing max students %s: %s", fields["maxstudents"], err))
		} else {
			class.Capacity = int32(cap)
		}
		if dayNum, err := strconv.ParseInt(fields["dayofweek"], 10, 0); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing weekday %s: %s", fields["dayofweek"], err))
		} else {
			class.Weekday = time.Weekday(dayNum)
		}
		if class.StartTime, err = time.Parse(timeFormat, fields["starttime"]); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing start time %s: %s", fields["starttime"], err))
		}
		if lengthMinutes, err := strconv.ParseInt(fields["length"], 10, 32); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing class length %s: %s", fields["length"], err))
		} else {
			class.Length = time.Duration(lengthMinutes) * time.Minute
		}
		c.Infof(fmt.Sprintf("%+v", class))

		s := model.NewScheduler(c)
		if err := s.AddNew(class); err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't add class: %s", err))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	session := model.GetSession(c, sessionID)
	if session == nil {
		return webapp.InternalError(fmt.Errorf("couldn't find session %d", sessionID))
	}
	data := map[string]interface{}{
		"Teachers": model.ListTeachers(c),
		"Token":    token,
		"Session":  session,
	}
	if err := addClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func deleteClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Error parsing class id %s: %s", r.FormValue("class"), err))
	}
	c := appengine.NewContext(r)
	s := model.NewScheduler(c)
	class := s.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}
		if err := s.DeleteClass(class); err != nil {
			return webapp.InternalError(fmt.Errorf("Error deleting class %d: %s", class.ID, err))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	data := map[string]interface{}{
		"Class": s.GetCalendarData(class),
		"Token": token,
	}
	if err := deleteClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func rescheduleClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Error parsing class id %s: %s", r.FormValue("class"), err))
	}
	c := appengine.NewContext(r)
	s := model.NewScheduler(c)
	class := s.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}

		fields, err := webapp.ParseRequiredValues(r, "dayofweek", "starttime", "length")
		if err != nil {
			return webapp.InternalError(err)
		}
		if dayNum, err := strconv.ParseInt(fields["dayofweek"], 10, 0); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing weekday %s: %s", fields["dayofweek"], err))
		} else {
			class.Weekday = time.Weekday(dayNum)
		}
		if class.StartTime, err = time.Parse(timeFormat, fields["starttime"]); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing start time %s: %s", fields["starttime"], err))
		}
		if lengthMinutes, err := strconv.ParseInt(fields["length"], 10, 32); err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing class length %s: %s", fields["length"], err))
		} else {
			class.Length = time.Duration(lengthMinutes) * time.Minute
		}

		if !class.DropInOnly {
			dates, err := webapp.ParseRequiredValues(r, "startdate", "enddate")
			if err != nil {
				return webapp.InternalError(err)
			}
			if class.BeginDate, err = time.Parse(dateFormat, dates["startdate"]); err != nil {
				return webapp.InternalError(fmt.Errorf("Error parsing start date %s: %s", dates["startdate"], err))
			}
			if class.EndDate, err = time.Parse(dateFormat, dates["enddate"]); err != nil {
				return webapp.InternalError(fmt.Errorf("Error parsing end date %s: %s", dates["enddate"], err))
			}
		}
		if err := s.WriteClass(class); err != nil {
			return webapp.InternalError(fmt.Errorf("Error writing class: %s", err))
		}
		http.Redirect(w, r, "/staff", http.StatusFound)
		return nil
	}
	data := map[string]interface{}{
		"Class": s.GetCalendarData(class),
		"Token": token,
	}
	if err := rescheduleClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func editClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Error parsing class id %s: %s", r.FormValue("class"), err))
	}
	c := appengine.NewContext(r)
	s := model.NewScheduler(c)
	class := s.GetClass(classID)
	if class == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find class %d", classID))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}
		fields, err := webapp.ParseRequiredValues(r, "title", "description")
		if err != nil {
			return webapp.InternalError(err)
		}
		class.Title = fields["title"]
		class.LongDescription = []byte(fields["description"])
		if err := s.WriteClass(class); err != nil {
			return webapp.InternalError(fmt.Errorf("Error writing class %q: %s", classID, err))
		}
		http.Redirect(w, r, "/staff", http.StatusFound)
		return nil
	}
	data := map[string]interface{}{
		"Class": class,
		"Token": token,
	}
	if err := editClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addAnnouncement(w http.ResponseWriter, r *http.Request) *webapp.Error {
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}

		fields, err := webapp.ParseRequiredValues(r, "text", "expiration")
		if err != nil {
			return webapp.InternalError(err)
		}
		expiration, err := time.Parse(dateFormat, fields["expiration"])
		if err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing expiration date %s: %s", fields["expiration"], err))
		}

		c := appengine.NewContext(r)
		if a := model.NewAnnouncement(c, fields["text"], expiration); a == nil {
			return webapp.InternalError(fmt.Errorf("Didn't write announcement."))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	if err := addAnnouncementPage.Execute(w, map[string]interface{}{
		"Token": token,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func deleteAnnouncement(w http.ResponseWriter, r *http.Request) *webapp.Error {
	token := webapp.GetXSRFToken(r)
	c := appengine.NewContext(r)
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't parse %s as announcement id: %s", r.FormValue("id"), err))
	}
	if r.Method == "POST" {
		if !token.Validate(r.FormValue("xsrf_token")) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token"))
		}
		model.DeleteAnnouncement(c, id)
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	a := model.GetAnnouncement(c, id)
	if a == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find announcement %d", id))
	}
	if err := deleteAnnouncementPage.Execute(w, map[string]interface{}{
		"Token":        token,
		"Announcement": a,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addSession(w http.ResponseWriter, r *http.Request) *webapp.Error {
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}
		fields, err := webapp.ParseRequiredValues(r, "name", "startdate", "enddate")
		if err != nil {
			return webapp.InternalError(err)
		}
		startDate, err := time.Parse(dateFormat, fields["startdate"])
		if err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing start date date %q: %s", fields["expiration"], err))
		}
		endDate, err := time.Parse(dateFormat, fields["enddate"])
		if err != nil {
			return webapp.InternalError(fmt.Errorf("Error parsing end date date %q: %s", fields["expiration"], err))
		}
		session := &model.Session{
			Name:  fields["name"],
			Start: startDate,
			End:   endDate,
		}
		c := appengine.NewContext(r)
		if err := model.AddSession(c, session); err != nil {
			return webapp.InternalError(fmt.Errorf("couldnt' write session: %s", err))
		}
		http.Redirect(w, r, "/staff", http.StatusFound)
		return nil
	}
	if err := addSessionPage.Execute(w, map[string]interface{}{
		"Token": token,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func session(w http.ResponseWriter, r *http.Request) *webapp.Error {
	token := webapp.GetXSRFToken(r)
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("couldn't parse id %q", r.FormValue("id")))
	}
	c := appengine.NewContext(r)
	session := model.GetSession(c, id)
	if session == nil {
		return webapp.InternalError(fmt.Errorf("couldn't find session %d", id))
	}
	classes := model.ListClasses(c, session)
	classesByDay := groupByDay(classes)
	if err := sessionPage.Execute(w, map[string]interface{}{
		"Session": session,
		"Token":   token,
		"Classes": classesByDay,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
