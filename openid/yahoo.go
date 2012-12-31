package openid

import (
	_ "fmt"
	"net/http"

	_ "appengine"
)

var (
	yahooDiscoveryURL = "https://me.yahoo.com/"
)

func init() {
	handle("/login/yahoo", startYahooLogin)
}

func startYahooLogin(w http.ResponseWriter, r *http.Request) error {
	return nil
}
