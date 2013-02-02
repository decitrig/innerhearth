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
		"indexAsWeekday": func(i int) time.Weekday { return time.Weekday(i) },
	}).ParseFiles("templates/base.html", "templates/staff/index.html"))
	addTeacherPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-teacher.html"))
	addClassPage        = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-class.html"))
	deleteClassPage     = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-class.html"))
	rescheduleClassPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"weekdayEquals": func(a, b time.Weekday) bool { return a == b },
	}).ParseFiles("templates/base.html", "templates/staff/reschedule-class.html"))
)

func staffOnly(handler webapp.AppHandler) webapp.AppHandlerFunc {
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

func handle(path string, handler webapp.AppHandler) {
	webapp.AppHandleFunc(path, staffOnly(handler))
}

func handleFunc(path string, fn webapp.AppHandlerFunc) {
	webapp.AppHandleFunc(path, staffOnly(fn))
}

func init() {
	handleFunc("/staff", staff)
	handleFunc("/staff/add-teacher", addTeacher)
	handleFunc("/staff/add-class", addClass)
	handleFunc("/staff/delete-class", deleteClass)
	handleFunc("/staff/reschedule-class", rescheduleClass)
}

func groupByDay(data []*model.ClassCalendarData) [][]*model.ClassCalendarData {
	days := make([][]*model.ClassCalendarData, 7)
	for _, d := range data {
		days[d.Weekday] = append(days[d.Weekday], d)
	}
	return days
}

func staff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListOpenClasses()
	classesByDay := groupByDay(scheduler.ListCalendarData(classes))
	data := map[string]interface{}{
		"Teachers": model.ListTeachers(c),
		"Classes":  classesByDay,
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

func parseSessionClass(r *http.Request) (*model.Class, error) {
	fields, err := webapp.ParseRequiredValues(r, "name", "description", "teacher", "maxstudents",
		"dayofweek", "starttime", "length", "startdate", "enddate")
	if err != nil {
		return nil, err
	}
	class := &model.Class{
		Title:           fields["name"],
		LongDescription: []byte(fields["description"]),
	}
	if cap, err := strconv.ParseInt(fields["maxstudents"], 10, 32); err != nil {
		return nil, fmt.Errorf("Error parsing max students %s: %s", fields["maxstudents"], err)
	} else {
		class.Capacity = int32(cap)
	}
	if dayNum, err := strconv.ParseInt(fields["dayofweek"], 10, 0); err != nil {
		return nil, fmt.Errorf("Error parsing weekday %s: %s", fields["dayofweek"], err)
	} else {
		class.Weekday = time.Weekday(dayNum)
	}
	if class.StartTime, err = time.Parse("15:04", fields["starttime"]); err != nil {
		return nil, fmt.Errorf("Error parsing start time %s: %s", fields["starttime"], err)
	}
	if lengthMinutes, err := strconv.ParseInt(fields["length"], 10, 32); err != nil {
		return nil, fmt.Errorf("Error parsing class length %s: %s", fields["length"], err)
	} else {
		class.Length = time.Duration(lengthMinutes) * time.Minute
	}
	if class.BeginDate, err = time.Parse("2006-01-02", fields["startdate"]); err != nil {
		return nil, fmt.Errorf("Error parsing start date %s: %s", fields["startdate"], err)
	}
	if class.EndDate, err = time.Parse("2006-01-02", fields["enddate"]); err != nil {
		return nil, fmt.Errorf("Error parsing end date %s: %s", fields["enddate"], err)
	}
	class.DropInOnly = false
	return class, nil
}

func parseDropInOnlyClass(r *http.Request) (*model.Class, error) {
	fields, err := webapp.ParseRequiredValues(r, "name", "description", "teacher", "maxstudents",
		"dayofweek", "starttime", "length")
	if err != nil {
		return nil, err
	}
	class := &model.Class{
		Title:           fields["name"],
		LongDescription: []byte(fields["description"]),
	}
	if cap, err := strconv.ParseInt(fields["maxstudents"], 10, 32); err != nil {
		return nil, fmt.Errorf("Error parsing max students %s: %s", fields["maxstudents"], err)
	} else {
		class.Capacity = int32(cap)
	}
	if dayNum, err := strconv.ParseInt(fields["dayofweek"], 10, 0); err != nil {
		return nil, fmt.Errorf("Error parsing weekday %s: %s", fields["dayofweek"], err)
	} else {
		class.Weekday = time.Weekday(dayNum)
	}
	if class.StartTime, err = time.Parse("15:04", fields["starttime"]); err != nil {
		return nil, fmt.Errorf("Error parsing start time %s: %s", fields["starttime"], err)
	}
	if lengthMinutes, err := strconv.ParseInt(fields["length"], 10, 32); err != nil {
		return nil, fmt.Errorf("Error parsing class length %s: %s", fields["length"], err)
	} else {
		class.Length = time.Duration(lengthMinutes) * time.Minute
	}
	class.DropInOnly = true
	return class, nil
}

func addClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.InternalError(fmt.Errorf("Invalid XSRF token %s", t))
		}
		teacher := model.GetAccountByEmail(c, r.FormValue("teacher"))
		if teacher == nil {
			return webapp.InternalError(fmt.Errorf("No such teacher %s", r.FormValue("teacher")))
		}

		var class *model.Class
		switch typ := r.FormValue("type"); typ {
		case "session":
			var err error
			class, err = parseSessionClass(r)
			if err != nil {
				return webapp.InternalError(fmt.Errorf("Couldn't parse class from post: %s", err))
			}

		case "dropin":
			var err error
			class, err = parseDropInOnlyClass(r)
			if err != nil {
				return webapp.InternalError(fmt.Errorf("Couldn't parse class from post: %s", err))
			}

		default:
			return webapp.InternalError(fmt.Errorf("Unknown class type '%s'", typ))
		}

		class.Teacher = model.MakeTeacherKey(c, teacher)
		s := model.NewScheduler(c)
		if err := s.AddNew(class); err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't add class: %s", err))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}

	data := map[string]interface{}{
		"Teachers": model.ListTeachers(c),
		"Token":    token,
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
		if class.StartTime, err = time.Parse("15:04", fields["starttime"]); err != nil {
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
			if class.BeginDate, err = time.Parse("2006-01-02", dates["startdate"]); err != nil {
				return webapp.InternalError(fmt.Errorf("Error parsing start date %s: %s", dates["startdate"], err))
			}
			if class.EndDate, err = time.Parse("2006-01-02", dates["enddate"]); err != nil {
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
