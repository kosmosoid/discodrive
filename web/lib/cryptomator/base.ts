/**
 * base.ts — utilities: base64url, base32, BE64, Uint8Array helpers.
 * Compatible with both browser and Node (WebCrypto globalThis.crypto.subtle).
 */

/** base64url decode — handles both padded and unpadded input. */
export function base64UrlDecode(s: string): Uint8Array {
  // Normalize: replace base64url characters with standard base64 characters
  let b64 = s.replace(/-/g, '+').replace(/_/g, '/');
  // Add padding if needed
  const pad = (4 - (b64.length % 4)) % 4;
  b64 += '='.repeat(pad);
  const bin = atob(b64);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    out[i] = bin.charCodeAt(i);
  }
  return out;
}

/** base64url encode without padding (raw). */
export function base64UrlEncode(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i]);
  }
  return btoa(bin).replace(/\+/g, '-').replace(/\//g, '_').replace(/=/g, '');
}

/** base64 (standard, with padding) decode. */
export function base64StdDecode(s: string): Uint8Array {
  const bin = atob(s);
  const out = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) {
    out[i] = bin.charCodeAt(i);
  }
  return out;
}

/** base64 (standard) encode with padding. */
export function base64StdEncode(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i++) {
    bin += String.fromCharCode(bytes[i]);
  }
  return btoa(bin);
}

/** base64url decode — with or without padding. */
export function base64UrlDecodeFlex(s: string): Uint8Array {
  return base64UrlDecode(s);
}

/** base32 RFC4648 uppercase without padding. */
const BASE32_ALPHABET = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

export function base32Encode(bytes: Uint8Array): string {
  let result = '';
  let buffer = 0;
  let bitsLeft = 0;
  for (let i = 0; i < bytes.length; i++) {
    buffer = (buffer << 8) | bytes[i];
    bitsLeft += 8;
    while (bitsLeft >= 5) {
      bitsLeft -= 5;
      result += BASE32_ALPHABET[(buffer >> bitsLeft) & 0x1f];
    }
  }
  if (bitsLeft > 0) {
    result += BASE32_ALPHABET[(buffer << (5 - bitsLeft)) & 0x1f];
  }
  return result;
}

/** BE64: big-endian uint64 → 8 bytes. */
export function be64(n: number | bigint): Uint8Array {
  const buf = new Uint8Array(8);
  const view = new DataView(buf.buffer);
  view.setBigUint64(0, BigInt(n), false);
  return buf;
}

/** Concatenate Uint8Arrays. */
export function concat(...arrays: Uint8Array[]): Uint8Array {
  const total = arrays.reduce((s, a) => s + a.length, 0);
  const out = new Uint8Array(total);
  let off = 0;
  for (const a of arrays) {
    out.set(a, off);
    off += a.length;
  }
  return out;
}

/** XOR two same-length Uint8Arrays. */
export function xorBytes(a: Uint8Array, b: Uint8Array): Uint8Array {
  const out = new Uint8Array(a.length);
  for (let i = 0; i < a.length; i++) {
    out[i] = a[i] ^ b[i];
  }
  return out;
}

/** UTF-8 string → Uint8Array. */
export function utf8Encode(s: string): Uint8Array {
  return new TextEncoder().encode(s);
}

/** Uint8Array → UTF-8 string. */
export function utf8Decode(b: Uint8Array): string {
  return new TextDecoder().decode(b);
}
