/**
 * cmac.ts — AES-CMAC (RFC 4493) via WebCrypto AES-CBC.
 *
 * CMAC(K, M):
 *   1. Derive subkeys K1, K2 from L = AES-CBC(K, zeros, zeros).
 *   2. Split M into 16-byte blocks.
 *   3. Last block: if complete — XOR with K1; if incomplete — pad with 0x80...00, XOR with K2.
 *   4. CBC-MAC: encrypt blocks sequentially using AES-CBC with IV=zeros.
 *
 * WebCrypto AES-CBC always appends PKCS7 padding, so we use it to encrypt
 * exactly one block at a time (taking only the first 16 bytes of output).
 */

import { xorBytes } from './base.js';

/** GF(2^128) doubling — as in RFC 4493. */
function dbl128(b: Uint8Array): Uint8Array {
  const out = new Uint8Array(16);
  let carry = 0;
  for (let i = 15; i >= 0; i--) {
    const next = (b[i] >> 7) & 1;
    out[i] = ((b[i] << 1) | carry) & 0xff;
    carry = next;
  }
  if (carry) {
    out[15] ^= 0x87;
  }
  return out;
}

/** Encrypt exactly one 16-byte block via AES-ECB (implemented as CBC with a zero IV). */
async function aesEncryptBlock(key: CryptoKey, block: Uint8Array): Promise<Uint8Array<ArrayBuffer>> {
  // AES-CBC with IV=zeros encrypts the first block identically to AES-ECB
  const iv = new Uint8Array(16);
  const ct = await crypto.subtle.encrypt({ name: 'AES-CBC', iv }, key, block as BufferSource);
  // ct = 32 bytes (16 data + 16 PKCS7 padding) — take the first 16
  return new Uint8Array(ct, 0, 16);
}

/** Import a key for use with AES-CBC. */
async function importAesCbcKey(keyBytes: Uint8Array): Promise<CryptoKey> {
  return crypto.subtle.importKey('raw', keyBytes as BufferSource, { name: 'AES-CBC' }, false, ['encrypt']);
}

/**
 * AES-CMAC(key, message) → 16-byte MAC.
 */
export async function aesCmac(keyBytes: Uint8Array, message: Uint8Array): Promise<Uint8Array> {
  const key = await importAesCbcKey(keyBytes);

  // Step 1: L = AES(K, 0^128)
  const zeros16 = new Uint8Array(16);
  const L = await aesEncryptBlock(key, zeros16);

  // Step 2: K1 = dbl(L), K2 = dbl(K1)
  const K1 = dbl128(L);
  const K2 = dbl128(K1);

  // Step 3: split message into blocks
  const n = Math.max(1, Math.ceil(message.length / 16));
  const lastBlockStart = (n - 1) * 16;
  const lastBlock = message.slice(lastBlockStart);
  const isComplete = lastBlock.length === 16 && message.length > 0;

  // Prepare the last block
  let lastProcessed: Uint8Array;
  if (isComplete) {
    // Complete block: XOR with K1
    lastProcessed = xorBytes(lastBlock, K1);
  } else {
    // Incomplete block: pad to 16 bytes with 0x80...00, XOR with K2
    const padded = new Uint8Array(16);
    padded.set(lastBlock);
    padded[lastBlock.length] = 0x80;
    lastProcessed = xorBytes(padded, K2);
  }

  // Step 4: CBC-MAC
  // Process blocks 0..n-2, then the last block
  let X = new Uint8Array(16); // IV = 0^128

  for (let i = 0; i < n - 1; i++) {
    const block = message.slice(i * 16, (i + 1) * 16);
    X = await aesEncryptBlock(key, xorBytes(X, block));
  }

  // Last block
  X = await aesEncryptBlock(key, xorBytes(X, lastProcessed));

  return X;
}
