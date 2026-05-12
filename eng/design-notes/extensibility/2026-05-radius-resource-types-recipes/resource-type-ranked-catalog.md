# Application Components: Ranked by Developer Adoption

*Data collected: May 2026*

## Summary

This document identifies the most important application components that developers depend on when building software, ranked by usage data. The goal is to prioritize which Radius resource types to build first so the library covers what developers actually use. Application components include both **compute runtimes** (where app code runs, e.g. containers, serverless functions) and **backing services** (what apps connect to, e.g. databases, caches, queues, APIs).

We gathered 173 candidate technologies from five independent sources: cloud provider managed-service catalogs (AWS, Azure, GCP), Docker Hub official images sorted by pull count, the Stack Overflow Developer Survey 2025 (~49,000 respondents), infrastructure-as-code registries (Terraform Registry and Helm/ArtifactHub), and package registry trending data (npm/PyPI fastest-growing packages). The first four sources capture established infrastructure; the fifth was added specifically to catch fast-moving categories like AI/LLM tooling that hadn't yet appeared in traditional sources. The 2025 survey included a "Large Language Models" category, with 81.4% of respondents reporting use of OpenAI GPT models and 42.8% using Claude, confirming AI as a first-class application dependency.

To measure actual adoption rather than just awareness, we looked up dedicated client libraries for each technology across four developer ecosystems: npm (JavaScript/TypeScript), PyPI (Python), NuGet (.NET), and RubyGems (Ruby). Go and Maven were initially planned data sources but excluded due to aggressive API rate limiting on deps.dev. Client library downloads are one of the strongest available proxies for active developer integration, reflecting actual code-level dependency on a technology rather than just awareness.

Technologies are ranked primarily by ecosystem breadth (how many of the 4 ecosystems have an active client library), and secondarily by a weighted combination of download volumes, developer survey usage, Docker pull counts, and cloud provider availability. Within each ecosystem-breadth tier, manual adjustments were applied only when raw metrics were materially distorted by shared SDKs, transitive dependencies, or protocol overlap. See Appendix B for details on specific adjustments.

---

## Ranked Catalog

### Tier 1: Build First

Core infrastructure and AI components most developers expect, with the highest adoption signals across multiple ecosystems.

> **Note:** Containers (Docker/OCI: long-running services, background workers, jobs, cron jobs) would rank #1 by adoption but already exist as `Radius.Compute/containers` in this repository.

| Rank | Technology | Concept | Application Architecture | Cloud Equivalents |
|------|-----------|---------|--------------------------|-------------------|
| 1 | PostgreSQL | Relational database with SQL interface | Web App, Microservices, Data Pipeline, AI/ML | AWS RDS/Aurora PostgreSQL, Azure Database for PostgreSQL, GCP Cloud SQL/AlloyDB |
| 2 | Redis | In-memory cache, pub/sub, and data structures | Web App, Microservices, Real-time, AI/ML | AWS ElastiCache for Redis, Azure Managed Redis, GCP Memorystore for Redis |
| 3 | Object Storage | Blob/file storage accessed via HTTP API (uploads, artifacts, media, backups, ML datasets) | Web App, Data Pipeline, AI/ML | AWS S3, Azure Blob Storage, GCP Cloud Storage |
| 4 | LLM Inference API | Hosted large language model inference (apiKey + model + baseUrl) | Web App, Microservices, AI/ML | Azure OpenAI Service, AWS Bedrock, GCP Vertex AI, OpenAI API, Anthropic API |
| 5 | MongoDB | Document database (JSON/BSON) | Web App, Microservices | AWS DocumentDB, Azure CosmosDB (Mongo API), GCP Firestore |
| 6 | MySQL | Relational database with SQL interface | Web App, Enterprise, AI/ML | AWS RDS/Aurora MySQL, Azure Database for MySQL, GCP Cloud SQL for MySQL |
| 7 | Kafka | Distributed event streaming platform | Microservices, Data Pipeline | AWS MSK, Azure Event Hubs (Kafka-compatible), GCP Managed Service for Apache Kafka |
| 8 | Elasticsearch / OpenSearch | Full-text search and analytics engine | Web App, Microservices, Data Pipeline | AWS OpenSearch, Azure AI Search, GCP Elastic Cloud |
| 9 | RabbitMQ | Multi-protocol message broker (AMQP, MQTT, STOMP) | Microservices, Enterprise | AWS Amazon MQ, Azure Service Bus (AMQP) |
| 10 | SQL Server | Relational database (T-SQL) | Enterprise | AWS RDS for SQL Server, Azure SQL Database, GCP Cloud SQL for SQL Server |

### Tier 2: Build Next

Well-established technologies and AI tooling with broad adoption. Build these once Tier 1 is solid.

