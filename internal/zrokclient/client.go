/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package zrokclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-openapi/strfmt"
	"github.com/openziti/zrok/v2/environment"
	"github.com/openziti/zrok/v2/environment/env_core"
	"github.com/openziti/zrok/v2/rest_client_zrok/share"

	httptransport "github.com/go-openapi/runtime/client"
	zrokrest "github.com/openziti/zrok/v2/rest_client_zrok"
	restenv "github.com/openziti/zrok/v2/rest_client_zrok/environment"
)

const DefaultAPIEndpoint = "https://api-v2.zrok.io"

const jsonKeyToken = "token"

// NameSelection is a reserved name binding.
type NameSelection struct {
	NamespaceToken string
	Name           string
}

// SharePublicRequest is the agent sharePublic payload.
type SharePublicRequest struct {
	Target               string
	BackendMode          string
	NameSelections       []NameSelection
	BasicAuth            []string
	Insecure             bool
	Closed               bool
	AccessGrants         []string
	OauthProvider        string
	OauthEmailDomains    []string
	OauthRefreshInterval string
}

// SharePublicResponse is returned by agent sharePublic.
type SharePublicResponse struct {
	Token             string
	FrontendEndpoints []string
}

// SharePrivateRequest is the agent sharePrivate payload.
type SharePrivateRequest struct {
	Target            string
	BackendMode       string
	PrivateShareToken string
	Closed            bool
	AccessGrants      []string
}

// SharePrivateResponse is returned by agent sharePrivate.
type SharePrivateResponse struct {
	Token string
}

// AccessPrivateRequest is the agent accessPrivate payload.
type AccessPrivateRequest struct {
	Token       string
	BindAddress string
}

// AccessPrivateResponse is returned by agent accessPrivate.
type AccessPrivateResponse struct {
	FrontendToken string `json:"frontendToken"`
}

// AgentStatus is a subset of agent status.
type AgentStatus struct {
	Shares []AgentShareStatus `json:"shares"`
}

// AgentShareStatus describes one running share.
type AgentShareStatus struct {
	Token            string   `json:"token"`
	FrontendEndpoint []string `json:"frontendEndpoint"`
}

// RESTClient talks to the zrok controller API.
type RESTClient interface {
	Enable(ctx context.Context, apiEndpoint, accountToken, host, description string) (envZID string, zitiCfg string, err error)
	Disable(ctx context.Context, apiEndpoint, accountToken, envZID string) error
	CreateShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error
	DeleteShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error
}

// AgentClient talks to a zrok2 agent HTTP console (gRPC-gateway).
type AgentClient interface {
	Status(ctx context.Context, baseURL string) (*AgentStatus, error)
	SharePublic(ctx context.Context, baseURL string, req SharePublicRequest) (*SharePublicResponse, error)
	SharePrivate(ctx context.Context, baseURL string, req SharePrivateRequest) (*SharePrivateResponse, error)
	ReleaseShare(ctx context.Context, baseURL, token string) error
	AccessPrivate(ctx context.Context, baseURL string, req AccessPrivateRequest) (*AccessPrivateResponse, error)
	ReleaseAccess(ctx context.Context, baseURL, token string) error
}

// Clients bundles REST + agent clients.
type Clients struct {
	REST  RESTClient
	Agent AgentClient
}

// NewDefaultClients returns production clients.
func NewDefaultClients(httpClient *http.Client) *Clients {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Clients{
		REST:  &HTTPRESTClient{HTTP: httpClient},
		Agent: &HTTPAgentClient{HTTP: httpClient},
	}
}

// HTTPRESTClient implements RESTClient against the zrok controller.
type HTTPRESTClient struct {
	HTTP *http.Client
}

func (c *HTTPRESTClient) clientFor(apiEndpoint string) (*zrokrest.Zrok, error) {
	endpoint := strings.TrimRight(apiEndpoint, "/")
	if endpoint == "" {
		endpoint = DefaultAPIEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse api endpoint: %w", err)
	}
	host := u.Host
	basePath := strings.TrimSuffix(u.Path, "/") + "/api/v2"
	if u.Path == "" || u.Path == "/" {
		basePath = "/api/v2"
	}
	schemes := []string{u.Scheme}
	if schemes[0] == "" {
		schemes = []string{"https"}
	}
	transport := httptransport.NewWithClient(host, basePath, schemes, c.HTTP)
	return zrokrest.New(transport, strfmt.Default), nil
}

func (c *HTTPRESTClient) Enable(ctx context.Context, apiEndpoint, accountToken, host, description string) (string, string, error) {
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return "", "", err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := restenv.NewEnableParamsWithContext(ctx)
	params.Body.Host = host
	params.Body.Description = description
	resp, err := client.Environment.Enable(params, auth)
	if err != nil {
		return "", "", fmt.Errorf("enable environment: %w", err)
	}
	return resp.Payload.Identity, resp.Payload.Cfg, nil
}

func (c *HTTPRESTClient) Disable(ctx context.Context, apiEndpoint, accountToken, envZID string) error {
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := restenv.NewDisableParamsWithContext(ctx)
	params.Body.Identity = envZID
	_, err = client.Environment.Disable(params, auth)
	if err != nil {
		return fmt.Errorf("disable environment: %w", err)
	}
	return nil
}

