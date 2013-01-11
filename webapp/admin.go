package webapp

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/user"

	"model"
)

var (
	index = template.Must(template.ParseFiles("templates/admin/base.html", "templates/admin/index.html"))
)

type adminUser struct {
	*model.UserAccount
	LogoutURL string
}

func newAdminUser(c appengine.Context) *adminUser {
	account, err := model.GetCurrentUserAccount(c)
	if account == nil {
		c.Errorf("Couldn't look up current user: %s", err)
		return nil
	}
	if !account.Role.IsStaff() {
		c.Errorf("User %s is not staff", account.Email)
		return nil
	}
	logout, err := user.LogoutURL(c, "/registration")
	if err != nil {
		c.Errorf("Error creating logout url: %s", err)
		return nil
	}
	return &adminUser{account, logout}
}

type adminHandler func(w http.ResponseWriter, r *http.Request, u *adminUser) *Error

func (fn adminHandler) Serve(w http.ResponseWriter, r *http.Request) *Error {
	c := appengine.NewContext(r)
	u := newAdminUser(c)
	if u == nil {
		return UnauthorizedError(fmt.Errorf("No authorized staff member logged in"))
	}
	return fn(w, r, u)
}

func handle(path string, h adminHandler) {
	AppHandle("/admin"+path, h)
}

func init() {
	handle("", admin)
}

func admin(w http.ResponseWriter, r *http.Request, u *adminUser) *Error {
	if err := index.Execute(w, map[string]interface{}{
		"Admin": u,
		"Email": "rwsims@gmail.com",
	}); err != nil {
		return InternalError(err)
	}
	return nil
}
