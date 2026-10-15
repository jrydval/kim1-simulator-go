#!/bin/sh
# Downloads CP/M-65 for the KIM-1 "directly attached SD card" port into
# testdata/cpm65/ (git-ignored): the bootsd.pap loader and the 32MB
# diskimage.raw, from the CP/M-65 development release. It also makes
# cpm65.raw, a working copy of the disk for the simulator to write to;
# run this script again (or copy diskimage.raw over it) for a fresh disk.
# See docs/cpm65.md for booting it.
set -eu

cd "$(dirname "$0")/../testdata"
mkdir -p cpm65
cd cpm65

curl -fSL -o kim-1-sdcard.zip https://github.com/davidgiven/cpm65/releases/download/dev/kim-1-sdcard.zip
unzip -o -q kim-1-sdcard.zip bootsd.pap diskimage.raw
rm kim-1-sdcard.zip
cp diskimage.raw cpm65.raw
echo "CP/M-65 is in testdata/cpm65/ (bootsd.pap, cpm65.raw)"
