/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

    10|Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package zrokclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/zrok/v2/environment"
	"github.com/openziti/zrok/v2/environment/env_core"
	"github.com/openziti/zrok/v2/rest_client_zrok/share"

	httptransport "github.com/go-openapi/runtime/client"
	zrokrest "github.com/openziti/zrok/v2/rest_client_zrok"
	restenv "github.com/openziti/zrok/v2/rest_client_zrok/environment"
)

const DefaultAPIEndpoint = "https://api-v2.zrok.io"

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

// AgentClient talks to a zrok2 agent over native gRPC (agentGrpc).
// addr is host:port of the TCP→unix proxy (see agent.AgentDialAddr).
type AgentClient interface {
	Status(ctx context.Context, addr string) (*AgentStatus, error)
	SharePublic(ctx context.Context, addr string, req SharePublicRequest) (*SharePublicResponse, error)
	SharePrivate(ctx context.Context, addr string, req SharePrivateRequest) (*SharePrivateResponse, error)
	ReleaseShare(ctx context.Context, addr, token string) error
	AccessPrivate(ctx context.Context, addr string, req AccessPrivateRequest) (*AccessPrivateResponse, error)
	ReleaseAccess(ctx context.Context, addr, token string) error
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
		Agent: &GRPCAgentClient{},
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
	// zrok API consumes/produces application/zrok.v1+json (not application/json).
	transport.Producers["application/zrok.v1+json"] = runtime.JSONProducer()
	transport.Consumers["application/zrok.v1+json"] = runtime.JSONConsumer()
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
