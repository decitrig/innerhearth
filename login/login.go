package login

import (
	"fmt"
	"html/template"
	"net/http"
	"net/url"

	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"

	"openid"
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

func urlMustParse(urlString string) *url.URL {
	u, err := url.Parse(urlString)
	if err != nil {
		panic(err)
	}
	return u
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
	handle("/login/discover", openIDDiscovery)
	handle("/login/verify", openIDVerification)
}

func login(w http.ResponseWriter, r *http.Request) error {
	data := map[string]interface{}{
		"Providers": providers,
	}
	if err := loginPage.Execute(w, data); err != nil {
		return fmt.Errorf("Error rendering login page template: %s", err)
	}
	return nil
}

func storeAssociation(c appengine.Context, assoc *openid.Association) {
	key := datastore.NewKey(c, "OpenIDAssociation", assoc.Endpoint, 0, nil)
	if _, err := datastore.Put(c, key, assoc); err != nil {
		c.Errorf("Error writing association for %s: %s", assoc.Endpoint, err)
	}
	// TODO(rwsims): cache associations
}

func lookupAssociation(c appengine.Context, endpoint string) *openid.Association {
	key := datastore.NewKey(c, "OpenIDAssociation", endpoint, 0, nil)
	assoc := &openid.Association{}
	// TODO(rwsims): cache associations
	if err := datastore.Get(c, key, assoc); err != nil {
		return nil
	}
	if assoc.Expired() {
		return nil
	}
	assoc.Endpoint = endpoint
	return assoc
}

func openIDDiscovery(w http.ResponseWriter, r *http.Request) error {
	// TODO(rwsims): Actual normalization, as per spec.
	discoveryURL, err := url.Parse(r.FormValue("openid_identifier"))
	if err != nil {
		return fmt.Errorf("Error parsing discovery url: %s", err)
	}
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	// TODO(rwsims): Need to handle claimed_id discovery, so DiscoverEndpoint needs to return an
	//               endpoint URL and (possibly) a claimed ID.
	endpoint, err := openid.DiscoverEndpoint(client, discoveryURL)
	if err != nil {
		return fmt.Errorf("Error during endpoint discovery on %s: %s", discoveryURL, err)
	}

	request := openid.NewAuthorizationRequest("", "http://innerhearthyoga.appspot.com/login/verify")
	assoc := lookupAssociation(c, endpoint.String())
	if assoc == nil {
		assoc, err := endpoint.RequestAssociation(client, nil)
		if err != nil {
			c.Infof("Unable to request association with %s: %s", endpoint, err)
		} else {
			storeAssociation(c, assoc)
		}
	}
	request.Association = assoc
	endpoint.SendAuthorizationRequest(w, r, request)
	return nil
}

func openIDVerification(w http.ResponseWriter, r *http.Request) error {
	response, err := openid.ParseIndirectResponse(r)
	if err != nil {
		return fmt.Errorf("Couldn't parse OpenID response: %s", err)
	}
	endpoint, err := openid.ParseEndpointFromResponse(response)
	if err != nil {
		return fmt.Errorf("Couldn't parse endpoint in response: %s", err)
	}
	c := appengine.NewContext(r)
	assoc := lookupAssociation(c, endpoint.String())
	if assoc == nil {
		client := urlfetch.Client(c)
		if !endpoint.ValidateResponse(client, response) {
			return fmt.Errorf("No association found and OP did not validate response")
		}
	}
	if err := assoc.VerifySignature(response); err != nil {
		return fmt.Errorf("Could not verify signature: %s", err)
	}
	fmt.Fprintf(w, "Got valid OpenID authentication response.")
	return nil
}
