package httpd

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type Controller interface {
	InstallSeed(peer net.IP) ([]byte, error)
	PxeConfig(peer net.IP) ([]byte, error)
	IPxeConfig(peer net.IP) ([]byte, error)
	CurrentPlan(ip net.IP) (string, error)
	SetPlan(ip net.IP, plan string) error
	AdvancePlan(peer net.IP) error
}

type getRequest struct {
	Address string
}

type getResponse struct {
	Plan string
}

type setRequest struct {
	Address string
	Plan    string
}

func getPeer(r *http.Request) net.IP {
	address, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		panic(err)
	}
	return net.ParseIP(address)
}

type HTTPD struct {
	Controller    Controller
	FileDirectory string
	endChan       chan<- bool
	httpServer    http.Server
}

func (h *HTTPD) ListenAndServe() (<-chan bool, error) {
	endChan := make(chan bool, 1)
	h.endChan = endChan

	muxer := http.NewServeMux()
	muxer.HandleFunc("/config.ipxe", h.ipxe)
	muxer.HandleFunc("/bios/pxelinux.cfg/default", h.pxelinux)
	muxer.HandleFunc("/efi32/pxelinux.cfg/default", h.pxelinux)
	muxer.HandleFunc("/efi64/pxelinux.cfg/default", h.pxelinux)
	muxer.HandleFunc("/bios/", h.serveFile)
	muxer.HandleFunc("/efi32/", h.serveFile)
	muxer.HandleFunc("/efi64/", h.serveFile)
	muxer.HandleFunc("/ipxe.efi", h.serveFile)
	muxer.HandleFunc("/installseed", h.installSeed)
	muxer.HandleFunc("/api/plan", h.plan)
	muxer.HandleFunc("/api/advanceplan", h.advanceplan)
	muxer.HandleFunc("/", h.handle404)
	h.httpServer = http.Server{
		Handler:  muxer,
		ErrorLog: log.New(os.Stderr, "http", log.LstdFlags),
	}

	go func() {
		err := h.httpServer.ListenAndServe()
		defer func() {
			endChan <- true
		}()
		if err != nil {
			panic(err)
		}
	}()

	return endChan, nil
}

func (h *HTTPD) serveFile(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stdout, "http serveFile %v\n", r.URL.Path)
	http.ServeFile(w, r, filepath.Join(h.FileDirectory, r.URL.EscapedPath()))
}

func (h *HTTPD) handle404(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stdout, "http 404 %v\n", r.URL)
	w.WriteHeader(404)
}

func (h *HTTPD) pxelinux(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stdout, "http pxelinux\n")
	address := getPeer(r)
	body, err := h.Controller.PxeConfig(address)
	if err != nil {
		fmt.Fprintf(os.Stdout, "http fuck %v\n", err)
		panic(err)
	}

	w.WriteHeader(200)
	w.Write(body)
}

func (h *HTTPD) ipxe(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(os.Stdout, "http ipxe\n")
	address := getPeer(r)
	body, err := h.Controller.IPxeConfig(address)
	if err != nil {
		fmt.Fprintf(os.Stdout, "http error %v\n", err)
		panic(err)
	}

	w.WriteHeader(200)
	w.Write(body)
	log.Println("ipxe config to", address, "\n", string(body))
}

func (h *HTTPD) installSeed(w http.ResponseWriter, r *http.Request) {
	address := getPeer(r)
	body, err := h.Controller.InstallSeed(address)
	if err != nil {
		panic(err)
	}

	w.WriteHeader(200)
	w.Write(body)
}

func (h *HTTPD) advanceplan(w http.ResponseWriter, r *http.Request) {
	address := getPeer(r)
	err := h.Controller.AdvancePlan(address)
	if err != nil {
		panic(err)
	}

	w.WriteHeader(200)
}

func (h *HTTPD) plan(w http.ResponseWriter, r *http.Request) {
	var body []byte
	var err error

	switch r.Method {
	case http.MethodGet:
		var request getRequest
		err = json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			panic(err)
		}
		address := net.ParseIP(request.Address)
		plan, err := h.Controller.CurrentPlan(address)
		if err != nil {
			panic(err)
		}
		w.WriteHeader(200)
		err = json.NewEncoder(w).Encode(getResponse{plan})

	case http.MethodPost:
		var request setRequest
		err = json.NewDecoder(r.Body).Decode(&request)
		if err != nil {
			panic(err)
		}
		address := net.ParseIP(request.Address)
		err = h.Controller.SetPlan(address, request.Plan)
	}

	if err != nil {
		panic(err)
	}
	w.WriteHeader(200)
	w.Write(body)
}
