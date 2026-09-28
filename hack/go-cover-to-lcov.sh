#!/usr/bin/env bash
# Convert a Go coverage profile (stdin) to LCOV (stdout).
#
# File paths are copied verbatim into the SF: records, so the profile must
# already hold repo-relative paths (e.g. daemon/api/data.go). This lets the
# coverage service place each file without resolving Go import paths, which is
# ambiguous when the repo holds several Go modules.
set -euo pipefail

awk '
  /^mode:/ { next }
  {
    file = $0
    sub(/:[0-9]+\.[0-9]+,.*$/, "", file)
    split(substr($0, length(file) + 2), a, /[., ]/)
    for (l = a[1]; l <= a[3]; l++) {
      key = file "\t" l
      if (!(key in hits) || a[6] > hits[key]) hits[key] = a[6]
    }
  }
  END { for (key in hits) print key "\t" hits[key] }
' | sort -t$'\t' -k1,1 -k2,2n | awk -F'\t' '
  function flush() {
    if (file != "") printf "LF:%d\nLH:%d\nend_of_record\n", lf, lh
  }
  $1 != file {
    flush()
    file = $1; lf = 0; lh = 0
    print "SF:" file
  }
  {
    print "DA:" $2 "," $3
    lf++
    if ($3 > 0) lh++
  }
  END { flush() }
'
