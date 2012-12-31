package openid

import (
	_ "encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"appengine"
	"appengine/urlfetch"
)

func init() {
	handle("/login/google", startGoogleLogin)
	handle("/login/google-response", googleResponse)
}

func urlMustParse(urlString string) *url.URL {
	u, err := url.Parse(urlString)
	if err != nil {
		panic(err)
	}
	return u
}

var (
	googleDiscovery = urlMustParse("http://www.google.com/accounts/o8/id")
)

func startGoogleLogin(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	endpoint, err := DiscoverOpenIDEndpoint(client, googleDiscovery)
	if err != nil {
		return fmt.Errorf("Error with OpenID discovery on %s: %s", googleDiscovery, err)
	}
	assoc, err := endpoint.RequestAssociation(client, nil)
	if err != nil {
		return fmt.Errorf("Error requesting association with %s: %s", endpoint, err)
	}

	request := NewRequest("checkid_setup")
	request.Set("openid.return_to", "http://innerhearthyoga.appspot.com/login/google-response")
	request.Set("openid.realm", "http://innerhearthyoga.appspot.com/")
	request.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")
	request.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	request.Set("openid.assoc_handle", assoc.Handle)
	endpoint.SendIndirectRequest(w, r, request)
	return nil
}

func googleResponse(w http.ResponseWriter, r *http.Request) error {
	response, err := ParseIndirectResponse(r)
	if err != nil {
		return fmt.Errorf("Error parsing indirect response: %s", err)
	}
	if mode := response.Get("openid.mode"); mode != "id_res" {
		return fmt.Errorf("Unsuccessful authentication: %s", mode)
	}
	endpoint, err := NewEndpoint(response.Get("openid.op_endpoint"))
	if err != nil {
		return fmt.Errorf("Couldn't parse op_endpoint in response: %s", response)
	}
	c := appengine.NewContext(r)
	client := urlfetch.Client(c)
	if !endpoint.ValidateWithOP(client, response) {
		return fmt.Errorf("OP did not validate authentication")
	}
	fmt.Fprintf(w, "Got valid openID authentication response")

	/*
		endpoint := r.FormValue("openid.op_endpoint")
		assoc := lookupAssociation(c, endpoint)
		if assoc == nil {
			return fmt.Errorf("Could not find Google session association")
		}
		if r.FormValue("openid.assoc_handle") != assoc.Handle {
			return fmt.Errorf("Association handle mismatch")
		}
		if assoc.HasExpired() {
			return fmt.Errorf("Attempting to use expired association")
		}

		id := r.FormValue("openid.claimed_id")
		if len(id) == 0 {
			return fmt.Errorf("Could not find claimed_id in OP response: %s", r.URL)
		}
		c.Infof("Got claimed google id %s", id)
		encodedID := base64.URLEncoding.EncodeToString([]byte(id))
		http.Redirect(w, r, "/login/account?id="+encodedID, http.StatusSeeOther)
	*/
	return nil
}
