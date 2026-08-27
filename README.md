# NanoLink: Distributed High-Throughput URL Shortener

[![CI/CD Pipeline](https://github.com/nanolink/nanolink/actions/workflows/ci.yml/badge.svg)](https://github.com/nanolink/nanolink/actions)
[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go)](https://golang.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-16-336791?logo=postgresql)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7.2-DC382D?logo=redis)](https://redis.io)
[![ClickHouse](https://img.shields.io/badge/ClickHouse-OLAP-FFCC01?logo=clickhouse)](https://clickhouse.com)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker)](https://www.docker.com)
[![Terraform](https://img.shields.io/badge/Terraform-AWS-844FBA?logo=terraform)](https://www.terraform.io)

NanoLink is a production-grade distributed URL shortening and redirection infrastructure engineered for **sub-10ms redirect resolution**, **real-time analytics stream processing**, **atomic token-bucket rate limiting**, and **high availability at 10,000+ requests/sec**.

---

## 🏗️ Architecture & Component Interaction

```
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
1. **Sub-10ms Latency for Reads**: Redis caching with Bloom filter pre-checks eliminates database roundtrips on hot paths and non-existent keys (preventing cache penetration).
2. **Asynchronous Ingestion for Analytics**: Redis Streams decouple read traffic from analytics aggregation, writing asynchronously to ClickHouse / PostgreSQL in micro-batches.
3. **High Availability & Fault Isolation**: Stateless API services behind Application Load Balancers with circuit breakers and horizontal auto-scaling.
4. **Scalability via Sharding**: Range and hash-based database partitioning for horizontal scalability beyond 100M+ URLs.

---

## ⚡ Mathematical Capacity Planning (5-Year Horizon)

| Metric | Calculation | Sizing Estimate |
| :--- | :--- | :--- |
| **URL Creations (Write)** | 100M URLs/month | $\approx 38.5$ writes/sec (Peak: 500 writes/sec) |
| **URL Redirections (Read)** | 100:1 Read-to-Write ratio | $\approx 3,850$ reads/sec (Peak: 10,000+ reads/sec) |
| **Total URLs (5 Years)** | $100\text{M} \times 12 \times 5$ | **6 Billion URLs** |
| **Average Record Size** | ID (8B) + Code (10B) + URL (500B) + Metadata (182B) | $\approx 700\text{ bytes/record}$ |
| **Total Storage (5 Years)**| $6\text{ Billion} \times 700\text{ bytes}$ | **$\approx 4.2\text{ TB}$ (Distributed across shards)** |
| **Cache Memory (80/20 Rule)** | 20% hot URLs generating 80% daily traffic | **$\approx 3.63\text{ GB}$ RAM** (Redis Cluster) |

---

## 🔬 Algorithmic Deep Dives

### 1. Snowflake sequence + Base62 Encoding
- 64-bit distributed sequence ID converted into 6-7 Base62 characters `[0-9a-zA-Z]`.
- $62^6 \approx 56.8\text{ Billion}$ unique codes; $62^7 \approx 3.52\text{ Trillion}$ unique codes.
- Zero collision probability with deterministic derivation from cluster nodes.

### 2. Token Bucket Rate Limiting (Atomic Redis Lua Script)
Atomically evaluates client token quotas, allowing burst capacities while replenishing tokens continuously per second:
```lua
local key = KEYS[1]
local capacity = tonumber(ARGV[1])
local refill_rate = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested = tonumber(ARGV[4])

local data = redis.call("HMGET", key, "tokens", "last_updated")
local tokens = tonumber(data[1]) or capacity
local last_updated = tonumber(data[2]) or now

local delta = math.max(0, now - last_updated)
tokens = math.min(capacity, tokens + delta * refill_rate)

if tokens >= requested then
    tokens = tokens - requested
    redis.call("HMSET", key, "tokens", tokens, "last_updated", now)
    redis.call("EXPIRE", key, math.ceil(capacity / refill_rate))
    return {1, math.floor(tokens)}
else
    redis.call("HMSET", key, "tokens", tokens, "last_updated", now)
    return {0, math.floor(tokens)}
end
```

### 3. Space-Efficient Bloom Filter
- Probabilistic filter configured with 1% target false-positive rate ($\approx 9.6$ bits/element).
- 100M items occupy only $\approx 114\text{ MB}$ of memory.
- Intercepts requests for non-existent codes to return instant HTTP 404 without hitting cache or disk.

---

## 📡 API Specification & Endpoints

| Method | Endpoint | Description | Auth Required |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/urls` | Shorten long URL (custom alias & TTL support) | Optional (`X-API-Key`) |
| `GET` | `/:code` or `/api/v1/urls/:code` | Fast sub-10ms redirect resolution (HTTP 302) | None |
| `GET` | `/api/v1/urls/:code/stats` | Real-time analytics breakdown & metrics | None |
| `DELETE` | `/api/v1/urls/:code` | Deactivate/delete shortened URL mapping | Required (`X-API-Key`) |
| `GET` | `/api/v1/urls` | List authenticated user's shortened links | Required (`X-API-Key`) |
| `GET` | `/api/v1/urls/:code/qr` | Generate QR code image (PNG format) | None |
| `GET` | `/health` | Service health status & uptime | None |
| `GET` | `/metrics` | Prometheus RED metrics endpoint | None |

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
# HTTP/1.1 302 Found
# Location: https://github.com/features/actions
```

#### 3. View Real-Time Analytics
```bash
curl http://localhost:8080/api/v1/urls/gh-actions/stats
```

---

## 🚀 Quickstart & Execution

### Option A: Distributed Stack via Docker Compose
Run the full distributed infrastructure (Nginx, Go API, PostgreSQL, Redis, ClickHouse, Prometheus, Grafana):
```bash
docker compose up --build -d
```

### Option B: Standalone High-Speed Engine
Run the standalone engine for immediate local testing:
```bash
cd engine
python3 main.py
```

### Running Tests & Benchmarks
```bash
# Run unit & integration test suite
python3 engine/test_nanolink.py

# Run concurrency and latency benchmark
python3 engine/benchmark.py
```

---

## ☁️ Infrastructure as Code (Terraform AWS)
The `terraform/` directory provisions:
- **AWS VPC**: Multi-AZ public and private isolated subnets.
- **RDS PostgreSQL 16**: Multi-AZ deployment with automated failover and read replicas.
- **ElastiCache Redis 7.2**: Cluster mode enabled with transit encryption.
- **ECS Fargate**: Auto-scaling API cluster (2 to 10 tasks) with ALB health checking.

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

---

## 📊 Observability & Monitoring
- **Prometheus**: Scrapes `/metrics` for RED metrics (`nanolink_http_requests_total`, `nanolink_http_request_duration_seconds`, cache hit ratio).
- **Grafana**: Pre-provisioned dashboards at `http://localhost:3000`.
- **Alertmanager**: Configured alerts for p99 latency $> 50\text{ms}$ and 5xx error rates $> 1\%$.

---

## 🚢 Deploying to Your GitHub

To push this complete project to your personal GitHub repository:

```bash
# 1. Initialize git and check status
git init
git add .
git commit -m "feat: initial release of NanoLink distributed URL shortener"

# 2. Add your GitHub repository remote
git branch -M main
git remote add origin https://github.com/<YOUR_USERNAME>/nanolink.git

# 3. Push to GitHub
git push -u origin main
```
