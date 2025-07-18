// /*
// Copyright 2025 IQiYi Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
// */

package checker

/*
UDPPing Checker Params:
-----------------------------------
name                value
-----------------------------------
send                non-empty string
receive             non-empty string
prxoy-protocol      v2
------------------------------------
*/

import (
	"fmt"
	"time"

	"github.com/golang/glog"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/types"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/utils"
)

var _ CheckMethod = (*UDPPingChecker)(nil)

// UDPPingChecker is a composite check method, who firstly performs Ping check,
// and then executes UDP check only after Ping check succeeds.
// It can alleviate the defect of ambiguous heatlh state in UDP checker.
type UDPPingChecker struct {
	*PingChecker
	*UDPChecker
}

func init() {
	registerMethod(CheckMethodUDPPing, &UDPPingChecker{})
}

func (c *UDPPingChecker) Check(target *utils.L3L4Addr, timeout time.Duration) (types.State, error) {
	if timeout <= time.Duration(0) {
		return types.Unknown, fmt.Errorf("zero timeout on UDPPing check")
	}

	start := time.Now()
	addr := target.Addr()
	glog.V(9).Infof("Start UDPPing check to %v ...", addr)

	state, err := c.PingChecker.Check(target, timeout)
	if err != nil {
		return types.Unknown, err
	}
	if state == types.Unhealthy {
		glog.V(9).Infof("UDPPing check %v %v: ping check failed", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	state, err = c.UDPChecker.Check(target, time.Until(start.Add(timeout)))
	glog.V(9).Infof("UDPPing check %v %v", addr, state)
	return state, err
}

func (c *UDPPingChecker) validate(params map[string]string) error {
	// PingChecker requires no params.

	return c.UDPChecker.validate(params)
}

func (c *UDPPingChecker) create(params map[string]string) (CheckMethod, error) {
	if err := c.validate(params); err != nil {
		return nil, fmt.Errorf("udpping param checker validation failed: %v", err)
	}

	pingChecker, err := c.PingChecker.create(nil)
	if err != nil {
		return nil, fmt.Errorf("fail to create udpping checker: %v", err)
	}
	udpChecker, err := c.UDPChecker.create(params)
	if err != nil {
		return nil, fmt.Errorf("fail to create udping checker: %v", err)
	}

	return &UDPPingChecker{
		PingChecker: pingChecker.(*PingChecker),
		UDPChecker:  udpChecker.(*UDPChecker),
	}, nil
}
