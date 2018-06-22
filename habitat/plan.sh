pkg_name=farspark
pkg_description="Fast and secure standalone proxy server for serving, resizing, and converting remote media"
pkg_upstream_url="https://github.com/gfodor/farspark"
pkg_origin=mozillareality
pkg_version="v1.2"
pkg_maintainer=''
pkg_maintainer="Mozilla Mixed Reality <mixreality@mozilla.com>"
pkg_license=("MIT")
pkg_source="https://github.com/gfodor/farspark"
pkg_bin_dirs=(bin)
pkg_deps=(core/glibc core/gcc-libs core/bash)
pkg_scaffolding=core/scaffolding-go
scaffolding_go_base_path=github.com/gfodor/farspark
scaffolding_go_build_deps=()

do_build() {
  do_default_build
}

do_install() {
  mkdir -p "$pkg_prefix/lib"
  mkdir -p "$pkg_prefix/include"
  mkdir -p "$pkg_prefix/share"

  cp -r "$HAB_CACHE_SRC_PATH/scaffolding-go-gopath/src/github.com/gfodor/farspark/vendor/github.com/discordapp/lilliput/deps/linux/lib" "$pkg_prefix"
  cp -r "$HAB_CACHE_SRC_PATH/scaffolding-go-gopath/src/github.com/gfodor/farspark/vendor/github.com/discordapp/lilliput/deps/linux/include" "$pkg_prefix"
  cp -r "$HAB_CACHE_SRC_PATH/scaffolding-go-gopath/src/github.com/gfodor/farspark/vendor/github.com/discordapp/lilliput/deps/linux/share" "$pkg_prefix"

  do_default_install
}
