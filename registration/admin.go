package registration

import (
	"fmt"
	"html/template"
	"net/http"
	"strconv"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"

	"model"
)

func daySelected(c *model.Class, day string) template.HTMLAttr {
	if c.DayOfWeek == day {
		return template.HTMLAttr(`selected="selected"`)
	}
	return template.HTMLAttr("")
}

var (
	adminPage               = template.Must(template.ParseFiles("registration/admin.html"))
	deleteClassConfirmation = template.Must(template.ParseFiles("registration/delete-confirm.html"))
	addClassForm            = template.Must(template.ParseFiles("registration/add-class.html"))
	rescheduleClassPage     = template.Must(template.New("reschedule-class.html").Funcs(template.FuncMap{
		"daySelected": daySelected,
	}).ParseFiles("registration/reschedule-class.html"))
)

func staffOnly(handler handler) handler {
	return xsrfProtected(func(w http.ResponseWriter, r *http.Request) *appError {
		u := getRequestUser(r)
		if !u.Role.IsStaff() {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return nil
		}
		return handler(w, r)
	})
}

func init() {
	http.Handle("/registration/admin", handler(staffOnly(admin)))
	http.Handle("/registration/admin/add-class", handler(staffOnly(addClass)))
	http.Handle("/registration/admin/delete-class", handler(staffOnly(deleteClass)))
	http.Handle("/registration/admin/reschedule-class", handler(staffOnly(rescheduleClass)))
}

func admin(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	url, err := user.LogoutURL(c, r.URL.String())
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClasses(false)
	teachers := scheduler.GetTeacherNames(classes)
	u := userVariable.Get(r).(*requestUser)
	data := map[string]interface{}{
		"Email":     u.UserAccount.Email,
		"LogoutURL": url,
		"Classes":   classes,
		"Teachers":  teachers,
	}
	if err := adminPage.Execute(w, data); err != nil {
		return &appError{err, "An error occured", http.StatusInternalServerError}
	}
	return nil
}

func getRequiredFields(r *http.Request, fields ...string) (map[string]string, error) {
	m := map[string]string{}
	for _, f := range fields {
		v := r.FormValue(f)
		if v == "" {
			return nil, fmt.Errorf("Failed to get field '%s'", f)
		}
		m[f] = v
	}
	return m, nil
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

func addClassFromPost(r *http.Request) error {
	fields, err := getRequiredFields(r, "name", "description", "teacher", "maxstudents",
		"dayofweek", "starttime", "length", "type")
	if err != nil {
		return err
	}
	c := appengine.NewContext(r)
	class := &model.Class{
		Title:         fields["name"],
		Description:   fields["description"],
		Teacher:       datastore.NewKey(c, "UserAccount", fields["teacher"], 0, nil),
		Capacity:      int32(mustParseInt(fields["maxstudents"], 32)),
		SpacesLeft:    int32(mustParseInt(fields["maxstudents"], 32)),
		DayOfWeek:     fields["dayofweek"],
		StartTime:     mustParseTime("15:04", fields["starttime"]),
		LengthMinutes: int32(mustParseInt(fields["length"], 32)),
		Active:        true,
	}
	switch fields["type"] {
	case "session":
		times, err := getRequiredFields(r, "startdate", "enddate")
		if err != nil {
			return err
		}
		class.BeginDate = mustParseTime("2006-01-02", times["startdate"])
		class.EndDate = mustParseTime("2006-01-02", times["enddate"])
		class.DropInOnly = false

	case "dropin":
		class.DropInOnly = true

	default:
		return fmt.Errorf("Unknown class type: %s", fields["type"])
	}
	scheduler := model.NewScheduler(c)
	if err := scheduler.AddNew(class); err != nil {
		return fmt.Errorf("Error adding new class %s: %s", class.Title, err)
	}
	return nil
}

func addClass(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	if r.Method == "POST" {
		if err := addClassFromPost(r); err != nil {
			if err != model.ErrClassExists {
				return &appError{err, "An error occurred", http.StatusInternalServerError}
			}
			return &appError{err, fmt.Sprintf("Class %s already exists", r.FormValue("name")), http.StatusInternalServerError}
		}
		c.Infof("Successfully added class %s", r.FormValue("name"))
		http.Redirect(w, r, "/registration/admin", http.StatusSeeOther)
		return nil
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	data := map[string]interface{}{
		"XSRFToken": token.Token,
		"Teachers":  model.ListTeachers(c),
	}
	if err := addClassForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func deleteClass(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	classID := int64(mustParseInt(r.FormValue("class"), 64))
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return &appError{fmt.Errorf("Couldn't find class %d", classID),
			"Couldn't find class", http.StatusInternalServerError}
	}
	if r.Method == "POST" {
		if err := scheduler.DeleteClass(class); err != nil {
			return &appError{err, "An error occurred", http.StatusInternalServerError}
		}
		c.Infof("Deleted class %d", class.ID)
		http.Redirect(w, r, "/registration/admin", http.StatusSeeOther)
		return nil
	}
	roster := model.NewRoster(c, class)
	regs := roster.ListRegistrations()
	if regs != nil && len(regs) > 0 {
		fmt.Fprintf(w, "This class is not empty")
		return nil
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	data := map[string]interface{}{
		"ClassName": class.Title,
		"ClassID":   class.ID,
		"XSRFToken": token.Token,
	}
	if err := deleteClassConfirmation.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func rescheduleClass(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	classID := mustParseInt(r.FormValue("class"), 64)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return &appError{
			fmt.Errorf("Couldn't find class %d", classID),
			"Couldn't find class",
			http.StatusInternalServerError,
		}
	}
	if r.Method == "POST" {
		fields, err := getRequiredFields(r, "dayofweek", "starttime", "length")
		if err != nil {
			return &appError{err, "Missing required fields", http.StatusBadRequest}
		}
		class.DayOfWeek = fields["dayofweek"]
		class.StartTime = mustParseTime("15:04", fields["starttime"])
		class.LengthMinutes = int32(mustParseInt(fields["length"], 32))
		if !class.DropInOnly {
			times, err := getRequiredFields(r, "startdate", "enddate")
			if err != nil {
				return &appError{err, "Missing required fields", http.StatusBadRequest}
			}
			class.BeginDate = mustParseTime("2006-01-02", times["startdate"])
			class.EndDate = mustParseTime("2006-01-02", times["enddate"])
		}
		if err := scheduler.WriteClass(class); err != nil {
			return &appError{err, "Error storing class", http.StatusInternalServerError}
		}
		http.Redirect(w, r, "/registration/admin", http.StatusSeeOther)
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	if err := rescheduleClassPage.Funcs(template.FuncMap{
		"daySelected": daySelected,
	}).Execute(w, map[string]interface{}{
		"Class":     class,
		"XSRFToken": token.Token,
	}); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}
