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

	"github.com/go-openapi/runtime"
	"github.com/go-openapi/strfmt"
	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"github.com/openziti/zrok/v2/environment"
	"github.com/openziti/zrok/v2/environment/env_core"
	"github.com/openziti/zrok/v2/rest_client_zrok/share"

	httptransport "github.com/go-openapi/runtime/client"
	zrokrest "github.com/openziti/zrok/v2/rest_client_zrok"
	restenv "github.com/openziti/zrok/v2/rest_client_zrok/environment"
	restmeta "github.com/openziti/zrok/v2/rest_client_zrok/metadata"
)

const DefaultAPIEndpoint = "https://api-v2.zrok.io"

// RESTClient talks to the zrok controller API.
type RESTClient interface {
	Enable(ctx context.Context, apiEndpoint, accountToken, host, description string) (envZID string, zitiCfg string, err error)
	Disable(ctx context.Context, apiEndpoint, accountToken, envZID string) error
	CreateShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error
	UpdateShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string, reserved bool) error
	DeleteShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string) error
	Unshare(ctx context.Context, apiEndpoint, accountToken, envZID, shareToken string) error
	ListShares(ctx context.Context, apiEndpoint, accountToken, envZID string) ([]RemoteShare, error)
}

// AgentClient talks to a zrok2 agent over native gRPC (agentGrpc).
// addr is host:port of the TCP→unix proxy (see agent.AgentDialAddr).
type AgentClient interface {
	Status(ctx context.Context, addr string) (*agentGrpc.StatusResponse, error)
	SharePublic(ctx context.Context, addr string, req *agentGrpc.SharePublicRequest) (*agentGrpc.SharePublicResponse, error)
	SharePrivate(ctx context.Context, addr string, req *agentGrpc.SharePrivateRequest) (*agentGrpc.SharePrivateResponse, error)
	ReleaseShare(ctx context.Context, addr, token string) error
	AccessPrivate(ctx context.Context, addr string, req *agentGrpc.AccessPrivateRequest) (*agentGrpc.AccessPrivateResponse, error)
	ReleaseAccess(ctx context.Context, addr, token string) error
}

// Clients bundles REST + agent clients.
type Clients struct {
	REST  RESTClient
	Agent AgentClient
}

// NewDefaultClients returns production clients.
// allowedAPIHosts are extra https hosts for spec.apiEndpoint; api-v2.zrok.io is always included.
func NewDefaultClients(httpClient *http.Client, allowedAPIHosts []string) *Clients {
	if httpClient == nil {
		httpClient = NewSecureHTTPClient()
	}
	return &Clients{
		REST:  &HTTPRESTClient{HTTP: httpClient, AllowedHosts: NormalizeAPIHosts(allowedAPIHosts)},
		Agent: &GRPCAgentClient{},
	}
}

// HTTPRESTClient implements RESTClient against the zrok controller.
type HTTPRESTClient struct {
	HTTP         *http.Client
	AllowedHosts []string
}

func (c *HTTPRESTClient) clientFor(apiEndpoint string) (*zrokrest.Zrok, error) {
	if err := ValidateAPIEndpoint(apiEndpoint, c.AllowedHosts); err != nil {
		return nil, err
	}
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
	transport := httptransport.NewWithClient(host, basePath, []string{"https"}, c.HTTP)
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
		// Treat already-exists as success for idempotency; caller should UpdateShareName to promote reserved.
		if strings.Contains(err.Error(), "409") || strings.Contains(strings.ToLower(err.Error()), "already") {
			return nil
		}
		return fmt.Errorf("create share name: %w", err)
	}
	return nil
}

func (c *HTTPRESTClient) UpdateShareName(ctx context.Context, apiEndpoint, accountToken, namespaceToken, name string, reserved bool) error {
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := share.NewUpdateShareNameParamsWithContext(ctx)
	params.Body = share.UpdateShareNameBody{
		NamespaceToken: namespaceToken,
		Name:           name,
		Reserved:       reserved,
	}
	_, err = client.Share.UpdateShareName(params, auth)
	if err != nil {
		return fmt.Errorf("update share name: %w", err)
	}
	return nil
}

// IsUnauthorized reports whether err is a zrok API 401 (invalid token or name owned by another account).
func IsUnauthorized(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "[401]") || strings.Contains(s, "updateShareNameUnauthorized")
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

func (c *HTTPRESTClient) Unshare(ctx context.Context, apiEndpoint, accountToken, envZID, shareToken string) error {
	if shareToken == "" {
		return nil
	}
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := share.NewUnshareParamsWithContext(ctx)
	params.Body = share.UnshareBody{
		EnvZID:     envZID,
		ShareToken: shareToken,
	}
	_, err = client.Share.Unshare(params, auth)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not found") {
			return nil
		}
		return fmt.Errorf("unshare: %w", err)
	}
	return nil
}

func (c *HTTPRESTClient) ListShares(ctx context.Context, apiEndpoint, accountToken, envZID string) ([]RemoteShare, error) {
	if envZID == "" {
		return nil, nil
	}
	client, err := c.clientFor(apiEndpoint)
	if err != nil {
		return nil, err
	}
	auth := httptransport.APIKeyAuth("X-TOKEN", "header", accountToken)
	params := restmeta.NewListSharesParamsWithContext(ctx)
	params.EnvZID = &envZID
	resp, err := client.Metadata.ListShares(params, auth)
	if err != nil {
		return nil, fmt.Errorf("list shares: %w", err)
	}
	if resp.Payload == nil {
		return nil, nil
	}
	out := make([]RemoteShare, 0, len(resp.Payload.Shares))
	for _, s := range resp.Payload.Shares {
		if s == nil {
			continue
		}
		out = append(out, RemoteShare{
			Token:             s.ShareToken,
			Target:            s.Target,
			ShareMode:         s.ShareMode,
			BackendMode:       s.BackendMode,
			FrontendEndpoints: s.FrontendEndpoints,
		})
	}
	return out, nil
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
