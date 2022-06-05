include $(TOPDIR)/rules.mk

PKG_NAME:=rtsp-simple-server
PKG_VERSION:=1
PKG_RELEASE:=1

PKG_SOURCE:=rtsp-simple-server-$(PKG_VERSION).tar.gz
PKG_SOURCE_VERSION:=9d3fd3bc37cf52adeb6436d1a788db8e96e391ad
PKG_SOURCE_URL:=https://github.com/aler9/rtsp-simple-server.git

PKG_SOURCE_PROTO:=git
PKG_SOURCE_DATE:=2022-06-05

PKG_MAINTAINER:=Kyle Miracle <kmiracle86@gmail.com>
PKG_LICENSE:=MIT
PKG_LICENSE_FILES:=LICENSE

PKG_BUILD_DEPENDS:=golang/host
PKG_BUILD_PARALLEL:=1
PKG_USE_MIPS16:=0

GO_PKG:=github.com/aler9/rtsp-simple-server

include $(INCLUDE_DIR)/package.mk
include /opt/openwrt/feeds/packages/lang/golang/golang-package.mk

define Package/rtsp-simple-server
  SECTION:=net
  CATEGORY:=Network
  TITLE:=rtsp-simple-server
  URL:=https://github.com/aler9/rtsp-simple-server
  DEPENDS:=$(GO_ARCH_DEPENDS)
endef

define Package/rtsp-simple-server/description
  rtsp-simple-server is a ready-to-use and zero-dependency server and proxy that allows users to publish, read and proxy live video and audio streams through various protocols
endef

$(eval $(call GoBinPackage,rtsp-simple-server))
$(eval $(call BuildPackage,rtsp-simple-server))
