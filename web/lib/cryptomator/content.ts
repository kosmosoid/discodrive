/**
 * content.ts — file content decryption (AES-GCM, Cryptomator format 8).
 *
 * Port of Go content.go DecryptContent.
 *
 * File format:
 *   Header 68B: headerNonce(12) ‖ AES-GCM(encKey, headerNonce, [0xFF*8 ‖ contentKey(32)], aad=nil) [56B = 40+16]
 *   Chunks starting at offset 68:
 *     chunkNonce(12) ‖ AES-GCM(contentKey, chunkNonce, plaintext≤32768, aad=BE64(i)‖headerNonce) [≤32768+16]
 *
 * Framing:
 *   Full frame: 12 + 32768 + 16 = 32796 bytes
 *   Last frame: 12 + <32768 + 16 (shorter)
 */

import { be64, concat } from './base.js';
import type { VaultKeys } from './keys.js';

const HEADER_NONCE_SIZE = 12;
const HEADER_CT_SIZE = 56; // 8 (0xFF) + 32 (contentKey) + 16 (GCM tag)
const HEADER_TOTAL = HEADER_NONCE_SIZE + HEADER_CT_SIZE; // 68

const CHUNK_PLAIN_SIZE = 32 * 1024; // 32768
const CHUNK_NONCE_SIZE = 12;
const GCM_TAG_SIZE = 16;
const FULL_CHUNK_FRAME = CHUNK_NONCE_SIZE + CHUNK_PLAIN_SIZE + GCM_TAG_SIZE; // 32796

/**
 * decryptContent — decrypt the content of a Cryptomator SIV_GCM file.
 * Accepts the entire ciphertext as a Uint8Array, returns the plaintext.
 */
export async function decryptContent(keys: VaultKeys, ciphertext: Uint8Array): Promise<Uint8Array> {
  if (ciphertext.length < HEADER_TOTAL) {
    throw new Error(`content: file too short: ${ciphertext.length} < ${HEADER_TOTAL}`);
  }

  // 1. Decrypt the header
  const headerNonce = ciphertext.slice(0, HEADER_NONCE_SIZE);
  const headerCT = ciphertext.slice(HEADER_NONCE_SIZE, HEADER_TOTAL);

  const encKeyObj = await crypto.subtle.importKey(
    'raw',
    keys.encKey as BufferSource,
    { name: 'AES-GCM' },
    false,
    ['decrypt']
  );

  let hPayload: ArrayBuffer;
  try {
    hPayload = await crypto.subtle.decrypt(
      { name: 'AES-GCM', iv: headerNonce, tagLength: 128 },
      encKeyObj,
      headerCT
    );
  } catch {
    throw new Error('content: header decryption failed');
  }

  const hPayloadBytes = new Uint8Array(hPayload);
  if (hPayloadBytes.length !== 40) {
    throw new Error(`content: unexpected header payload size: ${hPayloadBytes.length}`);
  }

  // Payload: 8 bytes of 0xFF + contentKey(32B)
  const contentKey = hPayloadBytes.slice(8, 40);

  // 2. Decrypt chunks
  const contentKeyObj = await crypto.subtle.importKey(
    'raw',
    contentKey,
    { name: 'AES-GCM' },
    false,
    ['decrypt']
  );

  const chunks: Uint8Array[] = [];
  let offset = HEADER_TOTAL;
  let chunkIdx = 0n;

  while (offset < ciphertext.length) {
    const remaining = ciphertext.length - offset;
    const frameSize = Math.min(FULL_CHUNK_FRAME, remaining);

    if (frameSize < CHUNK_NONCE_SIZE + GCM_TAG_SIZE) {
      throw new Error(`content: chunk ${chunkIdx} too short: ${frameSize} bytes`);
    }

    const frame = ciphertext.slice(offset, offset + frameSize);
    const chunkNonce = frame.slice(0, CHUNK_NONCE_SIZE);
    const chunkCT = frame.slice(CHUNK_NONCE_SIZE);

    // AAD = BE64(chunkIdx) ‖ headerNonce (8+12=20B)
    const aad = concat(be64(chunkIdx), headerNonce);

    let pt: ArrayBuffer;
    try {
      pt = await crypto.subtle.decrypt(
        { name: 'AES-GCM', iv: chunkNonce, tagLength: 128, additionalData: aad as BufferSource },
        contentKeyObj,
        chunkCT as BufferSource
      );
    } catch {
      throw new Error(`content: chunk ${chunkIdx} decryption failed`);
    }

    chunks.push(new Uint8Array(pt));
    offset += frameSize;
    chunkIdx++;
  }

  // Assemble the result
  const totalLen = chunks.reduce((s, c) => s + c.length, 0);
  const result = new Uint8Array(totalLen);
  let pos = 0;
  for (const chunk of chunks) {
    result.set(chunk, pos);
    pos += chunk.length;
  }
  return result;
}
