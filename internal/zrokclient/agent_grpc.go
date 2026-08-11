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
	"time"

	"github.com/openziti/zrok/v2/agent/agentGrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// GRPCAgentClient talks to the zrok2 agent via native gRPC (agentGrpc),
// dialing a TCP proxy to the agent's unix socket.
type GRPCAgentClient struct {
	DialTimeout time.Duration
	CallTimeout time.Duration
}

func (c *GRPCAgentClient) timeouts() (dial, call time.Duration) {
	dial = c.DialTimeout
	if dial == 0 {
		dial = 10 * time.Second
	}
	call = c.CallTimeout
	if call == 0 {
		call = 60 * time.Second
	}
	return dial, call
}

func (c *GRPCAgentClient) withClient(ctx context.Context, addr string, fn func(context.Context, agentGrpc.AgentClient) error) error {
	dialTO, callTO := c.timeouts()
	_ = dialTO

	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("dial agent gRPC %s: %w", addr, err)
	}
	defer conn.Close()

	callCtx, cancelCall := context.WithTimeout(ctx, callTO)
	defer cancelCall()
	return fn(callCtx, agentGrpc.NewAgentClient(conn))
}

func (c *GRPCAgentClient) Status(ctx context.Context, addr string) (*AgentStatus, error) {
	var out *AgentStatus
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.Status(ctx, &agentGrpc.StatusRequest{})
		if err != nil {
			return err
		}
		out = &AgentStatus{Shares: make([]AgentShareStatus, 0, len(resp.GetShares()))}
		for _, s := range resp.GetShares() {
			out.Shares = append(out.Shares, AgentShareStatus{
				Token:            s.GetToken(),
				FrontendEndpoint: s.GetFrontendEndpoint(),
			})
		}
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) SharePublic(ctx context.Context, addr string, req SharePublicRequest) (*SharePublicResponse, error) {
	var out *SharePublicResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		pb := &agentGrpc.SharePublicRequest{
			Target:               req.Target,
			BasicAuth:            req.BasicAuth,
			BackendMode:          req.BackendMode,
			Insecure:             req.Insecure,
			OauthProvider:        req.OauthProvider,
			OauthEmailDomains:    req.OauthEmailDomains,
			OauthRefreshInterval: req.OauthRefreshInterval,
			Closed:               req.Closed,
			AccessGrants:         req.AccessGrants,
		}
		for _, ns := range req.NameSelections {
			pb.NameSelections = append(pb.NameSelections, &agentGrpc.NameSelection{
				NamespaceToken: ns.NamespaceToken,
				Name:           ns.Name,
			})
		}
		resp, err := cli.SharePublic(ctx, pb)
		if err != nil {
			return err
		}
		out = &SharePublicResponse{
			Token:             resp.GetToken(),
			FrontendEndpoints: resp.GetFrontendEndpoints(),
		}
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) SharePrivate(ctx context.Context, addr string, req SharePrivateRequest) (*SharePrivateResponse, error) {
	var out *SharePrivateResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.SharePrivate(ctx, &agentGrpc.SharePrivateRequest{
			Target:            req.Target,
			BackendMode:       req.BackendMode,
			PrivateShareToken: req.PrivateShareToken,
			Closed:            req.Closed,
			AccessGrants:      req.AccessGrants,
		})
		if err != nil {
			return err
		}
		out = &SharePrivateResponse{Token: resp.GetToken()}
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) ReleaseShare(ctx context.Context, addr, token string) error {
	return c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		_, err := cli.ReleaseShare(ctx, &agentGrpc.ReleaseShareRequest{Token: token})
		return err
	})
}

func (c *GRPCAgentClient) AccessPrivate(ctx context.Context, addr string, req AccessPrivateRequest) (*AccessPrivateResponse, error) {
	var out *AccessPrivateResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.AccessPrivate(ctx, &agentGrpc.AccessPrivateRequest{
			Token:       req.Token,
			BindAddress: req.BindAddress,
		})
		if err != nil {
			return err
		}
		out = &AccessPrivateResponse{FrontendToken: resp.GetFrontendToken()}
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) ReleaseAccess(ctx context.Context, addr, token string) error {
	return c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		_, err := cli.ReleaseAccess(ctx, &agentGrpc.ReleaseAccessRequest{FrontendToken: token})
		return err
	})
}
