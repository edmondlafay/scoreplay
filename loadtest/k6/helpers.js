/**
 * Shared helpers for Scoreplay load tests.
 *
 * Environment variables:
 *   BASE_URL  — API base URL (default: http://localhost:8080)
 *   API_KEY   — sp_<64hex> key from `go run ./cmd/seed -name "load-test"`
 */

export const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
export const API_KEY  = __ENV.API_KEY  || '';

/** Default headers for authenticated JSON requests. */
export const authHeaders = {
  'X-API-Key': API_KEY,
  'Content-Type': 'application/json',
};

/** Authenticated GET with JSON body returned as parsed object. */
export function apiGet(path) {
  return http.get(`${BASE_URL}${path}`, { headers: authHeaders });
}

/**
 * Minimum JPEG magic bytes (12 bytes header + filler).
 * Passes magic-byte detection; kept tiny to minimise upload latency in tests.
 */
export const MIN_JPEG = new Uint8Array([
  0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46,
  0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01,
  0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9, // minimal valid JPEG close
]).buffer;

/**
 * Minimum MP4 magic bytes (ftyp box at offset 4).
 */
export const MIN_MP4 = new Uint8Array([
  0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70,
  0x6D, 0x70, 0x34, 0x32, 0x00, 0x00, 0x00, 0x00,
  0x6D, 0x70, 0x34, 0x32, 0x69, 0x73, 0x6F, 0x6D,
]).buffer;
