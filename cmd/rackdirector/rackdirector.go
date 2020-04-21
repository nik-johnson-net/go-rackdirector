package main

import (
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/nik-johnson-net/rackdirector/pkg/dhcpd"
	"github.com/nik-johnson-net/rackdirector/pkg/httpd"
	"github.com/nik-johnson-net/rackdirector/pkg/ipam"
	"github.com/nik-johnson-net/rackdirector/pkg/pxe"
	"github.com/nik-johnson-net/rackdirector/pkg/tftpd"
)

func main() {
	templateFiles, err := filepath.Glob("templates/*.template")
	if err != nil {
		panic(err)
	}

	templates, err := template.ParseFiles(templateFiles...)
	if err != nil {
		panic(err)
	}

	fmt.Fprintf(os.Stdout, "%s\n", templates.DefinedTemplates())

	ipamConfig := ipam.NewFromFile("hosts.json")
	dhcpServer := dhcpd.DHCPD{
		DHCPv4Handler: ipamConfig,
	}
	err = dhcpServer.ListenAndServe()
	if err != nil {
		panic(err)
	}

	tftpd := tftpd.Tftpd{
		Basedir: "tftp",
	}
	tftpd.ListenAndServe()

	controller := pxe.Pxe{
		StageTemplates: templates,
		IPAM:           ipamConfig,
	}

	httpd := httpd.HTTPD{
		Controller:    &controller,
		FileDirectory: "http",
	}

	httpDone, err := httpd.ListenAndServe()
	if err != nil {
		panic(err)
	}

	for {
		select {
		case <-httpDone:
			fmt.Fprintf(os.Stdout, "HTTPD done")
			return
		}
	}
}
