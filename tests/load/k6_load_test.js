import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

const errorRate = new Rate('errors');

export const options = {
  stages: [
    { duration: '10s', target: 50 },    // Warm-up to 50 VUs
    { duration: '30s', target: 500 },   // Ramp-up to 500 VUs
    { duration: '1m', target: 2000 },   // Peak stress load (10,000+ RPS)
    { duration: '20s', target: 0 },     // Cool down
  ],
  thresholds: {
    'http_req_duration{status:200}': ['p(95)<15', 'p(99)<50'], // sub-15ms p95, sub-50ms p99
    'http_req_duration{status:302}': ['p(95)<10', 'p(99)<25'], // sub-10ms redirect resolution
    'errors': ['rate<0.01'],                                   // error rate < 1%
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  // 1. Create short URL (1% of traffic)
  if (Math.random() < 0.01) {
    const payload = JSON.stringify({
      long_url: `https://example.com/item/${Math.floor(Math.random() * 100000)}`,
    });
    const headers = { 'Content-Type': 'application/json', 'X-API-Key': 'test-api-key' };
    const res = http.post(`${BASE_URL}/api/v1/urls`, payload, { headers });
    check(res, {
      'URL creation status is 201': (r) => r.status === 201,
    }) || errorRate.add(1);
  } else {
    // 2. Resolve short URL redirect (99% of traffic)
    const code = 'hot-promo-link';
    const res = http.get(`${BASE_URL}/${code}`, { redirects: 0 });
    check(res, {
      'Redirect status is 302 or 404': (r) => r.status === 302 || r.status === 404,
    }) || errorRate.add(1);
  }

  sleep(0.01);
}
