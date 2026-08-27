"""
Comprehensive Automated Test Suite for NanoLink
Validates all architectural requirements, algorithms, and endpoints.
"""

import time
import unittest
from fastapi.testclient import TestClient
from main import app, storage, bloom_filter, rate_limiter
from nanolink_core import base62_encode, base62_decode, SnowflakeGenerator, BloomFilter, TokenBucketRateLimiter

client = TestClient(app)

class TestNanoLinkAlgorithms(unittest.TestCase):
    def test_base62_roundtrip(self):
        numbers = [0, 1, 42, 61, 62, 1000, 123456789, 9876543210123]
        for num in numbers:
            encoded = base62_encode(num)
            decoded = base62_decode(encoded)
            self.assertEqual(decoded, num, f"Base62 mismatch for {num}: got {decoded}")

    def test_snowflake_uniqueness(self):
        gen = SnowflakeGenerator(node_id=2)
        generated = set()
        for _ in range(10_000):
            sf_id = gen.generate()
            self.assertNotIn(sf_id, generated, "Collision in Snowflake ID generator")
            generated.add(sf_id)
        self.assertEqual(len(generated), 10_000)

    def test_bloom_filter_accuracy(self):
        bf = BloomFilter(expected_elements=1000, fp_rate=0.01)
        test_items = [f"short-code-{i}" for i in range(500)]
        for item in test_items:
            bf.add(item)

        # True positives
        for item in test_items:
            self.assertTrue(bf.contains(item), f"Bloom filter failed to find inserted element {item}")

        # True negatives (testing non-existent)
        false_positives = 0
        non_existent = [f"non-existent-code-{i}" for i in range(1000)]
        for item in non_existent:
            if bf.contains(item):
                false_positives += 1
        fp_rate = false_positives / len(non_existent)
        self.assertLessEqual(fp_rate, 0.05, f"False positive rate too high: {fp_rate}")

    def test_token_bucket_rate_limiting(self):
        tb = TokenBucketRateLimiter(capacity=5.0, refill_rate=5.0)
        key = "test-user-limit"

        # Consume all 5 tokens
        for _ in range(5):
            allowed, _ = tb.allow(key, 1)
            self.assertTrue(allowed, "Should allow within capacity")

        # 6th request should be rejected
        allowed, _ = tb.allow(key, 1)
        self.assertFalse(allowed, "Should reject when capacity exhausted")

        # Wait for refill (0.4s should add 2 tokens at 5/s)
        time.sleep(0.45)
        allowed, _ = tb.allow(key, 1)
        self.assertTrue(allowed, "Should allow after refill period")

