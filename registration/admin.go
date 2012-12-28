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
)

func init() {
	http.Handle("/registration/admin", handler(adminOnly(admin)))
	http.Handle("/registration/admin/add-class", handler(adminOnly(xsrfProtected(addClass))))
	http.Handle("/registration/admin/delete-class", handler(adminOnly(deleteClass)))
}

func adminOnly(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		c := appengine.NewContext(r)
		if !user.IsAdmin(c) {
			redirectToLogin(w, r)
			return nil
		}
		return handler(w, r)
	}
}

func admin(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	data := make(map[string]interface{}, 0)
	url, err := user.LogoutURL(c, r.URL.String())
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	email := user.Current(c).Email
	token, err := model.GetXSRFToken(c, email)
	if err != nil {
		c.Infof("Could not find XSRFToken for  %s", email)
		token, err = model.MakeXSRFToken(c, email)
		if err != nil {
			return &appError{err, "An error occurred", http.StatusInternalServerError}
		}
	}
	classes, err := model.ListClasses(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data["LogoutURL"] = url
	data["XSRFToken"] = token.Token
	data["Classes"] = classes
	if err := adminPage.Execute(w, data); err != nil {
		return &appError{err, "An error occured", http.StatusInternalServerError}
	}
	return nil
}

func newClassFromPost(r *http.Request) (*model.Class, error) {
	if r == nil {
		return nil, errors.New("request must not be nil")
	}
	maxStudents, err := strconv.ParseInt(r.FormValue("maxstudents"), 10, 32)
	if err != nil {
		return nil, fmt.Errorf("could not parse %s as int32: %s",
			r.FormValue("maxstudents"),
			err)
	}
	return model.NewClass(r.FormValue("longname"), int32(maxStudents)), nil
}

func addClass(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	class, err := newClassFromPost(r)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	if err := class.Insert(c); err != nil {
		if err != model.ErrClassExists {
			return &appError{err, "An error occurred", http.StatusInternalServerError}
		}
		return &appError{err, fmt.Sprintf("Class %s already exists", class.Name), http.StatusInternalServerError}
	}
	c.Infof("Successfully added class %s", class.Name)
	w.Header().Set("Location", "/registration/admin")
	w.WriteHeader(http.StatusSeeOther)
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
		return xsrfProtected(doDelete)(w, r)
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
