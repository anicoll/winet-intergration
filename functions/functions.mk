# --- Deploy config (override via environment or command line) ---
RESOURCE_GROUP         ?= zappy-dev-rg
SQL_SERVER_NAME        ?= zappy-dev-sqlserver
API_FUNCTION_APP       ?= zappy-dev-api
INGESTION_FUNCTION_APP ?= zappy-dev-ingestion
DB_USER ?= user
DB_PASSWORD ?= password
DATABASE_URL ?= sqlserver://$(DB_USER):$(DB_PASSWORD)@$(SQL_SERVER_NAME).database.windows.net?database=zappy&encrypt=true
export DATABASE_URL

# DATABASE_URL must be set: sqlserver://user:pass@host?database=db&encrypt=true
.PHONY: migrate
migrate:
	$(eval MY_IP := $(shell curl -sf https://api.ipify.org))
	@echo "Adding firewall rule for $(MY_IP)..."
	az sql server firewall-rule create \
	  --resource-group $(RESOURCE_GROUP) \
	  --server $(SQL_SERVER_NAME) \
	  --name migrate-local \
	  --start-ip-address $(MY_IP) \
	  --end-ip-address $(MY_IP)
	go run ./cmd/migrate; \
	EXIT_CODE=$$?; \
	echo "Removing firewall rule..."; \
	az sql server firewall-rule delete \
	  --resource-group $(RESOURCE_GROUP) \
	  --server $(SQL_SERVER_NAME) \
	  --name migrate-local; \
	exit $$EXIT_CODE

.PHONY: build-function-api
build-function-api:
	GOOS=linux GOARCH=amd64 go build -o functions/api/api ./functions/api

.PHONY: build-function-ingestion
build-function-ingestion:
	GOOS=linux GOARCH=amd64 go build -o functions/ingestion/ingestion ./functions/ingestion

.PHONY: build-functions
build-functions: build-function-api build-function-ingestion

.PHONY: deploy-function-api
deploy-function-api: build-function-api
	mkdir -p .build
	rm -f .build/api.zip
	cd functions/api && python3 -m zipfile -c ../../.build/api.zip api host.json
	az functionapp deployment source config-zip \
	  --resource-group $(RESOURCE_GROUP) \
	  --name $(API_FUNCTION_APP) \
	  --src .build/api.zip

.PHONY: deploy-function-ingestion
deploy-function-ingestion: build-function-ingestion
	mkdir -p .build
	rm -f .build/ingestion.zip
	cd functions/ingestion && python3 -m zipfile -c ../../.build/ingestion.zip ingestion host.json
	az functionapp deployment source config-zip \
	  --resource-group $(RESOURCE_GROUP) \
	  --name $(INGESTION_FUNCTION_APP) \
	  --src .build/ingestion.zip

.PHONY: deploy-functions
deploy-functions: deploy-function-api deploy-function-ingestion
