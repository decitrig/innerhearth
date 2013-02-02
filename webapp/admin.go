/*
 *  Copyright 2013 Ryan W Sims (rwsims@gmail.com)
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */
package webapp

import (
	"fmt"
	"html/template"
	"net/http"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/user"

	"github.com/decitrig/innerhearth/model"
)

var (
	adminIndex = template.Must(template.ParseFiles(
		"templates/admin/base.html", "templates/admin/index.html"))
	noEndDateFixup = template.Must(template.ParseFiles("templates/admin/fixup-no-end-date.html"))
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

func regsWithNoDate(c appengine.Context) []*datastore.Key {
	t, err := time.Parse("2006-01-02", "2013-01-01")
	if err != nil {
		panic(err)
	}
	q := datastore.NewQuery("Registration").
		Filter("StartDate <", t).
		KeysOnly()
	keys, err := q.GetAll(c, nil)
	if err != nil {
		c.Errorf("Error looking up regs: %s", err)
		return nil
	}
	return keys
}

func admin(w http.ResponseWriter, r *http.Request, u *adminUser) *Error {
	c := appengine.NewContext(r)
	keys := regsWithNoDate(c)
	if err := adminIndex.Execute(w, map[string]interface{}{
		"Admin":     u,
		"Email":     "rwsims@gmail.com",
		"NoEndDate": keys,
	}); err != nil {
		return InternalError(err)
	}
	return nil
}

func fixupNoEndDate(w http.ResponseWriter, r *http.Request, u *adminUser) *Error {
	if r.Method != "POST" {
		if err := noEndDateFixup.Execute(w, map[string]interface{}{
			"XSRFToken": u.Token.Token,
			"Key":       r.FormValue("key"),
		}); err != nil {
			return InternalError(err)
		}
		return nil
	}
	if !u.Token.Validate(r.FormValue("xsrf_token")) {
		return UnauthorizedError(fmt.Errorf("XSRF token failed validation"))
	}
	c := appengine.NewContext(r)
	key, err := datastore.DecodeKey(r.FormValue("key"))
	if err != nil {
		return InternalError(fmt.Errorf("Error decoding key %s: %s", r.FormValue("key")))
	}
	reg := &model.Registration{}
	if err := datastore.Get(c, key, reg); err != nil {
		return InternalError(fmt.Errorf("Error getting registration: %s", err))
	}
	class := &model.Class{}
	if err := datastore.Get(c, key.Parent(), class); err != nil {
		return InternalError(fmt.Errorf("Error looking up class: %s", err))
	}
	reg.Date = class.EndDate
	if _, err := datastore.Put(c, key, reg); err != nil {
		return InternalError(fmt.Errorf("Error writing registration: %s", err))
	}
	http.Redirect(w, r, "/admin", http.StatusFound)
	return nil
}