| Rank | Technology | Concept | Application Architecture | Cloud Equivalents |
|------|-----------|---------|--------------------------|-------------------|
| 11 | Serverless Functions | Event-triggered stateless code execution (handler + trigger + runtime) | Web App, Microservices, Data Pipeline | AWS Lambda, Azure Functions, GCP Cloud Functions |
| 12 | Message Queue | Generic async messaging and job queue | Microservices, Enterprise | AWS SQS, Azure Queue Storage/Service Bus Queues, GCP Cloud Tasks |
| 13 | Mosquitto (MQTT) | Lightweight message broker for IoT | Real-time, IoT | AWS IoT Core, Azure IoT Hub |
| 14 | pgvector | Vector database (PostgreSQL extension) | AI/ML, Web App | AWS RDS PostgreSQL (pgvector), Azure Database for PostgreSQL (pgvector), GCP Cloud SQL (pgvector) |
| 15 | NATS | Lightweight messaging and streaming | Microservices, Real-time | Self-hosted (comparable: Azure Service Bus, AWS SNS) |
| 16 | Oracle Database | Enterprise relational database | Enterprise | AWS RDS for Oracle, Oracle AI Database@Azure, GCP Oracle Database@Google Cloud |
| 17 | Neo4j | Graph database | Web App, AI/ML | Self-hosted, Neo4j AuraDB, Azure CosmosDB (Gremlin API) |
| 18 | Vault | Secrets management and encryption (included because apps directly establish runtime connections to secrets providers, unlike org-level identity or observability platforms) | Microservices, Enterprise | AWS Secrets Manager, Azure Key Vault, GCP Secret Manager |
| 19 | Cassandra | Wide-column distributed database | Microservices, Data Pipeline | AWS Keyspaces, Azure Managed Instance for Cassandra |
| 20 | InfluxDB | Time-series database | Real-time, IoT, Data Pipeline | Self-hosted, InfluxDB Cloud, Azure Data Explorer (comparable) |

### Tier 3: Build Later

Emerging, niche, or platform-specific technologies. Build as demand materializes or for specific customer needs. Ordering incorporates emerging-growth signals in addition to ecosystem breadth.

| Rank | Technology | Concept | Application Architecture | Cloud Equivalents |
|------|-----------|---------|--------------------------|-------------------|
| 21 | Ollama | Local LLM serving and inference | AI/ML | Self-hosted (runs locally or on any VM) |
| 22 | Pub/Sub | Cloud-native publish/subscribe messaging (cloud-provider abstraction vs portable protocols like Kafka/RabbitMQ) | Microservices, Data Pipeline | AWS SNS/EventBridge, Azure Service Bus Topics, GCP Pub/Sub |
| 23 | ClickHouse | Column-oriented analytics database | Data Pipeline, Real-time | Self-hosted, ClickHouse Cloud, Azure Data Explorer (comparable) |
| 24 | Keycloak | Identity and access management | Microservices, Enterprise | Self-hosted, AWS Cognito, Microsoft Entra External ID, GCP Identity Platform |
| 25 | Spark | Distributed data processing engine | Data Pipeline | AWS EMR, Azure HDInsight/Synapse Spark, GCP Dataproc |
| 26 | MLflow | ML experiment tracking and model registry | AI/ML | Self-hosted, Databricks MLflow, Azure ML (built-in MLflow) |
| 27 | Memcached | Distributed in-memory cache (simple key-value) | Web App, Microservices, Real-time | AWS ElastiCache, Azure Cache (Memcached-compatible), GCP Memorystore |

---

## Priority Tiers

| Tier | Ranks | Criteria | What it means |
|------|-------|----------|---------------|
| **Tier 1: Build First** | 1–10 | Highest adoption + stable connection contracts suitable for cross-cloud abstraction | Core infrastructure every developer expects. Table-stakes for any resource type library. |
| **Tier 2: Build Next** | 11–20 | Strong adoption but higher abstraction complexity or narrower use cases | Well-established technologies and emerging AI tooling. Build once Tier 1 is solid. |
| **Tier 3: Build Later** | 21–27 | Client libraries in 3 or fewer ecosystems, or niche use cases | Emerging, niche, or platform-specific. Build as demand materializes or for specific customer needs. |

### Future Investigation

27 technologies were evaluated and deferred. Most were excluded because a higher-ranked entry already covers the same wire protocol or use case (e.g., MariaDB = MySQL, Valkey = Redis, Solr = Elasticsearch). Others were excluded as SDKs/libraries rather than provisionable infrastructure (Sentry, LangChain). Temporal was deferred for 0/3 cloud availability; revisit when managed offerings emerge.

### Shared Infrastructure Services

Some infrastructure is provisioned once at the platform or org level but apps still connect to it at runtime. These may warrant a **shared resource type**: no Recipe, just metadata holding a connection endpoint at the environment level that apps can bind to.

