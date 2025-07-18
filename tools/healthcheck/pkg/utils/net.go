// Copyright 2023 IQiYi Inc. All Rights Reserved.
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

package utils

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"syscall"
)

// AF represents a network address family.
type AF int

const (
	IPv4 AF = syscall.AF_INET
	IPv6 AF = syscall.AF_INET6
)

// String returns the string representation of an AF.
func (af AF) String() string {
	switch af {
	case IPv4:
		return "IPv4"
	case IPv6:
		return "IPv6"
	}
	return "(unknown)"
}

// AFs returns the supported address families.
func AFs() []AF {
	return []AF{IPv4, IPv6}
}

// IPAF returns the address family of an IP address.
func IPAF(ip net.IP) AF {
	if ip.To4() != nil {
		return IPv4
	}
	return IPv6
}

// IPAddrClone returns the deep-copied IP address.
func IPAddrClone(ip net.IP) net.IP {
	addr := make(net.IP, len(ip))
	copy(addr[:], ip[:])
	return addr
}

// IPProto specifies an IP protocol.
type IPProto uint16

const (
	IPProtoICMP   IPProto = syscall.IPPROTO_ICMP
	IPProtoICMPv6 IPProto = syscall.IPPROTO_ICMPV6
	IPProtoTCP    IPProto = syscall.IPPROTO_TCP
	IPProtoUDP    IPProto = syscall.IPPROTO_UDP
)

// String returns the name for the given protocol value.
func (proto IPProto) String() string {
	switch proto {
	case IPProtoICMP:
		return "ICMP"
	case IPProtoICMPv6:
		return "ICMPv6"
	case IPProtoTCP:
		return "TCP"
	case IPProtoUDP:
		return "UDP"
	}
	return fmt.Sprintf("IPProto(%d)", proto)
}

// ParseIPProto return an IPProto from its string representation.
func ParseIPProto(str string) IPProto {
	switch str {
	case "TCP":
		return IPProtoTCP
	case "UDP":
		return IPProtoUDP
	case "ICMP":
		return IPProtoICMP
	case "ICMPv6":
		return IPProtoICMPv6
	}
	return 0
}

// L3L4Addr represents a combination of IP, IPProto and Port.
type L3L4Addr struct {
	IP    net.IP
	Port  uint16
	Proto IPProto
}

// String returns the string representation of the given L3L4Addr value.
func (addr *L3L4Addr) String() string {
	return fmt.Sprintf("%s-%s-%d", addr.IP, addr.Proto, addr.Port)
}

func (in *L3L4Addr) DeepCopyInto(out *L3L4Addr) {
	*out = *in
	out.IP = make(net.IP, len(in.IP))
	copy(out.IP[:], in.IP[:])
	return
}

func (in *L3L4Addr) DeepCopy() *L3L4Addr {
	if in == nil {
		return nil
	}
	out := new(L3L4Addr)
	in.DeepCopyInto(out)
	return out
}

// Network returns the network name for net.Dailer
func (addr *L3L4Addr) Network() string {
	var network string
	version := 4
	if addr.IP.To4() == nil {
		version = 6
	}
	switch addr.Proto {
	case IPProtoTCP:
		network = fmt.Sprintf("tcp%d", version)
	case IPProtoUDP:
		network = fmt.Sprintf("udp%d", version)
	case IPProtoICMP:
		network = "ip4:icmp"
	case IPProtoICMPv6:
		network = "ip6:ipv6-icmp"
	default:
		return "(unknown)"
	}
	return network
}

// Addr returns the IP:Port representation for net.Dailer
func (addr *L3L4Addr) Addr() string {
	if addr.IP.To4() != nil {
		return fmt.Sprintf("%v:%d", addr.IP, addr.Port)
	}
	return fmt.Sprintf("[%v]:%d", addr.IP, addr.Port)
}

// ParseL3L4Addr produces a L3L4Addr from its string representation.
func ParseL3L4Addr(str string) *L3L4Addr {
	segs := strings.Split(str, "-")
	addr := L3L4Addr{}
	if len(segs) > 0 {
		if ip := net.ParseIP(segs[0]); ip != nil {
			addr.IP = ip
		} else {
			return nil
		}
	}
	segs = segs[1:]
	if len(segs) > 0 {
		if proto := ParseIPProto(segs[0]); proto != 0 {
			addr.Proto = proto
		} else {
			return nil
		}
	}
	segs = segs[1:]
	if len(segs) > 0 {
		if port, err := strconv.ParseUint(segs[0], 10, 16); err != nil {
			return nil
		} else {
			addr.Port = uint16(port)
		}
	}
	return &addr
}

// WriteFull tries to write the whole data in a slice to a net conn.
func WriteFull(conn net.Conn, b []byte) error {
	for len(b) > 0 {
		n, err := conn.Write(b)
		if err != nil {
			return err
		}
		b = b[n:]
	}
	return nil
}
