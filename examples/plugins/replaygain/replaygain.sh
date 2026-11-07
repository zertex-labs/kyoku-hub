#!/bin/sh
# ReplayGain via rsgain (https://github.com/complexlogic/rsgain).
#
#   rsgain easy -m MAX -S .
#
# -m MAX  scan with every core
# -S      skip files that already carry ReplayGain data, so a retried or resumed
#         download does not rescan what is already tagged
# -q      keep rsgain's progress out of the job log; the summary line is ours
#
# The album is scanned as a whole, which is what produces REPLAYGAIN_ALBUM_GAIN
# on top of the per-track values.
set -eu

dir="${KYOKU_ALBUM_DIR:-}"
if [ -z "$dir" ] || [ ! -d "$dir" ]; then
	echo "no album directory in this hook" >&2
	exit 3
fi

if ! command -v rsgain >/dev/null 2>&1; then
	echo "rsgain is not installed — pacman -S rsgain / apt install rsgain / brew install rsgain" >&2
	exit 1
fi

rsgain easy -m MAX -S -q "$dir" >&2

files=$(grep -al "REPLAYGAIN_TRACK_GAIN" "$dir"/* 2>/dev/null | wc -l | tr -d ' ')
if [ "$files" = "0" ]; then
	echo "rsgain produced no tags" >&2
	exit 1
fi

# REPLAYGAIN_ALBUM_GAIN=-7.42 dB, read straight out of the comment block.
gain=$(grep -ao "REPLAYGAIN_ALBUM_GAIN=[^[:cntrl:]]*" "$dir"/* 2>/dev/null | head -1 | cut -d= -f2)
if [ -n "$gain" ]; then
	printf '{"message": "album gain %s · %s file(s)"}\n' "$gain" "$files"
else
	printf '{"message": "track gain on %s file(s)"}\n' "$files"
fi
