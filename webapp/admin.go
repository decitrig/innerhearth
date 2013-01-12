package webapp

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"

	"model"
)

var (
	index = template.Must(template.ParseFiles("templates/admin/base.html", "templates/admin/index.html"))
)

type adminUser struct {
	*model.UserAccount
	LogoutURL string
	Token     *model.AdminXSRFToken
}

func newAdminUser(r *http.Request) *adminUser {
	c := appengine.NewContext(r)
	account, err := model.GetCurrentUserAccount(c)
	if account == nil {
		c.Errorf("Couldn't look up current user: %s", err)
		return nil
	}
	token, err := model.GetXSRFToken(c, account.AccountID)
	if err != nil {
		c.Errorf("Error looking up XSRF token: %s", err)
		return nil
	}
	SetXSRFToken(r, token)
	if !account.Role.IsStaff() {
		c.Errorf("User %s is not staff", account.Email)
		return nil
	}
	logout, err := user.LogoutURL(c, "/registration")
	if err != nil {
		c.Errorf("Error creating logout url: %s", err)
		return nil
	}
	return &adminUser{account, logout, GetXSRFToken(r)}
}

type adminHandler func(w http.ResponseWriter, r *http.Request, u *adminUser) *Error

func (fn adminHandler) Serve(w http.ResponseWriter, r *http.Request) *Error {
	u := newAdminUser(r)
	if u == nil {
		return UnauthorizedError(fmt.Errorf("No authorized staff member found"))
	}
	return fn(w, r, u)
}

func handle(path string, h adminHandler) {
	AppHandle("/admin"+path, h)
}

func init() {
	handle("", admin)
	handle("/fixup/no-end-date", fixupNoEndDate)
}

type regInfo struct {
	*model.UserAccount
	*model.Class
}

func regsWithNoDate(c appengine.Context) []*regInfo {
	c.Infof("Looking up registrations without expiration dates")
	q := datastore.NewQuery("Registration").
		Filter("Date =", time.Time{}).
		KeysOnly()
	keys, err := q.GetAll(c, nil)
	if err != nil {
		c.Errorf("Error looking up regs: %s", err)
		return nil
	}
	c.Infof("Found %d registrations", len(keys))
	classKeys := make([]*datastore.Key, len(keys))
	accountKeys := make([]*datastore.Key, len(keys))
	for i, k := range keys {
		classKeys[i] = k.Parent()
		accountKeys[i] = datastore.NewKey(c, "UserAccount", k.StringID(), 0, nil)
	}
	classes := make([]*model.Class, len(keys))
	students := make([]*model.UserAccount, len(keys))
	for i, _ := range classes {
		classes[i] = &model.Class{}
		students[i] = &model.UserAccount{}
	}
	if err := datastore.GetMulti(c, classKeys, classes); err != nil {
		c.Errorf("Error looking up classes: %s", err)
		return nil
	}
	if err := datastore.GetMulti(c, accountKeys, students); err != nil {
		c.Errorf("Error looking up accounts: %s", err)
		return nil
	}
	infos := make([]*regInfo, len(classes))
	for i, _ := range classes {
		infos[i] = &regInfo{students[i], classes[i]}
	}
	return infos
}

func admin(w http.ResponseWriter, r *http.Request, u *adminUser) *Error {
	c := appengine.NewContext(r)
	rs := regsWithNoDate(c)
	if err := index.Execute(w, map[string]interface{}{
		"Admin":     u,
		"Email":     "rwsims@gmail.com",
		"NoEndDate": rs,
	}); err != nil {
		return InternalError(err)
	}
	return nil
}

func fixupNoEndDate(w http.ResponseWriter, r *http.Request, u *adminUser) *Error {
	if r.Method != "POST" {
		http.NotFound(w, r)
		return nil
	}
	if !u.Token.Validate(r.FormValue("xsrf_token")) {
		return UnauthorizedError(fmt.Errorf("XSRF token failed validation"))
	}
	c := appengine.NewContext(r)
	q := datastore.NewQuery("Registration").
		Filter("Date =", time.Time{})
	regs := []*model.Registration{}
	keys, err := q.GetAll(c, &regs)
	if err != nil {
		return InternalError(fmt.Errorf("Error looking up regs: %s", err))
	}
	classKeys := make([]*datastore.Key, len(keys))
	for i, k := range keys {
		classKeys[i] = k.Parent()
	}
	classes := make([]*model.Class, len(keys))
	for i, _ := range classes {
		classes[i] = &model.Class{}
	}
	if err := datastore.GetMulti(c, classKeys, classes); err != nil {
		return InternalError(fmt.Errorf("Error looking up classes: %s", err))
	}
	for i, _ := range classes {
		regs[i].Date = classes[i].GetExpirationTime()
	}
	if _, err := datastore.PutMulti(c, keys, regs); err != nil {
		return InternalError(fmt.Errorf("Error updating registration expirations: %s"))
	}
	return nil
}
