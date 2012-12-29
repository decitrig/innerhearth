package registration

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/user"

	"model"
)

var (
	registrationForm = template.Must(template.ParseFiles("registration/form.html"))
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

func validXSRFToken(r *http.Request) bool {
	c := appengine.NewContext(r)
	token := r.FormValue("xsrf_token")
	email := user.Current(c).Email
	return model.ValidXSRFToken(c, email, token)
}

func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Error: %s", err.Error)
		http.Error(w, err.Message, err.Code)
	}
}

func init() {
	http.Handle("/registration", handler(registration))
	http.Handle("/registration/new", handler(newRegistration))
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

func registration(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	classes, err := model.ListClasses(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"Classes": classes,
	}
	if err := registrationForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func newRegistration(w http.ResponseWriter, r *http.Request) *appError {
	return &appError{nil, "Not implemented", http.StatusNotFound}
}
