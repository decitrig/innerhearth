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
	registrationConfirm = template.Must(template.ParseFiles("registration/registration-confirm.html"))
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
			return &appError{
				fmt.Errorf("Could not get XSRF token for id %s: %s", u.AccountID, err),
				"An error occurred",
				http.StatusInternalServerError,
			}
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
	http.Handle("/registration/new", handler(xsrfProtected(newRegistration)))
}

func filterRegisteredClasses(classes []*model.Class, registrations []*model.Registration) []*model.Class {
	if len(registrations) == 0 || len(classes) == 0 {
		return classes
	}
	registered := map[int64]bool{}
	for _, r := range registrations {
		registered[r.ClassID] = true
	}
	filtered := []*model.Class{}
	for _, c := range classes {
		if !registered[c.ID] {
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
	registrar := model.NewRegistrar(c, u.AccountID)
	registrations := registrar.ListRegistrations()
	classes = filterRegisteredClasses(classes, registrations)
	logout, err := user.LogoutURL(c, "/")
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	data := map[string]interface{}{
		"SessionClasses": classes,
		"XSRFToken":      token.Token,
		"LogoutURL":      logout,
		"Account":        u.UserAccount,
		"Registrations":  registrations,
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
	c := appengine.NewContext(r)
	classID := mustParseInt(r.FormValue("class"), 64)
	scheduler := model.NewScheduler(c)
	class := scheduler.GetClass(classID)
	if class == nil {
		return &appError{fmt.Errorf("Couldn't find class %d", classID),
			"An error occurred, please go back and try again",
			http.StatusInternalServerError}
	}
	if r.Method == "POST" {
		roster := model.NewRoster(c, class)
		if _, err := roster.AddStudent(u.AccountID); err != nil {
			if err == model.ErrClassFull {
				if err := classFullPage.Execute(w, nil); err != nil {
					return &appError{err, "An error occurred", http.StatusInternalServerError}
				}
			}
			return &appError{
				fmt.Errorf("Error when registering student %s in class %d: %s", u.AccountID, class.ID, err),
				"An error occurred, please go back and try again.",
				http.StatusInternalServerError,
			}
		}
		t := taskqueue.NewPOSTTask("/task/email-confirmation", map[string][]string{
			"account": {u.AccountID},
			"class":   {fmt.Sprintf("%d", class.ID)},
		})
		if _, err := taskqueue.Add(c, t, ""); err != nil {
			return &appError{fmt.Errorf("Error enqueuing email task for registration: %s", err),
				"An error occurred, please go back and try again",
				http.StatusInternalServerError}
		}
		data := map[string]interface{}{
			"Email": u.UserAccount.Email,
			"Class": class,
		}
		if err := newRegistrationPage.Execute(w, data); err != nil {
			return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
		}
		return nil
	}
	token := tokenVariable.Get(r).(*model.AdminXSRFToken)
	if err := registrationConfirm.Execute(w, map[string]interface{}{
		"XSRFToken": token.Token,
		"Class":     class,
	}); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}
