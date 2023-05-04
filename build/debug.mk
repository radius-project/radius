# ------------------------------------------------------------
# Copyright (c) Microsoft Corporation.
# Licensed under the MIT License.
# ------------------------------------------------------------

##@ Debugging

.PHONY: dump
dump: ## Outputs the values of all variables in the makefile.
	$(foreach v, \
		$(shell echo "$(filter-out .VARIABLES,$(.VARIABLES))" | tr ' ' '\n' | sort), \
		$(info $(shell printf "%-20s" "$(v)")= $(value $(v))) \
	)