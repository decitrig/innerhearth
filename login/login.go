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
package login

import (
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"html/template"
	"net/http"
	"strings"
	"time"

	"appengine"
	"appengine/taskqueue"
	"appengine/user"

	"github.com/decitrig/innerhearth/model"
)

type loginHandler func(w http.ResponseWriter, r *http.Request) error

func (fn loginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("Login handler error: %s", err)
		http.Error(w, "An error occurred", http.StatusInternalServerError)
	}
}

func handle(path string, handler loginHandler) {
	http.Handle(path, handler)
}

func postOnly(handler loginHandler) loginHandler {
	return func(w http.ResponseWriter, r *http.Request) error {
		if r.Method != "POST" {
			http.Error(w, "Authorization error", http.StatusUnauthorized)
			return nil
		}
		return handler(w, r)
	}
}

type Provider struct {
	Name       string
	Identifier string
}

var (
	directProviders = map[string]string{
		"Google":   "https://www.google.com/accounts/o8/id",
		"Yahoo":    "yahoo.com",
		"AOL":      "aol.com",
		"MyOpenID": "myopenid.com",
	}
	usernameProviders = map[string]string{
		"Flickr":    "flickr.com/{{.}}",
		"WordPress": "{{.}}.wordpress.com",
	}
	loginPage          = template.Must(template.ParseFiles("login/login.html"))
	newAccountPage     = template.Must(template.ParseFiles("login/new-account.html"))
	accountConfirmPage = template.Must(template.ParseFiles("login/confirm-account.html"))
	adminPage          = template.Must(template.ParseFiles("login/admin.html"))
	editRolePage       = template.Must(template.ParseFiles("login/edit-role.html"))
)

func init() {
	handle("/_ah/login_required", login)
	handle("/login/account", accountCheck)
	handle("/login/account/new", postOnly(createNewAccount))
	handle("/login/confirm", confirmNewAccount)
	handle("/login/confirm/resend", resendConfirmEmail)
	handle("/login/admin", admin)
	handle("/login/admin/edit-role", editUserRole)
}

func login(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	target := r.FormValue("continue")
	if target == "" {
		target = "/"
	}
	providerMap := make(map[string]string, 0)
	for name, id := range directProviders {
		if login, err := user.LoginURLFederated(c, "/login/account?continue="+target, id); err == nil {
			providerMap[name] = login
		}
	}
	data := map[string]interface{}{
		"DirectProviders": providerMap,
	}
	if err := loginPage.Execute(w, data); err != nil {
		return fmt.Errorf("Error rendering login page template: %s", err)
	}
	return nil
}

func admin(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	u, err := model.GetCurrentUserAccount(c)
	if err != nil {
		return fmt.Errorf("No logged in user")
	}
	logout, err := user.LogoutURL(c, r.URL.String())
	if err != nil {
		return fmt.Errorf("Error getting logout url: %s", err)
	}
	if err := adminPage.Execute(w, map[string]interface{}{
		"Email":     u.Email,
		"LogoutURL": logout,
		"Staff":     model.ListRoleAccounts(c, model.RoleStaff),
	}); err != nil {
		return fmt.Errorf("Error rendering admin page: %s", err)
	}
	return nil
}

func accountCheck(w http.ResponseWriter, r *http.Request) error {
	target := r.FormValue("continue")
	if target == "" {
		target = "/"
	}
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return fmt.Errorf("No logged in user")
	}
	if model.HasAccount(c, u) {
		http.Redirect(w, r, target, http.StatusSeeOther)
	}
	xsrfToken, err := model.GetXSRFToken(c, u.ID)
	if err != nil {
		return fmt.Errorf("Error getting XSRF token: %s", err)
	}
	data := map[string]interface{}{
		"XSRFToken": xsrfToken.Token,
		"Target":    target,
	}
	if err := newAccountPage.Execute(w, data); err != nil {
		return fmt.Errorf("Error executing new account template: %s", err)
	}
	return nil
}

func getRequiredFields(r *http.Request, fields ...string) (map[string]string, error) {
	values := map[string]string{}
	for _, f := range fields {
		v := r.FormValue(f)
		if v == "" {
			return nil, fmt.Errorf("Could not find value for %s", f)
		}
		values[f] = v
	}
	return values, nil
}

func newConfirmCode(email string) string {
	hash := sha512.New()
	hash.Write([]byte(email))
	hash.Write([]byte(time.Now().String()))
	return strings.Trim(base64.URLEncoding.EncodeToString(hash.Sum(nil)), "=")
}

