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
	"github.com/decitrig/innerhearth/yogassage"
)

var (
	staffPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": indexAsWeekday,
	}).ParseFiles("templates/base.html", "templates/staff/index.html"))
	addTeacherPage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-teacher.html"))
	addClassPage   = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"WeekdayAsInt": weekdayAsInt,
	}).ParseFiles("templates/base.html", "templates/staff/add-class.html"))
	addSessionPage  = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-session.html"))
	deleteClassPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"FormatLocal": formatLocal,
	}).ParseFiles("templates/base.html", "templates/staff/delete-class.html"))
	editClassPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"FormatLocal":     formatLocal,
		"WeekdayAsInt":    weekdayAsInt,
		"WeekdayEquals":   weekdayEquals,
		"TeacherHasEmail": teacherHasEmail,
		"Minutes":         minutes,
	}).ParseFiles("templates/base.html", "templates/staff/edit-class.html"))
	sessionPage = template.Must(template.New("base.html").Funcs(template.FuncMap{
		"indexAsWeekday": indexAsWeekday,
		"FormatLocal":    formatLocal,
	}).ParseFiles("templates/base.html", "templates/staff/session.html"))
	addAnnouncementPage    = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-announcement.html"))
	deleteAnnouncementPage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-announcement.html"))
	yinYogassagePage       = template.Must(template.ParseFiles("templates/base.html", "templates/staff/yin-yogassage.html"))
	deleteYinYogassagePage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/delete-yin-yogassage.html"))
)

func indexAsWeekday(i int) time.Weekday    { return time.Weekday((i + 1) % 7) }
func weekdayEquals(a, b time.Weekday) bool { return a == b }
func weekdayAsInt(w time.Weekday) int      { return int(w) }
func minutes(d time.Duration) int64        { return int64(d.Minutes()) }
func teacherHasEmail(t *classes.Teacher, email string) bool {
	if t == nil {
		return false
	}
	return t.Email == email
}

func init() {
	for url, fn := range map[string]webapp.HandlerFunc{
		"/staff":                      staffPortal,
		"/staff/add-teacher":          addTeacher,
		"/staff/add-announcement":     addAnnouncement,
		"/staff/delete-announcement":  deleteAnnouncement,
		"/staff/add-session":          addSession,
		"/staff/yin-yogassage":        yinYogassage,
		"/staff/delete-yin-yogassage": deleteYinYogassage,
		"/staff/session":              session,
		"/staff/add-class":            addClass,
		"/staff/edit-class":           editClass,
		"/staff/delete-class":         deleteClass,
	} {
		webapp.HandleFunc(url, userContextHandler(staffContextHandler(fn)))
	}
}

var daysInOrder = []time.Weekday{
	time.Monday,
	time.Tuesday,
	time.Wednesday,
	time.Thursday,
	time.Friday,
	time.Saturday,
	time.Sunday,
}

func staffPortal(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	teachers := classes.Teachers(c)
	sort.Sort(classes.TeachersByName(teachers))
	announcements := staff.CurrentAnnouncements(c, time.Now())
	sort.Sort(staff.AnnouncementsByExpiration(announcements))
	sessions := classes.Sessions(c, time.Now())
	sort.Sort(classes.SessionsByStartDate(sessions))
	yins := yogassage.Classes(c, time.Now())
	sort.Sort(yogassage.ByDate(yins))
	data := map[string]interface{}{
		"Teachers":            teachers,
		"Announcements":       announcements,
		"Sessions":            sessions,
		"YinYogassageClasses": yins,
	}
	if err := staffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func missingFields(w http.ResponseWriter) *webapp.Error {
	// TODO(rwsims): Clean up this error reporting.
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, "Please go back and enter all required data.")
	return nil
}

