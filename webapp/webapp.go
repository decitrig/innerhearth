package webapp

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"appengine"
	"appengine/user"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"

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
	if u := context.Get(r, currentUserKey); u != nil {
		return u.(*model.UserAccount)
	}
	return nil
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

var (
	router = mux.NewRouter()
)

func AppHandle(path string, h AppHandler) {
	router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if err := h.Serve(w, r); err != nil {
			c := appengine.NewContext(r)
			c.Errorf("%s", err)
			http.Error(w, err.Message, err.Code)
		}
	})
}

// MaybeLoggedIn returns an AppHandler which tries to look up the current logged-in user and store
// it in a context. It then delegates to the given handler.
func MaybeLoggedIn(handler AppHandler) AppHandler {
	return AppHandlerFunc(func(w http.ResponseWriter, r *http.Request) *Error {
		c := appengine.NewContext(r)
		if u := model.MaybeGetCurrentUser(c); u != nil {
			SetCurrentUser(r, u)
		}
		return handler.Serve(w, r)
	})
}

func AppHandleFunc(path string, f AppHandlerFunc) {
	AppHandle(path, AppHandler(f))
}

func init() {
	http.Handle("/", router)
	AppHandle("/", MaybeLoggedIn(AppHandlerFunc(index)))
	AppHandleFunc("/login", login)
	AppHandleFunc("/_ah/login_required", login)
}

var (
	indexPage = template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
	loginPage = template.Must(template.ParseFiles("templates/base.html", "templates/login.html"))
)

func index(w http.ResponseWriter, r *http.Request) *Error {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClasses(true)
	data := map[string]interface{}{
		"Classes": classes,
	}
	if u := GetCurrentUser(r); u != nil {
		data["LoggedIn"] = true
		data["User"] = u
		if url, err := user.LogoutURL(c, "/"); err != nil {
			return InternalError(fmt.Errorf("Error creating logout url: %s", err))
		} else {
			data["LogoutURL"] = url
		}
	} else {
		data["LoggedIn"] = false
	}
	if err := indexPage.Execute(w, data); err != nil {
		return InternalError(err)
	}
	return nil
}

type directProvider struct {
	Name       string
	Identifier string
}

type directProviderLink struct {
	Name string
	URL  string
}

var (
	directProviders = []directProvider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "yahoo.com"},
		{"AOL", "aol.com"},
		{"MyOpenID", "myopenid.com"},
	}
)

// PathOrRoot parses a string into a URL and returns the path component. If the URL cannot be
// parsed, or if the path is empty, returns the root path ("/").
// 
// TODO(rwsims): This should probably allow the query string through as well.
func pathOrRoot(urlString string) string {
	u, err := url.Parse(urlString)
	if err != nil || u.Path == "" {
		return "/"
	}
	return u.Path
}

func login(w http.ResponseWriter, r *http.Request) *Error {
	redirect, err := url.Parse("/login/account")
	if err != nil {
		return InternalError(err)
	}
	q := redirect.Query()
	q.Set("continue", pathOrRoot(r.FormValue("continue")))
	redirect.RawQuery = q.Encode()
	c := appengine.NewContext(r)
	directProviderLinks := []*directProviderLink{}
	for _, provider := range directProviders {
		url, err := user.LoginURLFederated(c, redirect.String(), provider.Identifier)
		if err != nil {
			c.Errorf("Error creating URL for %s: %s", provider.Name, err)
			continue
		}
		directProviderLinks = append(directProviderLinks, &directProviderLink{
			Name: provider.Name,
			URL:  url,
		})
	}
	if err := loginPage.Execute(w, map[string]interface{}{
		"DirectProviders": directProviderLinks,
	}); err != nil {
		return InternalError(err)
	}
	return nil
}
