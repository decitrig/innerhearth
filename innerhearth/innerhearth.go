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
	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/classes"
	"github.com/decitrig/innerhearth/staff"
	"github.com/decitrig/innerhearth/students"
	"github.com/decitrig/innerhearth/webapp"
	"github.com/decitrig/innerhearth/yogassage"
)

var (
	indexPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"ClassesByDay": classesByDay,
		"FormatLocal":  formatLocal,
		"TeacherName":  teacherName,
	}).ParseFiles("templates/base.html", "templates/index.html"))
	loginPage = template.Must(template.ParseFiles("templates/base.html", "templates/login.html"))
	classPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"WeekdayAsInt": weekdayAsInt,
		"FormatLocal":  formatLocal,
	}).ParseFiles("templates/base.html", "templates/class.html"))
	rosterPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"WeekdayAsInt": weekdayAsInt,
	}).ParseFiles("templates/base.html", "templates/roster.html"))
)

func weekdayEquals(a, b time.Weekday) bool { return a == b }
func weekdayAsInt(w time.Weekday) int      { return int(w) }
func minutes(d time.Duration) int64        { return int64(d.Minutes()) }
func teacherHasEmail(t *classes.Teacher, email string) bool {
	if t == nil {
		return false
	}
	return t.Email == email
}
func formatLocal(layout string, t time.Time) string {
	return t.In(local).Format(layout)
}

// This is necessary when a Teacher is field inside another struct.
func teacherName(t *classes.Teacher) string { return t.DisplayName() }

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

func classesByDay(clsses []*classes.Class) map[time.Weekday][]*classes.Class {
	return classes.GroupedByDay(clsses)
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
	webapp.Handle("/roster", userContextHandler(webapp.HandlerFunc(roster)))
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

func badRequest(w http.ResponseWriter, message string) *webapp.Error {
	// TODO(rwsims): Clean up this error reporting.
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, message)
	return nil
}

func missingFields(w http.ResponseWriter) *webapp.Error {
	return badRequest(w, "Please go back and enter all required data.")
}

