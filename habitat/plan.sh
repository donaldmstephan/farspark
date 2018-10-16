pkg_name=farspark
pkg_description="Fast and secure standalone proxy server for serving, resizing, and converting remote media"
pkg_upstream_url="https://github.com/MozillaReality/farspark"
pkg_origin=mozillareality
pkg_version="v1.4"
pkg_maintainer=''
pkg_maintainer="Mozilla Mixed Reality <mixreality@mozilla.com>"
pkg_license=("MIT")
pkg_source="https://github.com/MozillaReality/farspark"
pkg_bin_dirs=(bin)
pkg_deps=(core/glibc core/gcc-libs core/bash mozillareality/ghostscript)
pkg_build_deps=(mozillareality/ghostscript)
pkg_scaffolding=core/scaffolding-go
scaffolding_go_base_path=github.com/MozillaReality/farspark
scaffolding_go_build_deps=()

do_download() {
  # HACK: need to set CGO environment here since the download stage fails otherwise

  _build_environment
  export CGO_CFLAGS=$CFLAGS
  export CGO_LDFLAGS=$LDFLAGS

  do_default_download
}

do_build() {
  do_default_build
}

do_install() {
  do_default_install
}
