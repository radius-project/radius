# See https://everything.curl.dev/usingcurl/downloads/retry
#
# Unfortunately we can't use --retry-all-errors as the agent does not support it.
curl $@ --retry 5