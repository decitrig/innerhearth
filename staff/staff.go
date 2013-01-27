package staff

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	staffPage      = template.Must(template.ParseFiles("templates/base.html", "templates/staff/index.html"))
	addTeacherPage = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-teacher.html"))
	addClassPage   = template.Must(template.ParseFiles("templates/base.html", "templates/staff/add-class.html"))
)

func staffOnly(handler webapp.AppHandler) webapp.AppHandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) *webapp.Error {
		u := webapp.GetCurrentUser(r)
		if u == nil {
			webapp.RedirectToLogin(w, r, r.URL.Path)
			return nil
		}
		if !u.Role.IsStaff() {
			return webapp.UnauthorizedError(fmt.Errorf("%s is not staff", u.Email))
		}
		return handler.Serve(w, r)
	}
}

func handle(path string, handler webapp.AppHandler) {
	webapp.AppHandleFunc(path, staffOnly(handler))
}

func handleFunc(path string, fn webapp.AppHandlerFunc) {
	webapp.AppHandleFunc(path, staffOnly(fn))
}

func init() {
	handleFunc("/staff", staff)
	handleFunc("/staff/add-teacher", addTeacher)
	handleFunc("/staff/add-class", addClass)
}

func staff(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	data := map[string]interface{}{
		"Teachers": model.ListTeachers(c),
	}
	if err := staffPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addTeacher(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	vals, err := webapp.ParseRequiredValues(r, "email")
	if err != nil {
		return webapp.InternalError(err)
	}
	account := model.GetAccountByEmail(c, vals["email"])
	if account == nil {
		return webapp.InternalError(fmt.Errorf("Couldn't find account for '%s'", vals["email"]))
	}
	token := webapp.GetXSRFToken(r)
	if r.Method == "POST" {
		if t := r.FormValue("xsrf_token"); !token.Validate(t) {
			return webapp.UnauthorizedError(fmt.Errorf("Unauthorized XSRF token given: %s", t))
		}
		if teacher := model.AddNewTeacher(c, account); teacher == nil {
			return webapp.InternalError(fmt.Errorf("Couldn't create teacher for %s", account.Email))
		}
		http.Redirect(w, r, "/staff", http.StatusTemporaryRedirect)
		return nil
	}
	data := map[string]interface{}{
		"Token": token.Token,
		"User":  account,
	}
	if err := addTeacherPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

func addClass(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	data := map[string]interface{}{
		"Teachers": model.ListTeachers(c),
	}
	if err := addClassPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
