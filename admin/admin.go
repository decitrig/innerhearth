package admin

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	indexPage    = template.Must(template.ParseFiles("templates/base.html", "templates/admin/index.html"))
	addStaffPage = template.Must(template.ParseFiles("templates/base.html", "templates/admin/add-staff.html"))
)

func init() {
	webapp.AppHandleFunc("/admin", index)
	webapp.AppHandleFunc("/admin/add-staff", addStaff)
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	data := map[string]interface{}{
		"Staff": model.ListStaff(c),
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addStaff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	account := model.GetAccountByEmail(c, r.FormValue("email"))
	if account == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find user for email %s", r.FormValue("email")))
	}
	token := webapp.GetXSRFToken(r)
	if token == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't get XSRF token"))
	}
	if r.Method == "POST" {
		if !token.Validate(r.FormValue("xsrf_token")) {
			return webapp.UnauthorizedError(fmt.Errorf("Invalid XSRF token provided"))
		}
		if staff := model.AddNewStaff(c, account); staff == nil {
			return webapp.InternalError(fmt.Errorf("Couldn't add %s as staff", account.Email))
		}
		http.Redirect(w, r, "/admin", http.StatusSeeOther)
		return nil
	}
	data := map[string]interface{}{
		"Token": token,
		"User":  account,
	}
	if err := addStaffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
