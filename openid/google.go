package openid

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"

	"appengine"
)

func init() {
	handle("/login/google", startGoogleLogin)
	handle("/login/google-response", googleResponse)
}

var (
	googleXRDS = "http://www.google.com/accounts/o8/id"
)

func startGoogleLogin(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	xrds, err := getXRDSDocument(c, googleXRDS)
	if err != nil {
		return fmt.Errorf("Error getting Google XRDS document: %s", err)
	}

	endpoint := xrds.XRD.Service.URI

	assoc, err := associateWithOP(c, endpoint)
	if err != nil {
		return fmt.Errorf("Error associating with google: %s", err)
	}

	requestURL, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("Error parsing URI %s", xrds.XRD.Service.URI)
	}
	params := requestURL.Query()
	params.Set("openid.mode", "checkid_setup")
	params.Set("openid.ns", "http://specs.openid.net/auth/2.0")
	params.Set("openid.return_to", "http://innerhearthyoga.appspot.com/login/google-response")
	params.Set("openid.realm", "http://innerhearthyoga.appspot.com/")
	params.Set("openid.claimed_id", "http://specs.openid.net/auth/2.0/identifier_select")
	params.Set("openid.identity", "http://specs.openid.net/auth/2.0/identifier_select")
	params.Set("openid.assoc_handle", assoc.Handle)
	requestURL.RawQuery = params.Encode()

	http.Redirect(w, r, requestURL.String(), http.StatusSeeOther)
	return nil
}

func googleResponse(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	mode := r.FormValue("openid.mode")
	if mode != "id_res" {
		return fmt.Errorf("OpenID authentication failed, response from OP: %s", r.URL)
	}

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
	return nil
}
