package innerhearth

import (
	"fmt"
	"net/http"
	"strings"

	"appengine"

	"github.com/decitrig/innerhearth/auth"
	"github.com/decitrig/innerhearth/webapp"
)

func init() {
	webapp.HandleFunc("/login", login)
	webapp.HandleFunc("/_ah/login_required", login)
}

func login(w http.ResponseWriter, r *http.Request) *webapp.Error {
	c := appengine.NewContext(r)
	target := r.FormValue("continue")
	if !strings.HasPrefix(target, "/") {
		target = "/"
	}
	links, err := auth.MakeLinkList(c, auth.OpenIDProviders, target)
	if err != nil {
		return webapp.InternalError(fmt.Errorf("failed to create login links: %s", err))
	}
	data := map[string]interface{}{
		"LoginLinks": links,
	}
	if err := loginPage.Execute(w, data); err != nil {
		return webapp.InternalError(fmt.Errorf("Error rendering login page template: %s", err))
	}
	return nil
}
