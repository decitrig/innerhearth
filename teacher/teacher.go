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

package teacher

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"github.com/gorilla/context"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

const (
	teacherContextKey = "teacherContextKey"
	classContextKey   = "classContextKey"
)

var (
	indexPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": func(i int) time.Weekday { return time.Weekday(i) },
	}).ParseFiles("templates/base.html", "templates/teacher/index.html"))
	rosterPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"weekdayAsIndex": func(w time.Weekday) int { return int(w) },
	}).ParseFiles("templates/base.html", "templates/teacher/roster.html"))
)

func init() {
	webapp.HandleFunc("/teacher", teachersOnly(webapp.HandlerFunc(index)))
	webapp.HandleFunc("/teacher/roster", classTeacherOrStaffOnly(webapp.HandlerFunc(roster)))
}

func setTeacher(r *http.Request, t *model.Teacher) {
	context.Set(r, teacherContextKey, t)
}

func getTeacher(r *http.Request) *model.Teacher {
	return context.Get(r, teacherContextKey).(*model.Teacher)
}

func setClass(r *http.Request, c *model.Class) {
	context.Set(r, classContextKey, c)
}

func getClass(r *http.Request) *model.Class {
	return context.Get(r, classContextKey).(*model.Class)
}

func teachersOnly(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		u := webapp.GetCurrentUser(r)
		if u == nil {
			webapp.RedirectToLogin(w, r, r.URL.Path)
			return nil
		}
		c := appengine.NewContext(r)
		teacher := model.GetTeacher(c, u)
		if teacher == nil {
			return webapp.UnauthorizedError(fmt.Errorf("No Teacher for account %s", u.AccountID))
		}
		setTeacher(r, teacher)
		return handler.Serve(w, r)
	}
}

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
		setClass(r, class)

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

func groupByDay(classes []*model.Class) [][]*model.Class {
	days := make([][]*model.Class, 7)
	for _, c := range classes {
		days[c.Weekday] = append(days[c.Weekday], c)
	}
	return days
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	teacher := getTeacher(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClassesForTeacher(teacher)
	data := map[string]interface{}{
		"Teacher": teacher,
		"Classes": groupByDay(classes),
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func roster(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	roster := model.NewRoster(c, getClass(r))
	regs := roster.ListRegistrations()
	data := map[string]interface{}{
		"Class":         getClass(r),
		"Registrations": roster.GetStudents(regs),
		"Token":         webapp.GetXSRFToken(r),
	}
	if err := rosterPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
