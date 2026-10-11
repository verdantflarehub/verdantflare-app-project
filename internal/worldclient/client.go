// Package artifactclient implements the central Artifact v2 internal protocol.
package worldclient

import (
	"bytes"
	"context"

	"crypto/tls"
	"crypto/x509"

	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/verdantflarehub/verdantflare-app-project/internal/project"
	"github.com/verdantflarehub/verdantflare-app-project/internal/strictjson"
)

type Client struct {
	base, token string
	http        *http.Client
}

func New(base, token string) (*Client, error) {
	return NewWithCA(base, token, "")
}

// NewWithCA optionally extends the system trust store for an internal HTTPS
// Artifact endpoint. The endpoint and bearer token remain independently
// validated; the CA file is never sent to the service.
func NewWithCA(base, token, caFile string) (*Client, error) {
	u, e := url.Parse(base)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || len(token) < 32 || strings.ContainsAny(token, " \t\r\n") {
		return nil, project.ErrInvalid
	}
	ip := net.ParseIP(u.Hostname())
	if u.Scheme != "https" && !(u.Scheme == "http" && ip != nil && ip.IsLoopback()) {
		return nil, project.ErrInvalid
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, project.ErrInvalid
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, project.ErrInvalid
		}
		if transport.TLSClientConfig == nil {
			transport.TLSClientConfig = &tls.Config{}
		}
		transport.TLSClientConfig.RootCAs = pool
	}
	return &Client{base: strings.TrimRight(base, "/"), token: token, http: &http.Client{Transport: transport, Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) request(ctx context.Context, p project.Principal, method, path string, body []byte) (*http.Response, error) {
	if !p.Valid() {
		return nil, project.ErrForbidden
	}
	r, e := http.NewRequestWithContext(ctx, method, c.base+path, bytes.NewReader(body))
	if e != nil {
		return nil, project.ErrDependency
	}
	r.Header.Set("Authorization", "Bearer "+c.token)
	r.Header.Set("X-User-Id", p.SubjectID)
	r.Header.Set("X-Organization-Id", p.OrganizationID)
	r.Header.Set("X-Request-Id", p.RequestID)
	r.Header.Set("Content-Type", "application/json")
	response, e := c.http.Do(r)
	if e != nil {
		return nil, project.ErrDependency
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		return response, nil
	}
	defer response.Body.Close()
	var failure struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	}
	if strictjson.Decode(response.Body, 4096, &failure) != nil {
		return nil, project.ErrDependency
	}
	for code, err := range map[string]error{"INVALID_ARGUMENT": project.ErrInvalid, "PERMISSION_DENIED": project.ErrForbidden, "NOT_FOUND": project.ErrNotFound, "IDEMPOTENCY_CONFLICT": project.ErrIdempotency, "CONTENT_NOT_READY": project.ErrNotReady} {
		if failure.Code == code {
			return nil, err
		}
	}
	return nil, project.ErrDependency
}
func (c *Client) json(ctx context.Context, p project.Principal, method, path string, input, output any) error {
	var data []byte
	var e error
	if input != nil {
		data, e = json.Marshal(input)
		if e != nil {
			return project.ErrInvalid
		}
	}
	r, e := c.request(ctx, p, method, path, data)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 200 {
		return project.ErrDependency
	}
	if strictjson.Decode(r.Body, 4<<20, output) != nil {
		return project.ErrDependency
	}
	return nil
}
func (c *Client) Resolve(ctx context.Context, p project.Principal, ref project.AssetRef) (project.AssetResolution, error) {
	var result project.AssetResolution
	if !project.ValidID(ref.AssetID) || !project.ValidID(ref.VersionID) {
		return result, project.ErrInvalid
	}
	err := c.json(ctx, p, "POST", "/internal/v1/world/references", ref, &result)
	if err != nil {
		return result, err
	}
	if len(result.Refs) == 0 || len(result.Refs) > project.MaxFiles || len(result.Grants) == 0 || len(result.Grants) > 10000 {
		return result, project.ErrDependency
	}
	found := false
	for _, grant := range result.Grants {
		if !project.ValidID(grant.ResourceID) || grant.Write {
			return result, project.ErrDependency
		}
		if grant.ResourceID == ref.AssetID {
			found = true
		}
	}
	for _, item := range result.Refs {
		if !item.Valid() {
			return result, project.ErrDependency
		}
	}
	if !found {
		return result, project.ErrDependency
	}
	return result, nil
}
func (c *Client) References(ctx context.Context, p project.Principal, ref project.AssetRef) ([]project.ContentRef, error) {
	value, err := c.Resolve(ctx, p, ref)
	return value.Refs, err
}
func (c *Client) Receipt(ctx context.Context, p project.Principal, asset, commit string) (project.WorldReceipt, error) {
	var value project.WorldReceipt
	if !project.ValidID(asset) || !project.ValidID(commit) {
		return value, project.ErrInvalid
	}
	err := c.json(ctx, p, "POST", "/internal/v1/world/commit_receipt", map[string]string{"asset_id": asset, "commit_id": commit}, &value)
	if err == nil && (value.State != "committed" || value.Result == nil || value.Result.AssetID != asset || value.Version != value.Result.VersionID) {
		return value, project.ErrDependency
	}
	return value, err
}
