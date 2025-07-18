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

package actioner

/*
KernelRouteAddDel Actioner Params:
-------------------------------------------------
name                value
-------------------------------------------------
ifname              network interface name
with-route          also add a host route

-------------------------------------------------
*/

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/types"
	"github.com/iqiyi/dpvs/tools/healthcheck/pkg/utils"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"
)

var _ ActionMethod = (*KernelRouteAction)(nil)

const kernelRouteActionerName = "KernelRouteAddDel"

func init() {
	registerMethod(kernelRouteActionerName, &KernelRouteAction{})
}

type KernelRouteAction struct {
	target    *utils.L3L4Addr
	ifname    string
	withRoute bool
}

func findLinkByAddr(addr net.IP) (netlink.Link, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, fmt.Errorf("failed to list links: %w", err)
	}

	for _, link := range links {
		addrs, err := netlink.AddrList(link, netlink.FAMILY_ALL)
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if a.IP.Equal(addr) {
				return link, nil
			}
		}
	}

	return nil, fmt.Errorf("address %v not found on any interface", addr)
}

func isExistError(err error) bool {
	//return err == unix.EEXIST || err.Error() == "file exists"
	return errors.Is(err, unix.EEXIST)
}

var ErrCannotAssignRequestedAddress = errors.New("cannot assign requested address")

func isNotExistError(err error) bool {
	//return err == unix.ENOENT || err == unix.ESRCH || err.Error() == "cannot assign requested address"
	return errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ESRCH) || strings.Contains(strings.ToLower(err.Error()),
		"cannot assign requested address")
}

func (a *KernelRouteAction) Act(signal types.State, timeout time.Duration,
	data ...interface{}) (interface{}, error) {
	addr := a.target.IP

	if timeout <= 0 {
		return nil, fmt.Errorf("zero timeout on %s actioner %v", kernelRouteActionerName, addr)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	glog.V(7).Infof("starting %s actioner %v ...", kernelRouteActionerName, addr)

	done := make(chan error, 1)

	go func() {
		var link netlink.Link
		var err error

		/*
			// Notes:
			//	 Find ifname by IP is not feasible to deletion operation.

			if len(a.ifname) == 0 {
				if link, err = findLinkByAddr(addr); err != nil {
					done <- fmt.Errorf("failed to find link for address: %w", err)
					return
				}
			}
		*/
		link, err = netlink.LinkByName(a.ifname)
		if err != nil {
			done <- fmt.Errorf("failed to get link by name: %w", err)
			return
		}

		var ipNet *net.IPNet
		if addr.To4() != nil {
			ipNet = &net.IPNet{IP: addr, Mask: net.CIDRMask(32, 32)}
		} else {
			ipNet = &net.IPNet{IP: addr, Mask: net.CIDRMask(128, 128)}
		}

		ipAddr := &netlink.Addr{IPNet: ipNet}

		if signal != types.Unhealthy { // ADD
			if err := netlink.AddrAdd(link, ipAddr); err != nil {
				if isExistError(err) {
					glog.V(8).Infof("Warning: adding address %v already exists: %v\n", addr, err)
				} else {
					done <- fmt.Errorf("failed to add address %v to %s: %w", addr, a.ifname, err)
					return
				}
			}

			if a.withRoute {
				route := netlink.Route{
					LinkIndex: link.Attrs().Index,
					Dst:       ipAddr.IPNet,
				}
				if err := netlink.RouteAdd(&route); err != nil {
					if !isExistError(err) {
						done <- fmt.Errorf("failed to add host route %v to %s: %w", addr, a.ifname, err)
						return
					}
				}
			}
		} else { // DELETE
			if err := netlink.AddrDel(link, ipAddr); err != nil {
				if isNotExistError(err) {
					glog.V(8).Infof("Warning: deleting address %v does not exist: %v\n", addr, err)
				} else {
					done <- fmt.Errorf("failed to delete address %v from %s: %w", addr, a.ifname, err)
					return
				}
			}

			if a.withRoute {
				route := netlink.Route{
					LinkIndex: link.Attrs().Index,
					Dst:       ipAddr.IPNet,
				}
				if err := netlink.RouteDel(&route); err != nil {
					if !isNotExistError(err) {
						done <- fmt.Errorf("failed to delete route %v from %s: %w", addr, a.ifname, err)
						return
					}
				}
			}
		}
		done <- nil
	}()

	operation := "UP"
	if signal == types.Unhealthy {
		operation = "DOWN"
	}

	select {
	case <-ctx.Done():
		glog.Errorf("%s actioner %v %s timeout", kernelRouteActionerName, addr, operation)
		return nil, ctx.Err()
	case err := <-done:
		if err != nil {
			glog.Errorf("%s actioner %v %s failed: %v", kernelRouteActionerName, addr, operation, err)
			return nil, err
		}
	}
	glog.V(6).Infof("%s actioner %v %s succeed", kernelRouteActionerName, addr, operation)
	return nil, nil
}

func (a *KernelRouteAction) validate(params map[string]string) error {
	required := []string{"ifname"}
	var missed []string
	for _, param := range required {
		if _, ok := params[param]; !ok {
			missed = append(missed, param)
		}
	}
	if len(missed) > 0 {
		return fmt.Errorf("missing required action params: %v", strings.Join(missed, ","))
	}

	unsupported := make([]string, 0, len(params))
	for param, val := range params {
		switch param {
		case "ifname":
			if len(val) == 0 {
				return fmt.Errorf("empty action param %s", param)
			}
			// TODO: check if the interface exists on the system
		case "with-route":
			if _, err := utils.String2bool(val); err != nil {
				return fmt.Errorf("invalid action param %s=%s", param, val)
			}
		default:
			unsupported = append(unsupported, param)
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("unsupported action params: %s", strings.Join(unsupported, ","))
	}

	return nil
}

func (a *KernelRouteAction) create(target *utils.L3L4Addr, params map[string]string,
	extras ...interface{}) (ActionMethod, error) {
	if target == nil || len(target.IP) == 0 {
		return nil, fmt.Errorf("no target address for %s actioner", kernelRouteActionerName)
	}

	if err := a.validate(params); err != nil {
		return nil, fmt.Errorf("%s actioner param validation failed: %v", kernelRouteActionerName, err)
	}

	withRoute, _ := utils.String2bool(params["with-route"])
	return &KernelRouteAction{
		target:    target.DeepCopy(),
		ifname:    params["ifname"],
		withRoute: withRoute,
	}, nil
}
