GOFILES:=$(shell find cmd/ pkg/ -name '*.go')
TEMPLATES:=$(wildcard templates/*.template)

archive: build/rackdirector.tar.gz

deploy: build/rackdirector.tar.gz
	scp build/rackdirector.tar.gz root@10.0.1.10:
	ssh root@10.0.1.10 'tar -xf rackdirector.tar.gz -C /opt/rackdirector && systemctl restart rackdirector'

log-tail:
	ssh root@10.0.1.10 journalctl -fu rackdirector

build/rackdirector.tar.gz: package
	cd build && tar -cf 'rackdirector.tar.gz' -C package .

run: package
	cd build/package; sudo ./rackdirector

.PHONY: clean archive deploy

clean:
	rm -rf build/

package: cmd/rackdirector/rackdirector build/ipxe/src/bin/undionly.kpxe hosts.json build/package/http/efi32/syslinux.0 build/package/http/efi64/syslinux.0 build/package/http/bios/pxelinux.0 build/ipxe/src/bin-x86_64-efi/ipxe.efi $(addprefix build/package/,$(TEMPLATES))
	mkdir -p build/package;
	cp cmd/rackdirector/rackdirector build/package/rackdirector;
	cp hosts.json build/package;
	mkdir -p build/package/tftp;
	cp build/ipxe/src/bin/undionly.kpxe build/package/tftp/undionly.kpxe;
	cp build/ipxe/src/bin-x86_64-efi/ipxe.efi build/package/tftp/ipxe.efi;
	mkdir -p build/package/http;
	cp build/ipxe/src/bin-x86_64-efi/ipxe.efi build/package/http/ipxe.efi

cmd/rackdirector/rackdirector: $(GOFILES)
	cd cmd/rackdirector && go build -v

build/ipxe/src/bin-x86_64-efi/ipxe.efi: build/ipxe
	cd build/ipxe/src && make bin-x86_64-efi/ipxe.efi

build/ipxe/src/bin/undionly.kpxe: build/ipxe
	cd build/ipxe/src && make bin/undionly.kpxe

build/ipxe:
	mkdir -p build && cd build && git clone git://git.ipxe.org/ipxe.git

build/package/http/efi32/syslinux.0: build/cache/syslinux
	mkdir -p build/package/http/efi32;
	cp build/cache/syslinux/efi32/efi/syslinux.efi build/package/http/efi32;
	find build/cache/syslinux/efi32/com32 -name '*.c32' -or -name '*.e32' | xargs cp -t build/package/http/efi32

build/package/http/efi64/syslinux.0: build/cache/syslinux
	mkdir -p build/package/http/efi64;
	cp build/cache/syslinux/efi64/efi/syslinux.efi build/package/http/efi64;
	find build/cache/syslinux/efi64/com32 -name '*.c32' -or -name '*.e64' | xargs cp -t build/package/http/efi64

build/package/http/bios/pxelinux.0: build/cache/syslinux
	mkdir -p build/package/http/bios;
	cp build/cache/syslinux/bios/core/lpxelinux.0 build/package/http/bios/;
	cp build/cache/syslinux/bios/memdisk/memdisk build/package/http/bios/;
	find build/cache/syslinux/bios/com32 -name '*.c32' | xargs cp -t build/package/http/bios/

build/cache/syslinux:
	mkdir -p build/cache;
	cd build/cache && \
	wget https://mirrors.edge.kernel.org/pub/linux/utils/boot/syslinux/syslinux-6.03.tar.xz && \
	tar -xf syslinux-6.03.tar.xz && \
	mv syslinux-6.03 syslinux;

build/package/templates/%.template: templates/%.template
	mkdir -p build/package/templates;
	cp $? build/package/templates/