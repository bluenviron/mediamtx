include $(TOPDIR)/rules.mk

PKG_NAME:=rtsp-simple-server
PKG_VERSION:=v0.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/aler9/rtsp-simple-server
PKG_SOURCE_VERSION:=$(PKG_VERSION)

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0

GO_PKG:=github.com/aler9/rtsp-simple-server
GO_PKG_LDFLAGS_X:=github.com/aler9/rtsp-simple-server/internal/core.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

GO_MOD_ARGS:=-buildvcs=false

define Package/rtsp-simple-server
  SECTION:=net
  CATEGORY:=Network
  TITLE:=rtsp-simple-server
  URL:=https://github.com/aler9/rtsp-simple-server
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/rtsp-simple-server/description
  ready-to-use server and proxy that allows users to publish, read and proxy live video and audio streams through various protocols
endef

$(eval $(call GoBinPackage,rtsp-simple-server))
$(eval $(call BuildPackage,rtsp-simple-server))
