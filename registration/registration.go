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
	registrationForm = template.Must(template.ParseFiles("registration/form.html"))
	adminPage        = template.Must(template.ParseFiles("registration/admin.html"))
)

type appError struct {
	Error   error
	Message string
	Code    int
}

type handler func(w http.ResponseWriter, r *http.Request) *appError

func postOnly(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		if r.Method != "POST" {
			return &appError{fmt.Errorf("GET access to %s", r.URL), "Not Found", 404}
		}
		return handler(w, r)
	}
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

func xsrfProtected(handler handler) handler {
	return postOnly(func(w http.ResponseWriter, r *http.Request) *appError {
		c := appengine.NewContext(r)
		token := r.FormValue("xsrf_token")
		email := user.Current(c).Email
		if !model.ValidXSRFToken(c, user.Current(c).Email, token) {
			return &appError{fmt.Errorf("Could not validate token for %s", email), "Authorization failure", 403}
		}
		return handler(w, r)
	})
}

func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Error: %s", err.Error)
		http.Error(w, err.Message, err.Code)
	}
}

func init() {
	http.Handle("/registration", handler(register))
	http.Handle("/registration/new", handler(newRegistration))
	http.Handle("/registration/admin", handler(adminOnly(admin)))
	http.Handle("/registration/admin/add-class", handler(adminOnly(xsrfProtected(adminAddClass))))
}

func redirectToLogin(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	url, err := user.LoginURL(c, r.URL.String())
	if err != nil {
		return &appError{err, "An error occured", http.StatusInternalServerError}
	}
	w.Header().Set("Location", url)
	w.WriteHeader(http.StatusSeeOther)
	return nil
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
		c.Infof("Could not find XSRFToken for admin %s", email)
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

func adminAddClass(w http.ResponseWriter, r *http.Request) *appError {
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

func register(w http.ResponseWriter, r *http.Request) *appError {
	if err := registrationForm.Execute(w, nil); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func newRegistration(w http.ResponseWriter, r *http.Request) *appError {
	return &appError{nil, "Not implemented", http.StatusNotFound}
}
