package openid

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"model"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"appengine"
	"appengine/datastore"
	"appengine/urlfetch"
)

var (
	loginPage       = template.Must(template.ParseFiles("openid/login.html"))
	openIDNamespace = "http://specs.openid.net/auth/2.0"
)

type openIDHandler func(w http.ResponseWriter, r *http.Request) error

func (fn openIDHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := fn(w, r); err != nil {
		c := appengine.NewContext(r)
		c.Errorf("openID error: %s", err)
		http.Error(w, "An error occurred", http.StatusInternalServerError)
	}
}

func handle(path string, handler openIDHandler) {
	http.Handle(path, handler)
}

func init() {
	handle("/_ah/login_required", openIDLogin)
	handle("/login/account", openIDAccountCheck)
}

func openIDLogin(w http.ResponseWriter, r *http.Request) error {
	if err := loginPage.Execute(w, nil); err != nil {
		return fmt.Errorf("Error rendering login page: %s", err)
	}
	return nil
}

func openIDAccountCheck(w http.ResponseWriter, r *http.Request) error {
	id := r.FormValue("id")
	if len(id) == 0 {
		return fmt.Errorf("Empty id value in URL: %s", r.URL)
	}
	c := appengine.NewContext(r)
	account, err := model.GetOrCreateAccount(c, id)
	if err != nil {
		return err
	}
	if account.Fresh {
		http.Redirect(w, r, "/login/account/new", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/registration", http.StatusSeeOther)
	}
	return nil
}

type xrdsIdentifier struct {
	XMLName xml.Name "Service"
	Type    []string
	URI     string
	LocalID string
}

type xrd struct {
	XMLName xml.Name "XRD"
	Service xrdsIdentifier
}

type xrds struct {
	XMLName xml.Name "XRDS"
	XRD     xrd
}

func getXRDSDocument(c appengine.Context, url string) (*xrds, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating XDRS request: %s", err)
	}
	req.Header.Set("Accept", "application/xrds+xml")
	client := urlfetch.Client(c)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Error fetching XDRS document: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("XDRS request returned status %d", resp.StatusCode)
	}
	xrdsLocation := resp.Header.Get("X-XRDS-Location")
	if len(xrdsLocation) > 0 {
		c.Infof("Following location header: %s", xrdsLocation)
		return getXRDSDocument(c, xrdsLocation)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body")
	}
	xrds := &xrds{}
	if err := xml.Unmarshal(body, xrds); err != nil {
		return nil, fmt.Errorf("Error unmarshaling XRDS identifier: %s", err)
	}
	return xrds, nil
}

type DirectResponse map[string]string

func parseDirectResponse(body io.Reader) (DirectResponse, error) {
	reader := bufio.NewReader(body)
	values := make(map[string]string, 0)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("Error reading response body: %s", err)
		}
		idx := strings.Index(line, ":")
		if idx < 0 {
			return nil, fmt.Errorf("Malformed line in response body: %s", line)
		}
		key := strings.TrimSpace(line[0:idx])
		value := strings.TrimSpace(line[idx+1:])
		values[key] = value
	}
	if values["ns"] != openIDNamespace {
		return nil, fmt.Errorf("Missing/incorrect ns value %s in response %s", values["ns"], values)
	}
	return values, nil
}

func verifyWithOP(c appengine.Context, opURL string, values url.Values) (bool, error) {
	values["openid.mode"] = []string{"check_authentication"}
	client := urlfetch.Client(c)
	resp, err := client.PostForm(opURL, values)
	if err != nil {
		return false, fmt.Errorf("Error making verification request: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return false, fmt.Errorf("Verification request returned status %d", resp.StatusCode)
	}
	parsed, err := parseDirectResponse(resp.Body)
	if err != nil {
		return false, fmt.Errorf("Error parsing direct response: %s", err)
	}
	if parsed["is_valid"] != "true" {
		return false, fmt.Errorf("OP declared response invalid")
	}
	return true, nil
}

type OpenIDAssociation struct {
	Endpoint   string    `datastore: "-"`
	Handle     string    `datastore: ",noindex"`
	Type       string    `datastore: ",noindex"`
	Expiration time.Time `datastore: ",noindex"`
}

func (a *OpenIDAssociation) HasExpired() bool {
	return !time.Now().Before(a.Expiration)
}

func requestAssociation(c appengine.Context, endpoint string) (*OpenIDAssociation, error) {
	form := map[string][]string{
		"openid.ns":           {"http://specs.openid.net/auth/2.0"},
		"openid.mode":         {"associate"},
		"openid.assoc_type":   {"HMAC-SHA256"},
		"openid.session_type": {"no-encryption"},
	}
	client := urlfetch.Client(c)
	resp, err := client.PostForm(endpoint, form)
	if err != nil {
		return nil, fmt.Errorf("Error making request: %s", err)
	}
	defer resp.Body.Close()
	parsed, err := parseDirectResponse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error parsing direct request: %s", err)
	}
	if len(parsed["error"]) != 0 {
		return nil, fmt.Errorf("OP returned error: %s", parsed["error"])
	}
	lifetimeSeconds, err := strconv.ParseInt(parsed["expires_in"], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Could not parse %s into seconds", parsed["expires_in"])
	}
	association := &OpenIDAssociation{
		Endpoint:   endpoint,
		Handle:     parsed["assoc_handle"],
		Type:       parsed["assoc_type"],
		Expiration: time.Now().Add(time.Duration(lifetimeSeconds) * time.Second),
	}
	return association, nil
}

func associateWithOP(c appengine.Context, endpoint string) (*OpenIDAssociation, error) {
	key := datastore.NewKey(c, "OpenIDAssociation", endpoint, 0, nil)
	assoc := &OpenIDAssociation{Endpoint: endpoint}
	err := datastore.Get(c, key, assoc)
	if err != nil || !time.Now().Before(assoc.Expiration) {
		assoc, err := requestAssociation(c, endpoint)
		if err != nil {
			return nil, fmt.Errorf("Error associating with %s: %s", endpoint, err)
		}
		if _, err := datastore.Put(c, key, assoc); err != nil {
			c.Errorf("Error storing association with %s: %s", endpoint, err)
		}
	}
	return assoc, nil
}

func lookupAssociation(c appengine.Context, endpoint string) *OpenIDAssociation {
	key := datastore.NewKey(c, "OpenIDAssociation", endpoint, 0, nil)
	assoc := &OpenIDAssociation{Endpoint: endpoint}
	if err := datastore.Get(c, key, assoc); err != nil {
		if err != datastore.ErrNoSuchEntity {
			c.Errorf("Error looking up association for %s: %s", endpoint, err)
		}
		return nil
	}
	return assoc
}
