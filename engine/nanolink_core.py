"""
NanoLink Core Engine
Implements Base62 encoding, Snowflake ID generation, Bloom Filter,
Token Bucket Rate Limiting, Stream Analytics, and SQLite/In-memory caching.
"""

import time
import math
import hashlib
import threading
import sqlite3
import re
from typing import Optional, Dict, Any, List, Tuple
from datetime import datetime, timezone, timedelta

# ---------------------------------------------------------
# 1. Base62 & Snowflake ID Generation
# ---------------------------------------------------------
BASE62_ALPHABET = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
BASE62_BASE = len(BASE62_ALPHABET)

def base62_encode(num: int) -> str:
    if num == 0:
        return BASE62_ALPHABET[0]
    chars = []
    while num > 0:
        num, rem = divmod(num, BASE62_BASE)
        chars.append(BASE62_ALPHABET[rem])
    chars.reverse()
    return "".join(chars)

def base62_decode(s: str) -> int:
    num = 0
    for char in s:
        idx = BASE62_ALPHABET.find(char)
        if idx == -1:
            raise ValueError(f"Invalid Base62 character: {char}")
        num = num * BASE62_BASE + idx
    return num

class SnowflakeGenerator:
    """Distributed 64-bit unique ID generator."""
    def __init__(self, node_id: int = 1, epoch: int = 1704067200000): # 2024-01-01
        self.node_id = node_id & 0x3FF # 10 bits
        self.epoch = epoch
        self.sequence = 0
        self.last_timestamp = -1
        self.lock = threading.Lock()

    def generate(self) -> int:
        with self.lock:
            now = int(time.time() * 1000)
            if now < self.last_timestamp:
                now = self.last_timestamp

            if now == self.last_timestamp:
                self.sequence = (self.sequence + 1) & 0xFFF # 12 bits
                if self.sequence == 0:
                    while now <= self.last_timestamp:
                        now = int(time.time() * 1000)
            else:
                self.sequence = 0

            self.last_timestamp = now
            id_val = ((now - self.epoch) << 22) | (self.node_id << 12) | self.sequence
            return id_val

