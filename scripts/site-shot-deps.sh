#!/bin/sh
# Prepares a headless Chrome that renders text for `make site-shot`, without
# root. Chrome on Linux needs fontconfig and at least one system font to draw
# text reliably, and the full Chrome build needs a few shared libraries that
# minimal machines lack. This downloads those Debian packages with
# `apt-get download`, unpacks them under .build/chrome-deps, and writes a
# fonts.conf that points at the unpacked fonts. Nothing is installed
# system-wide. On machines without apt, it does nothing.
set -eu

root_directory=$(cd "$(dirname "$0")/.." && pwd)
deps=$root_directory/.build/chrome-deps
prefix=$deps/root
marker=$deps/ready

if [ -f "$marker" ]; then
	exit 0
fi
if ! command -v apt-get >/dev/null 2>&1 || ! command -v dpkg >/dev/null 2>&1; then
	echo "site-shot: apt-get is unavailable; using the headless shell without system fonts."
	exit 0
fi

packages="
fontconfig-config
fonts-dejavu-core
libfontconfig1
libcups2t64
libcairo2
libpango-1.0-0
libavahi-common3
libavahi-common-data
libavahi-client3
libxcb-render0
libxcb-shm0
libpixman-1-0
libfribidi0
libthai0
libdatrie1
libharfbuzz0b
libgraphite2-3
"

mkdir -p "$deps/debs" "$prefix/cache"
(
	cd "$deps/debs"
	# shellcheck disable=SC2086 # the package list is intentionally split into words
	apt-get download $packages >/dev/null
)
for package in "$deps"/debs/*.deb; do
	dpkg -x "$package" "$prefix"
done
cat >"$prefix/fonts.conf" <<EOF
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<fontconfig>
  <dir>$prefix/usr/share/fonts</dir>
  <cachedir>$prefix/cache</cachedir>
</fontconfig>
EOF
touch "$marker"
echo "site-shot: prepared Chrome libraries and fonts in .build/chrome-deps"
