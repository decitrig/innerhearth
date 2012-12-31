package login

import (
	"fmt"
	"html/template"
	"net/http"

	"appengine"
	"appengine/user"
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

type Provider struct {
	Name       string
	Identifier string
}

var (
	providers = []Provider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "https://me.yahoo.com"},
	}
	loginPage = template.Must(template.ParseFiles("login/login.html"))
)

func init() {
	handle("/_ah/login_required", login)
}

func login(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	providerMap := make(map[string]string, 0)
	for _, provider := range providers {
		providerMap[provider.Name], _ = user.LoginURLFederated(c, "/", provider.Identifier)
	}
	data := map[string]interface{}{
		"Providers": providerMap,
	}
	if err := loginPage.Execute(w, data); err != nil {
		return fmt.Errorf("Error rendering login page template: %s", err)
	}
	return nil
}
