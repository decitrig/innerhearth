package webapp

import (
	"html/template"
	"net/http"

	"appengine"
	"github.com/gorilla/context"

	"github.com/decitrig/innerhearth/model"
)

type Error struct {
	Err     error
	Message string
	Code    int
}

func (e *Error) Error() string {
	return e.Err.Error()
}

func InternalError(err error) *Error {
	return &Error{err, "An error occurred", http.StatusInternalServerError}
}

func UnauthorizedError(err error) *Error {
	return &Error{err, "Unauthorized", http.StatusUnauthorized}
}

const (
	xsrfTokenKey = iota
	currentUserKey
)

func GetXSRFToken(r *http.Request) *model.AdminXSRFToken {
	return context.Get(r, xsrfTokenKey).(*model.AdminXSRFToken)
}

func SetXSRFToken(r *http.Request, t *model.AdminXSRFToken) {
	context.Set(r, xsrfTokenKey, t)
}

func GetCurrentUser(r *http.Request) *model.UserAccount {
	return context.Get(r, currentUserKey).(*model.UserAccount)
}

func SetCurrentUser(r *http.Request, u *model.UserAccount) {
	context.Set(r, currentUserKey, u)
}

type AppHandler interface {
	Serve(w http.ResponseWriter, r *http.Request) *Error
}

type AppHandlerFunc func(w http.ResponseWriter, r *http.Request) *Error

func (fn AppHandlerFunc) Serve(w http.ResponseWriter, r *http.Request) *Error {
	return fn(w, r)
}

func AppHandle(path string, h AppHandler) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		c := appengine.NewContext(r)
		u := model.MaybeGetCurrentUser(c)
		SetCurrentUser(r, u)
		if err := h.Serve(w, r); err != nil {
			c.Errorf("%s", err)
			http.Error(w, err.Message, err.Code)
		}
	})
}

func AppHandleFunc(path string, f AppHandlerFunc) {
	AppHandle(path, AppHandler(f))
}

func init() {
	AppHandleFunc("/", index)
}

var (
	indexPage = template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
)

func index(w http.ResponseWriter, r *http.Request) *Error {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClasses(true)
	if err := indexPage.Execute(w, map[string]interface{}{
		"Classes": classes,
	}); err != nil {
		return InternalError(err)
	}
	return nil
}
