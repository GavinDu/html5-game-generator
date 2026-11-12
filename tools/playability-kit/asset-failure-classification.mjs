// Playability Kit asset triage. Never infer a successful asset delivery from
// an HTTP 200 or a cached buffer alone: a later request of the same URL might
// still have been aborted. Audio is reclassified only with a unique,
// authenticated request instance and its exact runtime decode provenance.
function provenDecodedAudioAbort(request, assets, audioEvidence, base) {
  if (request.reason !== 'net::ERR_ABORTED' || request.kind !== 'fetch' ||
      !audioEvidence || !Array.isArray(audioEvidence.network) ||
      !Array.isArray(audioEvidence.events) ||
      !Array.isArray(audioEvidence.decodedSoundIds) ||
      audioEvidence.activeDecodeJobs !== 0) return false;

  const candidates = (assets || []).filter(a => a.kind === 'audio' && a.id &&
    (a.url || a.uri) && new URL(a.url || a.uri, base).href === request.url);
  if (candidates.length !== 1) return false;
  const id = candidates[0].id;
  const matching = audioEvidence.network.filter(r => r.url === request.url);
  if (matching.length !== 1 || matching[0].phase !== 'failed' ||
      matching[0].reason !== 'net::ERR_ABORTED') return false;

  const events = audioEvidence.events.filter(e => e.assetId === id);
  const count = name => events.filter(e => e.event === name).length;
  // No retries or simultaneous same-ID downloads: only ONE request can
  // account for the complete response and decoded PCM observed in this
  // game session. An offline/restart abort without complete bytes is unsafe.
  if (!audioEvidence.decodedSoundIds.includes(id) ||
      count('request_started') !== 1 || count('response_200') !== 1 ||
      count('decode_ok') !== 1 || events.some(e =>
        /abort|error|timeout/.test(e.event))) return false;
  const started = events.find(e => e.event === 'request_started');
  const responded = events.find(e => e.event === 'response_200');
  const decoded = events.find(e => e.event === 'decode_ok');
  return Number.isFinite(started.atMs) &&
    started.atMs <= responded.atMs && responded.atMs <= decoded.atMs &&
    // Session reset invalidates all old cached audio and proof.
    !audioEvidence.events.some(e => e.event === 'session_reset');
}

// A completed decode is positive proof that one uniquely identified request
// delivered all bytes, even when Chromium later reports an aborted fetch.
// Retain full failure evidence and block every other audio failure.
export function classifyAssetRequestFailures({ requests, assets, texturesLine, base, audioEvidence }) {
  const declaredImages = (assets || []).filter(a => a.kind === 'image');
  const allRequiredTexturesVerified = Array.isArray(texturesLine) &&
    texturesLine[0] === texturesLine[1] &&
    texturesLine[1] === declaredImages.length && texturesLine[1] > 0;
  const declaredImageURLs = new Set(declaredImages
    .map(a => a.url || a.uri).filter(Boolean)
    .map(url => new URL(url, base).href));
  const blocking = [], redundant = [], decodedAudio = [];
  for (const request of requests) {
    if (allRequiredTexturesVerified &&
        request.reason === 'net::ERR_ABORTED' &&
        request.kind === 'fetch' &&
        declaredImageURLs.has(request.url)) {
      redundant.push(request);
    } else if (provenDecodedAudioAbort(request, assets, audioEvidence, base)) {
      decodedAudio.push(request);
    } else {
      blocking.push(`${request.url} [${request.reason}]`);
    }
  }
  return { blocking, redundant, decodedAudio, allRequiredTexturesVerified };
}
