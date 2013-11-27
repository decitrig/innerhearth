/*  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package webapp

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"appengine"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"

	"github.com/decitrig/innerhearth/model"
)

type Error struct {
	Err     error
	Message string
	Code    int
}

var (
	Router       = mux.NewRouter()
	notFoundPage = template.Must(template.ParseFiles("templates/base.html", "templates/error/not-found.html"))
)

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

func GetXSRFToken(r *http.Request) *model.XSRFToken {
	token := context.Get(r, xsrfTokenKey)
	if token != nil {
		return token.(*model.XSRFToken)
	}
	u := GetCurrentUser(r)
	c := appengine.NewContext(r)
	if u == nil {
		c.Errorf("Couldn't get XSRF token: no logged-in user")
		return nil
	}
	if t, err := model.GetXSRFToken(c, u.AccountID); err != nil {
		c.Errorf("Error getting XSRF token: %s", err)
		return nil
	} else {
		return t
	}
	return nil
}

func SetXSRFToken(r *http.Request, t *model.XSRFToken) {
	context.Set(r, xsrfTokenKey, t)
}

func GetCurrentUser(r *http.Request) *model.UserAccount {
	u := context.Get(r, currentUserKey)
	if u == nil {
		c := appengine.NewContext(r)
		u = model.MaybeGetCurrentUser(c)
		context.Set(r, currentUserKey, u)
	}
	if u != nil {
		return u.(*model.UserAccount)
	}
	return nil
}

type Handler interface {
	Serve(w http.ResponseWriter, r *http.Request) *Error
}

type HandlerFunc func(w http.ResponseWriter, r *http.Request) *Error

func (fn HandlerFunc) Serve(w http.ResponseWriter, r *http.Request) *Error {
	return fn(w, r)
}

func Handle(path string, h Handler) {
	Router.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if err := h.Serve(w, r); err != nil {
			c := appengine.NewContext(r)
			c.Errorf("%s", err)
			http.Error(w, err.Message, err.Code)
		}
	})
}

func HandleFunc(path string, f HandlerFunc) {
	Handle(path, Handler(f))
}

func init() {
	Router.NotFoundHandler = http.HandlerFunc(notFoundHandler)
}

func notFoundHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	if err := notFoundPage.Execute(w, nil); err != nil {
		http.Error(w, "An internal error ocurred, sorry!", http.StatusInternalServerError)
	}
}

func PostOnly(handler Handler) Handler {
	return HandlerFunc(func(w http.ResponseWriter, r *http.Request) *Error {
		if r.Method != "POST" {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return nil
		}
		return handler.Serve(w, r)
	})
}

// PathOrRoot parses a string into a URL and returns the path component. If the URL cannot be
// parsed, or if the path is empty, returns the root path ("/").
//
// TODO(rwsims): This should probably allow the query string through as well.
func PathOrRoot(urlString string) string {
	u, err := url.Parse(urlString)
	if err != nil || u.Path == "" {
		return "/"
	}
	return u.Path
}

// ParseRequiredValues checks for a form value in r for each key in keys and returns a map from key
// to it's first value. If a value is not found for a key, returns an error.
func ParseRequiredValues(r *http.Request, keys ...string) (map[string]string, error) {
	out := map[string]string{}
	for _, key := range keys {
		if v := r.FormValue(key); v == "" {
			return nil, fmt.Errorf("Missing value for %s", key)
		} else {
			out[key] = v
		}
	}
	return out, nil
}

func RedirectToLogin(w http.ResponseWriter, r *http.Request, urlString string) {
	u, _ := url.Parse("/login")
	q := u.Query()
	q.Set("continue", urlString)
	u.RawQuery = q.Encode()
	http.Redirect(w, r, u.String(), http.StatusTemporaryRedirect)
}
