package tftpd

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pin/tftp"
)

const (
	// FilenameBiosPxelinux is the request path to BIOS pxelinux file
	FilenameBiosPxelinux = "bios/pxelinux.0"

	// FilenameBiosGPxelinux is the request path to BIOS gpxelinux file
	FilenameBiosGPxelinux = "bios/gpxelinux.0"

	// FilenameBiosIPxelinux is the request path to BIOS ipxelinux file
	FilenameBiosIPxelinux = "bios/ipxelinux.0"

	// FilenameBiosLPxelinux is the request path to BIOS lpxelinux file
	FilenameBiosLPxelinux = "bios/lpxelinux.0"

	// FilenameEfi32Syslinux is the request path to EFI32 Syslinux file
	FilenameEfi32Syslinux = "efi32/syslinux.efi"

	// FilenameEfi64Syslinux is the request path to EFI64 Syslinux file
	FilenameEfi64Syslinux = "efi64/syslinux.efi"

	// FilenameUndionly is the request path to the Undionly file
	FilenameUndionly = "undionly.kpxe"

	// FilenameIPXE is the request path to the iPXE file
	FilenameIPXE = "ipxe.efi"
)

// Tftpd runs a TFTP server for serving chainload netboot files.
type Tftpd struct {
	Basedir string
	Listen  string
	Timeout time.Duration
	s       *tftp.Server
}

func (t *Tftpd) fileMapping(filename string) (string, error) {
	filename = strings.TrimPrefix(filename, "/")
	basename := path.Base(filename)
	path := path.Dir(filename)
	switch filename {
	case FilenameIPXE:
		return filepath.Join(t.Basedir, FilenameIPXE), nil
	case FilenameUndionly:
		return filepath.Join(t.Basedir, FilenameUndionly), nil
	case FilenameBiosPxelinux:
		fallthrough
	case FilenameBiosGPxelinux:
		fallthrough
	case FilenameBiosIPxelinux:
		fallthrough
	case FilenameBiosLPxelinux:
		return filepath.Join(t.Basedir, FilenameBiosLPxelinux), nil
	case FilenameEfi32Syslinux:
		fallthrough
	case FilenameEfi64Syslinux:
		return filepath.Join(t.Basedir, filename), nil
	}

	switch path {
	case "efi64":
		fallthrough
	case "efi32":
		return filepath.Join(t.Basedir, path, basename), nil
	}

	return "", os.ErrNotExist
}

// readHandler is called when client starts file download from server
func (t *Tftpd) readHandler(filename string, rf io.ReaderFrom) error {
	fileToOpen, err := t.fileMapping(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TFTP: Error answering request for %s: %v\n", filename, err)
		return err
	}
	file, err := os.Open(fileToOpen)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TFTP: Error answering request for %s: %v\n", filename, err)
		return err
	}
	// Set transfer size before calling ReadFrom.
	stat, err := file.Stat()
	if err != nil {
		fmt.Fprintf(os.Stderr, "TFTP: Error answering request for %s: %v\n", filename, err)
		return err
	}
	rf.(tftp.OutgoingTransfer).SetSize(stat.Size())
	n, err := rf.ReadFrom(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "TFTP: Error answering request for %s: %v\n", filename, err)
		return err
	}
	fmt.Printf("TFTP: Requested %s, Sent %s: %d bytes sent\n", filename, fileToOpen, n)
	return nil
}

// ListenAndServe starts listening for TFTP requests in the background
func (t *Tftpd) ListenAndServe() {
	// Set default to :69
	listenAddr := t.Listen
	if listenAddr == "" {
		listenAddr = ":69"
	}

	// Read only server, don't define a write handler
	t.s = tftp.NewServer(t.readHandler, nil)

	if t.Timeout != 0 {
		t.s.SetTimeout(t.Timeout)
	}

	go func() {
		fmt.Fprintf(os.Stdout, "tftp server on: %v\n", listenAddr)
		err := t.s.ListenAndServe(listenAddr) // blocks until s.Shutdown() is called
		if err != nil {
			fmt.Fprintf(os.Stdout, "server: %v\n", err)
			os.Exit(1)
		}
	}()
}

// Close stops the running TFTP server
func (t *Tftpd) Close() error {
	if t.s != nil {
		t.s.Shutdown()
	}
	return nil
}
