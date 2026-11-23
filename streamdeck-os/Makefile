# StreamDeck OS
#   make iso        build streamdeck-os.iso (Ubuntu 24.04 host; see os/Dockerfile otherwise)
#   make docker-iso build it inside Docker from any OS
#   make test       Go unit tests
#   make iso-test   end-to-end test in QEMU: UEFI USB boot -> install to NVMe -> reboot -> deck
#   make wifi-test  same, with no Ethernet and a (virtual) USB Wi-Fi dongle plugged in mid-install
#   make modeswitch-test  a "driver CD" Wi-Fi dongle plugged in after boot is switched to Wi-Fi mode
#   make media-test       install from Rufus "ISO mode" / Ventoy sticks; never wipe a disk without the files
#   make seed-test        install from streamdeck-setup.txt on a USB stick without typing; export it back

WORK ?= $(CURDIR)/.iso

.PHONY: iso docker-iso test iso-test wifi-test modeswitch-test media-test seed-test clean

iso:
	os/build-iso.sh $(WORK)

docker-iso:
	docker build -t streamdeck-os-builder os
	docker run --rm --privileged -v $(CURDIR):/src -w /src streamdeck-os-builder os/build-iso.sh /src/.iso

test:
	cd app && go vet ./... && go test ./...

iso-test:
	os/build-iso.sh $(WORK) --test
	test/iso-test.py $(WORK)

wifi-test:
	os/build-iso.sh $(WORK) --test
	test/wifi-test.py $(WORK)

modeswitch-test:
	os/build-iso.sh $(WORK) --test
	test/modeswitch-test.py $(WORK)

media-test:
	os/build-iso.sh $(WORK) --test
	test/media-test.py $(WORK)

seed-test:
	os/build-iso.sh $(WORK) --test
	test/seed-test.py $(WORK)

clean:
	rm -rf $(WORK)
