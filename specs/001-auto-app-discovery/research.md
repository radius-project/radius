# Research: Automatic Application Discovery

**Feature**: 001-auto-app-discovery  
**Date**: February 2, 2026 (Updated)

## Research Tasks

### 1. LLM-Based Codebase Analysis

**Question**: What's the best approach for detecting infrastructure dependencies across all popular languages?

**Findings**:
- **LLM-based analysis**: Per Q-9, using LLMs enables support for all programming languages without language-specific parsers
- **Azure OpenAI**: Enterprise-ready API with content filtering, compliance, and structured output (JSON mode)
- **Context window considerations**: GPT-4 Turbo supports 128k tokens; chunking still needed for very large codebases
- **Determinism**: JSON mode with temperature=0 provides reproducible outputs

**Decision**: **LLM-based analysis** using Azure OpenAI GPT-4 with JSON mode.

**Rationale**: 
- Language-agnostic approach covers Python, JS/TS, Go, Java, C#, and any other languages
- Eliminates need to maintain separate parsers per language
- LLM can understand semantic intent (e.g., database connection patterns) not just syntax
- Structured output ensures parseable responses

**Hybrid enhancement**: Package manifest parsing (`package.json`, `requirements.txt`, etc.) provides fast initial dependency list; LLM confirms usage patterns for confidence scoring.

---

### 2. Infrastructure Dependency Catalog

**Question**: What infrastructure libraries should be detected for each language?

**Findings**:

| Technology | Python | JavaScript/TS | Go | Java | C# |
|------------|--------|---------------|----|----- |----|
| PostgreSQL | psycopg2, asyncpg, sqlalchemy | pg, node-postgres | lib/pq, pgx | postgresql-jdbc | Npgsql |
| MySQL | mysql-connector-python, pymysql | mysql2, mysql | go-sql-driver/mysql | mysql-connector-j | MySql.Data |
| MongoDB | pymongo | mongodb, mongoose | mongo-driver | mongodb-driver | MongoDB.Driver |
| Redis | redis-py, aioredis | ioredis, redis | go-redis | jedis, lettuce | StackExchange.Redis |
| RabbitMQ | pika, aio-pika | amqplib | amqp091-go | amqp-client | RabbitMQ.Client |
| Kafka | kafka-python, confluent-kafka | kafkajs | sarama | kafka-clients | Confluent.Kafka |
| Azure Blob | azure-storage-blob | @azure/storage-blob | azblob | azure-storage-blob | Azure.Storage.Blobs |
| AWS S3 | boto3 | @aws-sdk/client-s3 | aws-sdk-go-v2 | aws-java-sdk-s3 | AWSSDK.S3 |

**Decision**: Build a curated catalog (YAML/JSON) mapping library names to infrastructure types. LLM uses this catalog as reference but can also identify unlisted libraries.

---

### 3. Team Practices Extraction from IaC

**Question**: How should existing Terraform/Bicep/ARM be parsed to extract team practices?

**Findings**:
- **Terraform**: HCL parser (hashicorp/hcl) can extract resource blocks, variable defaults, local values.
- **Bicep**: Bicep CLI has `build --stdout` for JSON output; can parse parameter decorators and resource properties.
- **ARM**: Standard JSON parsing; look for `parameters`, `variables`, `resources` sections.

**Patterns to extract**:
- Naming conventions: regex on resource names → `{env}-{service}-{resource}` patterns
- Tags: common tag keys across resources
- Sizing: SKU/tier values in resource properties
- Security: encryption settings, network isolation flags

**Decision**: Implement IaC parser that extracts patterns into structured `TeamPractice` objects. Start with Terraform (most common), add Bicep/ARM as P2.

---

### 4. Recipe Source Integration

**Question**: How to integrate with Azure Verified Modules and internal repos?

**Findings**:
- **AVM**: Published to Bicep registry (`br:mcr.microsoft.com/bicep/avm/...`). Has metadata files describing inputs/outputs.
- **Terraform Registry**: Standard API at `registry.terraform.io/v1/modules/{namespace}/{name}/{provider}`.
- **Internal repos**: Need to scan for `main.tf` (Terraform) or `main.bicep` files with specific structure.

**Decision**: 
1. AVM: Use registry API to search for modules matching resource type
2. Internal Terraform: Git clone + scan for module structure
3. Internal Bicep: Similar to Terraform, look for `.bicep` with parameter definitions

---

### 5. MCP Protocol Implementation

**Question**: How to implement MCP server for AI agent integration?

**Findings**:
- MCP (Model Context Protocol) is Anthropic's standard for AI ↔ tool communication
- Uses JSON-RPC 2.0 over stdio or HTTP
- Tools are defined with name, description, input schema (JSON Schema)
- Go implementations exist: `github.com/anthropics/anthropic-sdk-go` (check for MCP support)

**Decision**: Implement MCP server in Go with:
- Stdio transport for VS Code extensions (FR-30)
- HTTP transport for remote agents (FR-30)
- Each skill becomes an MCP tool with JSON Schema input/output

---

### 6. Resource Type Strategy (OQ-1 Resolution)

**Question**: Pre-defined catalog or generated Resource Types?

**Analysis**:
| Approach | Pros | Cons |
|----------|------|------|
| Pre-defined catalog | Tested, validated, consistent quality | Limited to what's in catalog |
| Generated on-demand | Flexible, handles any dependency | Quality varies, may produce invalid schemas |

**Decision**: **Option A - Pre-defined catalog** for v1.
- Ship with catalog of Resource Types for common dependencies (PostgreSQL, MySQL, Redis, etc.)
- Map detected dependencies to catalog entries
- Fall back to generic "ExtResource" type for unknown dependencies with user prompt

**Rationale**: Aligns with Constitution Principle XII (Resource Type Schema Quality). Generated schemas risk invalid or incomplete outputs.

---

### 7. Service Detection Patterns

**Question**: How to detect deployable services/entrypoints?

**Findings**:

| Pattern | Language/Framework | Detection |
|---------|-------------------|-----------|
| Dockerfile | Any | File presence, extract EXPOSE, CMD |
| main.go | Go | Package main + func main() |
| main.py | Python | `if __name__ == "__main__"` |
| index.js/ts | Node.js | File presence in root or src/ |
| package.json scripts | Node.js | "start", "serve", "dev" scripts |
| @SpringBootApplication | Java | Annotation in .java files |
| Program.cs | C#/.NET | File presence + `Host.CreateDefaultBuilder` |

**Decision**: Implement entrypoint detector that checks these patterns in priority order. Dockerfile takes precedence (explicit containerization intent).

---

## Alternatives Considered

### Alternative: Tree-sitter AST Parsing

**Considered but deferred**: 
- Tree-sitter provides consistent AST across languages
- Would require maintaining grammar files and extraction logic per language
- LLM approach is simpler and covers more languages including less common ones
- May revisit for performance optimization if LLM latency becomes an issue

### Alternative: Language-Specific Parsers

**Rejected because**: 
- Requires separate implementation for each language (Go `go/ast`, Python `ast`, etc.)
- High maintenance burden as languages evolve
- LLM provides language-agnostic solution

### Alternative: Pure manifest parsing (no code analysis)

**Rejected because**:
- Misses dynamically loaded dependencies
- Cannot detect connection patterns or usage context
- Insufficient for confidence scoring

---

## Open Items for Phase 1

1. Define exact JSON schema for each skill's input/output (→ contracts/)
2. Design data model for DetectedDependency, TeamPractice, ResourceType (→ data-model.md)
3. Create quickstart guide showing CLI flow (→ quickstart.md)
