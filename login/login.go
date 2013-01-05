package login

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/user"

	"model"
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
	loginPage      = template.Must(template.ParseFiles("login/login.html"))
	newAccountPage = template.Must(template.ParseFiles("login/new-account.html"))
)

func init() {
	handle("/_ah/login_required", login)
	handle("/login/account", accountCheck)
	handle("/login/account/new", postOnly(createNewAccount))
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
		FirstName: values["firstname"],
		LastName:  values["lastname"],
		Email:     values["email"],
	}
	if err := model.StoreAccount(c, u, account); err != nil {
		return fmt.Errorf("Error storing user account: %s", err)
	}
	http.Redirect(w, r, r.FormValue("target"), http.StatusSeeOther)
	return nil
}
