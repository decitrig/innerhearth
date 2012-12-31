package openid

import (
	"fmt"
	"net/http"

	"appengine"
)

var (
	yahooDiscoveryURL = "https://me.yahoo.com/"
)

func init() {
	handle("/login/yahoo", startYahooLogin)
}

func startYahooLogin(w http.ResponseWriter, r *http.Request) error {
	c := appengine.NewContext(r)
	xrds, err := getXRDSDocument(c, yahooDiscoveryURL)
	if err != nil {
		return fmt.Errorf("Error getting Yahoo XRDS document: %s", err)
	}
	c.Infof("xrds: %+v", xrds)
	return nil
}
