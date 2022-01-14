#!/bin/bash
###############################################################################
# (c) Copyright IBM Corporation 2019, 2020. All Rights Reserved.
# Note to U.S. Government Users Restricted Rights:
# U.S. Government Users Restricted Rights - Use, duplication or disclosure restricted by GSA ADP Schedule
# Contract with IBM Corp.
# Licensed Materials - Property of IBM
# Copyright (c) 2021 Red Hat, Inc.
# Copyright Contributors to the Open Cluster Management project
###############################################################################

#Project start year
origin_year=2016
#Back up year if system time is null or incorrect
back_up_year=2019
#Currrent year
current_year=$(date +"%Y")

TRAVIS_BRANCH=$1

ADDED_SINCE_1_MAR_2020=$(git log --name-status --pretty=oneline --since "1 Mar 2020" | egrep "^A\t" | awk '{print $2}' | sort | uniq |  grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore))
MODIFIED_SINCE_1_MAR_2020=$(diff --new-line-format="" --unchanged-line-format="" <(git log --name-status --pretty=oneline --since "1 Mar 2020" | egrep "^A\t|^M\t" | awk '{print $2}' | sort | uniq | grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore)) <(git log --name-status --pretty=oneline --since "1 Mar 2020" | egrep "^A\t" | awk '{print $2}' | sort | uniq | grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore)))
OLDER_GIT_FILES=$(git log --name-status --pretty=oneline | egrep "^A\t|^M\t" | awk '{print $2}' | sort | uniq |  grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore))

if [[ "x${TRAVIS_BRANCH}" != "x" ]]; then
  FILES_TO_SCAN=$(git diff --name-only --diff-filter=AM ${TRAVIS_BRANCH}...HEAD | grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore))
else
  FILES_TO_SCAN=$(find . -type f | grep -Ev '(\.git)' | grep -v -f <(sed 's/\([.|]\)/\\\1/g; s/\?/./g ; s/\*/.*/g' .copyrightignore))
fi

if [ -z "$current_year" ] || [ $current_year -lt $origin_year ]; then
  echo "Can't get correct system time\n  >>Use back_up_year=$back_up_year as current_year to check copyright in the file $f\n"
  current_year=$back_up_year
fi

lic_ibm_identifier=" (c) Copyright IBM Corporation"
lic_redhat_identifier=" Copyright (c) ${current_year} Red Hat, Inc."

lic_year=()
#All possible combination within [origin_year, current_year] range is valid format
#seq isn't recommanded after bash version 3.0
for ((start_year=origin_year;start_year<=current_year;start_year++)); 
do
  lic_year+=(" (c) Copyright IBM Corporation ${start_year}. All Rights Reserved.")
  for ((end_year=start_year+1;end_year<=current_year;end_year++)); 
  do
    lic_year+=(" (c) Copyright IBM Corporation ${start_year}, ${end_year}. All Rights Reserved.")
  done
done
lic_year_size=${#lic_year[@]}

#lic_rest to scan for rest copyright format's correctness
lic_rest=()
lic_rest+=(" Licensed Materials - Property of IBM")
lic_rest+=(" Note to U.S. Government Users Restricted Rights:")
lic_rest+=(" Use, duplication or disclosure restricted by GSA ADP Schedule")
lic_rest+=(" Contract with IBM Corp.")
lic_rest_size=${#lic_rest[@]}

#Used to signal an exit
ERROR=0
RETURNCODE=0

echo "##### Copyright check #####"
#Loop through all files. Ignore .FILENAME types
#for f in `find .. -type f ! -path "../.eslintrc.js" ! -path "../build-harness/*" ! -path "../auth-setup/*" ! -path "../sslcert/*" ! -path "../node_modules/*" ! -path "../coverage/*" ! -path "../test-output/*" ! -path "../build/*" ! -path "../nls/*" ! -path "../public/*"`; do
for f in $FILES_TO_SCAN; do
  if [ ! -f "$f" ]; then
   continue
  fi

  # Flags that indicate the licenses to check for
  must_have_redhat_license=false
  must_have_ibm_license=false
  flag_redhat_license=false
  flag_ibm_license=false

  FILETYPE=$(basename ${f##*.})
  case "${FILETYPE}" in
    js | go | scss | properties | java | rb | sh )
      COMMENT_PREFIX=""
      ;;
    *)
      #printf " Extension $FILETYPE not considered !!!\n"
      continue
  esac

  #Read the first 15 lines, most Copyright headers use the first 10 lines.
  header=`head -15 $f`

  # Strip directory prefix, if any
  if [[ $f == "./"* ]]; then
    f=${f:2}
  fi

  printf " ========>>>>>>   Scanning $f . . .\n"
  if [[ "${ADDED_SINCE_1_MAR_2020}" == *"$f"* ]]; then
    printf " ---> Added since 01/03/2020\n"
    must_have_redhat_license=true
    flag_ibm_license=true
  elif [[ "${MODIFIED_SINCE_1_MAR_2020}" == *"$f"* ]]; then
    printf " ---> Modified since 01/03/2020\n"
    must_have_redhat_license=true
    must_have_ibm_license=true
  elif [[ "${OLDER_GIT_FILES}" == *"$f"* ]]; then
    printf " ---> File older than 01/03/2020\n"
    must_have_ibm_license=true
    flag_redhat_license=true
  else
    # Default case, could be new file not yet in git(?) - only expect Red Hat license
    must_have_redhat_license=true
  fi

  if [[ "${must_have_redhat_license}" == "true" ]] && [[ "$header" != *"${lic_redhat_identifier}"* ]]; then
    printf " Missing copyright\n >> Could not find [${lic_redhat_identifier}] in the file.\n"
    ERROR=1
  fi

  if [[ "${flag_redhat_license}" == "true" ]] && [[ "$header" == *"${lic_redhat_identifier}"* ]]; then 
    printf " Warning: Older file, may not include Red Hat license.\n"
  fi

  if [[ "${flag_ibm_license}" == "true" ]] && [[ "$header" == *"${lic_ibm_identifier}"* ]]; then 
    printf " Warning: newer file, may not contain IBM license.\n"
  fi

  if [[ "${must_have_ibm_license}" == "true" ]]; then 
    # Verify IBM copyright is present
    #Check for year copyright single line
    year_line_count=0
    for ((i=0;i<${lic_year_size};i++));
    do
      #Validate year formart within [origin_year, current_year] range
      if [[ "$header" == *"${lic_year[$i]}"* ]]; then
        year_line_count=$((year_line_count + 1))
      fi
    done

    #Must find and only find one line valid year, otherwise invalid copyright formart
    if [[ $year_line_count != 1 ]]; then
      printf "Missing copyright\n  >>Could not find correct copyright year in the file $f\n"
      ERROR=1
      #break 
    fi

    #Check for rest copyright lines
    for ((i=0;i<${lic_rest_size};i++));
    do
      #Validate the copyright line being checked is present
      if [[ "$header" != *"${lic_rest[$i]}"* ]]; then
        printf "Missing copyright\n  >>Could not find [${lic_rest[$i]}] in the file $f\n"
        ERROR=1
        #break 2
      fi
    done
  fi # end must_have_ibm_license

  #Add a status message of OK, if all copyright lines are found
  if [[ "$ERROR" == 0 ]]; then
    printf "OK\n"
  else
    RETURNCODE=$ERROR
    ERROR=0 # Reset error
  fi
done

echo "##### Copyright check ##### ReturnCode: ${RETURNCODE}"
exit $RETURNCODE
