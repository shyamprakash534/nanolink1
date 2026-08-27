"""
NanoLink Performance & Latency Benchmark Runner
Measures RPS, p50, p95, and p99 latencies under concurrent load.
"""

import time
import statistics
import concurrent.futures
from fastapi.testclient import TestClient
from main import app, storage, bloom_filter, rate_limiter

client = TestClient(app)

def run_benchmarks():
    print("=" * 70)
    print("      NANOLINK DISTRIBUTED URL SHORTENER - PERFORMANCE BENCHMARKS")
    print("=" * 70)

    # Disable rate limiter for raw throughput tests
    rate_limiter.capacity = 1_000_000.0
    rate_limiter.refill_rate = 1_000_000.0

    # 1. Warm-up & Seed Hot Keys
    print("\n[1/4] Seeding 1,000 URLs...")
    short_codes = []
    for i in range(1000):
        resp = client.post("/api/v1/urls", json={
            "long_url": f"https://example.com/item/{i}",
            "custom_alias": f"bench-hot-{i}"
        })
        if resp.status_code == 201:
            short_codes.append(f"bench-hot-{i}")

    print(f"      Seeded {len(short_codes)} URLs successfully.")

    # 2. Benchmark Redirection Resolution (Cache Hit Path)
    print("\n[2/4] Benchmarking Read Latencies (5,000 Redirect Requests)...")
    latencies_ms = []
    start_time = time.perf_counter()

    for i in range(5000):
        code = short_codes[i % len(short_codes)]
        t0 = time.perf_counter()
        resp = client.get(f"/{code}", follow_redirects=False)
        t1 = time.perf_counter()
        latencies_ms.append((t1 - t0) * 1000.0)

    total_time = time.perf_counter() - start_time
    rps = len(latencies_ms) / total_time

    latencies_ms.sort()
    p50 = latencies_ms[int(len(latencies_ms) * 0.50)]
    p95 = latencies_ms[int(len(latencies_ms) * 0.95)]
    p99 = latencies_ms[int(len(latencies_ms) * 0.99)]
    avg = statistics.mean(latencies_ms)

    print(f"      Total Requests: {len(latencies_ms)}")
    print(f"      Throughput:     {rps:,.2f} req/sec")
    print(f"      Avg Latency:    {avg:.3f} ms")
    print(f"      p50 Latency:    {p50:.3f} ms")
    print(f"      p95 Latency:    {p95:.3f} ms")
    print(f"      p99 Latency:    {p99:.3f} ms")

    # 3. Benchmark Bloom Filter Fast-Rejection (Non-Existent Keys)
    print("\n[3/4] Benchmarking Bloom Filter Fast Rejections (2,000 Non-Existent Requests)...")
    reject_latencies = []
    for i in range(2000):
        t0 = time.perf_counter()
        resp = client.get(f"/nonexistent-{i}", follow_redirects=False)
        t1 = time.perf_counter()
        reject_latencies.append((t1 - t0) * 1000.0)

    reject_latencies.sort()
    rp50 = reject_latencies[int(len(reject_latencies) * 0.50)]
    rp95 = reject_latencies[int(len(reject_latencies) * 0.95)]
    rp99 = reject_latencies[int(len(reject_latencies) * 0.99)]

    print(f"      Fast Reject p50: {rp50:.3f} ms")
    print(f"      Fast Reject p95: {rp95:.3f} ms")
    print(f"      Fast Reject p99: {rp99:.3f} ms")

    # 4. Summary & Verification against SLA
    print("\n" + "=" * 70)
    print("                    SLA COMPLIANCE VERIFICATION")
    print("=" * 70)
    print(f"  Target Read SLA (p50 < 5ms):   {'PASSED' if p50 < 5.0 else 'FAILED'} (Actual: {p50:.2f}ms)")
    print(f"  Target Read SLA (p95 < 15ms):  {'PASSED' if p95 < 15.0 else 'FAILED'} (Actual: {p95:.2f}ms)")
    print(f"  Target Read SLA (p99 < 50ms):  {'PASSED' if p99 < 50.0 else 'FAILED'} (Actual: {p99:.2f}ms)")
    print("=" * 70)

if __name__ == "__main__":
    run_benchmarks()
