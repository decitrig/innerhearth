package openid

import (
	"bufio"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	openIDNamespace           = "http://specs.openid.net/auth/2.0"
	openIDEndpointServiceType = "http://specs.openid.net/auth/2.0/server"
	identifierSelect          = "http://specs.openid.net/auth/2.0/identifier_select"
)

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
	return e.endpoint.String()
}

func ParseEndpointFromResponse(message Message) (*Endpoint, error) {
	endpointURL, err := url.Parse(message.Get("openid.op_endpoint"))
	if err != nil {
		return nil, fmt.Errorf("Couldn't parse endpoint from message")
	}
	return &Endpoint{endpointURL}, nil
}

func DiscoverEndpoint(client *http.Client, discovery *url.URL) (*Endpoint, error) {
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

type AuthorizationRequest struct {
	Realm       string
	ReturnTo    string
	Identity    string
	Association *Association
}

func NewAuthorizationRequest(identity, returnTo string) *AuthorizationRequest {
	return &AuthorizationRequest{
		Identity: identity,
		ReturnTo: returnTo,
	}
}

func (r *AuthorizationRequest) GetIdentity() string {
	if r.Identity == "" {
		return identifierSelect
	}
	return r.Identity
}

func (r *AuthorizationRequest) toMessage() Message {
	message := NewRequest("checkid_setup")
	message.Set("openid.claimed_id", r.GetIdentity())
	message.Set("openid.identity", r.GetIdentity())
	message.Set("openid.return_to", r.ReturnTo)
	if realm := r.Realm; realm != "" {
		message.Set("openid.realm", realm)
	}
	if assoc := r.Association; assoc != nil {
		message.Set("openid.assoc_handle", assoc.Handle)
	}
	return message
}

func (e *Endpoint) SendAuthorizationRequest(
	w http.ResponseWriter, r *http.Request, request *AuthorizationRequest) {
	message := request.toMessage()
	e.SendIndirectRequest(w, r, message)
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
	MACKey     string
	Type       string
	Expiration time.Time
}

func (a *Association) GetHMACAlgorithm() (hash.Hash, error) {
	key, err := base64.StdEncoding.DecodeString(a.MACKey)
	if err != nil {
		return nil, fmt.Errorf("Error decoding MAC key: %s", err)
	}
	switch a.Type {
	case "HMAC-SHA1":
		return hmac.New(sha1.New, key), nil
	case "HMAC-SHA256":
		return hmac.New(sha256.New, key), nil
	}
	return nil, fmt.Errorf("Unknown algorithm: %s", a.Type)
}

func (a *Association) ComputeSignature(message Message) (string, error) {
	hmac, err := a.GetHMACAlgorithm()
	if err != nil {
		return "", fmt.Errorf("Error with HMAC algorithm: %s", err)
	}
	signed := strings.Split(message.Get("openid.signed"), ",")
	for _, field := range signed {
		val := message.Get("openid." + field)
		hmac.Write([]byte(fmt.Sprintf("%s:%s\n", field, val)))
	}
	return base64.StdEncoding.EncodeToString(hmac.Sum(nil)), nil
}

func (a *Association) VerifySignature(message Message) bool {
	expected, err := a.ComputeSignature(message)
	if err != nil {
		return false
	}
	received := message.Get("openid.sig")
	return received == expected
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
		MACKey:     message.Get("mac_key"),
		Type:       message.Get("assoc_type"),
		Expiration: time.Now().Add(time.Duration(lifetimeSeconds) * time.Second),
	}
	return association, nil
}

func (a *Association) HasExpired() bool {
	return !time.Now().Before(a.Expiration)
}

func (e *Endpoint) ValidateResponse(client *http.Client, response Message) bool {
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
