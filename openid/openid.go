package openid

import (
	"bufio"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"appengine"
)

var (
	loginPage                 = template.Must(template.ParseFiles("openid/login.html"))
	openIDNamespace           = "http://specs.openid.net/auth/2.0"
	openIDEndpointServiceType = "http://specs.openid.net/auth/2.0/server"
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

type XRDSIdentifier struct {
	XMLName xml.Name "Service"
	Type    []string
	URI     string
	LocalID string
}

type XRD struct {
	XMLName xml.Name "XRD"
	Service []XRDSIdentifier
}

type XRDS struct {
	XMLName xml.Name "XRDS"
	XRD     XRD
}

func (x *XRDS) GetOpenIDEndpoint() (*url.URL, error) {
	for _, service := range x.XRD.Service {
		for _, serviceType := range service.Type {
			if serviceType == openIDEndpointServiceType {
				return url.Parse(service.URI)
			}
		}
	}
	return nil, fmt.Errorf("No OpenID endpoint found in XRDS %s", x)
}

func (x *XRDS) String() string {
	marshaled, err := xml.MarshalIndent(x, "", "  ")
	if err != nil {
		return fmt.Sprintf("%+v", x)
	}
	return string(marshaled)
}

type Endpoint struct {
	endpoint *url.URL
}

func (e *Endpoint) String() string {
	return fmt.Sprintf("Endpoint %s", e.endpoint)
}

func NewEndpoint(urlString string) (*Endpoint, error) {
	endpointURL, err := url.Parse(urlString)
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse %s as URL: %s", urlString, err)
	}
	return &Endpoint{endpointURL}, nil
}

func DiscoverOpenIDEndpoint(client *http.Client, discovery *url.URL) (*Endpoint, error) {
	if client == nil {
		client = &http.Client{}
	}
	xrds, err := getXRDSDocument(client, discovery.String())
	if err != nil {
		return nil, fmt.Errorf("Error getting XRDS document from %s: %s", discovery, err)
	}
	endpoint, err := xrds.GetOpenIDEndpoint()
	if err != nil {
		return nil, fmt.Errorf("Error looking up endpoint URI: %s", err)
	}
	return &Endpoint{endpoint: endpoint}, nil
}

func getXRDSDocument(client *http.Client, url string) (*XRDS, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("Error creating XDRS request: %s", err)
	}
	req.Header.Set("Accept", "application/xrds+xml")
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
		return getXRDSDocument(client, xrdsLocation)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("Error reading response body")
	}
	xrds := &XRDS{}
	if err := xml.Unmarshal(body, xrds); err != nil {
		return nil, fmt.Errorf("Error unmarshaling XRDS document: %s", err)
	}
	return xrds, nil
}

func (e *Endpoint) SendDirectRequest(client *http.Client, message Message) (Message, error) {
	resp, err := client.PostForm(e.endpoint.String(), url.Values(message))
	if err != nil {
		return nil, fmt.Errorf("Error making direct request: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("Direct request returned status code %d", resp.StatusCode)
	}
	return parseDirectResponse(resp.Body)
}

func (e *Endpoint) SendIndirectRequest(w http.ResponseWriter, r *http.Request, request Message) {
	requestURL := &url.URL{}
	*requestURL = *(e.endpoint)
	requestURL.RawQuery = url.Values(request).Encode()
	http.Redirect(w, r, requestURL.String(), http.StatusSeeOther)
}

type Message map[string][]string

func newMessage() Message {
	return make(map[string][]string)
}

func NewRequest(mode string) Message {
	return Message{
		"openid.ns":   {openIDNamespace},
		"openid.mode": {mode},
	}
}

func (m Message) Set(key, value string) {
	m[key] = []string{value}
}

func (m Message) Get(key string) string {
	value := m[key]
	if value == nil || len(value) == 0 {
		return ""
	}
	return value[0]
}

func parseDirectResponse(body io.Reader) (Message, error) {
	reader := bufio.NewReader(body)
	message := newMessage()
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
		message.Set(key, value)
	}
	if ns := message.Get("ns"); ns != openIDNamespace {
		return nil, fmt.Errorf("Missing/incorrect ns value %s in response %s", ns, message)
	}
	return message, nil
}

func ParseIndirectResponse(r *http.Request) (Message, error) {
	values := r.URL.Query()
	message := newMessage()
	for k, v := range values {
		message[k] = v
	}
	if ns := message.Get("openid.ns"); ns != openIDNamespace {
		return nil, fmt.Errorf("Missing/incorrect ns value %s in response %s", ns, values)
	}
	return message, nil
}

type Association struct {
	Endpoint   string
	Handle     string
	Type       string
	Expiration time.Time
}

type AssociationRequestOptions struct {
	AssociationType string
	SessionType     string
}

func (o *AssociationRequestOptions) GetAssociationType() string {
	if o == nil || o.AssociationType == "" {
		return "HMAC-SHA256"
	}
	return o.AssociationType
}

func (o *AssociationRequestOptions) GetSessionType() string {
	if o == nil || o.SessionType == "" {
		return "no-encryption"
	}
	return o.SessionType
}

func (e *Endpoint) RequestAssociation(client *http.Client, opts *AssociationRequestOptions) (*Association, error) {
	req := NewRequest("associate")
	req.Set("openid.assoc_type", opts.GetAssociationType())
	req.Set("openid.session_type", opts.GetSessionType())

	reply, err := e.SendDirectRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("Error making association request %s: %s", req, err)
	}
	return parseAssociation(e.endpoint, reply)
}

func parseAssociation(endpoint *url.URL, message Message) (*Association, error) {
	if err := message.Get("error"); err != "" {
		return nil, fmt.Errorf("OP refused association request: %s", err)
	}
	lifetimeSeconds, err := strconv.ParseInt(message.Get("expires_in"), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("Could not parse 'expires_in' from message: %s", err)
	}
	association := &Association{
		Endpoint:   endpoint.String(),
		Handle:     message.Get("assoc_handle"),
		Type:       message.Get("assoc_type"),
		Expiration: time.Now().Add(time.Duration(lifetimeSeconds) * time.Second),
	}
	return association, nil
}

func (a *Association) HasExpired() bool {
	return !time.Now().Before(a.Expiration)
}

func (e *Endpoint) ValidateWithOP(client *http.Client, response Message) bool {
	request := newMessage()
	for k, v := range response {
		request[k] = v
	}
	request.Set("openid.mode", "check_authentication")
	verification, err := e.SendDirectRequest(client, request)
	if err != nil {
		return false
	}
	return verification.Get("is_valid") == "true"
}
