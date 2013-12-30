package innerhearth

import (
	"fmt"
	"net/http"

	"appengine"
	"appengine/user"

	"github.com/gorilla/context"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/staff"
	"github.com/decitrig/innerhearth/webapp"
)

const (
	userAccountKey = iota
	staffKey
	teacherKey
)

func userContext(r *http.Request) (*auth.UserAccount, bool) {
	if user, ok := context.GetOk(r, userAccountKey); ok {
		u, ok := user.(*auth.UserAccount)
		return u, ok
	}
	return nil, false
}

func setUserContext(r *http.Request, account *auth.UserAccount) {
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
		account, err := auth.LookupUser(c, u)
		switch {
		case err == nil:
			break
		case err == auth.ErrUserNotFound:
			old, err := auth.LookupOldUser(c, u)
			switch {
			case err == nil:
				c.Infof("found old-style user: %v", old)
				if convertErr := old.ConvertToNewUser(c, u); convertErr != nil {
					return webapp.InternalError(fmt.Errorf("failed to convert old user: %s", err))
				}
			case err == auth.ErrUserNotFound:
				webapp.RedirectToLogin(w, r, r.URL.Path)
				return nil
			default:
				return webapp.InternalError(fmt.Errorf("failed to look up old user: %s", err))
			}
		default:
			return webapp.InternalError(fmt.Errorf("failed to look up current user: %s", err))
		}
		setUserContext(r, account)
		return handler.Serve(w, r)
	}
}

func staffContextHandler(handler webapp.Handler) webapp.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		c := appengine.NewContext(r)
		account, ok := userContext(r)
		if !ok {
			return webapp.InternalError(fmt.Errorf("staff context requires user context"))
		}
		staffer, err := staff.ForUserAccount(c, account)
		switch {
		case err == nil:
			break
		case err == staff.ErrUserIsNotStaff:
			return webapp.UnauthorizedError(fmt.Errorf("%s is not staff", account.Email))
		default:
			return webapp.InternalError(fmt.Errorf("failed to look up staff for %q: %s", account.AccountID, err))
		}
		setStaffContext(r, staffer)
		return handler.Serve(w, r)
	}
}
