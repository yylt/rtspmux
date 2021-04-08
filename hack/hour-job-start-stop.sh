#!/bin/bash

set -x

Root="/opt/dodo"
MergeDir="$Root/merged"
now=$(date +%R)

stime="08:30"
etime="17:30"


function clear()
{
   # remove before 3 days files
   find ${Root}  -mtime +3 -exec rm -rf {} \;

   for file in `ls $MergeDir`; do
      ists=$(echo $file|grep -ci "ts$")
      if [ $ists -eq 0 ] ;then
	      echo "$file is not  ts file,skip"
	      continue
      fi
      fname=${file%%\.*}
      fpath=$MergeDir/$file
      dstp=$MergeDir/$fname.mp4

      if [ -f "$dstp" ]; then
         echo "$dstp exist, skip trans format"
	 continue
      fi
      ffmpeg -i "$fpath" -c copy "$dstp"
      rm -f $fpath
   done
}

if [[ "$now" < "$stime" || "$now" > "$etime" ]]; then
   systemctl stop dlm3u
   clear
   exit 0
fi
echo "start service"
systemctl start dlm3u
