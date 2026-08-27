"""
NanoLink FastAPI Server Implementation
"""

import io
import time
import hashlib
import re
import uuid
from typing import Optional, Dict, Any, List
from datetime import datetime, timezone, timedelta

from fastapi import FastAPI, Request, Response, HTTPException, status, Header, Depends, Query
from fastapi.responses import RedirectResponse, JSONResponse, StreamingResponse
from pydantic import BaseModel, HttpUrl
from PIL import Image, ImageDraw

from nanolink_core import (
    base62_encode,
    SnowflakeGenerator,
    BloomFilter,
    TokenBucketRateLimiter,
    StorageManager,
    StreamAnalyticsWorker,
)

# ---------------------------------------------------------
# App Initialization & Global State
# ---------------------------------------------------------
app = FastAPI(title="NanoLink Distributed URL Shortener", version="1.0.0")

storage = StorageManager("/tmp/nanolink_data.db")
snowflake = SnowflakeGenerator(node_id=1)
bloom_filter = BloomFilter(expected_elements=1_000_000, fp_rate=0.01)
rate_limiter = TokenBucketRateLimiter(capacity=100.0, refill_rate=10.0)
analytics_worker = StreamAnalyticsWorker(storage, flush_interval=0.2)
analytics_worker.start()

start_time = time.time()
BASE_URL = "http://localhost:8080"
ALIAS_REGEX = re.compile(r"^[a-zA-Z0-9_-]{3,30}$")

# In-memory Prometheus metric counters
metrics_counters = {
    "requests_total": 0,
    "redirects_total": 0,
    "shortens_total": 0,
    "bloom_rejections_total": 0,
    "cache_hits_total": 0,
    "cache_misses_total": 0,
}

# Pre-populate bloom filter from existing DB rows
conn = storage._get_connection()
cur = conn.cursor()
for row in cur.execute("SELECT short_code FROM urls WHERE is_active = 1"):
    bloom_filter.add(row["short_code"])
conn.close()

# ---------------------------------------------------------
# Request & Response Models
# ---------------------------------------------------------
class ShortenRequest(BaseModel):
    long_url: str
    custom_alias: Optional[str] = None
    ttl_seconds: Optional[int] = None

class ShortenResponse(BaseModel):
    short_code: str
    short_url: str
    long_url: str
    created_at: str
    expires_at: Optional[str] = None

# ---------------------------------------------------------
# Middleware: Rate Limiting & Metrics
# ---------------------------------------------------------
@app.middleware("http")
async def rate_limiting_and_metrics_middleware(request: Request, call_next):
    metrics_counters["requests_total"] += 1
    
    # Exclude health and metrics endpoints from strict rate limiting
    if request.url.path in ["/health", "/metrics"]:
        return await call_next(request)

    client_ip = request.client.host if request.client else "127.0.0.1"
    api_key = request.headers.get("X-API-Key")
    limiter_key = f"key:{api_key}" if api_key else f"ip:{client_ip}"

    allowed, remaining = rate_limiter.allow(limiter_key, 1)
    if not allowed:
        return JSONResponse(
            status_code=status.HTTP_429_TOO_MANY_REQUESTS,
            content={
                "error": "rate limit exceeded",
                "message": "Too many requests. Please retry after rate limit bucket refills.",
            },
            headers={"Retry-After": "1", "X-RateLimit-Remaining": "0"}
        )

    response = await call_next(request)
    response.headers["X-RateLimit-Remaining"] = str(remaining)
    return response

