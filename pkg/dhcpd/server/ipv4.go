package server

import (
	"fmt"
	"net"
	"os"

	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

type DHCPv4Handler func(*dhcpv4.DHCPv4, net.IP, net.Addr) (*dhcpv4.DHCPv4, error)

type DHCPv4Handlers struct {
	Discover DHCPv4Handler
	Offer    DHCPv4Handler
	Request  DHCPv4Handler
	Ack      DHCPv4Handler
	Inform   DHCPv4Handler
	Release  DHCPv4Handler
}

type DHCPDv4 struct {
	Handlers      DHCPv4Handlers
	ListenAddress net.UDPAddr
	server        *server4.Server
}

func (d *DHCPDv4) Close() error {
	if d.server != nil {
		return d.server.Close()
	}
	return nil
}

func (d *DHCPDv4) handle(conn net.PacketConn, peer net.Addr, m *dhcpv4.DHCPv4) {
	localAddr := conn.LocalAddr().(*net.UDPAddr).IP
	if localAddr.IsUnspecified() || len(localAddr) != 4 {
		localAddr = net.ParseIP("10.0.1.10").To4()
	}

	var handler DHCPv4Handler
	switch m.MessageType() {
	case dhcpv4.MessageTypeAck:
		handler = d.Handlers.Ack
	case dhcpv4.MessageTypeDecline:
		break
	case dhcpv4.MessageTypeDiscover:
		handler = d.Handlers.Discover
	case dhcpv4.MessageTypeInform:
		handler = d.Handlers.Inform
	case dhcpv4.MessageTypeNak:
		break
	case dhcpv4.MessageTypeNone:
		break
	case dhcpv4.MessageTypeOffer:
		handler = d.Handlers.Offer
	case dhcpv4.MessageTypeRelease:
		handler = d.Handlers.Release
	case dhcpv4.MessageTypeRequest:
		handler = d.Handlers.Request
	}

	if handler != nil {
		response, err := handler(m, localAddr, peer)
		if err != nil {
			return
		}

		if response == nil {
			return
		}

		_, err = conn.WriteTo(response.ToBytes(), peer)
		if err != nil {
			return
		}
	} else {
		fmt.Fprintf(os.Stdout, "Unhandled message type %v", m.MessageType())
	}
}

func (d *DHCPDv4) ListenAndServe() error {
	server, err := server4.NewServer("", &d.ListenAddress, d.handle)
	if err != nil {
		return err
	}
	d.server = server

	return server.Serve()
}
