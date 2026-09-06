# NanoLink: Distributed High-Throughput URL Shortener

[![CI/CD Pipeline](https://github.com/shyamprakash534/nanolink1/actions/workflows/ci.yml/badge.svg)](https://github.com/shyamprakash534/nanolink1/actions)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7.2-DC382D?logo=redis)](https://redis.io)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-OLAP-FFCC01?logo=clickhouse)](https://clickhouse.com)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://www.docker.com)
[![Terraform](https://img.shields.io/badge/Terraform-AWS-844FBA?logo=terraform)](https://www.terraform.io)

NanoLink is a distributed URL shortening and redirection system built with Go and designed around fast redirects, asynchronous analytics, Redis-based caching and rate limiting, and horizontally scalable infrastructure.

---

## 🚀 Live Deployment

- **Primary:** https://nanolink1.onrender.com
- **Secondary:** https://nanolink1-1ba.onrender.com
- **Health:** `/health`
- **Metrics:** `/metrics`

> **Deployment note:** The current Render deployment runs the core URL-shortener service with ClickHouse analytics storage intentionally disabled because no production ClickHouse instance is attached. ClickHouse remains part of the local/distributed architecture and analytics implementation.

---

## 🏗️ Architecture & Component Interaction

```text
                        +----------------------------+
                        |   Clients / Web / Mobile   |
                        +--------------+-------------+
                                       |
                                       v
                        +----------------------------+
                        | Nginx Reverse Proxy / ALB  |
                        +--------------+-------------+
                                       |
                     +-----------------+-----------------+
                     |                                   |
                     v                                   v
        +-------------------------+         +-------------------------+
        |   NanoLink API Node 1   |         |   NanoLink API Node N   |
        |      (Go 1.22 / Gin)    |         |      (Go 1.22 / Gin)    |
        +----+---------------+----+         +----+---------------+----+
             |               |                   |               |
             v               v                   v               v
     +---------------+ +---------------+ +---------------+ +---------------+
     |  Bloom Filter | |  Redis Cache  | | Redis Streams | | Token Bucket  |
     | (Fast Reject) | | (URL Hot Path)| | (Click Stream)| | (Rate Limiter)|
     +---------------+ +-------+-------+ +-------+-------+ +---------------+
                               |                 |
                               v                 v
                     +-----------------+ +-------------------+
                     | PostgreSQL 16   | | Analytics Worker  |
                     | (Hash Sharded)  | | (Batch Consumer)  |
                     +-----------------+ +---------+---------+
                                                   |
                                                   v
                                         +-------------------+
                                         | ClickHouse OLAP   |
                                         | (AggregatingTree) |
                                         +-------------------+
```

### Core Architectural Tenets
1. **Fast redirect path**: Redis caching and Bloom-filter checks reduce unnecessary database access for hot and missing keys.
2. **Asynchronous analytics**: Redis Streams decouple redirect traffic from analytics processing and ClickHouse writes.
3. **Horizontal scalability**: Stateless Go API nodes can run behind a load balancer and scale independently.
4. **Fault isolation**: Cache, analytics, and persistence workloads are separated so the redirect path remains lightweight.

---

## ⚡ Capacity Planning Example

The repository includes a capacity-planning model for a large-scale deployment. These figures are **design estimates**, not production measurements.

| Metric | Calculation | Sizing Estimate |
| :--- | :--- | :--- |
| **URL Creations (Write)** | 100M URLs/month | ≈ 38.5 writes/sec (Peak: 500 writes/sec) |
| **URL Redirections (Read)** | 100:1 Read-to-Write ratio | ≈ 3,850 reads/sec (Peak: 10,000+ reads/sec) |
| **Total URLs (5 Years)** | 100M × 12 × 5 | **6 Billion URLs** |
| **Average Record Size** | ID + code + URL + metadata | **≈ 700 bytes/record** |
| **Total Storage (5 Years)** | 6B × 700 bytes | **≈ 4.2 TB** |
| **Cache Memory (80/20 model)** | 20% hot URLs / 80% traffic | **≈ 3.63 GB RAM** |

---

## 🔬 Algorithmic Deep Dives

### 1. Snowflake Sequence + Base62 Encoding
- 64-bit distributed sequence IDs are converted into compact Base62 codes.
- `62^6 ≈ 56.8B` possible six-character combinations.
- `62^7 ≈ 3.52T` possible seven-character combinations.
- The implementation is designed to avoid collisions through deterministic distributed ID generation.

### 2. Token Bucket Rate Limiting
NanoLink uses an atomic Redis Lua operation to evaluate token availability, support bursts, replenish tokens continuously, and update bucket state without race-prone read/modify/write sequences.

### 3. Space-Efficient Bloom Filter
- Probabilistic membership checking is used to reject likely-missing URL codes early.
- The configured target false-positive rate is approximately 1%.
- This reduces unnecessary cache and database lookups for invalid codes.

---

## 📡 API Specification & Endpoints

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/urls` | Shorten long URL (custom alias & TTL support) | Optional (`X-API-Key`) |
| `GET` | `/:code` or `/api/v1/urls/:code` | Resolve shortened URL | None |
| `GET` | `/api/v1/urls/:code/stats` | Analytics breakdown & metrics | None |
| `DELETE` | `/api/v1/urls/:code` | Deactivate/delete shortened URL mapping | Required (`X-API-Key`) |
| `GET` | `/api/v1/urls` | List authenticated user's shortened links | Required (`X-API-Key`) |
| `GET` | `/api/v1/urls/:code/qr` | Generate QR code image | None |
| `GET` | `/health` | Service health status | None |
| `GET` | `/metrics` | Prometheus metrics endpoint | None |

### Example cURL Commands

#### 1. Shorten a URL
```bash
curl -X POST http://localhost:8080/api/v1/urls \
  -H "Content-Type: application/json" \
  -H "X-API-Key: my-api-token" \
  -d '{"long_url": "https://github.com/features/actions", "custom_alias": "gh-actions", "ttl_seconds": 86400}'
```

#### 2. Resolve Redirection
```bash
curl -i http://localhost:8080/gh-actions
```

#### 3. View Analytics
```bash
curl http://localhost:8080/api/v1/urls/gh-actions/stats
```

---

## 🚀 Quickstart & Execution

### Option A: Distributed Stack via Docker Compose

Run the distributed local stack:

```bash
docker compose up --build -d
```

### Option B: Standalone Engine

Run the standalone Python/FastAPI engine for local testing:

```bash
cd engine
python3 main.py
```

### Running Tests & Benchmarks

```bash
# Go tests
 go test -v -race ./internal/...

# Standalone engine tests
python3 engine/test_nanolink.py

# Concurrency and latency benchmark
python3 engine/benchmark.py
```

> **Note:** Benchmark numbers should be treated as environment-dependent measurements. Run the benchmark on the target deployment environment before making production performance claims.

---

## ☁️ Infrastructure as Code (Terraform AWS)

The `terraform/` directory contains infrastructure configuration for:
- **AWS VPC** with public/private networking.
- **RDS PostgreSQL 16** for persistent URL metadata.
- **ElastiCache Redis 7.2** for caching and distributed coordination.
- **ECS Fargate** for containerized API workloads.

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

Review and configure AWS credentials, variables, networking, and security settings before applying infrastructure.

---

## 📊 Observability & Monitoring

NanoLink exposes Prometheus-compatible metrics and includes configuration for:
- **Prometheus** — metrics collection.
- **Grafana** — dashboards and visualization.
- **Alertmanager** — alert routing.

The local Grafana service is exposed on `http://localhost:3000` when running the corresponding Docker Compose stack.

---

## 🔄 CI/CD

GitHub Actions validates the project on pushes and pull requests to the main/master branches.

The current pipeline:
1. Sets up Go 1.22 and Python 3.11.
2. Installs the Python test dependencies.
3. Runs Go tests with the race detector.
4. Runs the standalone NanoLink integration test suite.
5. Builds the Docker image with Docker Buildx.

---

## 📁 Repository Structure

```text
nanolink1/
├── cmd/                 # Go application entry points
├── internal/            # Core Go services and business logic
├── engine/              # Standalone Python/FastAPI engine and benchmarks
├── migrations/          # Database migrations
├── terraform/           # AWS infrastructure as code
├── docker-compose.yml   # Local distributed stack
├── Dockerfile           # Production container image
└── .github/workflows/   # CI/CD automation
```

---

## 👨‍💻 Author

**Shyam Prakash**

GitHub: https://github.com/shyamprakash534

LinkedIn: https://www.linkedin.com/in/shyam-prakash-269a74208/

---

## 📄 License

See the repository for the current project license and source distribution terms.
