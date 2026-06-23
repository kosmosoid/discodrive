/**
 * name.ts — file name encryption and DirIdHash (Cryptomator format 8).
 *
 * Port of Go name.go DecryptName, DirIdHash, EncryptName.
 *
 * IMPORTANT:
 * - sivKey = macKey ‖ encKey (64B) — matching Go: sivKey = macKey||encKey
 * - DecryptName: aad = Uint8Array(parentDirId) ��� even an empty string "" → aad != null
 * - DirIdHash: sivEncrypt(dirId, aad=null) → SHA1 → base32 → "d/XX/YYY..."
 */

import { sivDecrypt, sivEncrypt } from './siv.js';
import { base64UrlDecode, base64UrlEncode, base32Encode, utf8Encode } from './base.js';
import type { VaultKeys } from './keys.js';

/** Build sivKey = macKey ‖ encKey. */
function makeSivKey(keys: VaultKeys): Uint8Array {
  const k = new Uint8Array(64);
  k.set(keys.macKey, 0);
  k.set(keys.encKey, 32);
  return k;
}

/**
 * DecryptName — decrypt an encrypted .c9r file name.
 * Port of Go vault.go DecryptName.
 *
 * @param keys — vault keys
 * @param encNameC9r — file name with or without the .c9r suffix
 * @param parentDirId — parent directory ID (string; "" for the root)
 */
export async function decryptName(
  keys: VaultKeys,
  encNameC9r: string,
  parentDirId: string
): Promise<string> {
  // Strip the .c9r suffix
  let encName = encNameC9r;
  if (encName.endsWith('.c9r')) {
    encName = encName.slice(0, -4);
  }

  // base64url decode (with or without padding)
  const ct = base64UrlDecode(encName);

  const sivKey = makeSivKey(keys);

  // AAD = utf8(parentDirId) — ALWAYS a Uint8Array (even for an empty string)
  const aad = utf8Encode(parentDirId);

  const pt = await sivDecrypt(sivKey, ct, aad);
  return new TextDecoder().decode(pt);
}

/**
 * EncryptName — encrypt a file name bound to a parentDirId.
 * Port of Go vault.go EncryptName.
 *
 * @returns base64url(ciphertext) + ".c9r"
 */
export async function encryptName(
  keys: VaultKeys,
  name: string,
  parentDirId: string
): Promise<string> {
  const sivKey = makeSivKey(keys);
  const aad = utf8Encode(parentDirId);
  const ct = await sivEncrypt(sivKey, utf8Encode(name), aad);
  return base64UrlEncode(ct) + '.c9r';
}

/**
 * DirIdHash — compute the storage path for a directory.
 * Algorithm: AES-SIV(dirId, aad=null) → SHA1 → base32(NoPadding) → "d/XX/YYY..."
 * Port of Go vault.go DirIdHash.
 */
export async function dirIdHash(keys: VaultKeys, dirId: string): Promise<string> {
  const sivKey = makeSivKey(keys);

  // aad = null (not passed) — matching Go: sivEncrypt(v.macKey, v.encKey, []byte(dirID), nil)
  const ct = await sivEncrypt(sivKey, utf8Encode(dirId), null);

  // SHA1 of the ciphertext
  const hashBuf = await crypto.subtle.digest('SHA-1', ct as BufferSource);
  const hash = new Uint8Array(hashBuf);

  // Base32 RFC4648 uppercase without padding
  const b32 = base32Encode(hash);

  // Path: "d/" + first 2 characters + "/" + remaining 30 characters
  return 'd/' + b32.slice(0, 2) + '/' + b32.slice(2);
}
