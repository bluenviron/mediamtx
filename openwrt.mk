include $(TOPDIR)/rules.mk

PKG_NAME:=mediamtx
PKG_VERSION:=v0.0.0
PKG_RELEASE:=1

PKG_SOURCE_PROTO:=git
PKG_SOURCE_URL:=https://github.com/aler9/mediamtx
PKG_SOURCE_VERSION:=$(PKG_VERSION)

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0

GO_PKG:=github.com/aler9/mediamtx
GO_PKG_LDFLAGS_X:=github.com/aler9/mediamtx/internal/core.version=$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk
include $(TOPDIR)/feeds/packages/lang/golang/golang-package.mk

GO_MOD_ARGS:=-buildvcs=false

define Package/mediamtx
  SECTION:=net
  CATEGORY:=Network
  TITLE:=mediamtx
  URL:=https://github.com/aler9/mediamtx
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/mediamtx/description
  ready-to-use server and proxy that allows users to publish, read and proxy live video and audio streams through various protocols
endef

$(eval $(call GoBinPackage,mediamtx))
$(eval $(call BuildPackage,mediamtx))
