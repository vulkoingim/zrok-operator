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
	"context"
	"sync"
)

// FakeREST is an in-memory RESTClient for tests.
type FakeREST struct {
	Mu            sync.Mutex
	EnableCalls   int
	DisableCalls  int
	CreateNames   []string
	DeleteNames   []string
	EnableErr     error
	DisableErr    error
	CreateNameErr error
	DeleteNameErr error
	EnvZID        string
	ZitiCfg       string
}

func (f *FakeREST) Enable(_ context.Context, _, _, _, _ string) (string, string, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.EnableCalls++
	if f.EnableErr != nil {
		return "", "", f.EnableErr
	}
	zid := f.EnvZID
	if zid == "" {
		zid = "env-zid"
	}
	cfg := f.ZitiCfg
	if cfg == "" {
		cfg = "{}"
	}
	return zid, cfg, nil
}

func (f *FakeREST) Disable(_ context.Context, _, _, _ string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.DisableCalls++
	return f.DisableErr
}

func (f *FakeREST) CreateShareName(_ context.Context, _, _, _, name string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.CreateNames = append(f.CreateNames, name)
	return f.CreateNameErr
}

func (f *FakeREST) DeleteShareName(_ context.Context, _, _, _, name string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.DeleteNames = append(f.DeleteNames, name)
	return f.DeleteNameErr
}

// FakeAgent is an in-memory AgentClient for tests.
type FakeAgent struct {
	Mu             sync.Mutex
	StatusResp     *AgentStatus
	StatusErr      error
	PublicResp     *SharePublicResponse
	PublicErr      error
	PrivateResp    *SharePrivateResponse
	PrivateErr     error
	AccessResp     *AccessPrivateResponse
	AccessErr      error
	ReleasedShares []string
	ReleasedAccess []string
	ReleaseErr     error
	ShareCalls     int
}

func (f *FakeAgent) Status(_ context.Context, _ string) (*AgentStatus, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.StatusErr != nil {
		return nil, f.StatusErr
	}
	if f.StatusResp != nil {
		return f.StatusResp, nil
	}
	return &AgentStatus{}, nil
}

func (f *FakeAgent) SharePublic(_ context.Context, _ string, _ SharePublicRequest) (*SharePublicResponse, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.ShareCalls++
	if f.PublicErr != nil {
		return nil, f.PublicErr
	}
	if f.PublicResp != nil {
		return f.PublicResp, nil
	}
	return &SharePublicResponse{
		Token:             "shr-token",
		FrontendEndpoints: []string{"https://demo.share.zrok.io"},
	}, nil
}

func (f *FakeAgent) SharePrivate(_ context.Context, _ string, _ SharePrivateRequest) (*SharePrivateResponse, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.ShareCalls++
	if f.PrivateErr != nil {
		return nil, f.PrivateErr
	}
	if f.PrivateResp != nil {
		return f.PrivateResp, nil
	}
	return &SharePrivateResponse{Token: "priv-token"}, nil
}

func (f *FakeAgent) ReleaseShare(_ context.Context, _, token string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.ReleasedShares = append(f.ReleasedShares, token)
	return f.ReleaseErr
}

func (f *FakeAgent) AccessPrivate(_ context.Context, _ string, _ AccessPrivateRequest) (*AccessPrivateResponse, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.AccessErr != nil {
		return nil, f.AccessErr
	}
	if f.AccessResp != nil {
		return f.AccessResp, nil
	}
	return &AccessPrivateResponse{FrontendToken: "access-token"}, nil
}

func (f *FakeAgent) ReleaseAccess(_ context.Context, _, token string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.ReleasedAccess = append(f.ReleasedAccess, token)
	return f.ReleaseErr
}
