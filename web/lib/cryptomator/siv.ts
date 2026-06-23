/**
 * siv.ts — AES-SIV (RFC 5297) with CMAC.
 *
 * sivKey = macKey(32) ‖ encKey(32) — matching Cryptomator name.go:
 *   sivKey = macKey || encKey
 *
 * S2V logic exactly mirrors Go name.go s2vCMAC:
 *   - aad != null  → CMAC(aad) included as the first component (even for an empty string!)
 *   - aad == null  → plaintext only (single component, no AAD)
 *
 * This distinction is critical for Cryptomator:
 *   - DecryptName: aad = parentDirId ([]byte, even empty "") → aad != null
 *   - DirIdHash:   aad = null (not passed) → plaintext only
 */

import { aesCmac } from './cmac.js';
import { xorBytes, concat } from './base.js';

/** GF(2^128) doubling — matching Go dbl16. */
function dbl16(b: Uint8Array): Uint8Array {
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

/**
 * S2V(macKey, aad, plaintext) → 16-byte SIV tag.
 *
 * aad: Uint8Array | null
 *   - Uint8Array (including empty): AAD is included
 *   - null: AAD is not included
 */
async function s2v(macKey: Uint8Array, aad: Uint8Array | null, plaintext: Uint8Array): Promise<Uint8Array> {
  // Step 1: D = CMAC(K, 0^128)
  const zeros = new Uint8Array(16);
  let D = await aesCmac(macKey, zeros);

  // Step 2: If aad != null — include CMAC(aad) as the first component
  if (aad !== null) {
    const aadMac = await aesCmac(macKey, aad);
    D = dbl16(D);
    D = xorBytes(D, aadMac);
  }

  // Step 3: Last component — plaintext
  let tag: Uint8Array;
  if (plaintext.length >= 16) {
    // xorend: XOR the last 16 bytes of plaintext with D
    const padded = new Uint8Array(plaintext);
    const off = plaintext.length - 16;
    for (let i = 0; i < 16; i++) {
      padded[off + i] ^= D[i];
    }
    tag = await aesCmac(macKey, padded);
  } else {
    // pad: extend to 16 bytes with 0x80...00, XOR with dbl(D)
    const padded = new Uint8Array(16);
    padded.set(plaintext);
    padded[plaintext.length] = 0x80;
    D = dbl16(D);
    const paddedXor = xorBytes(padded, D);
    tag = await aesCmac(macKey, paddedXor);
  }

  return tag;
}

/** AES-CTR encrypt/decrypt (symmetric operation). */
async function aesCtr(encKey: Uint8Array, iv: Uint8Array, data: Uint8Array): Promise<Uint8Array> {
  const key = await crypto.subtle.importKey('raw', encKey as BufferSource, { name: 'AES-CTR' }, false, ['encrypt']);
  // WebCrypto AES-CTR: counter = iv (16 bytes), counter length = 128 bits (full block)
  const ct = await crypto.subtle.encrypt(
    { name: 'AES-CTR', counter: iv as BufferSource, length: 128 },
    key,
    data as BufferSource
  );
  return new Uint8Array(ct);
}

/**
 * sivDecrypt(sivKey, ciphertext, aad) → plaintext
 *
 * sivKey: 64 bytes = macKey(32) ‖ encKey(32)
 * aad: Uint8Array (even empty) or null
 * CT structure: tag(16) ‖ AES-CTR(encKey, iv=tag_with_zeroed_bits_31_63, data)
 */
export async function sivDecrypt(
  sivKey: Uint8Array,
  ciphertext: Uint8Array,
  aad: Uint8Array | null
): Promise<Uint8Array> {
  if (ciphertext.length < 16) {
    throw new Error('siv: ciphertext too short');
  }

  const macKey = sivKey.slice(0, 32);
  const encKey = sivKey.slice(32, 64);

  const tag = ciphertext.slice(0, 16);
  const enc = ciphertext.slice(16);

  // IV = tag with bits 31 and 63 zeroed (RFC 5297)
  const iv = new Uint8Array(tag);
  iv[8] &= 0x7f;  // bit 63 (0-indexed from MSB: byte 8, bit 7)
  iv[12] &= 0x7f; // bit 31 (byte 12, bit 7)

  // Decrypt via CTR
  const plaintext = await aesCtr(encKey, iv, enc);

  // Verify the tag via S2V
  const computedTag = await s2v(macKey, aad, plaintext);

  // Constant-time comparison
  let diff = 0;
  for (let i = 0; i < 16; i++) {
    diff |= tag[i] ^ computedTag[i];
  }
  if (diff !== 0) {
    throw new Error('siv: message authentication failed');
  }

  return plaintext;
}

/**
 * sivEncrypt(sivKey, plaintext, aad) → ciphertext
 *
 * sivKey: 64 bytes = macKey(32) ‖ encKey(32)
 * aad: Uint8Array or null
 */
export async function sivEncrypt(
  sivKey: Uint8Array,
  plaintext: Uint8Array,
  aad: Uint8Array | null
): Promise<Uint8Array> {
  const macKey = sivKey.slice(0, 32);
  const encKey = sivKey.slice(32, 64);

  // Compute the SIV tag
  const tag = await s2v(macKey, aad, plaintext);

  // IV = tag with bits 31 and 63 zeroed
  const iv = new Uint8Array(tag);
  iv[8] &= 0x7f;
  iv[12] &= 0x7f;

  // Encrypt via CTR
  const encData = await aesCtr(encKey, iv, plaintext);

  // CT = tag ‖ encData
  return concat(tag, encData);
}
