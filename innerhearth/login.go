package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"appengine"
	"appengine/user"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	newAccountPage = template.Must(template.ParseFiles("templates/base.html", "templates/new-account.html"))
)

func init() {
	webapp.HandleFunc("/login", login)
	webapp.HandleFunc("/_ah/login_required", login)
	webapp.HandleFunc("/login/new", newAccount)
}

func continueTarget(r *http.Request) string {
	target := r.FormValue("continue")
	if !strings.HasPrefix(target, "/") {
		return "/"
	}
	return target
}

func login(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	target := continueTarget(r)
	links, err := auth.MakeLinkList(c, auth.OpenIDProviders, target)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("failed to create login links: %s", err))
	}
	data := map[string]interface{}{
		"LoginLinks": links,
	}
	if err := loginPage.Execute(w, data); err != nil {
		return webapp.InternalError(fmt.Errorf("Error rendering login page template: %s", err))
	}
	return nil
}

func newAccount(w http.ResponseWriter, r *http.Request) *webapp.Error {
	target := continueTarget(r)
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		webapp.RedirectToLogin(w, r, "/")
		return nil
	}
	if _, err := auth.AccountForUser(c, u); err != auth.ErrUserNotFound {
		if err != nil {
			return webapp.InternalError(fmt.Errorf("failed to account for current user: %s", err))
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return nil
	}
	id, err := auth.AccountID(u)
	if err != nil {
		return webapp.InternalError(err)
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, id, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("no stored token for request"))
		}
		if !token.IsValid(r.FormValue(auth.TokenFieldName), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("invalid XSRF token"))
		}
		fields, err := webapp.ParseRequiredValues(r, "email", "firstname", "lastname")
		if err != nil {
			// TODO(rwsims): Clean up this error reporting.
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "Please go back and enter all required data.")
			return nil
		}
		claim := auth.NewClaimedEmail(c, id, fields["email"])
		switch err := claim.Claim(c); {
		case err == nil:
			break
		case err == auth.ErrEmailAlreadyClaimed:
			// TODO(rwsims): Clean up this error reporting.
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprintf(w, "That email is already in use; please use a different email")
			return nil
		default:
			return webapp.InternalError(fmt.Errorf("failed to claim email %q: %s", claim.Email, err))
		}
		info := auth.UserInfo{
			FirstName: fields["firstname"],
			LastName:  fields["lastname"],
			Email:     fields["email"],
		}
		if phone := r.FormValue("phone"); phone != "" {
			info.Phone = phone
		}
		account, err := auth.NewUserAccount(u, info)
		if err != nil {
			return webapp.InternalError(fmt.Errorf("failed to create user account: %s", err))
		}
		if err := account.Store(c); err != nil {
			return webapp.InternalError(fmt.Errorf("failed to write new user account: %s", err))
		}
		if err := account.SendConfirmation(c); err != nil {
			c.Errorf("Failed to send confirmation email to %q: %s", account.Email, err)
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
		token.Delete(c)
		return nil
	}
	token, err := auth.NewToken(id, r.URL.Path, time.Now())
	if err != nil {
		return webapp.InternalError(fmt.Errorf("failed to create auth token: %s", err))
	}
	if err := token.Store(c); err != nil {
		return webapp.InternalError(fmt.Errorf("failed to store token: %s", err))
	}
	data := map[string]interface{}{
		"Target": target,
		"Token":  token.Encode(),
	}
	if err := newAccountPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
