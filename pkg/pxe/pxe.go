package pxe

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/nik-johnson-net/rackdirector/pkg/ipam"
)

var planMap = map[string][]string{
	"reinstall-centos-8": {
		"install-centos-8",
	},
	"reinstall-centos-7": {
		"install-centos-7",
	},
}

type plandef struct {
	Name         string
	Stages       []string
	CurrentStage uint
}

type interfaceTemplate struct {
	Device      string
	IPv4Address string
	IPv4Netmask string
	IPv4Gateway string
}

type stageTemplate struct {
	Hostname     string
	DNS          []string
	DomainSearch string
	Server       string
	Interfaces   []interfaceTemplate
}

type pxeTemplate struct {
	Hostname  string
	Address   string
	Stage     string
	Default   string
	Server    string
	OSServer  string
	Netmask   string
	Gateway   string
	Interface string
}

type Pxe struct {
	StageTemplates *template.Template
	IPAM           *ipam.StaticIpam
	hostPlans      map[string]plandef
}

func (p *Pxe) InstallSeed(peer net.IP) ([]byte, error) {
	plan, exists := p.hostPlans[peer.String()]
	if !exists {
		return nil, fmt.Errorf("%v not in a plan", peer)
	}
	stage := plan.Stages[plan.CurrentStage]

	if !strings.HasPrefix(stage, "install-") {
		return nil, fmt.Errorf("%v not in an install stage. Current stage is %v", peer, stage)
	}
	return p.installTemplate(peer, stage)
}

func (p *Pxe) installTemplate(peer net.IP, stage string) ([]byte, error) {
	var buffer bytes.Buffer
	peerInfo, err := p.IPAM.Get(peer)
	if err != nil {
		return nil, err
	}

	templateName := fmt.Sprintf("%s.template", stage)

	dns := []string{
		"1.1.1.1",
	}

	if p.StageTemplates.Lookup(templateName) == nil {
		return buffer.Bytes(), fmt.Errorf("Stage doesn't exist %v", stage)
	}
	interfaces := make([]interfaceTemplate, 0)
	for _, interf := range peerInfo.Interfaces {
		var gateway string
		if len(interf.Ipv4Gateway) != 0 {
			gateway = interf.Ipv4Gateway.String()
		}
		interfaces = append(interfaces, interfaceTemplate{
			Device:      interf.Device,
			IPv4Address: interf.Ipv4.String(),
			IPv4Gateway: gateway,
			IPv4Netmask: networkToNetmask(interf.Network.Mask),
		})
	}
	err = p.StageTemplates.ExecuteTemplate(&buffer, templateName, stageTemplate{
		Hostname:   peerInfo.Hostname,
		DNS:        dns,
		Server:     "10.0.1.10",
		Interfaces: interfaces,
	})
	fmt.Fprintf(os.Stdout, "Delivering kickstart to %v:\n%v\n", peerInfo.Hostname, buffer.String())
	return buffer.Bytes(), err
}

func networkToNetmask(network net.IPMask) string {
	if len(network) != 4 {
		return ""
	}
	var builder bytes.Buffer
	for idx, num := range network {
		if idx != 0 {
			builder.WriteByte('.')
		}
		segment := fmt.Sprintf("%d", num)
		builder.WriteString(segment)
	}
	return builder.String()
}

func (p *Pxe) PxeConfig(peer net.IP) ([]byte, error) {
	plan, exists := p.hostPlans[peer.String()]
	defaultMenu := "localboot"
	stage := ""
	if exists {
		stage := plan.Stages[plan.CurrentStage]
		if strings.HasPrefix(stage, "install-") {
			split := strings.SplitN(stage, "-", 2)
			defaultMenu = split[1]
		} else {
			defaultMenu = "rackdirector-environment"
		}
	}

	peerInfo, err := p.IPAM.Get(peer)
	if err != nil {
		return nil, err
	}

	var buffer bytes.Buffer
	err = p.StageTemplates.ExecuteTemplate(&buffer, "pxemenu.template", pxeTemplate{
		Hostname:  peerInfo.Hostname,
		Address:   peer.String(),
		Stage:     stage,
		Default:   defaultMenu,
		Server:    "10.0.1.10",
		OSServer:  "192.168.0.10",
		Netmask:   networkToNetmask(peerInfo.Interfaces[0].Network.Mask),
		Gateway:   peerInfo.Interfaces[0].Ipv4Gateway.String(),
		Interface: "eth0",
	})
	return buffer.Bytes(), err
}

func (p *Pxe) CurrentPlan(peer net.IP) (string, error) {
	plan, exists := p.hostPlans[peer.String()]
	if !exists {
		return "", fmt.Errorf("%v not in a plan", peer)
	}
	return plan.Name, nil
}

func (p *Pxe) SetPlan(ip net.IP, plan string) error {
	currentPlan, exists := p.hostPlans[ip.String()]
	if exists {
		return fmt.Errorf("%v already in a plan (%v)", ip, currentPlan.Name)
	}

	newplan, err := p.newPlan(plan)
	if err != nil {
		return err
	}

	if p.hostPlans == nil {
		p.hostPlans = make(map[string]plandef)
	}
	p.hostPlans[ip.String()] = newplan
	if err := p.ipmireboot(ip); err != nil {
		return err
	}
	return nil
}

func (p *Pxe) newPlan(plan string) (plandef, error) {
	stages, ok := planMap[plan]
	if !ok {
		return plandef{}, fmt.Errorf("plan %v doesn't exist", plan)
	}

	return plandef{
		Name:         plan,
		Stages:       stages,
		CurrentStage: 0,
	}, nil
}

func (p *Pxe) AdvancePlan(peer net.IP) error {
	peerInfo, err := p.IPAM.Get(peer)
	if err != nil {
		return err
	}
	plan, exists := p.hostPlans[peer.String()]
	if !exists {
		return fmt.Errorf("%v not in a plan", peer)
	}
	plan.CurrentStage++
	if plan.CurrentStage == uint(len(plan.Stages)) {
		// Plan done
		fmt.Fprintf(os.Stdout, "Plan for %v finished.\n", peerInfo.Hostname)
		delete(p.hostPlans, peer.String())
	} else {
		p.hostPlans[peer.String()] = plan
	}

	return nil
}

func (p *Pxe) ipmireboot(ip net.IP) error {
	mgmtHostname := p.findMgmt(ip)
	cmd := exec.Command("ipmitool", "-I", "lanplus", "-H", mgmtHostname, "-U", "ADMIN", "-E", "power", "cycle")
	cmd.Env = []string{"IPMI_PASSWORD=ADMIN"}
	fmt.Fprintf(os.Stdout, "Running %s\n", cmd.String())
	return cmd.Run()
}

func (p *Pxe) findMgmt(ip net.IP) string {
	peerInfo, err := p.IPAM.Get(ip)
	if err != nil {
		panic(err)
	}
	return peerInfo.Bmc.Hostname
}