- **Identity/Auth**: apps verify tokens and initiate OAuth flows against a shared provider (Cognito, Entra, Keycloak)
- **Observability**: apps push traces and metrics to shared collectors (OpenTelemetry, Jaeger, Prometheus)
- **Logging**: apps send logs to shared backends (ELK, Loki, CloudWatch)
- **Email/SMTP**: apps connect to a shared email service to send messages (SES, SendGrid)
- **Feature Flags**: apps query a shared flag service at runtime (LaunchDarkly, Unleash)
- **API Gateway**: apps register routes with a shared gateway (Kong, Azure APIM)
- **Load Balancers**: platform-level traffic routing, apps don't connect directly

---

## Application Architecture Patterns

Each technology in the catalog is tagged with the application architectures it commonly supports. These patterns describe *how* developers use a technology, not just *what* it is:

| Pattern | What it means | Example |
|---------|---------------|---------|
| **Web App** | Traditional request/response web applications (monolith or MVC) | PostgreSQL backing a Django/Rails/Express app |
| **Microservices** | Distributed services communicating via APIs or messages | Kafka for event-driven communication between services |
| **Data Pipeline** | Batch or streaming ETL, analytics, and data processing | Spark processing data from S3 into a data warehouse |
| **Real-time** | Low-latency event processing, WebSockets, live updates | Redis pub/sub powering live notifications |
| **Enterprise** | Line-of-business applications with compliance and integration needs | SQL Server behind a .NET enterprise application |
| **AI/ML** | LLM inference, vector search, experiment tracking, model management | OpenAI API powering a RAG chatbot with pgvector |
| **IoT** | Device telemetry ingestion and command/control | MQTT broker collecting sensor data from edge devices |

---

## Appendix A: Detailed Adoption Metrics

Ranks in this appendix match the main catalog. Technologies are grouped by ecosystem breadth (how many of npm, PyPI, NuGet, RubyGems have a dedicated client library).

### Tier 1: Backing services with highest adoption

> **Note:** Containers (`Radius.Compute/containers`) already exists in this repository and is excluded from ranking. By adoption it would be #1 (Docker 71.1%, K8s 28.5% survey; dockerode 3.9M npm/wk; docker 226.8M + kubernetes 173.0M PyPI/mo).

| Rank | Technology | npm/wk | PyPI/mo | NuGet total | Gems total | Survey % | Docker pulls | Cloud (x/3) |
|------|-----------|--------|---------|-------------|------------|----------|--------------|-------------|
| 1 | postgresql | 36.8M | 416.0M | 802.5M | 439.7M | 55.6% | 14.5B | 3/3 |
| 2 | redis | 26.2M | 228.4M | 994.9M | 553.3M | 28.0% | 14.4B | 3/3 |
| 3 | object-storage ⚠️ | 25.8M | 2.9B | 589.0M | 1.0B | — | — | 3/3 |
| 4 | llm-inference-api | 41.0M | 420.2M | 48.2M | 42.7M | 81.4%* | — | 3/3 |
| 5 | mongodb | 16.0M | 108.9M | 374.7M | 147.5M | 24.0% | 6.5B | 3/3 |
| 6 | mysql | 10.7M | 155.6M | 367.7M | 232.0M | 40.5% | 5.4B | 3/3 |
| 7 | kafka | 2.7M | 87.4M | 228.0M | 102.1M | — | 332.4M | 3/3 |
| 8 | elasticsearch/opensearch | 3.2M | 120.5M | 251.5M | 256.2M | 16.7% | 1.1B | 2/3 |
| 9 | rabbitmq | 2.3M | 67.9M | 455.5M | 73.8M | — | 4.5B | 2/3 |
| 10 | sqlserver | 4.9M | 88.7M | 2.5B | 23.4M | 30.1% | — | 3/3 |

### Tier 2: Established technologies and compute

| Rank | Technology | npm/wk | PyPI/mo | NuGet total | Gems total | Survey % | Docker pulls | Cloud (x/3) |
|------|-----------|--------|---------|-------------|------------|----------|--------------|-------------|
| 11 | serverless-functions | 9.7M (combined) | 9.2M (functions-framework) | — | — | — | — | 3/3 |
| 12 | message-queue ⚠️ | 6.4M | 51.7M | 209.1M | 314.2M | — | — | 3/3 |
| 13 | mosquitto | 2.0M | 6.0M | 28.5M | 5.1M | — | 668.5M | 0/3 |
| 14 | pgvector | 307K | 21.1M | 4.7M | 19.8M | — | 96.2M | 3/3 |
| 15 | nats | 653K | 3.4M | 15.9M | 8.8M | — | 387.1M | 0/3 |
| 16 | oracle-database | 515K | 22.4M | 110.5M | 4.5M | 10.6% | — | 2/3 |
| 17 | neo4j | 453K | 10.2M | 4.2M | 491K | 2.6% | 325.5M | 0/3 |
| 18 | vault | 391K | 28.4M | 36.3M | 57.3M | — | 552.5M | 0/3 |
| 19 | cassandra | 196K | 8.1M | 14.8M | 11.3M | 2.9% | 322.8M | 2/3 |
| 20 | influxdb | 163K | 8.4M | 6.7M | 23.1M | 3.7% | 2.0B | 0/3 |