func invalidData(w http.ResponseWriter, message string) *webapp.Error {
	// TODO(rwsims): Clean up this error reporting.
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, message)
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
		if err := teacher.Put(c); err != nil {
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
			return missingFields(w)
		}
		expiration, err := parseLocalDate(fields["expiration"])
		if err != nil {
			return invalidData(w, "Invalid date entry; please use mm/dd/yyyy format.")
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
		return missingFields(w)
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

func addSession(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may add sessions"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "name", "startdate", "enddate")
		if err != nil {
			return missingFields(w)
		}
		start, err := parseLocalDate(fields["startdate"])
		if err != nil {
			return invalidData(w, "Invalid start date; please use mm/dd/yyyy format.")
		}
		end, err := parseLocalDate(fields["enddate"])
		if err != nil {
			return invalidData(w, "Invalid end date; please use mm/dd/yyyy format.")
		}
		session := classes.NewSession(fields["name"], start, end)
		if err := session.Insert(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to put session: %s", err))
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
	if err := addSessionPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func session(w http.ResponseWriter, r *http.Request) *webapp.Error {
	idString := r.FormValue("id")
	if idString == "" {
		return missingFields(w)
	}
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return invalidData(w, fmt.Sprintf("Couldn't parse %q as ID", idString))
	}
	c := appengine.NewContext(r)
	session, err := classes.SessionWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrSessionNotFound:
		return invalidData(w, fmt.Sprintf("No such session"))
	default:
		return webapp.InternalError(fmt.Errorf("failed to find session %d: %s", id, err))
	}
	classList := session.Classes(c)
	sort.Sort(classes.ClassesByStartTime(classList))
	teachers := classes.TeachersByClass(c, classList)
	data := map[string]interface{}{
		"Session":     session,
		"Classes":     classes.GroupedByDay(classList),
		"DaysInOrder": daysInOrder,
		"Teachers":    teachers,
	}
	if err := sessionPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func yinYogassage(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may add yins"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "date", "signup")
		if err != nil {
			return missingFields(w)
		}
		date, err := parseLocalDate(fields["date"])
		if err != nil {
			return invalidData(w, "Invalid date; please use mm/dd/yyyy format.")
		}
		yin := yogassage.New(date, fields["signup"])
		if err := yin.Insert(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to write yogassage: %s", err))
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
	if err := yinYogassagePage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func deleteYinYogassage(w http.ResponseWriter, r *http.Request) *webapp.Error {
	fields, err := webapp.ParseRequiredValues(r, "id")
	if err != nil {
		return webapp.InternalError(err)
	}
	id, err := strconv.ParseInt(fields["id"], 10, 64)
	if err != nil {
		return invalidData(w, fmt.Sprintf("Invalid yogassage ID"))
	}
	c := appengine.NewContext(r)
	yin, err := yogassage.WithID(c, id)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("failed to find yogassage %d: %s", id, err))
	}
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may delete yogassage classes"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		if err := yin.Delete(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to delete yogassage %d: %s", yin.ID, err))
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
		"Class": yin,
	}
	if err := deleteYinYogassagePage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func parseWeekday(s string) (time.Weekday, error) {
	idx, err := strconv.ParseInt(s, 10, 0)
	if err != nil {
		return 0, err
	}
	if idx < 0 || idx > 6 {
		return 0, fmt.Errorf("out of range")
	}
	return time.Weekday(idx), nil
}

func parseMinutes(s string) (time.Duration, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	if n <= 0 {
		return 0, fmt.Errorf("out of range")
	}
	return time.Duration(n) * time.Minute, nil
}

func addClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	idString := r.FormValue("session")
	if idString == "" {
		return missingFields(w)
	}
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return invalidData(w, fmt.Sprintf("Invalid session ID"))
	}
	c := appengine.NewContext(r)
	session, err := classes.SessionWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrSessionNotFound:
		return invalidData(w, "No such session.")
	default:
		return webapp.InternalError(fmt.Errorf("failed to look up session %d: %s", id, err))
	}
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may add classes"))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "name", "description", "maxstudents", "dayofweek", "starttime", "length", "dropinonly")
		if err != nil {
			return missingFields(w)
		}
		weekday, err := parseWeekday(fields["dayofweek"])
		if err != nil {
			return invalidData(w, "Invalid weekday")
		}
		maxStudents, err := strconv.ParseInt(fields["maxstudents"], 10, 32)
		if err != nil || maxStudents <= 0 {
			return invalidData(w, "Invalid student capacity")
		}
		length, err := parseMinutes(fields["length"])
		if err != nil {
			return invalidData(w, "Invalid length")
		}
		start, err := parseLocalTime(fields["starttime"])
		if err != nil {
			return invalidData(w, "Invalid start time; please use HH:MMpm format (e.g., 3:04pm)")
		}
		class := &classes.Class{
			Title:           fields["name"],
			LongDescription: []byte(fields["description"]),
			Weekday:         weekday,
			DropInOnly:      fields["dropinonly"] == "true",
			Capacity:        int32(maxStudents),
			Length:          length,
			StartTime:       start,
			Session:         session.ID,
		}
		if email := r.FormValue("teacher"); email != "" {
			teacher, err := classes.TeacherWithEmail(c, email)
			if err != nil {
				return invalidData(w, "Invalid teacher selected")
			}
			class.Teacher = teacher.Key(c)
		}
		if err := class.Insert(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to add class: %s", err))
		}
		c.Infof("class ID: %d", class.ID)
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
		"Token":       token.Encode(),
		"Session":     session,
		"Teachers":    classes.Teachers(c),
		"DaysInOrder": daysInOrder,
	}
	if err := addClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func editClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	idString := r.FormValue("class")
	if idString == "" {
		return missingFields(w)
	}
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return invalidData(w, fmt.Sprintf("Invalid class ID"))
	}
	c := appengine.NewContext(r)
	class, err := classes.ClassWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrClassNotFound:
		return invalidData(w, "No such class.")
	default:
		return webapp.InternalError(fmt.Errorf("failed to look up class %d: %s", id, err))
	}
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may edit classes"))
	}
	if r.Method == "POST" {
		c.Infof("updating class %d", class.ID)
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "name", "description", "maxstudents", "dayofweek", "starttime", "length", "dropinonly")
		if err != nil {
			return missingFields(w)
		}
		class.Title = fields["name"]
		class.LongDescription = []byte(fields["description"])
		class.DropInOnly = fields["dropinonly"] == "yes"
		weekday, err := parseWeekday(fields["dayofweek"])
		if err != nil {
			return invalidData(w, "Invalid weekday")
		}
		class.Weekday = weekday
		maxStudents, err := strconv.ParseInt(fields["maxstudents"], 10, 32)
		if err != nil || maxStudents <= 0 {
			return invalidData(w, "Invalid student capacity")
		}
		class.Capacity = int32(maxStudents)
		length, err := parseMinutes(fields["length"])
		if err != nil {
			return invalidData(w, "Invalid length")
		}
		class.Length = length
		start, err := parseLocalTime(fields["starttime"])
		if err != nil {
			return invalidData(w, "Invalid start time; please use HH:MMpm format (e.g., 3:04pm)")
		}
		class.StartTime = start
		if email := r.FormValue("teacher"); email == "" {
			class.Teacher = nil
		} else {
			teacher, err := classes.TeacherWithEmail(c, email)
			if err != nil {
				return invalidData(w, "Invalid teacher selected")
			}
			class.Teacher = teacher.Key(c)
		}
		if err := class.Update(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to update class %d: %s", class.ID, err))
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
		"Token":       token.Encode(),
		"Class":       class,
		"Teacher":     class.TeacherEntity(c),
		"Teachers":    classes.Teachers(c),
		"DaysInOrder": daysInOrder,
	}
	if err := editClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func deleteClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	idString := r.FormValue("class")
	if idString == "" {
		return missingFields(w)
	}
	id, err := strconv.ParseInt(idString, 10, 64)
	if err != nil {
		return invalidData(w, fmt.Sprintf("Invalid class ID"))
	}
	c := appengine.NewContext(r)
	class, err := classes.ClassWithID(c, id)
	switch err {
	case nil:
		break
	case classes.ErrClassNotFound:
		return invalidData(w, "No such class.")
	default:
		return webapp.InternalError(fmt.Errorf("failed to look up class %d: %s", id, err))
	}
	staffAccount, ok := staffContext(r)
	if !ok {
		return webapp.UnauthorizedError(fmt.Errorf("only staff may delete classes"))
	}
	if r.Method == "POST" {
		c.Infof("updating class %d", class.ID)
		token, err := auth.TokenForRequest(c, staffAccount.ID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid auth token"))
		}
		if err := class.Delete(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to delete class %d: %s", class.ID, err))
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
		"Token":   token.Encode(),
		"Class":   class,
		"Teacher": class.TeacherEntity(c),
	}
	if err := deleteClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