func (c *HTTPRESTClient) CreateShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error {
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := share.NewCreateShareNameParamsWithContext(ctx)
	params.Body = share.CreateShareNameBody{
		NamespaceToken: namespaceToken,
		Name:           name,
	}
	_, err = client.Share.CreateShareName(params, auth)
	if err != nil {
		// Treat already-exists as success for idempotency.
		if strings.Contains(err.Error(), "409") || strings.Contains(strings.ToLower(err.Error()), "already") {
			return nil
		}
		return fmt.Errorf("create share name: %w", err)
	}
	return nil
}

func (c *HTTPRESTClient) DeleteShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error {
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := share.NewDeleteShareNameParamsWithContext(ctx)
	params.Body = share.DeleteShareNameBody{
		NamespaceToken: namespaceToken,
		Name:           name,
	}
	_, err = client.Share.DeleteShareName(params, auth)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("delete share name: %w", err)
	}
	return nil
}

// HTTPAgentClient implements AgentClient over the agent HTTP console.
type HTTPAgentClient struct {
	HTTP *http.Client
}

func (c *HTTPAgentClient) Status(ctx context.Context, baseURL string) (*AgentStatus, error) {
	var out AgentStatus
	if err := c.doJSON(ctx, http.MethodGet, join(baseURL, "/v1/agent/status"), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPAgentClient) SharePublic(ctx context.Context, baseURL string, req SharePublicRequest) (*SharePublicResponse, error) {
	body := map[string]any{
		"target":      req.Target,
		"backendMode": req.BackendMode,
		"insecure":    req.Insecure,
		"closed":      req.Closed,
	}
	if len(req.BasicAuth) > 0 {
		body["basicAuth"] = req.BasicAuth
	}
	if len(req.AccessGrants) > 0 {
		body["accessGrants"] = req.AccessGrants
	}
	if req.OauthProvider != "" {
		body["oauthProvider"] = req.OauthProvider
		body["oauthEmailDomains"] = req.OauthEmailDomains
		body["oauthRefreshInterval"] = req.OauthRefreshInterval
	}
	if len(req.NameSelections) > 0 {
		sels := make([]map[string]string, 0, len(req.NameSelections))
		for _, ns := range req.NameSelections {
			sels = append(sels, map[string]string{
				"namespaceToken": ns.NamespaceToken,
				"name":           ns.Name,
			})
		}
		body["nameSelections"] = sels
	}
	var out SharePublicResponse
	if err := c.doJSON(ctx, http.MethodPost, join(baseURL, "/v1/agent/sharePublic"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPAgentClient) SharePrivate(ctx context.Context, baseURL string, req SharePrivateRequest) (*SharePrivateResponse, error) {
	body := map[string]any{
		"target":            req.Target,
		"backendMode":       req.BackendMode,
		"privateShareToken": req.PrivateShareToken,
		"closed":            req.Closed,
		"accessGrants":      req.AccessGrants,
	}
	var out SharePrivateResponse
	if err := c.doJSON(ctx, http.MethodPost, join(baseURL, "/v1/agent/sharePrivate"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPAgentClient) ReleaseShare(ctx context.Context, baseURL, token string) error {
	return c.doJSON(ctx, http.MethodPost, join(baseURL, "/v1/agent/releaseShare"), map[string]string{jsonKeyToken: token}, nil)
}

func (c *HTTPAgentClient) AccessPrivate(ctx context.Context, baseURL string, req AccessPrivateRequest) (*AccessPrivateResponse, error) {
	body := map[string]any{
		jsonKeyToken:  req.Token,
		"bindAddress": req.BindAddress,
	}
	var out AccessPrivateResponse
	if err := c.doJSON(ctx, http.MethodPost, join(baseURL, "/v1/agent/accessPrivate"), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *HTTPAgentClient) ReleaseAccess(ctx context.Context, baseURL, token string) error {
	return c.doJSON(ctx, http.MethodPost, join(baseURL, "/v1/agent/releaseAccess"), map[string]string{jsonKeyToken: token}, nil)
}

func (c *HTTPAgentClient) doJSON(ctx context.Context, method, urlStr string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, urlStr, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("agent %s %s: status %d: %s", method, urlStr, resp.StatusCode, string(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode agent response: %w", err)
	}
	return nil
}

func join(base, path string) string {
	return strings.TrimRight(base, "/") + path
}

// PersistEnabledEnvironment writes enable results into a local root dir (for tests / optional manager-side enable).
func PersistEnabledEnvironment(rootDir, apiEndpoint, accountToken, envZID, zitiCfg string) error {
	environment.SetRootDirName(rootDir)
	root, err := environment.LoadRoot()
	if err != nil {
		return err
	}
	endpoint := apiEndpoint
	if endpoint == "" {
		endpoint = DefaultAPIEndpoint
	}
	if err := root.SetEnvironment(&env_core.Environment{
		AccountToken: accountToken,
		ZitiIdentity: envZID,
		ApiEndpoint:  endpoint,
	}); err != nil {
		return err
	}
	return root.SaveZitiIdentityNamed(root.EnvironmentIdentityName(), zitiCfg)
}