# ---------------------------------------------------------
# Helper Functions
# ---------------------------------------------------------
def generate_simple_qr(data: str) -> bytes:
    """Generate a high-contrast QR-pattern placeholder image using Pillow."""
    img = Image.new('RGB', (256, 256), color=(255, 255, 255))
    draw = ImageDraw.Draw(img)
    # Draw simple visual pattern representing QR
    draw.rectangle([10, 10, 60, 60], outline=(0, 0, 0), width=6)
    draw.rectangle([25, 25, 45, 45], fill=(0, 0, 0))
    draw.rectangle([196, 10, 246, 60], outline=(0, 0, 0), width=6)
    draw.rectangle([211, 25, 231, 45], fill=(0, 0, 0))
    draw.rectangle([10, 196, 60, 246], outline=(0, 0, 0), width=6)
    draw.rectangle([25, 211, 45, 231], fill=(0, 0, 0))
    
    # Hash data to generate deterministically varied internal grid
    h = hashlib.sha256(data.encode()).hexdigest()
    for i in range(16):
        for j in range(16):
            if int(h[(i * 16 + j) % len(h)], 16) % 2 == 1:
                x = 70 + i * 7
                y = 70 + j * 7
                draw.rectangle([x, y, x + 5, y + 5], fill=(0, 0, 0))

    buf = io.BytesIO()
    img.save(buf, format='PNG')
    return buf.getvalue()

# ---------------------------------------------------------
# API Endpoints
# ---------------------------------------------------------
@app.get("/health")
def health():
    return {
        "status": "healthy",
        "service": "nanolink-api",
        "uptime_seconds": round(time.time() - start_time, 2),
        "timestamp": datetime.now(timezone.utc).isoformat()
    }

@app.get("/metrics")
def metrics():
    lines = [
        "# HELP nanolink_requests_total Total HTTP requests processed",
        "# TYPE nanolink_requests_total counter",
        f"nanolink_requests_total {metrics_counters['requests_total']}",
        "# HELP nanolink_redirects_total Total URL redirects resolved",
        "# TYPE nanolink_redirects_total counter",
        f"nanolink_redirects_total {metrics_counters['redirects_total']}",
        "# HELP nanolink_shortens_total Total URLs shortened",
        "# TYPE nanolink_shortens_total counter",
        f"nanolink_shortens_total {metrics_counters['shortens_total']}",
        "# HELP nanolink_bloom_rejections_total Fast 404 rejections by Bloom filter",
        "# TYPE nanolink_bloom_rejections_total counter",
        f"nanolink_bloom_rejections_total {metrics_counters['bloom_rejections_total']}",
        "# HELP nanolink_cache_hits_total In-memory cache hits",
        "# TYPE nanolink_cache_hits_total counter",
        f"nanolink_cache_hits_total {metrics_counters['cache_hits_total']}",
    ]
    return Response(content="\n".join(lines) + "\n", media_type="text/plain")

@app.post("/api/v1/urls", status_code=status.HTTP_201_CREATED, response_model=ShortenResponse)
def shorten_url(payload: ShortenRequest, request: Request, x_api_key: Optional[str] = Header(None)):
    long_url = payload.long_url.strip()
    if not (long_url.startswith("http://") or long_url.startswith("https://")):
        raise HTTPException(status_code=400, detail="Invalid URL format. Must start with http:// or https://")

    is_custom = False
    if payload.custom_alias:
        alias = payload.custom_alias.strip()
        if not ALIAS_REGEX.match(alias):
            raise HTTPException(status_code=400, detail="Custom alias must be 3-30 alphanumeric characters, hyphens or underscores")
        
        # Check uniqueness in DB
        conn = storage._get_connection()
        cur = conn.cursor()
        cur.execute("SELECT id FROM urls WHERE short_code = ? AND is_active = 1", (alias,))
        row = cur.fetchone()
        conn.close()
        if row:
            raise HTTPException(status_code=400, detail="Custom alias already taken")
        
        short_code = alias
        is_custom = True
    else:
        sf_id = snowflake.generate()
        short_code = base62_encode(sf_id)
        if len(short_code) < 6:
            short_code = short_code.zfill(6)

    now_utc = datetime.now(timezone.utc)
    expires_at_str = None
    expires_at_dt = None
    if payload.ttl_seconds and payload.ttl_seconds > 0:
        expires_at_dt = now_utc + timedelta(seconds=payload.ttl_seconds)
        expires_at_str = expires_at_dt.isoformat()

    user_id = str(uuid.uuid5(uuid.NAMESPACE_DNS, x_api_key)) if x_api_key else None

    # Persist to SQLite
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("""
        INSERT INTO urls (short_code, long_url, custom_alias, user_id, created_at, expires_at, click_count, is_active)
        VALUES (?, ?, ?, ?, ?, ?, 0, 1)
    """, (
        short_code,
        long_url,
        1 if is_custom else 0,
        user_id,
        now_utc.isoformat(),
        expires_at_str
    ))
    conn.commit()
    conn.close()

    # Pre-warm Cache and Bloom Filter
    bloom_filter.add(short_code)
    storage.set_cache(short_code, long_url, payload.ttl_seconds)
    metrics_counters["shortens_total"] += 1

    return ShortenResponse(
        short_code=short_code,
        short_url=f"{BASE_URL}/{short_code}",
        long_url=long_url,
        created_at=now_utc.isoformat(),
        expires_at=expires_at_str
    )

