/**
 * Smoke test — 1 VU, 30 s.
 *
 * Verifies every endpoint returns the expected status code under minimal load.
 * Run this first after any deployment to confirm the service is alive and
 * behaving correctly before proceeding to load or stress tests.
 *
 * Usage:
 *   k6 run -e API_KEY=sp_<key> loadtest/k6/smoke.js
 */

import http from 'k6/http';
import { check, group, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const API_KEY  = __ENV.API_KEY  || '';

const authHeaders = { 'X-API-Key': API_KEY, 'Content-Type': 'application/json' };

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: {
    http_req_failed:               ['rate<0.01'],          // <1% errors
    http_req_duration:             ['p(99)<1000'],         // 99th p < 1 s
    'http_req_duration{type:read}': ['p(95)<300'],         // reads < 300 ms p95
  },
};

// Seed one tag and one media item in setup so reads have data to return.
export function setup() {
  const tagRes = http.post(
    `${BASE_URL}/tags`,
    JSON.stringify({ name: `smoke-tag-${Date.now()}` }),
    { headers: authHeaders },
  );
  if (tagRes.status !== 201) {
    console.error(`setup: create tag failed ${tagRes.status} ${tagRes.body}`);
    return {};
  }
  const tag = JSON.parse(tagRes.body);

  // Upload a minimal JPEG photo.
  const jpegBytes = new Uint8Array([
    0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
    0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
    0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9,
  ]);
  const mediaRes = http.post(
    `${BASE_URL}/media`,
    {
      name: 'smoke-photo',
      tags: tag.id,
      file: http.file(jpegBytes.buffer, 'smoke.jpg', 'image/jpeg'),
    },
    { headers: { 'X-API-Key': API_KEY } },
  );
  if (mediaRes.status !== 201) {
    console.error(`setup: create media failed ${mediaRes.status} ${mediaRes.body}`);
    return { tagID: tag.id };
  }
  const media = JSON.parse(mediaRes.body);
  return { tagID: tag.id, mediaID: media.id };
}

export default function (data) {
  group('unprotected endpoints', () => {
    const h = http.get(`${BASE_URL}/health`);
    check(h, { 'health → 200': r => r.status === 200 });

    const spec = http.get(`${BASE_URL}/openapi.yaml`);
    check(spec, { 'openapi.yaml → 200': r => r.status === 200 });
  });

  group('tags', () => {
    const list = http.get(`${BASE_URL}/tags`, { headers: authHeaders, tags: { type: 'read' } });
    check(list, {
      'GET /tags → 200':   r => r.status === 200,
      'has items array':   r => Array.isArray(JSON.parse(r.body).items),
    });

    const bad = http.post(`${BASE_URL}/tags`, JSON.stringify({ name: '' }), {
      headers: authHeaders,
      responseCallback: http.expectedStatuses(422), // intentional error — not a failure
    });
    check(bad, {
      'empty name → 422':       r => r.status === 422,
      'error code present':     r => JSON.parse(r.body).error?.code === 'VALIDATION_ERROR',
    });
  });

  group('media', () => {
    const list = http.get(`${BASE_URL}/media`, { headers: authHeaders, tags: { type: 'read' } });
    check(list, {
      'GET /media → 200': r => r.status === 200,
      'has items array':  r => Array.isArray(JSON.parse(r.body).items),
    });

    if (data.tagID) {
      const byTag = http.get(
        `${BASE_URL}/media?tag_id=${data.tagID}`,
        { headers: authHeaders, tags: { type: 'read' } },
      );
      check(byTag, { 'GET /media?tag_id → 200': r => r.status === 200 });
    }

    if (data.mediaID) {
      const single = http.get(
        `${BASE_URL}/media/${data.mediaID}`,
        { headers: authHeaders, tags: { type: 'read' } },
      );
      check(single, {
        'GET /media/{id} → 200': r => r.status === 200,
        'correct id':            r => JSON.parse(r.body).id === data.mediaID,
      });
    }

    const notFound = http.get(`${BASE_URL}/media/00000000-0000-0000-0000-000000000000`, {
      headers: authHeaders,
      responseCallback: http.expectedStatuses(404), // intentional error — not a failure
    });
    check(notFound, {
      'unknown id → 404':   r => r.status === 404,
      'MEDIA_NOT_FOUND code': r => JSON.parse(r.body).error?.code === 'MEDIA_NOT_FOUND',
    });

    const noKey = http.get(`${BASE_URL}/media`, {
      responseCallback: http.expectedStatuses(401), // intentional error — not a failure
    });
    check(noKey, {
      'no key → 401':          r => r.status === 401,
      'API_KEY_REQUIRED code': r => JSON.parse(r.body).error?.code === 'API_KEY_REQUIRED',
    });
  });

  sleep(1);
}
