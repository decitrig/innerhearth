package webapp

import (
	"net/http"

	"appengine"
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

type AppHandler interface {
	Serve(w http.ResponseWriter, r *http.Request) *Error
}

type AppHandlerFunc func(w http.ResponseWriter, r *http.Request) *Error

func (fn AppHandlerFunc) Serve(w http.ResponseWriter, r *http.Request) *Error {
	return fn(w, r)
}

func AppHandle(path string, h AppHandler) {
	http.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		if err := h.Serve(w, r); err != nil {
			c := appengine.NewContext(r)
			c.Errorf("%s", err)
			http.Error(w, err.Message, err.Code)
		}
	})
}

func AppHandleFunc(path string, f AppHandlerFunc) {
	AppHandle(path, AppHandler(f))
}