@app.get("/{code}")
@app.get("/api/v1/urls/{code}")
def redirect_to_url(code: str, request: Request):
    # Step 1: Bloom filter fast reject (Sub-10ms path)
    if not bloom_filter.contains(code):
        metrics_counters["bloom_rejections_total"] += 1
        raise HTTPException(status_code=404, detail="Short code not found")

    # Step 2: Cache lookup
    cached_url = storage.get_cache(code)
    if cached_url:
        metrics_counters["cache_hits_total"] += 1
        long_url = cached_url
    else:
        # Step 3: DB lookup
        metrics_counters["cache_misses_total"] += 1
        conn = storage._get_connection()
        cur = conn.cursor()
        cur.execute("SELECT long_url, expires_at, is_active FROM urls WHERE short_code = ?", (code,))
        row = cur.fetchone()
        conn.close()

        if not row or not row["is_active"]:
            raise HTTPException(status_code=404, detail="Short code not found")

        if row["expires_at"]:
            exp = datetime.fromisoformat(row["expires_at"])
            if datetime.now(timezone.utc) > exp:
                raise HTTPException(status_code=404, detail="Short code expired")

        long_url = row["long_url"]
        storage.set_cache(code, long_url, ttl_seconds=86400)

    # Step 4: Asynchronous click telemetry ingestion
    client_ip = request.client.host if request.client else "127.0.0.1"
    user_agent = request.headers.get("User-Agent", "Unknown")
    referrer = request.headers.get("Referer", "direct")
    ip_hash = hashlib.sha256(client_ip.encode()).hexdigest()

    ua_lower = user_agent.lower()
    device_type = "desktop"
    if "mobile" in ua_lower or "android" in ua_lower or "iphone" in ua_lower:
        device_type = "mobile"
    elif "tablet" in ua_lower or "ipad" in ua_lower:
        device_type = "tablet"

    browser = "other"
    if "chrome" in ua_lower:
        browser = "chrome"
    elif "safari" in ua_lower:
        browser = "safari"
    elif "firefox" in ua_lower:
        browser = "firefox"

    os = "other"
    if "windows" in ua_lower:
        os = "windows"
    elif "mac" in ua_lower:
        os = "macos"
    elif "linux" in ua_lower:
        os = "linux"
    elif "android" in ua_lower:
        os = "android"
    elif "ios" in ua_lower or "iphone" in ua_lower:
        os = "ios"

    telemetry_event = {
        "short_code": code,
        "clicked_at": datetime.now(timezone.utc).isoformat(),
        "ip_hash": ip_hash,
        "country_code": "US",
        "user_agent": user_agent,
        "device_type": device_type,
        "browser": browser,
        "os": os,
        "referrer": referrer
    }
    storage.publish_click_stream(telemetry_event)

    # Update total click count
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("UPDATE urls SET click_count = click_count + 1 WHERE short_code = ?", (code,))
    conn.commit()
    conn.close()

    metrics_counters["redirects_total"] += 1
    return RedirectResponse(url=long_url, status_code=status.HTTP_302_FOUND)

