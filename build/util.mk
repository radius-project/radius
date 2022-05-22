# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

# go-install-tool will 'go install' any package $2 if it is missing from $1
define go-install-tool
@[ -f $(1) ] || { \
set -e;\
go install $(2);\
}
endef