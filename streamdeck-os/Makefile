# StreamDeck OS
#   make rpi4 | rpi5 | rpi3   build the SD card image (Buildroot, ~1h first time)
#   make test                 Go unit tests
#   make vm-test              end-to-end test in QEMU (needs apt, qemu-system-x86, zstd, cpio, netpbm)

BR_VERSION ?= 2025.02.7
BR_DIR     ?= $(CURDIR)/.buildroot
EXTERNAL   := $(CURDIR)/buildroot-external
VM_WORK    ?= $(CURDIR)/.vm

.PHONY: rpi3 rpi4 rpi5 test vm-test clean

$(BR_DIR)/Makefile:
	git clone --depth 1 --branch $(BR_VERSION) https://gitlab.com/buildroot.org/buildroot.git $(BR_DIR)

rpi3 rpi4 rpi5: $(BR_DIR)/Makefile
	$(MAKE) -C $(BR_DIR) BR2_EXTERNAL=$(EXTERNAL) O=$(CURDIR)/output/$@ streamdeck_$@_defconfig
	$(MAKE) -C $(BR_DIR) O=$(CURDIR)/output/$@
	@echo "Image: output/$@/images/sdcard.img"

test:
	cd app && go vet ./... && go test ./...

vm-test:
	test/vm/build-initramfs.sh $(VM_WORK)
	test/vm/vm-test.py $(VM_WORK)

clean:
	rm -rf output $(VM_WORK)