@app.get("/api/v1/urls/{code}/stats")
def get_url_stats(code: str):
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("SELECT short_code, long_url, created_at, click_count FROM urls WHERE short_code = ? AND is_active = 1", (code,))
    url_row = cur.fetchone()
    if not url_row:
        conn.close()
        raise HTTPException(status_code=404, detail="Short code not found")

    # Aggregations
    cur.execute("SELECT country_code, count(*) as count FROM click_events WHERE short_code = ? GROUP BY country_code", (code,))
    countries = {row["country_code"]: row["count"] for row in cur.fetchall()}

    cur.execute("SELECT device_type, count(*) as count FROM click_events WHERE short_code = ? GROUP BY device_type", (code,))
    devices = {row["device_type"]: row["count"] for row in cur.fetchall()}

    cur.execute("SELECT referrer, count(*) as count FROM click_events WHERE short_code = ? GROUP BY referrer", (code,))
    referrers = {row["referrer"]: row["count"] for row in cur.fetchall()}

    cur.execute("SELECT strftime('%Y-%m-%d %H:00', clicked_at) as hour, count(*) as count FROM click_events WHERE short_code = ? GROUP BY hour ORDER BY hour DESC LIMIT 24", (code,))
    hourly = [{"hour": row["hour"], "count": row["count"]} for row in cur.fetchall()]

    conn.close()

    return {
        "short_code": url_row["short_code"],
        "long_url": url_row["long_url"],
        "total_clicks": url_row["click_count"],
        "created_at": url_row["created_at"],
        "clicks_by_country": countries,
        "clicks_by_device": devices,
        "clicks_by_referrer": referrers,
        "hourly_clicks": hourly
    }

@app.delete("/api/v1/urls/{code}")
def delete_url(code: str, x_api_key: Optional[str] = Header(None)):
    if not x_api_key:
        raise HTTPException(status_code=401, detail="Missing X-API-Key header")

    user_id = str(uuid.uuid5(uuid.NAMESPACE_DNS, x_api_key))
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("UPDATE urls SET is_active = 0 WHERE short_code = ? AND user_id = ?", (code, user_id))
    rows_affected = cur.rowcount
    conn.commit()
    conn.close()

    storage.invalidate_cache(code)

    if rows_affected == 0:
        raise HTTPException(status_code=404, detail="Short URL not found or unauthorized")

    return {"message": "URL successfully deactivated", "short_code": code}

@app.get("/api/v1/urls")
def list_urls(x_api_key: Optional[str] = Header(None), limit: int = Query(20, ge=1, le=100), offset: int = Query(0, ge=0)):
    if not x_api_key:
        raise HTTPException(status_code=401, detail="Missing X-API-Key header")

    user_id = str(uuid.uuid5(uuid.NAMESPACE_DNS, x_api_key))
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("""
        SELECT short_code, long_url, custom_alias, created_at, expires_at, click_count 
        FROM urls 
        WHERE user_id = ? AND is_active = 1
        ORDER BY created_at DESC
        LIMIT ? OFFSET ?
    """, (user_id, limit, offset))
    rows = cur.fetchall()
    conn.close()

    urls = [
        {
            "short_code": r["short_code"],
            "short_url": f"{BASE_URL}/{r['short_code']}",
            "long_url": r["long_url"],
            "custom_alias": bool(r["custom_alias"]),
            "created_at": r["created_at"],
            "expires_at": r["expires_at"],
            "click_count": r["click_count"]
        }
        for r in rows
    ]
    return {"urls": urls, "limit": limit, "offset": offset}

@app.get("/api/v1/urls/{code}/qr")
def get_qr_code(code: str):
    conn = storage._get_connection()
    cur = conn.cursor()
    cur.execute("SELECT long_url FROM urls WHERE short_code = ? AND is_active = 1", (code,))
    row = cur.fetchone()
    conn.close()

    if not row:
        raise HTTPException(status_code=404, detail="Short code not found")

    png_bytes = generate_simple_qr(f"{BASE_URL}/{code}")
    return Response(content=png_bytes, media_type="image/png")

if __name__ == "__main__":
    import uvicorn
    uvicorn.run("main:app", host="0.0.0.0", port=8080, log_level="info")
