/**
 * Load test — realistic traffic mix, sustained for 5 minutes.
 *
 * Models a read-heavy API (80 % reads, 20 % writes) with gradual ramp-up and
 * ramp-down. Thresholds enforce SLOs; the test fails CI if they are breached.
 *
 *   Stage         Duration   VUs
 *   ──────────    ────────   ───
 *   Ramp up       1 m        0 → 50
 *   Sustained     5 m        50
 *   Ramp down     30 s       50 → 0
 *
 * Usage:
 *   k6 run -e API_KEY=sp_<key> loadtest/k6/load.js
 *
 * Override target VUs:
 *   k6 run -e API_KEY=sp_<key> -e TARGET_VUS=100 loadtest/k6/load.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const BASE_URL  = __ENV.BASE_URL    || 'http://localhost:8080';
const API_KEY   = __ENV.API_KEY     || '';
const TARGET    = parseInt(__ENV.TARGET_VUS || '50', 10);

const authHeaders = { 'X-API-Key': API_KEY, 'Content-Type': 'application/json' };

// Custom metrics
const tagCreated  = new Counter('tags_created_total');
const mediaListed = new Counter('media_listed_total');
const writeErrors = new Rate('write_error_rate');
const tagLatency  = new Trend('tag_list_duration', true);   // true = high-res
const mediaLatency = new Trend('media_list_duration', true);

export const options = {
  stages: [
    { duration: '1m',  target: TARGET },   // ramp up
    { duration: '5m',  target: TARGET },   // sustained load
    { duration: '30s', target: 0 },        // ramp down
  ],
  thresholds: {
    // Overall error rate
    http_req_failed:                  ['rate<0.01'],   // <1% 5xx/network errors

    // Latency SLOs by endpoint type
    'http_req_duration{type:read}':   ['p(95)<300', 'p(99)<500'],
    'http_req_duration{type:write}':  ['p(95)<600', 'p(99)<1000'],

    // Custom metrics
    tag_list_duration:                ['p(95)<200'],
    media_list_duration:              ['p(95)<250'],
    write_error_rate:                 ['rate<0.02'],
  },
};

const JPEG_BYTES = new Uint8Array([
  0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
  0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
  0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
]);

// setup() runs once before any VU starts. Creates seed tags used by read tests.
export function setup() {
  const tags = [];
  for (let i = 0; i < 10; i++) {
    const res = http.post(
      `${BASE_URL}/tags`,
      JSON.stringify({ name: `load-tag-${i}-${Date.now()}` }),
      { headers: authHeaders },
    );
    if (res.status === 201) {
      tags.push(JSON.parse(res.body));
    }
  }

  // Create media items tagged with seed tags so filtered searches return results.
  const mediaIDs = [];
  for (let i = 0; i < 5; i++) {
    const tag = tags[i % tags.length];
    const res = http.post(
      `${BASE_URL}/media`,
      {
        name: `load-photo-${i}`,
        tags: tag ? tag.id : '',
        file: http.file(JPEG_BYTES.buffer, `load-${i}.jpg`, 'image/jpeg'),
      },
      { headers: { 'X-API-Key': API_KEY } },
    );
    if (res.status === 201) {
      mediaIDs.push(JSON.parse(res.body).id);
    }
  }

  console.log(`setup: seeded ${tags.length} tags, ${mediaIDs.length} media items`);
  return { tags, mediaIDs };
}

export default function (data) {
  const { tags, mediaIDs } = data;
  const rand = Math.random();

  if (rand < 0.40) {
    // 40 % — list tags (paginated)
    group('list tags', () => {
      const start = Date.now();
      const res = http.get(`${BASE_URL}/tags?limit=20`, {
        headers: authHeaders,
        tags: { type: 'read' },
      });
      tagLatency.add(Date.now() - start);
      check(res, { 'list tags 200': r => r.status === 200 });
      mediaListed.add(1);
    });

  } else if (rand < 0.70) {
    // 30 % — list/search media
    group('list media', () => {
      const tag = tags[Math.floor(Math.random() * tags.length)];
      const url = tag && Math.random() < 0.5
        ? `${BASE_URL}/media?tag_id=${tag.id}&limit=20`
        : `${BASE_URL}/media?limit=20`;
      const start = Date.now();
      const res = http.get(url, { headers: authHeaders, tags: { type: 'read' } });
      mediaLatency.add(Date.now() - start);
      check(res, { 'list media 200': r => r.status === 200 });
      mediaListed.add(1);
    });

  } else if (rand < 0.85) {
    // 15 % — get media by ID
    group('get media by id', () => {
      if (!mediaIDs.length) return;
      const id = mediaIDs[Math.floor(Math.random() * mediaIDs.length)];
      const res = http.get(`${BASE_URL}/media/${id}`, {
        headers: authHeaders,
        tags: { type: 'read' },
      });
      check(res, { 'get media 200': r => r.status === 200 });
    });

  } else if (rand < 0.95) {
    // 10 % — create tag (write)
    group('create tag', () => {
      const res = http.post(
        `${BASE_URL}/tags`,
        JSON.stringify({ name: `vu-${__VU}-iter-${__ITER}` }),
        { headers: authHeaders, tags: { type: 'write' } },
      );
      const ok = check(res, { 'create tag 201': r => r.status === 201 });
      writeErrors.add(!ok);
      if (ok) tagCreated.add(1);
    });

  } else {
    // 5 % — upload photo (write, heaviest operation)
    group('upload photo', () => {
      const res = http.post(
        `${BASE_URL}/media`,
        {
          name: `vu-${__VU}-photo-${__ITER}`,
          file: http.file(JPEG_BYTES.buffer, 'load.jpg', 'image/jpeg'),
        },
        { headers: { 'X-API-Key': API_KEY }, tags: { type: 'write' } },
      );
      const ok = check(res, { 'upload photo 201': r => r.status === 201 });
      writeErrors.add(!ok);
    });
  }

  sleep(Math.random() * 0.5 + 0.1); // 100–600 ms think time
}
