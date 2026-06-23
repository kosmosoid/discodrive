/**
 * index.ts — re-exports the public API of the Cryptomator crypto module.
 */

export { WrongPasswordError } from './keys.js';
export type { VaultKeys } from './keys.js';
export { openVault } from './keys.js';
export { decryptName, encryptName, dirIdHash } from './name.js';
export { decryptContent } from './content.js';
export { sivEncrypt, sivDecrypt } from './siv.js';
export { aesCmac } from './cmac.js';

// Alias for backwards compatibility
export { WrongPasswordError as ErrWrongPassword } from './keys.js';
