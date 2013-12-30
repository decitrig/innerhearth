package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"appengine"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/staff"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	adminPage    = template.Must(template.ParseFiles("templates/base.html", "templates/admin/index.html"))
	addStaffPage = template.Must(template.ParseFiles("templates/base.html", "templates/admin/add-staff.html"))
)

func init() {
	webapp.HandleFunc("/admin", userContextHandler(webapp.HandlerFunc(admin)))
	webapp.HandleFunc("/admin/add-staff", userContextHandler(webapp.HandlerFunc(addStaff)))
}

func admin(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	staff, err := staff.All(c)
	if err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{
		"Staff": staff,
	}
	if err := adminPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addStaff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	adminAccount, ok := userContext(r)
	if !ok {
		return webapp.InternalError(fmt.Errorf("user not logged in"))
	}
	account, err := auth.AccountWithEmail(c, r.FormValue("email"))
	if err != nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find user for email %s", r.FormValue("email")))
	}
	if r.Method == "POST" {
		token, err := auth.TokenForRequest(c, adminAccount.AccountID, r.URL.Path)
		if err != nil {
			return webapp.UnauthorizedError(fmt.Errorf("didn't find an auth token"))
		}
		if !token.IsValid(r.FormValue("xsrf_token"), time.Now()) {
			return webapp.UnauthorizedError(fmt.Errorf("Invalid XSRF token provided"))
		}
		staff := staff.New(account)
		if err := staff.Store(c); err != nil {
			return webapp.InternalError(fmt.Errorf("Couldn't add %s as staff", account.Email))
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return nil
	}
	token, err := auth.NewToken(adminAccount.AccountID, r.URL.Path, time.Now())
	if err != nil {
		return webapp.InternalError(err)
	}
	if err := token.Store(c); err != nil {
		return webapp.InternalError(err)
	}
	data := map[string]interface{}{
		"Token": token.Encode(),
		"User":  account,
	}
	if err := addStaffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
