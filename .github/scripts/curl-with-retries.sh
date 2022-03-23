for i in {1..5}
do
    curl $@
    echo "$?"
    if [[ "$?" -eq 0 ]]
    then
    break
    fi
done