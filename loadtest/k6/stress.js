/**
 * Stress test — ramps VUs to find the service's breaking point.
 *
 * Starts at 10 VUs and doubles every 2 minutes until either:
 *   - the error rate exceeds 5 %, or
 *   - p99 latency exceeds 2 s.
 *
 * After the peak, ramps down to verify the service recovers cleanly.
 *
 *   Stage          Duration   VUs
 *   ───────────    ────────   ────
 *   Warm up        1 m        10
 *   Ramp           2 m        10 → 50
 *   Ramp           2 m        50 → 100
 *   Ramp           2 m        100 → 200
 *   Peak hold      2 m        200
 *   Recovery       1 m        200 → 10
 *   Verify         1 m        10
 *
 * Usage:
 *   k6 run -e API_KEY=sp_<key> loadtest/k6/stress.js
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY  = __ENV.API_KEY  || '';

const authHeaders = { 'X-API-Key': API_KEY, 'Content-Type': 'application/json' };

const errorRate   = new Rate('errors');
const p99Duration = new Trend('req_duration_p99', true);

export const options = {
  stages: [
    { duration: '1m',  target: 10  },   // warm up
    { duration: '2m',  target: 50  },   // ramp 1
    { duration: '2m',  target: 100 },   // ramp 2
    { duration: '2m',  target: 200 },   // ramp 3
    { duration: '2m',  target: 200 },   // peak hold
    { duration: '1m',  target: 10  },   // recovery
    { duration: '1m',  target: 10  },   // verify recovery
  ],
  thresholds: {
    // Test is considered to have found the breaking point if these breach —
    // that is expected; the goal is to observe where it happens, not to pass.
    http_req_failed:   ['rate<0.10'],   // alert if >10% — catastrophic failure
    http_req_duration: ['p(99)<3000'],  // alert if p99 > 3 s
  },
};

export function setup() {
  // Seed tags for read operations.
  const tags = [];
  for (let i = 0; i < 5; i++) {
    const res = http.post(
      `${BASE_URL}/tags`,
      JSON.stringify({ name: `stress-tag-${i}-${Date.now()}` }),
      { headers: authHeaders },
    );
    if (res.status === 201) tags.push(JSON.parse(res.body));
  }
  console.log(`setup: seeded ${tags.length} tags`);
  return { tags };
}

export default function ({ tags }) {
  const start = Date.now();

  // Pure read mix — stress tests DB and query path.
  const rand = Math.random();
  let res;

  if (rand < 0.50) {
    res = http.get(`${BASE_URL}/media?limit=20`, {
      headers: authHeaders,
      tags: { type: 'read' },
    });
  } else if (rand < 0.80) {
    const tag = tags[Math.floor(Math.random() * tags.length)];
    const url = tag
      ? `${BASE_URL}/media?tag_id=${tag.id}&limit=20`
      : `${BASE_URL}/media?limit=20`;
    res = http.get(url, { headers: authHeaders, tags: { type: 'read' } });
  } else {
    res = http.get(`${BASE_URL}/tags?limit=20`, {
      headers: authHeaders,
      tags: { type: 'read' },
    });
  }

  const duration = Date.now() - start;
  p99Duration.add(duration);

  const ok = check(res, {
    'status 2xx': r => r.status >= 200 && r.status < 300,
    'no server error': r => r.status < 500,
  });
  errorRate.add(!ok);

  sleep(0.05); // minimal think time — maximise concurrency pressure
}
