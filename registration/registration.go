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
	registrationForm    = template.Must(template.ParseFiles("registration/form.html"))
	newRegistrationPage = template.Must(template.ParseFiles("registration/registration-new.html"))
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
	http.Handle("/registration/cookiecheck", handler(cookieCheck))
	http.Handle("/registration/new", handler(postOnly(newRegistration)))
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

func newSessionCookie() *http.Cookie {
	return &http.Cookie{
		Name:     sessionCookieName,
		Value:    model.MakeSessionID(),
		HttpOnly: true,
	}
}

func readOrCreateSessionCookie(r *http.Request) *http.Cookie {
	if cookie, err := r.Cookie(sessionCookieName); err != nil {
		return cookie
	}
	cookie := newSessionCookie()
	r.AddCookie(cookie)
	return cookie
}

func registration(w http.ResponseWriter, r *http.Request) *appError {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		w.Header().Set("Location", "/registration/cookiecheck")
		http.SetCookie(w, newSessionCookie())
		w.WriteHeader(http.StatusSeeOther)
		return nil
	}
	c := appengine.NewContext(r)
	classes, err := model.ListClasses(c)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	token, err := model.GetXSRFToken(c, cookie.Value)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	data := map[string]interface{}{
		"Classes":   classes,
		"XSRFToken": token.Token,
	}
	if err := registrationForm.Execute(w, data); err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	return nil
}

func newRegistration(w http.ResponseWriter, r *http.Request) *appError {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return &appError{err, "An error occurred", http.StatusInternalServerError}
	}
	c := appengine.NewContext(r)
	if !model.ValidXSRFToken(c, cookie.Value, r.FormValue("xsrf_token")) {
		return &appError{fmt.Errorf("Invalid XSRF token for %s", cookie.Value), "Authorization error", http.StatusUnauthorized}
	}
	reg := model.NewRegistration(c, r.FormValue("class"), r.FormValue("email"))
	if err := reg.Insert(c); err != nil {
		return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
	}
	data := map[string]interface{} {
		"Email": r.FormValue("email"),
		"Class": r.FormValue("class"),
	}
	if err = newRegistrationPage.Execute(w, data); err != nil {
		return &appError{err, "An error occurred; please go back and try again.", http.StatusInternalServerError}
	}
	return nil
}

func cookieCheck(w http.ResponseWriter, r *http.Request) *appError {
	if _, err := r.Cookie(sessionCookieName); err != nil {
		if err != http.ErrNoCookie {
			return &appError{err, "An error occurred", http.StatusInternalServerError}
		}
		fmt.Fprintf(w, "We were unable to write a session cookie; registration requires that cookies be enabled.")
		return nil
	}
	w.Header().Set("Location", "/registration")
	w.WriteHeader(http.StatusSeeOther)
	return nil
}
