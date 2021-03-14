package dhcpd

import (
	"fmt"
	"net"
	"os"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/iana"
	"github.com/nik-johnson-net/rackdirector/pkg/dhcpd/server"
)

// DefaultDHCPv4ServerPort is the default server port for DHCPv4 servers
const DefaultDHCPv4ServerPort = 67
const ArchHTTPClient iana.Arch = 16
const pxeLinuxMagic = "f1:00:74:7e"

type DHCPOption struct {
	Code  int
	Value []byte
}

type DHCPResponse struct {
	IP             net.IP
	Network        net.IPNet
	Gateway        net.IP
	DNS            []net.IP
	Lease          uint32
	Hostname       string
	DomainSearch   string
	TFTPServerName string
	Options        []DHCPOption
}

type DHCPv4Handler interface {
	Handle(circuitID string, subscriberID string, macAddress net.HardwareAddr, gatewayIP net.IP) (DHCPResponse, error)
}

// DHCPD is a DHCP server integrated with IPAM
type DHCPD struct {
	DHCPv4Handler DHCPv4Handler
	dhcpdv4       *server.DHCPDv4
}

// ListenAndServe does something
func (d *DHCPD) ListenAndServe() error {
	d.dhcpdv4 = &server.DHCPDv4{
		Handlers: server.DHCPv4Handlers{
			Discover: d.dhcpv4OnDiscover,
			Request:  d.dhcpv4OnDiscover,
		},
		ListenAddress: net.UDPAddr{
			Port: 67,
		},
	}
	go func() {
		d.dhcpdv4.ListenAndServe()
	}()

	return nil
}

// Close closes things
func (d *DHCPD) Close() error {
	return d.dhcpdv4.Close()
}

func localIP(peer net.Addr) net.IP {
	host, _, err := net.SplitHostPort(peer.String())
	if err != nil {
		panic(err)
	}
	return net.ParseIP(host)
}

func (d *DHCPD) dhcpv4OnDiscover(m *dhcpv4.DHCPv4, localAddr net.IP, peer net.Addr) (*dhcpv4.DHCPv4, error) {
	var circuitID string
	var subscriberID string
	var userClass string
	modifiers := make([]dhcpv4.Modifier, 0)
	clientArch := iana.INTEL_X86PC

	if agentInfo := m.RelayAgentInfo(); agentInfo != nil {
		circuitID = string(agentInfo.Get(dhcpv4.AgentCircuitIDSubOption))
		subscriberID = string(agentInfo.Get(dhcpv4.SubscriberIDSubOption))
	}

	if userclass := m.UserClass(); userclass != nil && len(userclass) > 0 {
		userClass = userclass[0]
	}

	if len(m.ClientArch()) > 0 {
		clientArch = m.ClientArch()[0]
	}

	/*if circuitID == "ge-0/0/29.0:management" {
		fmt.Fprintf(os.Stderr, "BMC DHCP Request: %v", m.Summary())
	} else {
		fmt.Fprintf(os.Stderr, "Compute DHCP Request: %v", m.Summary())
	}*/

	response, err := d.DHCPv4Handler.Handle(circuitID, subscriberID, m.ClientHWAddr, m.GatewayIPAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error responding to DHCPv4 packet - %v:\n%s\n", err, m.Summary())
		return nil, nil
	}

	// If it's EFI, HTTP syslinux. If not, chainload lpxelinux

	switch clientArch {
	case iana.INTEL_X86PC:
		if userClass == "iPXE" {
			fmt.Printf("Serving legacy iPXE\n")
			bootfile := fmt.Sprintf("http://%s/config.ipxe", localAddr.String())
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionBootfileName, []byte(bootfile)))
		} else {
			fmt.Printf("Serving legacy %s to load undionly.kpxe\n", userClass)
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionTFTPServerName, []byte(response.TFTPServerName)))
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionBootfileName, []byte("undionly.kpxe")))
		}
	default:
		switch userClass {
		case "iPXE":
			fmt.Printf("Serving UEFI (arch %s) iPXE\n", clientArch)
			bootfile := fmt.Sprintf("http://%s/config.ipxe", localAddr.String())
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionBootfileName, []byte(bootfile)))
		case "HTTPClient":
			fmt.Printf("Serving UEFI (arch %s) HTTPClient to load ipxe\n", clientArch)
			bootfile := fmt.Sprintf("http://%s/ipxe.efi", localAddr.String())
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionBootfileName, []byte(bootfile)))
		default:
			fmt.Printf("Serving UEFI (arch %s) %s to load ipxe\n", clientArch, userClass)
			bootfile := "ipxe.efi"
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionBootfileName, []byte(bootfile)))
			modifiers = append(modifiers, dhcpv4.WithGeneric(dhcpv4.OptionTFTPServerName, []byte(response.TFTPServerName)))
		}
	}

	var replyType dhcpv4.MessageType
	switch m.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		replyType = dhcpv4.MessageTypeOffer
	case dhcpv4.MessageTypeRequest:
		if m.RequestedIPAddress().Equal(response.IP) {
			replyType = dhcpv4.MessageTypeAck
		} else {
			replyType = dhcpv4.MessageTypeNak
		}
	}

	modifiers = append(modifiers,
		dhcpv4.WithGeneric(dhcpv4.GenericOptionCode(54), localAddr),
		dhcpv4.WithYourIP(response.IP),
		dhcpv4.WithServerIP(localAddr),
		dhcpv4.WithRouter(response.Gateway),
		dhcpv4.WithDNS(response.DNS...),
		dhcpv4.WithLeaseTime(response.Lease),
		dhcpv4.WithDomainSearchList(response.DomainSearch),
		dhcpv4.WithNetmask(response.Network.Mask),
		dhcpv4.WithMessageType(replyType),
	)

	fmt.Fprintf(os.Stdout, "handing address %v to %v\n", response, circuitID)
	reply, err := dhcpv4.NewReplyFromRequest(m, modifiers...)
	/*if circuitID == "ge-0/0/29.0:management" {
		fmt.Fprintf(os.Stderr, "BMC DHCP Reply: %v", reply.Summary())
	} else {
		fmt.Fprintf(os.Stderr, "Compute DHCP Reply: %v", reply.Summary())
	}*/
	return reply, err
}
