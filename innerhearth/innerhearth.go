package innerhearth

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"appengine"
	"appengine/user"

	"github.com/decitrig/innerhearth/model"
	"github.com/decitrig/innerhearth/webapp"
)

var (
	indexPage = template.Must(template.ParseFiles("templates/base.html", "templates/index.html"))
	loginPage = template.Must(template.ParseFiles("templates/base.html", "templates/login.html"))
)

func init() {
	http.Handle("/", webapp.Router)
	webapp.AppHandleFunc("/", index)
	webapp.AppHandleFunc("/login", login)
	webapp.AppHandleFunc("/_ah/login_required", login)
}

func index(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	scheduler := model.NewScheduler(c)
	classes := scheduler.ListClasses(true)
	data := map[string]interface{}{
		"Classes": classes,
	}
	if u := webapp.GetCurrentUser(r); u != nil {
		data["LoggedIn"] = true
		data["User"] = u
		if url, err := user.LogoutURL(c, "/"); err != nil {
			return webapp.InternalError(fmt.Errorf("Error creating logout url: %s", err))
		} else {
			data["LogoutURL"] = url
		}
	} else {
		data["LoggedIn"] = false
	}
	if err := indexPage.Execute(w, data); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}

type directProvider struct {
	Name       string
	Identifier string
}

type directProviderLink struct {
	Name string
	URL  string
}

var (
	directProviders = []directProvider{
		{"Google", "https://www.google.com/accounts/o8/id"},
		{"Yahoo", "yahoo.com"},
		{"AOL", "aol.com"},
		{"MyOpenID", "myopenid.com"},
	}
)

func login(w http.ResponseWriter, r *http.Request) *webapp.Error {
	redirect, err := url.Parse("/login/account")
	if err != nil {
		return webapp.InternalError(err)
	}
	q := redirect.Query()
	q.Set("continue", webapp.PathOrRoot(r.FormValue("continue")))
	redirect.RawQuery = q.Encode()
	c := appengine.NewContext(r)
	directProviderLinks := []*directProviderLink{}
	for _, provider := range directProviders {
		url, err := user.LoginURLFederated(c, redirect.String(), provider.Identifier)
		if err != nil {
			c.Errorf("Error creating URL for %s: %s", provider.Name, err)
			continue
		}
		directProviderLinks = append(directProviderLinks, &directProviderLink{
			Name: provider.Name,
			URL:  url,
		})
	}
	if err := loginPage.Execute(w, map[string]interface{}{
		"DirectProviders": directProviderLinks,
	}); err != nil {
		return webapp.InternalError(err)
	}
	return nil
}
