package innerhearth

import (
	"html/template"
	"net/http"
	"time"

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
		"/staff": staff,
		/*
			"/staff/add-teacher":          addTeacher,
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
	data := map[string]interface{}{
		"Teachers":            nil,
		"Announcements":       nil,
		"Sessions":            nil,
		"YinYogassageClasses": nil,
	}
	if err := staffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
