package registration

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/taskqueue"
	"appengine/user"

	"model"
)

var (
	registrationForm    = template.Must(template.ParseFiles("registration/form.html"))
	newRegistrationPage = template.Must(template.ParseFiles("registration/registration-new.html"))
	classFullPage       = template.Must(template.ParseFiles("registration/full-class.html"))
	sessionCookieName   = "innerhearth-session-id"
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
	http.Handle("/registration/new", handler(postOnly(newRegistration)))
}

func filterRegisteredClasses(classes []*model.Class, registrations []*model.Registration) []*model.Class {
	if len(registrations) == 0 || len(classes) == 0 {
		return classes
	}
	registered := map[string]bool{}
	for _, r := range registrations {
		registered[r.ClassName] = true
	}
	filtered := []*model.Class{}
	for _, c := range classes {
		if !registered[c.Name] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func registration(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return &appError{fmt.Errorf("No logged in user"), "An error occurred", http.StatusInternalServerError}
	}
	classes, err := model.ListClasses(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	token, err := model.GetXSRFToken(c, u.ID)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	account, err := model.GetAccount(c, u)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	registrations := model.ListUserRegistrations(c, account.AccountID)
	classes = filterRegisteredClasses(classes, registrations)
	logout, err := user.LogoutURL(c, "/")
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"Classes":       classes,
		"XSRFToken":     token.Token,
		"LogoutURL":     logout,
		"Account":       account,
		"Registrations": registrations,
	}
	if err := registrationForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func classFull(w http.ResponseWriter, r *http.Request, class string) *appError {
	if err := classFullPage.Execute(w, class); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func newRegistration(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	account, err := model.GetCurrentUserAccount(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	if !model.ValidXSRFToken(c, account.AccountID, r.FormValue("xsrf_token")) {
		return &appError{fmt.Errorf("Invalid XSRF token for %s", account.AccountID), "Authorization error", http.StatusUnauthorized}
	}
	reg := model.NewRegistration(c, r.FormValue("class"), account.AccountID)
	if err := reg.Insert(c); err != nil {
		if fullError, ok := err.(*model.ClassFullError); ok {
			return classFull(w, r, fullError.Class)
		}
		return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
	}
	t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
		"account": {account.AccountID},
		"class":   {r.FormValue("class")},
	})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		return &appError{fmt.Errorf("Error enqueuing email task for registration %v: %s", reg, err),
			"An error occurred, please go back and try again",
			http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"Email": account.Email,
		"Class": r.FormValue("class"),
	}
	if err = newRegistrationPage.Execute(w, data); err != nil {
		return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
	}
	return nil
}