func createNewAccount(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return fmt.Errorf("No logged in user")
	}
	if !model.ValidXSRFToken(c, u.ID, r.FormValue("xsrf_token")) {
		return fmt.Errorf("Invalid XSRF token")
	}
	values, err := getRequiredFields(r, "email", "firstname", "lastname")
	if err != nil {
		fmt.Fprintf(w, "Please go back and enter all required data.")
		return fmt.Errorf("Missing required fields: %s", err)
	}
	if err := model.ClaimEmail(c, u.ID, values["email"]); err != nil {
		if err == model.ErrEmailInUse {
			fmt.Fprintf(w, "That email is in use, please go back and choose another email.")
			return nil
		}
		return fmt.Errorf("Error claiming email %s: %s", values["email"], err)
	}
	account := &model.UserAccount{
		FirstName:        values["firstname"],
		LastName:         values["lastname"],
		Email:            values["email"],
		ConfirmationCode: newConfirmCode(values["email"]),
	}
	if err := model.StoreAccount(c, u, account); err != nil {
		return fmt.Errorf("Error storing user account: %s", err)
	}
	t := taskqueue.NewPOSTTask("/task/email-account-confirmation", map[string][]string{
		"email": {account.Email},
		"code":  {account.ConfirmationCode},
	})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		c.Errorf("Error enqueuing account email task: %s", err)
	}
	http.Redirect(w, r, r.FormValue("target"), http.StatusSeeOther)
	return nil
}

func confirmNewAccount(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return fmt.Errorf("No logged in user")
	}
	account, err := model.GetAccount(c, u)
	if err != nil {
		return fmt.Errorf("Error looking up account for user %s: %s", u.ID, err)
	}
	xsrfToken, err := model.GetXSRFToken(c, account.AccountID)
	if err != nil {
		return fmt.Errorf("Error getting XSRF token: %s", err)
	}
	if r.Method == "POST" {
		if !model.ValidXSRFToken(c, account.AccountID, r.FormValue("xsrf_token")) {
			return fmt.Errorf("Invalid XSRF token")
		}
		values, err := getRequiredFields(r, "code")
		if err != nil {
			return fmt.Errorf("Missing required fields: %s", err)
		}
		if err := model.ConfirmAccount(c, values["code"], account); err != nil {
			return fmt.Errorf("Couldn't confirm account %s: %s", account.AccountID, err)
		}
		http.Redirect(w, r, "/registration", http.StatusSeeOther)
		return nil
	}

	if err := accountConfirmPage.Execute(w, map[string]interface{}{
		"Email":     account.Email,
		"XSRFToken": xsrfToken.Token,
		"Code":      r.FormValue("code"),
	}); err != nil {
		return fmt.Errorf("Error rendering template: %s", err)
	}
	return nil
}

func resendConfirmEmail(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	u := user.Current(c)
	if u == nil {
		return fmt.Errorf("No logged in user")
	}
	account, err := model.GetAccount(c, u)
	if err != nil {
		return fmt.Errorf("Error looking up account for user %s: %s", u.ID, err)
	}
	if r.Method != "POST" {
		http.Error(w, "Not found", http.StatusNotFound)
	}
	if !model.ValidXSRFToken(c, account.AccountID, r.FormValue("xsrf_token")) {
		return fmt.Errorf("Invalid XSRF token")
	}
	t := taskqueue.NewPOSTTask("/task/email-account-confirmation", map[string][]string{
		"email": {account.Email},
		"code":  {account.ConfirmationCode},
	})
	if _, err := taskqueue.Add(c, t, ""); err != nil {
		c.Errorf("Error enqueuing account email task: %s", err)
	}
	http.Redirect(w, r, r.FormValue("target"), http.StatusSeeOther)
	return nil
}

func editUserRole(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	current, err := model.GetCurrentUserAccount(c)
	if err != nil {
		return fmt.Errorf("No logged in user: %s", err)
	}
	if r.Method == "POST" {
		fields, err := getRequiredFields(r, "xsrf_token", "userid", "role")
		if err != nil {
			return fmt.Errorf("Couldn't parse required fields: %s", err)
		}
		if !model.ValidXSRFToken(c, current.AccountID, fields["xsrf_token"]) {
			return fmt.Errorf("Invalid XSRF token")
		}
		account, err := model.GetAccountByID(c, fields["userid"])
		if err != nil {
			return fmt.Errorf("Couldn't find user account %s: %s", account.AccountID, err)
		}
		account.SetRole(model.ParseRole(fields["role"]))
		if err := model.StoreAccount(c, nil, account); err != nil {
			return fmt.Errorf("Error writing account %s: %s", account.AccountID, err)
		}
		http.Redirect(w, r, "/login/admin", http.StatusSeeOther)
	}
	xsrfToken, err := model.GetXSRFToken(c, current.AccountID)
	if err != nil {
		return fmt.Errorf("Error getting XSRF token: %s", err)
	}
	fields, err := getRequiredFields(r, "email")
	if err != nil {
		return fmt.Errorf("Couldn't parse required fields: %s", err)
	}
	account := model.GetAccountByEmail(c, fields["email"])
	if account == nil {
		return fmt.Errorf("Couldnt' find account for %s", fields["email"])
	}
	if err := editRolePage.Execute(w, map[string]interface{}{
		"Account":   account,
		"XSRFToken": xsrfToken.Token,
	}); err != nil {
		return fmt.Errorf("Couldn't execute template: %s", err)
	}
	return nil
}
