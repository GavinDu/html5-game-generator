export function compareFrameHashes(first, second) {
  if (!Array.isArray(first) || !Array.isArray(second) || first.length === 0 || second.length === 0) {
    throw new TypeError('both renders must contain a non-empty frame_sha256 array');
  }
  if (![...first, ...second].every((hash) => typeof hash === 'string' && /^[a-f0-9]{64}$/.test(hash))) {
    throw new TypeError('frame_sha256 entries must be lowercase SHA-256 hex digests');
  }
  const sharedLength = Math.min(first.length, second.length);
  for (let index = 0; index < sharedLength; index += 1) {
    if (first[index] !== second[index]) {
      return { match: false, frame_count: sharedLength, first_mismatch_index: index };
    }
  }
  if (first.length !== second.length) {
    return { match: false, frame_count: sharedLength, first_mismatch_index: sharedLength };
  }
  return { match: true, frame_count: first.length, first_mismatch_index: null };
}
