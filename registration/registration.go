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
	"sort"
	"strconv"
	"sync"
	"time"

	"appengine"
	"appengine/taskqueue"
	"appengine/user"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	registrationForm    = template.Must(template.ParseFiles("registration/form.html"))
	newRegistrationPage = template.Must(template.ParseFiles("registration/registration-new.html"))
	registrationConfirm = template.Must(template.ParseFiles("registration/registration-confirm.html"))
	teacherPage         = template.Must(template.ParseFiles("registration/teacher.html"))
	teacherRosterPage   = template.Must(template.New("roster.html").
				Funcs(template.FuncMap{"dayNumber": dayNumber}).
				ParseFiles("registration/roster.html"))
	teacherRegisterPage = template.Must(template.ParseFiles("registration/teacher-register.html"))
	dropinPage          = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"dayNumber": dayNumber,
	}).ParseFiles("templates/base.html", "templates/registration/dropin.html"))

	classFullPage         = template.Must(template.ParseFiles("templates/base.html", "templates/registration/class-full.html"))
	alreadyRegisteredPage = template.Must(template.ParseFiles("templates/base.html", "templates/registration/already-registered.html"))
)

var days = map[string]int{
	"Sunday":    0,
	"Monday":    1,
	"Tuesday":   2,
	"Wednesday": 3,
	"Thursday":  4,
	"Friday":    5,
	"Saturday":  6,
}

func dayNumber(day string) int {
	return days[day]
}

type requestVariable struct {
	lock sync.Mutex
	m    map[*http.Request]interface{}
}

func (v *requestVariable) Get(r *http.Request) interface{} {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.m == nil {
		return nil
	}
	return v.m[r]
}

func (v *requestVariable) Set(r *http.Request, val interface{}) {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.m == nil {
		v.m = map[*http.Request]interface{}{}
	}
	v.m[r] = val
}

type requestUser struct {
	*user.User
	*model.UserAccount
}

var (
	userVariable  = &requestVariable{}
	tokenVariable = &requestVariable{}
)

type appError struct {
	Error   error
	Message string
	Code    int
}

type handler func(w http.ResponseWriter, r *http.Request) *appError

func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Error: %s", err.Error)
		http.Error(w, err.Message, err.Code)
	}
}

func postOnly(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		if r.Method != "POST" {
			return &appError{fmt.Errorf("GET access to %s", r.URL), "Not Found", 404}
		}
		return handler(w, r)
	}
}

func needsUser(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		c := appengine.NewContext(r)
		u := user.Current(c)
		if u == nil {
			return &appError{fmt.Errorf("No logged in user"), "An error occurred", http.StatusInternalServerError}
		}
		account, err := model.GetAccount(c, u)
		if err != nil {
			http.Redirect(w, r, "/login/account?continue="+r.URL.Path, http.StatusSeeOther)
			return nil
		}
		userVariable.Set(r, &requestUser{u, account})
		return handler(w, r)
	}
}

func getRequestUser(r *http.Request) *requestUser {
	return userVariable.Get(r).(*requestUser)
}

func xsrfProtected(handler handler) handler {
	return needsUser(func(w http.ResponseWriter, r *http.Request) *appError {
		u := userVariable.Get(r).(*requestUser)
		if u == nil {
			return &appError{fmt.Errorf("No user in request"), "An error ocurred", http.StatusInternalServerError}
		}
		c := appengine.NewContext(r)
		token, err := model.GetXSRFToken(c, u.AccountID)
		if err != nil {
			return &appError{
				fmt.Errorf("Could not get XSRF token for id %s: %s", u.AccountID, err),
				"An error occurred",
				http.StatusInternalServerError,
			}
		}
		tokenVariable.Set(r, token)
		if r.Method == "POST" && !token.Validate(r.FormValue("xsrf_token")) {
			return &appError{fmt.Errorf("Invalid XSRF token"), "Unauthorized", http.StatusUnauthorized}
		}
		return handler(w, r)
	})
}

func teachersOnly(handler handler) handler {
	return xsrfProtected(func(w http.ResponseWriter, r *http.Request) *appError {
		u := getRequestUser(r)
		if !(u.Role.IsStaff() || u.Role.CanTeach()) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil
		}
		return handler(w, r)
	})
}

