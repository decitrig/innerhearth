package registration

import (
	"fmt"
	"html/template"
	"net/http"
	"sync"

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

type requestVariable struct {
	lock sync.Mutex
	m    map[*http.Request]interface{}
}

func (v *requestVariable) Get(r *http.Request) interface{} {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.m == nil {
		return nil
	}
	return v.m[r]
}

func (v *requestVariable) Set(r *http.Request, val interface{}) {
	v.lock.Lock()
	defer v.lock.Unlock()
	if v.m == nil {
		v.m = map[*http.Request]interface{}{}
	}
	v.m[r] = val
}

type requestUser struct {
	*user.User
	*model.UserAccount
}

var (
	userVariable  = &requestVariable{}
	tokenVariable = &requestVariable{}
)

type appError struct {
	Error   error
	Message string
	Code    int
}

type handler func(w http.ResponseWriter, r *http.Request) *appError

func (fn handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Error: %s", err.Error)
		http.Error(w, err.Message, err.Code)
	}
}

func postOnly(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		if r.Method != "POST" {
			return &appError{fmt.Errorf("GET access to %s", r.URL), "Not Found", 404}
		}
		return handler(w, r)
	}
}

func needsUser(handler handler) handler {
	return func(w http.ResponseWriter, r *http.Request) *appError {
		c := appengine.NewContext(r)
		u := user.Current(c)
		if u == nil {
			return &appError{fmt.Errorf("No logged in user"), "An error occurred", http.StatusInternalServerError}
		}
		account, err := model.GetAccount(c, u)
		if err != nil {
			http.Redirect(w, r, "/login/account?continue="+r.URL.Path, http.StatusSeeOther)
			return nil
		}
		userVariable.Set(r, &requestUser{u, account})
		return handler(w, r)
	}
}

func xsrfProtected(handler handler) handler {
	return needsUser(func(w http.ResponseWriter, r *http.Request) *appError {
		u := userVariable.Get(r).(*requestUser)
		if u == nil {
			return &appError{fmt.Errorf("No user in request"), "An error ocurred", http.StatusInternalServerError}
		}
		c := appengine.NewContext(r)
		token, err := model.GetXSRFToken(c, u.AccountID)
		if err != nil {
			return &appError{fmt.Errorf("Could not get XSRF token for id %s: %s", u.AccountID, err), "An error occurred", http.StatusInternalServerError}
		}
		tokenVariable.Set(r, token)
		if r.Method == "POST" && !token.Validate(r.FormValue("xsrf_token")) {
			return &appError{fmt.Errorf("Invalid XSRF token"), "Unauthorized", http.StatusUnauthorized}
		}
		return handler(w, r)
	})
}

func init() {
	http.Handle("/registration", handler(xsrfProtected(registration)))
	http.Handle("/registration/new", handler(postOnly(xsrfProtected(newRegistration))))
}

func filterRegisteredClasses(classes []*model.Class, registrations []*model.Registration) []*model.Class {
	if len(registrations) == 0 || len(classes) == 0 {
		return classes
	}
	registered := map[string]bool{}
	for _, r := range registrations {
		registered[r.ClassTitle] = true
	}
	filtered := []*model.Class{}
	for _, c := range classes {
		if !registered[c.Title] {
			filtered = append(filtered, c)
		}
	}
	return filtered
}

func registration(w http.ResponseWriter, r *http.Request) *appError {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClasses(true)
	u := userVariable.Get(r).(*requestUser)
	registrar := model.NewStudentRegistrar(c, u.AccountID)
	registrations := registrar.ListRegistrations()
	classes = filterRegisteredClasses(classes, registrations)
	logout, err := user.LogoutURL(c, "/")
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	data := map[string]interface{}{
		"Classes":       classes,
		"XSRFToken":     token.Token,
		"LogoutURL":     logout,
		"Account":       u.UserAccount,
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
	u := userVariable.Get(r).(*requestUser)
	/*
		reg := model.NewRegistration(c, r.FormValue("class"), account.AccountID)
		if err := reg.Insert(c); err != nil {
			if fullError, ok := err.(*model.ClassFullError); ok {
				return classFull(w, r, fullError.Class)
			}
			return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
		}
	*/
	t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
		"account": {u.AccountID},
		"class":   {r.FormValue("class")},
	})
	c := appengine.NewContext(r)
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		return &appError{fmt.Errorf("Error enqueuing email task for registration: %s", err),
			"An error occurred, please go back and try again",
			http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"Email": u.UserAccount.Email,
		"Class": r.FormValue("class"),
	}
	if err := newRegistrationPage.Execute(w, data); err != nil {
		return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
	}
	return nil
}
