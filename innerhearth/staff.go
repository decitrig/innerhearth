package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"time"

	"appengine"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/classes"
	"github.com/decitrig/innerhearth/staff"
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

func indexAsWeekday(i int) time.Weekday    { return time.Weekday((i + 1) % 7) }
func weekdayEquals(a, b time.Weekday) bool { return a == b }

func init() {
	for url, fn := range map[string]webapp.HandlerFunc{
		"/staff":                     staffPortal,
		"/staff/add-teacher":         addTeacher,
		"/staff/add-announcement":    addAnnouncement,
		"/staff/delete-announcement": deleteAnnouncement,
		/*
			"/staff/add-class":            addClass,
			"/staff/add-session":          addSession,
			"/staff/delete-class":         deleteClass,
			"/staff/edit-class":           editClass,
			"/staff/reschedule-class":     rescheduleClass,
			"/staff/session":              session,
			"/staff/assign-classes":       assignClasses,
			"/staff/yin-yogassage":        yinYogassage,
			"/staff/delete-yin-yogassage": deleteYinYogassage,
		*/
	} {
		webapp.HandleFunc(url, userContextHandler(staffContextHandler(fn)))
	}
}

func staffPortal(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	data := make(map[string]interface{})
	teachers := classes.Teachers(c)
	sort.Sort(classes.TeachersByName(teachers))
	data["Teachers"] = teachers
	announcements := staff.CurrentAnnouncements(c, time.Now())
	sort.Sort(staff.AnnouncementsByExpiration(announcements))
	data["Announcements"] = announcements
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
	account, err := account.WithEmail(c, vals["email"])
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find account for '%s'", vals["email"]))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		teacher := classes.NewTeacher(account)
		if err := staffAccount.PutTeacher(c, teacher); err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't store teacher for %q: %s", account.Email, err))
		}
		token.Delete(c)
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	token, err := auth.NewToken(staffAccount.ID, r.URL.Path, time.Now())
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

func addAnnouncement(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may add announcements"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "text", "expiration")
		if err != nil {
			// TODO(rwsims): Clean up this error reporting.
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Please go back and enter all required data.")
			return nil
		}
		expiration, err := parseLocalDate(fields["expiration"])
		if err != nil {
			// TODO(rwsims): Clean up this error reporting.
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Invalid date entry; please use mm/dd/yyyy format.")
			return nil
		}
		c.Infof("expiration: %s", expiration)
		announce := staff.NewAnnouncement(fields["text"], expiration)
		if err := staffAccount.AddAnnouncement(c, announce); err != nil {
			return webapp.InternalError(fmt.Errorf("staff: failed to add announcement: %s", err))
		}
		token.Delete(c)
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	token, err := auth.NewToken(staffAccount.ID, r.URL.Path, time.Now())
	if err != nil {
		return webapp.InternalError(err)
	}
	if err := token.Store(c); err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{
		"Token": token.Encode(),
	}
	if err := addAnnouncementPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func deleteAnnouncement(w http.ResponseWriter, r *http.Request) *webapp.Error {
	fields, err := webapp.ParseRequiredValues(r, "id")
	if err != nil {
		return webapp.InternalError(err)
	}
	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("failed to parse %q as announcement ID: %s", fields["id"], err))
	}
	c := appengine.NewContext(r)
	announce, err := staff.AnnouncementWithID(c, id)
	if err != nil {
		// TODO(rwsims): Clean up this error reporting
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "We couldn't find the announcement you're looking for.")
		return nil
	}
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may delete announcements"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		if err := announce.Delete(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to delete announcement %d: %s", announce.ID, err))
		}
		token.Delete(c)
		http.Redirect(w, r, "/staff", http.StatusSeeOther)
		return nil
	}
	token, err := auth.NewToken(staffAccount.ID, r.URL.Path, time.Now())
	if err != nil {
		return webapp.InternalError(err)
	}
	if err := token.Store(c); err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{
		"Token":        token.Encode(),
		"Announcement": announce,
	}
	if err := deleteAnnouncementPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
