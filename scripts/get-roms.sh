#!/bin/sh
# Downloads the two KIM-1 6530 RIOT ROM images into testdata/, where the
# simulator looks for them by default. They aren't in this repository
# (third-party copyrighted content); this fetches the widely mirrored
# dumps from the Funet Commodore archive (falling back to zimmers.net)
# and checks them against known SHA-256 sums. ROM files already present
# in testdata/ are left alone.
set -eu

cd "$(dirname "$0")/../testdata"

MIRRORS="https://bio.nic.funet.fi/pub/cbm/src/kim-1 http://www.zimmers.net/anonftp/pub/cbm/src/kim-1"

sha256() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | cut -d' ' -f1
	else
		shasum -a 256 "$1" | cut -d' ' -f1
	fi
}

fetch() {
	name=$1 want=$2
	if [ -f "$name" ]; then
		echo "testdata/$name already exists, leaving it alone"
		return
	fi
	for mirror in $MIRRORS; do
		if curl -fsSL -o "$name.part" "$mirror/$name"; then
			if [ "$(sha256 "$name.part")" = "$want" ]; then
				mv "$name.part" "$name"
				echo "downloaded testdata/$name"
				return
			fi
			echo "warning: $mirror/$name has an unexpected checksum, trying the next mirror" >&2
		fi
	done
	rm -f "$name.part"
	echo "error: couldn't download $name; see testdata/README.md for other sources" >&2
	exit 1
}

fetch 6530-002.bin 79c6dbd54aa3911f53c1068c4310aabceb7a6ce9714d1c402b73af76ea109237
fetch 6530-003.bin 80f5c6393e0bbcfc8a5b68fd73858da34e6d6a415f32d4820f21edb52fecafec
