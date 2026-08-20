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
	_, callTO := c.timeouts()

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

func (c *GRPCAgentClient) Status(ctx context.Context, addr string) (*agentGrpc.StatusResponse, error) {
	var out *agentGrpc.StatusResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.Status(ctx, &agentGrpc.StatusRequest{})
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) SharePublic(ctx context.Context, addr string, req *agentGrpc.SharePublicRequest) (*agentGrpc.SharePublicResponse, error) {
	var out *agentGrpc.SharePublicResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.SharePublic(ctx, req)
		if err != nil {
			return err
		}
		out = resp
		return nil
	})
	return out, err
}

func (c *GRPCAgentClient) SharePrivate(ctx context.Context, addr string, req *agentGrpc.SharePrivateRequest) (*agentGrpc.SharePrivateResponse, error) {
	var out *agentGrpc.SharePrivateResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.SharePrivate(ctx, req)
		if err != nil {
			return err
		}
		out = resp
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

func (c *GRPCAgentClient) AccessPrivate(ctx context.Context, addr string, req *agentGrpc.AccessPrivateRequest) (*agentGrpc.AccessPrivateResponse, error) {
	var out *agentGrpc.AccessPrivateResponse
	err := c.withClient(ctx, addr, func(ctx context.Context, cli agentGrpc.AgentClient) error {
		resp, err := cli.AccessPrivate(ctx, req)
		if err != nil {
			return err
		}
		out = resp
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
