################################################################################
#
# streamdeckd
#
################################################################################

STREAMDECKD_VERSION = 1.0
STREAMDECKD_SITE = $(BR2_EXTERNAL_STREAMDECK_OS_PATH)/../app
STREAMDECKD_SITE_METHOD = local
STREAMDECKD_LICENSE = MIT
STREAMDECKD_GOMOD = streamdeckd
STREAMDECKD_BUILD_TARGETS = cmd/streamdeckd
STREAMDECKD_LDFLAGS = -s -w -X main.version=$(STREAMDECKD_VERSION)

$(eval $(golang-package))
