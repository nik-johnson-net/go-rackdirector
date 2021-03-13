package ipam

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/nik-johnson-net/rackdirector/pkg/dhcpd"
)

type HostAddressInfo struct {
	Hostname     string
	Address      net.IP
	Network      net.IPNet
	Gateway      net.IP
	DNS          []net.IP
	DomainSearch string
}

type jsonIpamInterface struct {
	Device      string
	Port        string
	Ipv4        string
	Ipv4Gateway string `json:"ipv4_gateway"`
}

type jsonIpamBmc struct {
	Hostname    string
	Port        string
	Ipv4        string
	Ipv4Gateway string `json:"ipv4_gateway"`
}

type jsonIpamHost struct {
	Hostname   string
	Interfaces []jsonIpamInterface
	Bmc        jsonIpamBmc
}

type jsonIpamConfig struct {
	Hosts []jsonIpamHost
}

type BMC struct {
	Hostname    string
	Port        string
	Ipv4        net.IP
	Network     net.IPNet
	Ipv4Gateway net.IP
}

type Interface struct {
	Device      string
	Port        string
	Ipv4        net.IP
	Network     net.IPNet
	Ipv4Gateway net.IP
}

type Host struct {
	Hostname   string
	Interfaces []Interface
	Bmc        BMC
}

type ipamConfig struct {
	Hosts []Host
}

func (i ipamConfig) GetHost(port string, relayIP net.IP) (Host, bool) {
	for _, entry := range i.Hosts {
		for _, interf := range entry.Interfaces {
			if interf.Network.Contains(relayIP) && interf.Port == port {
				return entry, true
			}
		}
		if entry.Bmc.Network.Contains(relayIP) && entry.Bmc.Port == port {
			return entry, true
		}
	}
	return Host{}, false
}

func (i ipamConfig) GetHostByIP(ip net.IP) (Host, bool) {
	for _, entry := range i.Hosts {
		for _, interf := range entry.Interfaces {
			if interf.Ipv4.Equal(ip) {
				return entry, true
			}
		}
		if entry.Bmc.Ipv4.Equal(ip) {
			return entry, true
		}
	}
	return Host{}, false
}

func (i ipamConfig) GetHostByHostname(hostname string) (Host, bool) {
	for _, entry := range i.Hosts {
		if entry.Hostname == hostname {
			return entry, true
		}
		if entry.Bmc.Hostname == hostname {
			return entry, true
		}
	}
	return Host{}, false
}

type StaticIpam struct {
	config ipamConfig
}

func NewFromFile(file string) *StaticIpam {
	configFile, err := os.Open(file)
	if err != nil {
		panic(err)
	}
	defer configFile.Close()
	decoder := json.NewDecoder(configFile)

	var jsonConfig jsonIpamConfig
	err = decoder.Decode(&jsonConfig)
	if err != nil {
		panic(err)
	}

	config := ipamConfig{
		Hosts: make([]Host, 0),
	}
	for _, host := range jsonConfig.Hosts {
		hostObj := Host{
			Hostname:   host.Hostname,
			Interfaces: make([]Interface, 0),
		}
		for _, interf := range host.Interfaces {
			ip, network, err := net.ParseCIDR(interf.Ipv4)
			if err != nil {
				panic(err)
			}
			hostObj.Interfaces = append(hostObj.Interfaces, Interface{
				Device:      interf.Device,
				Port:        interf.Port,
				Ipv4:        ip,
				Network:     *network,
				Ipv4Gateway: net.ParseIP(interf.Ipv4Gateway),
			})
		}

		ip, network, err := net.ParseCIDR(host.Bmc.Ipv4)
		if err != nil {
			panic(err)
		}
		hostObj.Bmc = BMC{
			Hostname:    host.Bmc.Hostname,
			Port:        host.Bmc.Port,
			Ipv4:        ip,
			Network:     *network,
			Ipv4Gateway: net.ParseIP(host.Bmc.Ipv4Gateway),
		}

		config.Hosts = append(config.Hosts, hostObj)
	}

	fmt.Fprintf(os.Stdout, "Build database %v\n", config)

	return &StaticIpam{
		config: config,
	}
}

func computeGateway(ip net.IPNet) net.IP {
	network := ip.IP.Mask(ip.Mask)
	network[3]++
	return network
}

func getDomain(hostname string) string {
	split := strings.SplitN(hostname, ".", 2)
	return split[1]
}

func (s *StaticIpam) Handle(circuitID string, subscriberID string, macAddress net.HardwareAddr, gatewayIP net.IP) (dhcpd.DHCPResponse, error) {
	if h, ok := s.config.GetHost(circuitID, gatewayIP); ok {
		for _, interf := range h.Interfaces {
			if interf.Port == circuitID {
				return dhcpd.DHCPResponse{
					IP:             interf.Ipv4,
					Network:        interf.Network,
					Gateway:        interf.Ipv4Gateway,
					DNS:            []net.IP{net.ParseIP("1.1.1.1")},
					Lease:          86400,
					Hostname:       h.Hostname,
					DomainSearch:   getDomain(h.Hostname),
					TFTPServerName: "10.0.1.10",
				}, nil
			}
		}
		if h.Bmc.Port == circuitID {
			return dhcpd.DHCPResponse{
				IP:           h.Bmc.Ipv4,
				Network:      h.Bmc.Network,
				Gateway:      h.Bmc.Ipv4Gateway,
				DNS:          []net.IP{net.ParseIP("1.1.1.1")},
				Lease:        86400,
				Hostname:     h.Hostname,
				DomainSearch: getDomain(h.Hostname),
			}, nil
		}
	}
	return dhcpd.DHCPResponse{}, fmt.Errorf("not found")
}

func (s *StaticIpam) Get(peer net.IP) (Host, error) {
	if h, ok := s.config.GetHostByIP(peer); ok {
		return h, nil
	}
	return Host{}, fmt.Errorf("not found")
}

func (s *StaticIpam) GetByHostname(hostname string) (Host, error) {
	if h, ok := s.config.GetHostByHostname(hostname); ok {
		return h, nil
	}
	return Host{}, fmt.Errorf("not found")
}
