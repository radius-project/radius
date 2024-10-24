# ------------------------------------------------------------
# Copyright 2023 The Radius Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#    
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
# ------------------------------------------------------------

POSTGRES_USER=postgres
POSTGRES_PASSWORD=ucprulz!
POSTGRES_IMAGE=ghcr.io/radius-project/mirror/postgres:latest
POSTGRES_CONTAINER_NAME=radius-postgres

##@ Database

.PHONY: db-init
db-init: db-dependency-docker-running ## Initialize a local PostgresSQL database for testing
	@echo "$(ARROW) Initializing local PostgresSQL database"
	@if [ "$$( docker container inspect -f '{{.State.Running}}' $(POSTGRES_CONTAINER_NAME) 2> /dev/null)" = "true" ]; then \
		echo "PostgresSQL container $(POSTGRES_CONTAINER_NAME) is already runnning"; \
	elif [ "$$( docker container inspect -f '{{.State.Running}}' $(POSTGRES_CONTAINER_NAME) 2> /dev/null)" = "false" ]; then \
		echo "PostgresSQL container $(POSTGRES_CONTAINER_NAME) is not running"; \
		echo "This might have been a crash"; \
		echo ""; \
		docker logs $(POSTGRES_CONTAINER_NAME); \
		echo ""; \
		echo "Restarting PostgresSQL container $(POSTGRES_CONTAINER_NAME)"  \
		docker start $(POSTGRES_CONTAINER_NAME) 1> /dev/null; \
	else \
		docker run \
			--detach \
			--name $(POSTGRES_CONTAINER_NAME) \
			--publish 5432:5432 \
			--env POSTGRES_USER=$(POSTGRES_USER) \
			--env POSTGRES_PASSWORD=$(POSTGRES_PASSWORD) \
			--volume $(PWD)/deploy/init-db/:/docker-entrypoint-initdb.d/ \
			$(POSTGRES_IMAGE) 1> /dev/null; \
		echo "Started PostgresSQL container $(POSTGRES_CONTAINER_NAME)"; \
	fi;
	@echo ""
	@echo "Use PostgreSQL in tests:"
	@echo ""
	@echo "export TEST_POSTGRES_URL=postgresql://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/ucp"
	@echo ""
	@echo "Makefile cheatsheet:"
	@echo "  - Stop the database $(ARROW) make db-stop"
	@echo "  - Reset the database $(ARROW) make db-reset"
	@echo "  - Logs $(ARROW) docker logs $(POSTGRES_CONTAINER_NAME)"
	@echo "  - Connect to the database server: make db-shell"
	@echo "  - Shell tip: Connect to UCP database $(ARROW) \\\\c ucp" 
	@echo "  - Shell tip: Connect to applications_rp database $(ARROW) \\\\c applications_rp" 
	@echo "  - Shell tip: List resources $(ARROW) select * from resources;" 

.PHONY: db-stop
db-stop: db-dependency-docker-running ## Stop the local PostgresSQL database
	@echo "$(ARROW) Stopping local PostgresSQL database..."
	@if [ "$$( docker container inspect -f '{{.State.Running}}' $(POSTGRES_CONTAINER_NAME) 2> /dev/null)" = "true" ]; then \
		docker stop $(POSTGRES_CONTAINER_NAME) 1> /dev/null; \
	else \
		echo "PostgresSQL container $(POSTGRES_CONTAINER_NAME) is not running"; \
	fi;

.PHONY: db-shell
db-shell: db-postgres-running ## Open a shell to the local PostgresSQL database
	@echo "$(ARROW) Connecting to local PostgresSQL database..."
	@DOCKER_CLI_HINTS=false docker exec \
		--interactive \
		--tty \
		$(POSTGRES_CONTAINER_NAME) \
		psql \
		"postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432"

.PHONY: db-reset
db-reset: db-postgres-running ## Reset the local PostgresSQL database
	@echo "$(ARROW) Resetting local PostgresSQL database"
	@echo ""
	@echo "Resetting ucp resources..."
	@DOCKER_CLI_HINTS=false docker exec \
		--interactive \
		--tty \
		$(POSTGRES_CONTAINER_NAME) \
		psql \
		"postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/ucp" \
		--command "DELETE FROM resources;"
	@echo ""
	@echo "Resetting applications_rp resources..."
	@DOCKER_CLI_HINTS=false docker exec \
		--interactive \
		--tty \
		$(POSTGRES_CONTAINER_NAME) \
		psql \
		"postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@localhost:5432/applications_rp" \
		--command "DELETE FROM resources;"

.PHONY: db-dependency-docker-running
db-dependency-docker-running:
	@if [ ! docker info > /dev/null 2>&1 ]; then \
		echo "Docker is not installed or not running. Please install docker and try again."; \
		exit 1; \
	fi;

.PHONY: db-postgres-running
db-postgres-running: db-dependency-docker-running
	@if [ "$$( docker container inspect -f '{{.State.Running}}' $(POSTGRES_CONTAINER_NAME) 2> /dev/null)" = "true" ]; then \
		exit 0; \
	else \
		echo "PostgresSQL container $(POSTGRES_CONTAINER_NAME) is not running"; \
		exit 1; \
	fi;