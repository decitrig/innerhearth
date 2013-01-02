package registration

import (
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"strconv"

	"appengine"
	"appengine/user"

	"model"
)

var (
	adminPage               = template.Must(template.ParseFiles("registration/admin.html"))
	deleteClassConfirmation = template.Must(template.ParseFiles("registration/delete-confirm.html"))
	addClassForm            = template.Must(template.ParseFiles("registration/add-class.html"))
)

func init() {
	http.Handle("/registration/admin", handler(admin))
	http.Handle("/registration/admin/add-class", handler(addClass))
	http.Handle("/registration/admin/delete-class", handler(deleteClass))
}

func admin(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	url, err := user.LogoutURL(c, r.URL.String())
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	classes, err := model.ListClasses(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	for idx, class := range classes {
		count, err := model.CountRegistrations(c, class.Name)
		if err != nil {
			c.Errorf("error: %s", err)
			continue
		}
		classes[idx].Registrations = count
	}
	data := map[string]interface{}{
		"LogoutURL": url,
		"Classes":   classes,
	}
	if err := adminPage.Execute(w, data); err != nil {
		return &appError{err, "An error occured", http.StatusInternalServerError}
	}
	return nil
}

func addClassFromPost(r *http.Request) error {
	if r == nil {
		return errors.New("request must not be nil")
	}
	maxStudents, err := strconv.ParseInt(r.FormValue("maxstudents"), 10, 32)
	if err != nil {
		return fmt.Errorf("could not parse %s as int32: %s",
			r.FormValue("maxstudents"),
			err)
	}
	class := model.NewClass(r.FormValue("name"), int32(maxStudents))
	c := appengine.NewContext(r)
	return class.Insert(c)
}

func addClass(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	if r.Method == "POST" {
		if !validXSRFToken(r) {
			return &appError{errors.New("Invalid XSRF Token"), "Authorization failure", http.StatusUnauthorized}
		}
		if err := addClassFromPost(r); err != nil {
			if err != model.ErrClassExists {
				return &appError{err, "An error occurred", http.StatusInternalServerError}
			}
			return &appError{err, fmt.Sprintf("Class %s already exists", r.FormValue("name")), http.StatusInternalServerError}
		}
		c.Infof("Successfully added class %s", r.FormValue("name"))
		w.Header().Set("Location", "/registration/admin")
		w.WriteHeader(http.StatusSeeOther)
		return nil
	}
	token, err := model.GetXSRFToken(c, user.Current(c).Email)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"XSRFToken": token.Token,
	}
	if err := addClassForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func doDelete(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	className := r.FormValue("class")
	if err := model.DeleteClass(c, className); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	c.Infof("Deleted class %s", className)
	w.Header().Set("Location", "/registration/admin")
	w.WriteHeader(http.StatusSeeOther)
	return nil
}

func deleteClass(w http.ResponseWriter, r *http.Request) *appError {
	className := r.FormValue("class")
	c := appengine.NewContext(r)
	if r.Method == "POST" {
		if !validXSRFToken(r) {
			return &appError{errors.New("Invalid XSRF Token"), "Authorization failure", http.StatusUnauthorized}
		}
	}
	token, err := model.GetXSRFToken(c, user.Current(c).Email)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"ClassName": className,
		"XSRFToken": token.Token,
	}
	if err := deleteClassConfirmation.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}