func invalidData(w http.ResponseWriter, message string) *webapp.Error {
	return badRequest(w, message)
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

func maybeOldStudent(c appengine.Context, a *account.Account, u *user.User, class *classes.Class) (*students.Student, error) {
	switch student, err := students.WithIDInClass(c, a.ID, class, time.Now()); err {
	case nil:
		return student, nil
	case students.ErrStudentNotFound:
		break
	default:
		return nil, err
	}
	student, err := students.WithIDInClass(c, u.ID, class, time.Now())
	if err != nil {
		return nil, err
	}
	c.Warningf("Found student under old ID %q", u.ID)
	if err := student.Delete(c); err != nil {
		c.Errorf("Failed to delete old student: %s")
	}
	student.ID = a.ID
	if err := student.Put(c); err != nil {
		return nil, fmt.Errorf("failed to store new student for %s in %d: %s", a.Email, class.ID, err)
	}
	return student, nil
}

func storeNewToken(c appengine.Context, userID, path string) (*auth.Token, error) {
	token, err := auth.NewToken(userID, path, time.Now())
	if err != nil {
		return nil, err
	}
	if err := token.Store(c); err != nil {
		return nil, err
	}
	return token, nil
}

func checkToken(c appengine.Context, userID, path, encoded string) (*auth.Token, bool) {
	token, err := auth.TokenForRequest(c, userID, path)
	if err != nil {
		return nil, false
	}
	if token.IsValid(encoded, time.Now()) {
		return token, true
	}
	return nil, false
}

type sessionSchedule struct {
	Session         *classes.Session
	ClassesByDay    map[time.Weekday][]*classes.Class
	TeachersByClass map[int64]*classes.Teacher
}

type registration struct {
	Class   *classes.Class
	Teacher *classes.Teacher
	Student *students.Student
}

func registrationsForUser(c appengine.Context, userID string) []*registration {
	studentsForUser := students.WithID(c, userID)
	studentsForUser = students.ExceptExpiredDropIns(studentsForUser, time.Now())
	classIDs := make([]int64, len(studentsForUser))
	for i, student := range studentsForUser {
		classIDs[i] = student.ClassID
	}
	classList := classes.ClassesWithIDs(c, classIDs)
	regs := make([]*registration, len(classList))
	for i, class := range classList {
		regs[i] = &registration{
			Class:   class,
			Student: studentsForUser[i],
			Teacher: class.TeacherEntity(c),
		}
	}
	return regs
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	announcements := staff.CurrentAnnouncements(c, time.Now())
	sort.Sort(staff.AnnouncementsByExpiration(announcements))
	sessions := classes.Sessions(c, time.Now())
	schedules := []sessionSchedule{}
	for _, session := range sessions {
		sessionClasses := session.Classes(c)
		if len(sessionClasses) == 0 {
			continue
		}
		sched := sessionSchedule{
			Session:         session,
			ClassesByDay:    classes.GroupedByDay(sessionClasses),
			TeachersByClass: classes.TeachersByClass(c, sessionClasses),
		}
		schedules = append(schedules, sched)
	}
	classesBySession := make(map[int64][]*classes.Class)
	for _, session := range sessions {
		classesBySession[session.ID] = session.Classes(c)
	}
	yins := yogassage.Classes(c, dateOnly(time.Now()))
	sort.Sort(yogassage.ByDate(yins))
	data := map[string]interface{}{
		"Announcements": announcements,
		"Schedules":     schedules,
		"DaysInOrder":   daysInOrder,
		"YinYogassage":  yins,
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
		data["Admin"] = user.IsAdmin(c)
		regs := registrationsForUser(c, acct.ID)
		if len(regs) == 0 {
			regs = registrationsForUser(c, u.ID)
		}
		data["Registrations"] = regs
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func canViewRoster(s *staff.Staff, a *account.Account, classTeacher *classes.Teacher) bool {
	if s != nil {
		return true
	}
	if a != nil && classTeacher != nil {
		return a.Email == classTeacher.Email
	}
	return false
}

func class(w http.ResponseWriter, r *http.Request) *webapp.Error {
	id, err := strconv.ParseInt(r.FormValue("id"), 10, 64)
	if err != nil {
		return invalidData(w, "Couldn't parse class ID")
	}
	c := appengine.NewContext(r)
	class, err := classes.ClassWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrClassNotFound:
		return invalidData(w, "No such class")
	default:
		return webapp.InternalError(fmt.Errorf("failed to find class %d: %s", id, err))
	}
	teacher := class.TeacherEntity(c)
	data := map[string]interface{}{
		"Class":   class,
		"Teacher": teacher,
	}
	if u := user.Current(c); u != nil {
		switch a, err := maybeOldAccount(c, u); err {
		case nil:
			data["User"] = a
			staffer, _ := maybeOldStaff(c, a, u)
			data["CanViewRoster"] = canViewRoster(staffer, a, teacher)
			sessionToken, err := storeNewToken(c, a.ID, "/register/session")
			if err != nil {
				return webapp.InternalError(fmt.Errorf("failed to store token: %s"))
			}
			data["SessionToken"] = sessionToken.Encode()
			oneDayToken, err := storeNewToken(c, a.ID, "/register/oneday")
			if err != nil {
				return webapp.InternalError(fmt.Errorf("failed to store token: %s"))
			}
			data["OneDayToken"] = oneDayToken.Encode()
			switch student, err := maybeOldStudent(c, a, u, class); err {
			case nil:
				data["Student"] = student
			case students.ErrStudentNotFound:
				break
			default:
				c.Errorf("failed to find student %q in %d: %s", a.ID, class.ID, err)
			}
		case account.ErrUserNotFound:
			break
		default:
			c.Errorf("Failed to find user account: %s", err)
		}
	}
	if err := classPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func throwError(w http.ResponseWriter, r *http.Request) *webapp.Error {
	return webapp.InternalError(fmt.Errorf("this is an intentional error"))
}

func roster(w http.ResponseWriter, r *http.Request) *webapp.Error {
	id, err := strconv.ParseInt(r.FormValue("class"), 10, 64)
	if err != nil {
		return invalidData(w, "Invalid class ID")
	}
	c := appengine.NewContext(r)
	class, err := classes.ClassWithID(c, id)
	if err != nil {
		return invalidData(w, "No such class.")
	}
	acct, ok := userContext(r)
	if !ok {
		return badRequest(w, "Must be logged in.")
	}
	staff, _ := staff.WithID(c, acct.ID)
	if !canViewRoster(staff, acct, class.TeacherEntity(c)) {
		return webapp.UnauthorizedError(fmt.Errorf("only staff or teachers can view rosters"))
	}
	classStudents := students.In(c, class, time.Now())
	sort.Sort(students.ByName(classStudents))
	token, err := storeNewToken(c, acct.ID, "/register/paper")
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Failed to store token: %s", err))
	}
	data := map[string]interface{}{
		"Class":    class,
		"Students": classStudents,
		"Token":    token.Encode(),
	}
	if err := rosterPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
