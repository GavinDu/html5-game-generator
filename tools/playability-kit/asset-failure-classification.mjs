// Asset request triage for real-browser Playability Kit. This is not a
// whitelist for failed required resources: only canceled *redundant* fetches
// for image assets whose entire required texture set has successfully loaded
// may be reclassified as informational. Any missing, failed, unknown or
// unauthenticated asset stays a blocking error.
export function classifyAssetRequestFailures({ requests, assets, texturesLine, base }) {
  const declaredImages = (assets || []).filter(a => a.kind === 'image');
  const allRequiredTexturesVerified = Array.isArray(texturesLine) &&
    texturesLine[0] === texturesLine[1] &&
    texturesLine[1] === declaredImages.length && texturesLine[1] > 0;
  const declaredImageURLs = new Set(declaredImages
    .map(a => a.url || a.uri).filter(Boolean)
    .map(url => new URL(url, base).href));
  const blocking = [], redundant = [];
  for (const request of requests) {
    if (allRequiredTexturesVerified &&
        request.reason === 'net::ERR_ABORTED' &&
        request.kind === 'fetch' &&
        declaredImageURLs.has(request.url)) {
      redundant.push(request);
    } else {
      blocking.push(`${request.url} [${request.reason}]`);
    }
  }
  return { blocking, redundant, allRequiredTexturesVerified };
}
