// Copyright 2015 Keybase, Inc. All rights reserved. Use of
// this source code is governed by the included BSD license.

package externals

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	libkb "github.com/keybase/client/go/libkb"
	keybase1 "github.com/keybase/client/go/protocol/keybase1"
)

// SupportedVersion is which version of ParamProofs is supported by this client.
const SupportedVersion int = 1

// staticProofServies are only used for testing or for basic assertion
// validation
type staticProofServices struct {
	collection map[string]libkb.ServiceType
}

func newStaticProofServices() libkb.ExternalServicesCollector {
	staticServices := getStaticProofServices()
	p := staticProofServices{
		collection: make(map[string]libkb.ServiceType),
	}
	p.register(staticServices)
	return &p
}

func (p *staticProofServices) register(services []libkb.ServiceType) {
	for _, st := range services {
		if !useDevelProofCheckers && st.IsDevelOnly() {
			continue
		}
		for _, k := range st.AllStringKeys() {
			p.collection[k] = st
		}
	}
}

func (p *staticProofServices) GetServiceType(s string) libkb.ServiceType {
	return p.collection[strings.ToLower(s)]
}

func (p *staticProofServices) ListProofCheckers() []string {
	var ret []string
	for k := range p.collection {
		ret = append(ret, k)
	}
	return ret
}

// Contains both the statically known services and loads the configurations for
// known services from the server
type proofServices struct {
	sync.Mutex
	libkb.Contextified
	collection map[string]libkb.ServiceType
	loaded     bool
}

func NewProofServices(g *libkb.GlobalContext) libkb.ExternalServicesCollector {
	return newProofServices(g)
}

func newProofServices(g *libkb.GlobalContext) *proofServices {
	p := &proofServices{
		Contextified: libkb.NewContextified(g),
		collection:   make(map[string]libkb.ServiceType),
	}

	staticServices := getStaticProofServices()
	p.Lock()
	defer p.Unlock()
	p.register(staticServices)
	return p
}

func (p *proofServices) register(services []libkb.ServiceType) {
	for _, st := range services {
		if !useDevelProofCheckers && st.IsDevelOnly() {
			continue
		}
		for _, k := range st.AllStringKeys() {
			p.collection[k] = st
		}
	}
}

func (p *proofServices) GetServiceType(s string) libkb.ServiceType {
	p.Lock()
	defer p.Unlock()
	p.loadParamProofServices()
	return p.collection[strings.ToLower(s)]
}

func (p *proofServices) ListProofCheckers() []string {
	p.Lock()
	defer p.Unlock()
	p.loadParamProofServices()
	var ret []string
	for k := range p.collection {
		ret = append(ret, k)
	}
	return ret
}

func (p *proofServices) loadParamProofServices() {
	shouldRun := p.G().Env.GetFeatureFlags().Admin() || p.G().Env.GetRunMode() == libkb.DevelRunMode || p.G().Env.RunningInCI()

	if !shouldRun {
		return
	}

	mctx := libkb.NewMetaContext(context.TODO(), p.G())
	entry, err := p.G().GetParamProofStore().GetLatestEntry(mctx)
	if err != nil {
		p.G().Log.CDebugf(context.TODO(), "unable to load paramproofs: %v", err)
		return
	}
	serviceConfigs, err := p.parseServiceConfigs(entry)
	if err != nil {
		p.G().Log.CDebugf(context.TODO(), "unable to parse paramproofs: %v", err)
		return
	}
	services := []libkb.ServiceType{}
	for _, config := range serviceConfigs {
		services = append(services, NewGenericSocialProofServiceType(config))
	}
	p.register(services)
}

type proofServicesT struct {
	Services []keybase1.ParamProofServiceConfig `json:"services"`
}

func (p *proofServices) parseServiceConfigs(entry keybase1.MerkleStoreEntry) (res []*GenericSocialProofConfig, err error) {
	b := []byte(entry.Entry)
	services := proofServicesT{}

	if err := json.Unmarshal(b, &services); err != nil {
		return nil, err
	}

	// Do some basic validation of what we parsed
	for _, config := range services.Services {
		validConf, err := NewGenericSocialProofConfig(config)
		if err != nil {
			p.G().Log.CDebugf(context.TODO(), "Unable to validate config for %s: %v", config.DisplayName, err)
			continue
		}
		res = append(res, validConf)
	}
	return res, nil
}
