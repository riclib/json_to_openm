yell() { echo "`date -u +"%Y-%m-%dT%H:%M:%SZ"`: $*" >&2; }
die() { yell "$*"; exit 111; }
try() { "$@" || die "cannot $*"; }

if [ -z "$TEST" ]; then  
  DATA=" /mnt/ems-dev/import/is"
  OUTPUT="/data/prometheus_is"
  BIN="/apps/solidmon/bin"
  TMP="/tmp"
else 
  DATA="test"
  OUTPUT="test/blocks"
  BIN="."
  TMP="/tmp"
fi

echo $BIN

#!/bin/bash
mv $DATA/inbox/* $DATA/processing || die " No Files to process"

# generate the open metrics
try $BIN/json_to_openm --out /tmp/out.prom $DATA/processing/*.json

# stop prometheus
echo test is $TEST
if [ -z "$TEST" ]; then
   try pkill -f conf/is/prom
else
   yell "Not killing prometheus as just testing"
fi

# generate blocks
try $BIN/promtool tsdb create-blocks-from openmetrics --max-block-duration=168h $TMP/out.prom $OUTPUT

# start prometheus
if [ -z "$TEST" ]; then
  try $BIN/prom_is_start.sh
else 
  yell "Not starting prometheus as just testing"
fi

# archive the processed files
try mv $DATA/processing/* $DATA/archive
