package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"time"

	"appengine"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/scheduling"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	staffPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": indexAsWeekday,
	}).ParseFiles("templates/base.html", "templates/staff/index.html"))
	addTeacherPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-teacher.html"))
	addClassPage        = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-class.html"))
	addSessionPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-session.html"))
	deleteClassPage     = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-class.html"))
	editClassPage       = template.Must(template.ParseFiles("templates/base.html", "templates/staff/edit-class.html"))
	rescheduleClassPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"weekdayEquals": weekdayEquals,
	}).ParseFiles("templates/base.html", "templates/staff/reschedule-class.html"))
	sessionPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": indexAsWeekday,
	}).ParseFiles("templates/base.html", "templates/staff/session.html"))
	addAnnouncementPage    = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-announcement.html"))
	deleteAnnouncementPage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-announcement.html"))
	assignClassesPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/assign-classes.html"))
	yinYogassagePage       = template.Must(template.ParseFiles("templates/base.html", "templates/staff/yin-yogassage.html"))
	deleteYinYogassagePage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-yin-yogassage.html"))
)

const (
	dateFormat = "01/02/2006"
	timeFormat = "3:04pm"
)

func indexAsWeekday(i int) time.Weekday    { return time.Weekday((i + 1) % 7) }
func weekdayEquals(a, b time.Weekday) bool { return a == b }

func init() {
	for url, fn := range map[string]webapp.HandlerFunc{
		"/staff":             staff,
		"/staff/add-teacher": addTeacher,
		/*
			"/staff/add-class":            addClass,
			"/staff/add-session":          addSession,
			"/staff/delete-class":         deleteClass,
			"/staff/edit-class":           editClass,
			"/staff/reschedule-class":     rescheduleClass,
			"/staff/session":              session,
			"/staff/add-announcement":     addAnnouncement,
			"/staff/delete-announcement":  deleteAnnouncement,
			"/staff/assign-classes":       assignClasses,
			"/staff/yin-yogassage":        yinYogassage,
			"/staff/delete-yin-yogassage": deleteYinYogassage,
		*/
	} {
		webapp.HandleFunc(url, userContextHandler(staffContextHandler(fn)))
	}
}

func staff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	data := make(map[string]interface{})
	teachers := scheduling.AllTeachers(c)
	sort.Sort(scheduling.TeachersByName(teachers))
	data["Teachers"] = teachers
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
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may add teachers"))
	}
	account, err := auth.LookupUserByEmail(c, vals["email"])
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find account for '%s'", vals["email"]))
	}
	if r.Method == "POST" {
		token, err := auth.LookupToken(c, staffAccount.AccountID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue("xsrf_token"), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		teacher := scheduling.NewTeacher(account)
		if err := teacher.Store(c); err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't store teacher for %q: %s", account.Email, err))
		}
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	token, err := auth.NewToken(staffAccount.AccountID, r.URL.Path, time.Now())
	if err != nil {
		return webapp.InternalError(err)
	}
	if err := token.Store(c); err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{
		"Token": token.Encode(),
		"User":  account,
	}
	if err := addTeacherPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