### Tier 3: Client libraries in 3-4 ecosystems

| Rank | Technology | npm/wk | PyPI/mo | NuGet total | Gems total | Survey % | Docker pulls | Cloud (x/3) |
|------|-----------|--------|---------|-------------|------------|----------|--------------|-------------|
| 22 | pub-sub | 3.6M | 83.1M | 48.7M | — | — | — | 3/3 |
| 23 | clickhouse | 1.5M | 34.8M | 4.9M | — | 2.4% | 6.6M | 0/3 |

### Tier 3 (continued): Fewer ecosystems (emerging)

| Rank | Technology | Primary signal | Survey % | Docker pulls | Cloud (x/3) |
|------|-----------|---------------|----------|--------------|-------------|
| 21 | ollama | npm 506K/wk, PyPI 11.9M/mo | — | 132.2M | 0/3 |
| 24 | keycloak | npm 879K/wk, PyPI 6.4M/mo | — | 33.5M | 0/3 |
| 25 | spark | PyPI 52.1M/mo | — | 17.4M | 3/3 |
| 26 | mlflow | PyPI 36.3M/mo | — | 743K | 0/3 |
| 27 | memcached | npm 3.5M/wk, PyPI 5.8M/mo | — | 14.4B | 2/3 |

---

## Appendix B: Caveats

- **`serverless-functions`** (rank 11) combines AWS Lambda SDK (7.2M npm/wk), GCP Functions Framework (1.5M npm/wk, 9.2M PyPI/mo), and Azure Functions (1.0M npm/wk). Each cloud has a different SDK and trigger model; the combined number reflects total serverless adoption.
- ⚠️ **`object-storage`** PyPI numbers are inflated by `boto3` (2.9B monthly downloads). `boto3` is the AWS SDK containing ALL services, so it's impossible to attribute downloads to S3 specifically. Their npm/NuGet/Gems numbers use service-specific packages and are reliable.
- ⚠️ **`message-queue`** aggregates generic queue patterns (Bull/BullMQ for Redis-backed queues, Celery, Sidekiq, MassTransit). These are abstract patterns, not a single technology, but the demand signal is real.
- **LLM Inference API** (rank 4) merges OpenAI, Anthropic, and Google Generative AI, all sharing the same connection contract (apiKey + model + baseUrl). Combined npm: 41M/wk, combined PyPI: 420M/mo. Survey percentages marked with * are from the 2025 survey's "Large Language Models" section (% of developers using LLM models).
- **Vector databases**: pgvector (#14) is the recommended entry point (PostgreSQL extension, 3/3 clouds, same connection contract). Dedicated vector DBs (Qdrant, Pinecone, Weaviate, ChromaDB, Milvus) are deferred since all have proprietary protocols, 0/3 cloud-native support, and lower adoption than pgvector's ecosystem.
- **NuGet and RubyGems** report lifetime total downloads (not periodic), so older technologies have naturally higher numbers. Use npm (weekly) and PyPI (monthly) for recency signal.
- **Azure Database for MariaDB** was retired in September 2025; only AWS RDS remains as a managed option.
- **GCP IoT Core** was shut down in August 2023; removed from Mosquitto's cloud equivalents.

---

## Appendix C: Methodology

This catalog was produced using the Discover → Measure → Rank methodology documented in detail in [`resource-type-discovery-methodology.md`](resource-type-discovery-methodology.md). In brief:

1. **Discovery**: 173 technologies gathered from 5 independent sources: cloud provider catalogs (AWS/Azure/GCP), Docker Hub official images, Stack Overflow Developer Survey 2025, IaC registries (Terraform + Helm), and package registry trending data (npm/PyPI fastest-growing packages, added to catch emerging categories like AI/LLM).
2. **Normalization and alias consolidation**: Vendor-specific names merged (e.g., "Amazon RDS for PostgreSQL" + "Azure Database for PostgreSQL" → `postgresql`).
3. **Client library measurement**: For each technology, found dedicated client libraries across 4 ecosystems (npm, PyPI, NuGet, RubyGems) and recorded download volumes. Go and Maven were excluded due to aggressive rate limiting on the deps.dev API.
4. **Ranking**: Primary: number of ecosystems with client libraries (4/4 > 3/4 > fewer). Secondary: weighted combination of download volumes, survey usage, Docker pulls, and cloud provider availability, with manual adjustments for technologies whose raw numbers are misleading (see Appendix B).
