#! /bin/bash
if [[ -z $BICEP_PATH ]]
then
    echo "usage: BICEP_PATH=path/to/bicep ./build/validate-bicep.sh"
    exit 1
fi

FILES=$(find . -type f -name "*.bicep")

# Get the first bicep file with Radius and AWS extensions from the list to restore extensions
FIRST_FILE_RAD=""
FIRST_FILE_AWS=""
for F in $FILES
do
    # Check if the file contains the word "extension radius"
    if [ -z "$FIRST_FILE_RAD" ] && grep -q "extension radius" "$F"; then
        FIRST_FILE_RAD="$F"
    fi
    # Check if the file contains the word "extension aws"
    if [ -z "$FIRST_FILE_AWS" ] && grep -q "extension aws" "$F"; then
        FIRST_FILE_AWS="$F"
    fi
    # Break the loop if both files are found
    if [ -n "$FIRST_FILE_RAD" ] && [ -n "$FIRST_FILE_AWS" ]; then
        break
    fi
done

# Restore the extensions once 
echo "running Radius: $BICEP_PATH build $FIRST_FILE_RAD"
STDERR=$($BICEP_PATH build $FIRST_FILE_RAD --stdout 2>&1 1>/dev/null)
echo "Restoring Radius extension with response: $STDERR..."

echo "running AWS: $BICEP_PATH build $FIRST_FILE_AWS"
STDERR=$($BICEP_PATH build $FIRST_FILE_AWS --stdout 2>&1 1>/dev/null)
echo "Restoring AWS extension with response: $STDERR..."

FAILURES=()
WARNINGS=()
for F in $FILES
do
    echo "validating $F"
    # We need to run bicep and fail in one of two cases:
    # - non-zero exit code
    # - non-empty stderr 
    #
    # We also don't want to dirty any files on disk.
    #
    # This complicated little block does that:
    # - Compiled output (ARM templates) go to bicep's stdout
    # - bicep's stdout goes to /dev/null
    # - bicep's stderr goes to the variable
    if grep -q "extension" $F
    then
        exec 3>&1
        echo "running: $BICEP_PATH build --no-restore $F"
        STDERR=$($BICEP_PATH build --no-restore $F --stdout 2>&1 1>/dev/null)
        EXITCODE=$?
        exec 3>&-
    fi
    
    if [[ $STDERR == *"Warning"* ]]
    then
        echo $STDERR
        WARNINGS+=$F
    fi

    if [[ ! $EXITCODE -eq 0 || $STDERR == *"Error"* ]]
    then
        echo $STDERR
        FAILURES+=$F
    fi
done

for F in $FAILURES
do
  echo "Failed: $F"
done

for F in $WARNINGS
do
  echo "Warning: $F"
done

exit ${#FAILURES[@]}