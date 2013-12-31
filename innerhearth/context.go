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
		acct, err := account.ForUser(c, u)
		switch {
		case err == nil:
			break
		case err == account.ErrUserNotFound:
			old, err := account.OldAccountForUser(c, u)
			switch {
			case err == nil:
				c.Infof("found old-style user: %v", old)
				if convertErr := old.RewriteID(c, u); convertErr != nil {
					return webapp.InternalError(fmt.Errorf("failed to convert old user: %s", err))
				}
			case err == account.ErrUserNotFound:
				http.Redirect(w, r, "/login/new", http.StatusSeeOther)
				return nil
			default:
				return webapp.InternalError(fmt.Errorf("failed to look up old user: %s", err))
			}
		default:
			return webapp.InternalError(fmt.Errorf("failed to look up current user: %s", err))
		}
		setUserContext(r, acct)
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
			return webapp.InternalError(fmt.Errorf("failed to look up staff for %q: %s", account.ID, err))
		}
		setStaffContext(r, staffer)
		return handler.Serve(w, r)
	}
}
