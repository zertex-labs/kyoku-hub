#!/bin/sh
# A plugin receives one JSON document on stdin and may answer with JSON on stdout.
#
#   exit 0  ok
#   exit 3  skipped (not applicable to this file/album) — reported once, not as an error
#   other   failure, stderr is shown in the download log
#
# The working directory is this plugin's directory, never the album: take paths
# from the payload.

payload=$(cat)

hook=$(printf '%s' "$payload" | python3 -c 'import json,sys; print(json.load(sys.stdin)["hook"])')

case "$hook" in
  track.tagged)
    title=$(printf '%s' "$payload" | python3 -c 'import json,sys; print(json.load(sys.stdin)["track"]["title"])')
    echo "hello: tagged $title" >&2
    printf '{"message": "saw %s"}\n' "$title"
    ;;
  album.beforePublish)
    dir=$(printf '%s' "$payload" | python3 -c 'import json,sys; print(json.load(sys.stdin)["album"]["dir"])')
    count=$(printf '%s' "$payload" | python3 -c 'import json,sys; print(len(json.load(sys.stdin)["files"]))')
    printf 'hello was here\n' > "$dir/hello.txt"
    printf '{"message": "album has %s files, wrote hello.txt"}\n' "$count"
    ;;
esac