class TestNanoLinkAPIEndpoints(unittest.TestCase):
    def setUp(self):
        # Reset rate limiter capacity for tests
        rate_limiter.buckets.clear()

    def test_01_health_check(self):
        resp = client.get("/health")
        self.assertEqual(resp.status_code, 200)
        data = resp.json()
        self.assertEqual(data["status"], "healthy")
        self.assertEqual(data["service"], "nanolink-api")

    def test_02_url_shortening_auto_base62(self):
        payload = {"long_url": "https://en.wikipedia.org/wiki/Distributed_computing"}
        resp = client.post("/api/v1/urls", json=payload, headers={"X-API-Key": "test-key-alpha"})
        self.assertEqual(resp.status_code, 201)
        data = resp.json()
        self.assertIn("short_code", data)
        self.assertEqual(data["long_url"], payload["long_url"])
        self.assertTrue(data["short_url"].endswith(data["short_code"]))

        # Verify redirect resolution
        code = data["short_code"]
        redir_resp = client.get(f"/{code}", follow_redirects=False)
        self.assertEqual(redir_resp.status_code, 302)
        self.assertEqual(redir_resp.headers["location"], payload["long_url"])

    def test_03_url_shortening_custom_alias(self):
        alias = "my-custom-launch-2026"
        payload = {
            "long_url": "https://github.com/features/actions",
            "custom_alias": alias
        }
        resp = client.post("/api/v1/urls", json=payload, headers={"X-API-Key": "test-key-alpha"})
        self.assertEqual(resp.status_code, 201)
        data = resp.json()
        self.assertEqual(data["short_code"], alias)

        # Test collision rejection on duplicate custom alias
        dup_resp = client.post("/api/v1/urls", json=payload, headers={"X-API-Key": "test-key-alpha"})
        self.assertEqual(dup_resp.status_code, 400)
        self.assertIn("already taken", dup_resp.json()["detail"])

    def test_04_bloom_filter_fast_rejection(self):
        # Non-existent short code
        resp = client.get("/definitely-does-not-exist-99999", follow_redirects=False)
        self.assertEqual(resp.status_code, 404)

    def test_05_ttl_expiration(self):
        alias = "temp-short-link"
        payload = {
            "long_url": "https://news.ycombinator.com",
            "custom_alias": alias,
            "ttl_seconds": 1 # 1 second TTL
        }
        resp = client.post("/api/v1/urls", json=payload)
        self.assertEqual(resp.status_code, 201)

        # Immediate redirect works
        redir_resp = client.get(f"/{alias}", follow_redirects=False)
        self.assertEqual(redir_resp.status_code, 302)

        # Wait for expiration
        time.sleep(1.2)
        storage.invalidate_cache(alias) # force DB re-check

        expired_resp = client.get(f"/{alias}", follow_redirects=False)
        self.assertEqual(expired_resp.status_code, 404)

    def test_06_analytics_telemetry_aggregation(self):
        alias = "analytics-link"
        payload = {"long_url": "https://clickhouse.com", "custom_alias": alias}
        client.post("/api/v1/urls", json=payload)

        # Simulate 5 clicks with various headers
        headers_list = [
            {"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 16_0 like Mac OS X)", "Referer": "https://twitter.com"},
            {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/110.0.0.0", "Referer": "https://google.com"},
            {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Safari/605.1.15", "Referer": "direct"},
            {"User-Agent": "Mozilla/5.0 (Linux; Android 13; SM-S908B)", "Referer": "https://reddit.com"},
            {"User-Agent": "Mozilla/5.0 (X11; Linux x86_64; rv:109.0) Gecko/20100101 Firefox/115.0", "Referer": "https://google.com"},
        ]

        for h in headers_list:
            client.get(f"/{alias}", headers=h, follow_redirects=False)

        # Wait for stream batch worker flush
        time.sleep(0.6)

        # Fetch stats
        stats_resp = client.get(f"/api/v1/urls/{alias}/stats")
        self.assertEqual(stats_resp.status_code, 200)
        stats = stats_resp.json()
        self.assertEqual(stats["total_clicks"], 5)
        self.assertIn("mobile", stats["clicks_by_device"])
        self.assertIn("desktop", stats["clicks_by_device"])
        self.assertIn("https://google.com", stats["clicks_by_referrer"])

    def test_07_qr_code_generation(self):
        alias = "qr-test-link"
        client.post("/api/v1/urls", json={"long_url": "https://www.postgresql.org", "custom_alias": alias})

        qr_resp = client.get(f"/api/v1/urls/{alias}/qr")
        self.assertEqual(qr_resp.status_code, 200)
        self.assertEqual(qr_resp.headers["content-type"], "image/png")
        self.assertGreater(len(qr_resp.content), 100)

    def test_08_authenticated_url_listing_and_deletion(self):
        api_key = "user-secret-api-key-99"
        alias = "deletable-link"
        client.post("/api/v1/urls", json={"long_url": "https://redis.io", "custom_alias": alias}, headers={"X-API-Key": api_key})

        # List URLs
        list_resp = client.get("/api/v1/urls", headers={"X-API-Key": api_key})
        self.assertEqual(list_resp.status_code, 200)
        urls = list_resp.json()["urls"]
        self.assertTrue(any(u["short_code"] == alias for u in urls))

        # Delete URL
        del_resp = client.delete(f"/api/v1/urls/{alias}", headers={"X-API-Key": api_key})
        self.assertEqual(del_resp.status_code, 200)

        # Verify deactivated
        check_resp = client.get(f"/{alias}", follow_redirects=False)
        self.assertEqual(check_resp.status_code, 404)

    def test_09_prometheus_metrics_scraping(self):
        resp = client.get("/metrics")
        self.assertEqual(resp.status_code, 200)
        content = resp.text
        self.assertIn("nanolink_requests_total", content)
        self.assertIn("nanolink_redirects_total", content)

if __name__ == "__main__":
    unittest.main(verbosity=2)
