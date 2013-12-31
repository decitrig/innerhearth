package innerhearth

import (
	"fmt"
	"net/http"

	"appengine"
	"appengine/user"

	"github.com/gorilla/context"

	"github.com/decitrig/innerhearth/account"
	"github.com/decitrig/innerhearth/staff"
	"github.com/decitrig/innerhearth/webapp"
)

const (
	userAccountKey = iota
	staffKey
	teacherKey
)

func userContext(r *http.Request) (*account.Account, bool) {
	if user, ok := context.GetOk(r, userAccountKey); ok {
		u, ok := user.(*account.Account)
		return u, ok
	}
	return nil, false
}

func setUserContext(r *http.Request, account *account.Account) {
	context.Set(r, userAccountKey, account)
}

func staffContext(r *http.Request) (*staff.Staff, bool) {
	if staffer, ok := context.GetOk(r, staffKey); ok {
		return staffer.(*staff.Staff), true
	}
	return nil, false
}

func setStaffContext(r *http.Request, staff *staff.Staff) {
	context.Set(r, staffKey, staff)
}

func userContextHandler(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		c := appengine.NewContext(r)
		u := user.Current(c)
		if u == nil {
			webapp.RedirectToLogin(w, r, r.URL.Path)
			return nil
		}
		switch acct, err := maybeOldAccount(c, u); err {
		case nil:
			setUserContext(r, acct)
			return handler.Serve(w, r)
		case account.ErrUserNotFound:
			http.Redirect(w, r, "/login/new", http.StatusSeeOther)
			return nil
		default:
			return webapp.InternalError(fmt.Errorf("failed to look up current user: %s", err))
		}
	}
}

func staffContextHandler(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		c := appengine.NewContext(r)
		account, ok := userContext(r)
		if !ok {
			return webapp.InternalError(fmt.Errorf("staff context requires user context"))
		}
		u := user.Current(c)
		switch staffer, err := maybeOldStaff(c, account, u); err {
		case nil:
			setStaffContext(r, staffer)
			return handler.Serve(w, r)
		case staff.ErrUserIsNotStaff:
			return webapp.UnauthorizedError(fmt.Errorf("%s is not staff", account.Email))
		default:
			return webapp.InternalError(fmt.Errorf("failed to look up staff for %q: %s", account.ID, err))
		}
	}
}