# ---------------------------------------------------------
# 2. Bloom Filter (Space-Efficient Fast Reject)
# ---------------------------------------------------------
class BloomFilter:
    """
    Space-efficient probabilistic Bloom filter for cache penetration prevention.
    """
    def __init__(self, expected_elements: int = 1_000_000, fp_rate: float = 0.01):
        self.expected_elements = expected_elements
        self.fp_rate = fp_rate
        # Optimal m and k
        self.m = int(math.ceil(-expected_elements * math.log(fp_rate) / (math.log(2) ** 2)))
        self.k = int(round((self.m / expected_elements) * math.log(2)))
        self.bit_array = bytearray((self.m + 7) // 8)
        self.lock = threading.Lock()

    def _hashes(self, item: str) -> List[int]:
        # Kirsch-Mitzenmacher optimization using 2 hash functions
        h1 = int(hashlib.md5(item.encode('utf-8')).hexdigest(), 16)
        h2 = int(hashlib.sha256(item.encode('utf-8')).hexdigest(), 16)
        return [(h1 + i * h2) % self.m for i in range(self.k)]

    def add(self, item: str):
        indices = self._hashes(item)
        with self.lock:
            for idx in indices:
                byte_idx = idx // 8
                bit_idx = idx % 8
                self.bit_array[byte_idx] |= (1 << bit_idx)

    def contains(self, item: str) -> bool:
        indices = self._hashes(item)
        with self.lock:
            for idx in indices:
                byte_idx = idx // 8
                bit_idx = idx % 8
                if not (self.bit_array[byte_idx] & (1 << bit_idx)):
                    return False
        return True

# ---------------------------------------------------------
# 3. Token Bucket Rate Limiter
# ---------------------------------------------------------
class TokenBucketRateLimiter:
    """
    Atomic Token Bucket rate limiter replicating Redis Lua script behavior.
    """
    def __init__(self, capacity: float = 100.0, refill_rate: float = 10.0):
        self.capacity = float(capacity)
        self.refill_rate = float(refill_rate)
        self.buckets: Dict[str, Dict[str, float]] = {}
        self.lock = threading.Lock()

    def allow(self, key: str, requested: int = 1) -> Tuple[bool, int]:
        now = time.time()
        with self.lock:
            if key not in self.buckets:
                self.buckets[key] = {"tokens": self.capacity, "last_updated": now}

            bucket = self.buckets[key]
            elapsed = max(0.0, now - bucket["last_updated"])
            bucket["tokens"] = min(self.capacity, bucket["tokens"] + elapsed * self.refill_rate)
            bucket["last_updated"] = now

            if bucket["tokens"] >= requested:
                bucket["tokens"] -= requested
                return True, int(bucket["tokens"])
            else:
                return False, int(bucket["tokens"])

# ---------------------------------------------------------
# 4. Storage & In-Memory Redis Cache Simulation
# ---------------------------------------------------------
class StorageManager:
    def __init__(self, db_path: str = ":memory:"):
        self.db_path = db_path
        self.cache: Dict[str, Tuple[str, Optional[float]]] = {} # code -> (long_url, expire_ts)
        self.streams: List[Dict[str, Any]] = []
        self.stream_lock = threading.Lock()
        self.cache_lock = threading.Lock()
        self._init_db()

    def _get_connection(self):
        conn = sqlite3.connect(self.db_path, check_same_thread=False)
        conn.row_factory = sqlite3.Row
        return conn

    def _init_db(self):
        conn = self._get_connection()
        cur = conn.cursor()
        cur.execute("""
            CREATE TABLE IF NOT EXISTS users (
                id TEXT PRIMARY KEY,
                email TEXT UNIQUE NOT NULL,
                api_key_hash TEXT UNIQUE NOT NULL,
                tier TEXT DEFAULT 'free',
                rate_limit_per_hr INTEGER DEFAULT 100,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
            )
        """)
        cur.execute("""
            CREATE TABLE IF NOT EXISTS urls (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                short_code TEXT UNIQUE NOT NULL,
                long_url TEXT NOT NULL,
                custom_alias INTEGER DEFAULT 0,
                user_id TEXT,
                created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                expires_at TIMESTAMP,
                click_count INTEGER DEFAULT 0,
                is_active INTEGER DEFAULT 1
            )
        """)
        cur.execute("""
            CREATE TABLE IF NOT EXISTS click_events (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                short_code TEXT NOT NULL,
                clicked_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
                ip_hash TEXT NOT NULL,
                country_code TEXT DEFAULT 'XX',
                user_agent TEXT,
                device_type TEXT,
                browser TEXT,
                os TEXT,
                referrer TEXT
            )
        """)
        cur.execute("CREATE INDEX IF NOT EXISTS idx_urls_code ON urls(short_code)")
        cur.execute("CREATE INDEX IF NOT EXISTS idx_clicks_code ON click_events(short_code)")
        conn.commit()
        conn.close()

    def set_cache(self, short_code: str, long_url: str, ttl_seconds: Optional[int] = None):
        expire_ts = (time.time() + ttl_seconds) if ttl_seconds else None
        with self.cache_lock:
            self.cache[short_code] = (long_url, expire_ts)

    def get_cache(self, short_code: str) -> Optional[str]:
        with self.cache_lock:
            if short_code in self.cache:
                long_url, expire_ts = self.cache[short_code]
                if expire_ts is not None and time.time() > expire_ts:
                    del self.cache[short_code]
                    return None
                return long_url
        return None

    def invalidate_cache(self, short_code: str):
        with self.cache_lock:
            self.cache.pop(short_code, None)

    def publish_click_stream(self, event: Dict[str, Any]):
        with self.stream_lock:
            self.streams.append(event)

    def pop_stream_batch(self, batch_size: int = 1000) -> List[Dict[str, Any]]:
        with self.stream_lock:
            batch = self.streams[:batch_size]
            self.streams = self.streams[batch_size:]
            return batch

# ---------------------------------------------------------
# 5. Background Stream Analytics Worker
# ---------------------------------------------------------
class StreamAnalyticsWorker:
    def __init__(self, storage: StorageManager, flush_interval: float = 0.5):
        self.storage = storage
        self.flush_interval = flush_interval
        self.running = False
        self.thread: Optional[threading.Thread] = None

    def start(self):
        self.running = True
        self.thread = threading.Thread(target=self._run, daemon=True)
        self.thread.start()

    def _run(self):
        while self.running:
            time.sleep(self.flush_interval)
            batch = self.storage.pop_stream_batch()
            if batch:
                conn = self.storage._get_connection()
                cur = conn.cursor()
                for ev in batch:
                    cur.execute("""
                        INSERT INTO click_events 
                        (short_code, clicked_at, ip_hash, country_code, user_agent, device_type, browser, os, referrer)
                        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, (
                        ev["short_code"],
                        ev["clicked_at"],
                        ev["ip_hash"],
                        ev["country_code"],
                        ev["user_agent"],
                        ev["device_type"],
                        ev["browser"],
                        ev["os"],
                        ev["referrer"]
                    ))
                conn.commit()
                conn.close()

    def stop(self):
        self.running = False
        if self.thread:
            self.thread.join(timeout=2.0)
