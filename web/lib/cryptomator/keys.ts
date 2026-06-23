/**
 * keys.ts — openVault: scrypt → KEK → AES-KW unwrap → JWT verify → {encKey, macKey}
 *
 * Port of Go vault.go Open().
 *
 * Dependencies: scrypt-js (npm package), WebCrypto.
 */

import { scrypt } from 'scrypt-js';
import { base64StdDecode, base64UrlDecode, utf8Encode, concat } from './base.js';

/** ErrWrongPassword — wrong password (AES-KW integrity check failed). */
export class WrongPasswordError extends Error {
  constructor() {
    super('wrong vault password');
    this.name = 'WrongPasswordError';
  }
}

/** Return type of openVault. */
export interface VaultKeys {
  encKey: Uint8Array; // 32B
  macKey: Uint8Array; // 32B
}

/** masterkeyFile — shape of masterkey.cryptomator. */
interface MasterkeyFile {
  scryptSalt: string;
  scryptCostParam: number;
  scryptBlockSize: number;
  primaryMasterKey: string;
  hmacMasterKey: string;
}

/**
 * AES Key Wrap (RFC 3394) unwrap — JS implementation.
 * Port of Go keywrap.go aesKWUnwrap.
 * kek: 32B, wrapped: 40B → plaintext: 32B.
 */
async function aesKWUnwrap(kek: Uint8Array, wrapped: Uint8Array): Promise<Uint8Array> {
  // Use WebCrypto AES-KW
  const kekKey = await crypto.subtle.importKey(
    'raw',
    kek as BufferSource,
    { name: 'AES-KW' },
    false,
    ['unwrapKey']
  );

  // unwrapKey with target algorithm 'AES-GCM' (required by the API;
  // we then export as raw bytes)
  let rawKey: Uint8Array;
  try {
    const unwrapped = await crypto.subtle.unwrapKey(
      'raw',
      wrapped as BufferSource,
      kekKey,
      { name: 'AES-KW' },
      { name: 'AES-GCM' },
      true,
      ['encrypt', 'decrypt']
    );
    const exported = await crypto.subtle.exportKey('raw', unwrapped);
    rawKey = new Uint8Array(exported);
  } catch {
    throw new WrongPasswordError();
  }

  return rawKey;
}

/**
 * Open a vault from the masterkey and JWT JSON strings with a password.
 *
 * @param masterkeyJson — contents of masterkey.cryptomator
 * @param vaultJwt — contents of vault.cryptomator (JWT string)
 * @param password — vault password
 * @returns {encKey, macKey}
 * @throws WrongPasswordError on wrong password
 */
export async function openVault(
  masterkeyJson: string,
  vaultJwt: string,
  password: string
): Promise<VaultKeys> {
  // 1. Parse masterkey.cryptomator
  const mk: MasterkeyFile = JSON.parse(masterkeyJson);

  // 2. Decode salt and wrapped keys (standard base64 with padding)
  const salt = base64StdDecode(mk.scryptSalt);
  const wrappedEnc = base64StdDecode(mk.primaryMasterKey);
  const wrappedMac = base64StdDecode(mk.hmacMasterKey);

  // 3. Derive KEK via scrypt
  const passwordBytes = utf8Encode(password);
  const N = mk.scryptCostParam;
  const r = mk.scryptBlockSize;
  const p = 1;
  const dkLen = 32;

  // scrypt-js: returns a Uint8Array
  const kek = await scrypt(passwordBytes, salt, N, r, p, dkLen);

  // 4. AES-KW unwrap (wrong password → WrongPasswordError)
  const encKey = await aesKWUnwrap(new Uint8Array(kek), wrappedEnc);
  const macKey = await aesKWUnwrap(new Uint8Array(kek), wrappedMac);

  // 5. Verify the vault.cryptomator JWT (HS256, key = encKey ‖ macKey)
  const jwt = vaultJwt.trim();
  await verifyVaultJWT(jwt, encKey, macKey);

  return { encKey, macKey };
}

/**
 * Verify the JWT signature and payload (format=8, cipherCombo=SIV_GCM).
 * Port of Go vault.go verifyVaultJWT.
 * Signing key: encKey ‖ macKey (64B), algorithm HS256.
 */
async function verifyVaultJWT(token: string, encKey: Uint8Array, macKey: Uint8Array): Promise<void> {
  const parts = token.split('.');
  if (parts.length !== 3) {
    throw new Error('invalid JWT: expected 3 parts');
  }

  // Key = encKey ‖ macKey
  const sigKeyBytes = concat(encKey, macKey);
  const sigKey = await crypto.subtle.importKey(
    'raw',
    sigKeyBytes as BufferSource,
    { name: 'HMAC', hash: 'SHA-256' },
    false,
    ['verify']
  );

  // Verify the HS256 signature
  const sigInput = utf8Encode(parts[0] + '.' + parts[1]);
  const sig = base64UrlDecode(parts[2]);

  const valid = await crypto.subtle.verify('HMAC', sigKey, sig as BufferSource, sigInput as BufferSource);
  if (!valid) {
    throw new Error('JWT signature mismatch — masterkey does not match vault');
  }

  // Parse the payload
  const payloadBytes = base64UrlDecode(parts[1]);
  const payload = JSON.parse(new TextDecoder().decode(payloadBytes));

  if (payload.format !== 8) {
    throw new Error(`unsupported vault format: ${payload.format} (expected 8)`);
  }
  if (payload.cipherCombo !== 'SIV_GCM') {
    throw new Error(`unsupported cipherCombo: "${payload.cipherCombo}" (expected SIV_GCM)`);
  }
}