func init() {
	http.Handle("/registration", handler(xsrfProtected(registration)))
	http.Handle("/registration/new", handler(xsrfProtected(newRegistration)))
	http.Handle("/registration/teacher/register", handler(teachersOnly(teacherRegister)))
	http.Handle("/registration/dropin", handler(xsrfProtected(dropin)))

	webapp.HandleFunc("/registration/session", sessionRegistration)
	webapp.HandleFunc("/registration/oneday", oneDayRegistration)
}

func filterRegisteredClasses(classes, registered []*model.Class) []*model.Class {
	if len(registered) == 0 || len(classes) == 0 {
		return classes
	}
	ids := map[int64]bool{}
	for _, r := range registered {
		ids[r.ID] = true
	}
	filtered := []*model.Class{}
	for _, c := range classes {
		if !ids[c.ID] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

type registrations []*model.Registration

func (r registrations) Len() int {
	return len(r)
}

func (r registrations) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

type ByDate struct {
	registrations
}

func (s ByDate) Less(i, j int) bool {
	a := s.registrations[i]
	b := s.registrations[j]
	return a.Date.Before(b.Date)
}

type regAndStudent struct {
	*model.Registration
	*model.UserAccount
}

func teacherRegister(w http.ResponseWriter, r *http.Request) *appError {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return nil
	}
	fields, err := getRequiredFields(r, "xsrf_token", "class", "firstname", "lastname", "email", "type")
	if err != nil {
		return &appError{err, "Missing required fields", http.StatusBadRequest}
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(mustParseInt(fields["class"], 64))
	if class == nil {
		return &appError{
			fmt.Errorf("Couldn't find class %d", fields["class"]),
			"Error looking up class",
			http.StatusInternalServerError,
		}
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
			return &appError{err, "An error occurred", http.StatusInternalServerError}
		}
	}
	{
		roster := model.NewRoster(c, class)
		switch t := fields["type"]; t {
		case "dropin":
			day, err := time.Parse("2006-01-02", r.FormValue("date"))
			if err != nil {
				return &appError{err, "Wrong time format entered: use YYYY-MM-DD", http.StatusBadRequest}
			}
			if _, err = roster.AddDropIn(account.AccountID, day); err != nil {
				if err == model.ErrAlreadyRegistered {
					return &appError{err, "Student already registered for this class", http.StatusBadRequest}
				}
				return internalError("Error registering drop in: %s", err)
			}
		case "session":
			if _, err = roster.AddStudent(account.AccountID); err != nil {
				if err == model.ErrAlreadyRegistered {
					return &appError{err, "Student already registered for this class", http.StatusBadRequest}
				}
				return internalError("Error registering session: %s", err)
			}
		default:
			return internalError("Invalid registration type '%s'", t)
		}
	}
	http.Redirect(w, r, fmt.Sprintf("/registration/teacher/roster?class=%d", class.ID), http.StatusFound)
	return nil
}

type regs []*regAndClass

func (r regs) Len() int {
	return len(r)
}

func (r regs) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

type classes []*model.Class

func (cs classes) Len() int {
	return len(cs)
}

func (cs classes) Swap(i, j int) {
	cs[i], cs[j] = cs[j], cs[i]
}

type byDayThenTime struct{ classes }
type regsByDayThenTime struct{ regs }

var weekdays = []string{
	"Monday",
	"Tuesday",
	"Wednesday",
	"Thursday",
	"Friday",
	"Saturday",
	"Sunday",
}

func dayLess(d1, d2 string) bool {
	var i, j int
	for idx, day := range weekdays {
		if day == d1 {
			i = idx
		}
		if day == d2 {
			j = idx
		}
	}
	return i < j
}

func (s byDayThenTime) Less(i, j int) bool {
	l, r := s.classes[i], s.classes[j]
	if l.DayOfWeek != r.DayOfWeek {
		return dayLess(l.DayOfWeek, r.DayOfWeek)
	}
	if l.StartTime != r.StartTime {
		return l.StartTime.Before(r.StartTime)
	}
	return l.ID < r.ID
}

func (rs regsByDayThenTime) Less(i, j int) bool {
	l, r := rs.regs[i], rs.regs[j]
	if l.DayOfWeek != r.DayOfWeek {
		return dayLess(l.DayOfWeek, r.DayOfWeek)
	}
	if l.StartTime != r.StartTime {
		return l.StartTime.Before(r.StartTime)
	}
	return l.ID < r.ID
}

func sessionClassesOnly(in []*model.Class) []*model.Class {
	out := []*model.Class{}
	for _, c := range in {
		if !c.DropInOnly {
			out = append(out, c)
		}
	}
	return out
}

type regAndClass struct {
	*model.Registration
	*model.Class
}

func registration(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	u := userVariable.Get(r).(*requestUser)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListOpenClasses()
	teachers := scheduler.GetTeacherNames(classes)
	sort.Sort(byDayThenTime{classes})
	logout, err := user.LogoutURL(c, "/registration")
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	data := map[string]interface{}{
		"SessionClasses": classes,
		"XSRFToken":      token.Token,
		"LogoutURL":      logout,
		"Account":        u.UserAccount,
		"IsAdmin":        u.Role.IsStaff(),
		"Teachers":       teachers,
	}
	if err := registrationForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func newRegistration(w http.ResponseWriter, r *http.Request) *appError {
	u := userVariable.Get(r).(*requestUser)
	c := appengine.NewContext(r)
	if u.Confirmed.IsZero() {
		http.Redirect(w, r, "/registration", http.StatusSeeOther)
		return nil
	}
	classID := mustParseInt(r.FormValue("class"), 64)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	teacher := scheduler.GetTeacher(class)
	if class == nil {
		return &appError{fmt.Errorf("Couldn't find class %d", classID),
			"An error occurred, please go back and try again",
			http.StatusInternalServerError}
	}
	if r.Method == "POST" {
		roster := model.NewRoster(c, class)
		if _, err := roster.AddStudent(u.AccountID); err != nil {
			if err == model.ErrClassFull {
				if err2 := classFullPage.Execute(w, map[string]interface{}{
					"Class": class,
				}); err2 != nil {
					return &appError{err, "An error occurred", http.StatusInternalServerError}
				}
				return nil
			}
			return &appError{
				fmt.Errorf("Error when registering student %s in class %d: %s", u.AccountID, class.ID, err),
				"An error occurred, please go back and try again.",
				http.StatusInternalServerError,
			}
		}
		t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
			"account": {u.AccountID},
			"class":   {fmt.Sprintf("%d", class.ID)},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			return &appError{fmt.Errorf("Error enqueuing email task for registration: %s", err),
				"An error occurred, please go back and try again",
				http.StatusInternalServerError}
		}
		data := map[string]interface{}{
			"Email":   u.UserAccount.Email,
			"Class":   class,
			"Teacher": teacher,
		}
		if err := newRegistrationPage.Execute(w, data); err != nil {
			return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
		}
		return nil
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	if err := registrationConfirm.Execute(w, map[string]interface{}{
		"XSRFToken": token.Token,
		"Class":     class,
	}); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func internalError(format string, vals ...interface{}) *appError {
	return &appError{fmt.Errorf(format, vals...), "An internal error occurred", http.StatusInternalServerError}
}

func dropin(w http.ResponseWriter, r *http.Request) *appError {
	classID, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return internalError("Couldn't parse class ID '%s': %s", r.FormValue("class"), err)
	}
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return internalError("Couldn't find class %d", classID)
	}
	if r.Method == "POST" {
		date, err := time.Parse("2006-01-02", r.FormValue("date"))
		if err != nil {
			return internalError("Error parsing date '%s': %s", r.FormValue("date"), err)
		}
		roster := model.NewRoster(c, class)
		account := userVariable.Get(r).(*requestUser)
		if _, err := roster.AddDropIn(account.AccountID, date); err != nil {
			if err == model.ErrClassFull {
				return &appError{
					err,
					"Sorry, that class is full. Please go back and choose another class.",
					http.StatusOK,
				}
			}
			if err == model.ErrInvalidDropInDate {
				return &appError{
					err,
					"Sorry, that is an invalid date for that class. Please go back and choose another date.",
					http.StatusOK,
				}
			}
			if err == model.ErrAlreadyRegistered {
				return &appError{
					err,
					"You appear to alredy be registered for that class. Please go back and choose another class.",
					http.StatusOK,
				}
			}
			return internalError("Error registering dropin: %s", err)
		}
		t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
			"account": {account.AccountID},
			"class":   {fmt.Sprintf("%d", class.ID)},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			return &appError{fmt.Errorf("Error enqueuing email task for registration: %s", err),
				"An error occurred, please go back and try again",
				http.StatusInternalServerError}
		}
		http.Redirect(w, r, "/registration", http.StatusFound)
		return nil
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	if err := dropinPage.Execute(w, map[string]interface{}{
		"Class": class,
		"Token": token,
	}); err != nil {
		return internalError("Error executing template: %s", err)
	}
	return nil
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
	http.Redirect(w, r, "/", http.StatusSeeOther)
	return nil
}
