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
UDP Checker Params:
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
	"net"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/types"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/utils"
)

var _ CheckMethod = (*UDPChecker)(nil)

type UDPChecker struct {
	send       string
	receive    string
	proxyProto string // "v2"
}

func init() {
	registerMethod(CheckMethodUDP, &UDPChecker{})
}

func (c *UDPChecker) Check(target *utils.L3L4Addr, timeout time.Duration) (types.State, error) {
	if timeout <= time.Duration(0) {
		return types.Unknown, fmt.Errorf("zero timeout on UDP check")
	}

	network := target.Network()
	addr := target.Addr()
	glog.V(9).Infof("Start UDP check to %s ...", addr)

	start := time.Now()
	deadline := start.Add(timeout)

	dial := net.Dialer{
		Timeout: timeout,
	}
	conn, err := dial.Dial(network, addr)
	if err != nil {
		glog.V(9).Infof("UDP check %v %v: failed to dial", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}
	defer conn.Close()

	udpConn, ok := conn.(*net.UDPConn)
	if !ok {
		glog.V(9).Infof("UDP check %v %v: failed to create udp socket", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	err = udpConn.SetDeadline(deadline)
	if err != nil {
		glog.V(9).Infof("UDP check %v %v: failed to set deadline", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	if "v2" == c.proxyProto {
		if err = utils.WriteFull(udpConn, proxyProtoV2LocalCmd); err != nil {
			glog.V(9).Infof("UDP check %v %v: failed to send proxy protocol v2 data",
				addr, types.Unhealthy)
			return types.Unhealthy, nil
		}
	}

	if len(c.send) > 0 {
		err = utils.WriteFull(udpConn, []byte(c.send))
	} else {
		_, err = udpConn.Write([]byte{})
	}
	if err != nil {
		glog.V(9).Infof("UDP check %v %v: failed to write", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	buf := make([]byte, len(c.receive))
	n, _, err := udpConn.ReadFrom(buf)
	if err != nil {
		if len(c.send) == 0 && len(c.receive) == 0 {
			if neterr, ok := err.(net.Error); ok {
				if neterr.Timeout() {
					// Intuitively, we should assign types.Unknown to the check result.
					// But it can lead to inconsistent problem when health states changed.
					// Thus return types.Healthy instead.
					glog.V(9).Infof("UDP check %v %v: i/o timeout, state %v returned", addr,
						types.Unknown, types.Healthy)
					return types.Healthy, nil
				}
			}
		}
		glog.V(9).Infof("UDP check %v %v: failed to read", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	got := string(buf[:n])
	if got != c.receive {
		glog.V(9).Infof("UDP check %v %v: unexpected response", addr, types.Unhealthy)
		return types.Unhealthy, nil
	}

	glog.V(9).Infof("UDP check %v %v: succeed", addr, types.Healthy)
	return types.Healthy, nil
}

func (c *UDPChecker) validate(params map[string]string) error {
	unsupported := make([]string, 0, len(params))
	for param, val := range params {
		switch param {
		case "send":
			if len(val) == 0 {
				return fmt.Errorf("empty udp checker param: %s", param)
			}
		case "receive":
			if len(val) == 0 {
				return fmt.Errorf("empty udp checker param: %s", param)
			}
		case ParamProxyProto:
			val = strings.ToLower(val)
			if val != "v2" {
				return fmt.Errorf("invalid udp checker param value: %s:%s", param, params[param])
			}
		default:
			unsupported = append(unsupported, param)
		}
	}

	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported udp checker params: %q", strings.Join(unsupported, ","))
	}
	return nil
}

func (c *UDPChecker) create(params map[string]string) (CheckMethod, error) {
	if err := c.validate(params); err != nil {
		return nil, fmt.Errorf("udp checker param validation failed: %v", err)
	}

	checker := &UDPChecker{}

	if val, ok := params["send"]; ok {
		c.send = val
	}
	if val, ok := params["receive"]; ok {
		c.receive = val
	}
	if val, ok := params[ParamProxyProto]; ok {
		c.proxyProto = val
	}

	return checker, nil
}
