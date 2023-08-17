#! /bin/bash
if [[ -z $BICEP_PATH ]]
then
    echo "usage: BICEP_PATH=path/to/bicep ./build/validate-bicep.sh"
    exit 1
fi

FILES=$(find . -type f -name "*.bicep")
FAILURES=()
for F in $FILES
do
    echo "validating $F"
    # We need to run the rad-bicep and fail in one of two cases:
    # - non-zero exit code
    # - non-empty stderr 
    #
    # We also don't want to dirty any files on disk.
    #
    # This complicated little block does that:
    # - Compiled output (ARM templates) go to rad-bicep's stdout
    # - rad-bicep's stdout goes to /dev/null
    # - rad-bicep's stderr goes to the variable
    if grep -q "import radius as radius" $F
    then
        exec 3>&1
        echo "running: $BICEP_PATH build $F"
        STDERR=$($BICEP_PATH build $F --stdout 2>&1 1>/dev/null)
        EXITCODE=$?
        exec 3>&-
    fi
    
    if [[ ! $EXITCODE -eq 0 || ! -z $STDERR ]]
    then
        echo $STDERR
        FAILURES+=$F
    fi
done

for F in $FAILURES
do
  echo "Failed: $F"
done

exit ${#FAILURES[@]}