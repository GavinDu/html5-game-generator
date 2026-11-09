// Honest media readiness for the full-screen swipe feed.
// A poster proves a still frame was served; it is NOT a verified preview clip.
// A count of four is not four independently tested viewport profiles.
// Reject duplicate, missing or mislabeled phone/tablet captures.
export const REQUIRED_FEED_VIEWPORTS = Object.freeze([
  'iphone-portrait', 'iphone-landscape', 'ipad-portrait', 'ipad-landscape',
]);

export function hasCompleteViewportCoverage(viewports, expectedViewports = 4) {
  if (viewports.length !== expectedViewports) return false;
  const observed = viewports.map(v => v?.viewport);
  if (observed.some(name => typeof name !== 'string')) return false;
  if (new Set(observed).size !== expectedViewports) return false;
  return expectedViewports !== REQUIRED_FEED_VIEWPORTS.length ||
    REQUIRED_FEED_VIEWPORTS.every(name => observed.includes(name));
}

export function evaluateFeedPreviewReadiness(viewports, expectedViewports = 4) {
  const complete = hasCompleteViewportCoverage(viewports, expectedViewports) &&
    viewports.every(v => v.passed === true && v.beforePreview && v.afterPreview);
  const cards = viewports.flatMap(v => [v?.beforePreview, v?.afterPreview].filter(Boolean));
  const verifiedPosterCount = cards.filter(c => c.type === 'verified-poster' && c.valid).length;
  const honestFallbackCount = cards.filter(c => c.type === 'honest-placeholder' && c.valid).length;
  const unknownOrBrokenCount = cards.length - verifiedPosterCount - honestFallbackCount;
  const posterBacked = complete && cards.length === expectedViewports * 2 &&
    verifiedPosterCount === cards.length;
  return {
    status: posterBacked ? 'poster-backed-clip-unverified' : 'needs-reviewed-gameplay-media',
    posterBacked,
    viewportCoverageComplete: hasCompleteViewportCoverage(viewports, expectedViewports),
    clipVerified: false, // A still-image test must not claim recorded gameplay video.
    verifiedPosterCount,
    honestFallbackCount,
    unknownOrBrokenCount,
    inspectedCardSurfaces: cards.length,
    notes: posterBacked
      ? 'All sampled pages served real posters; recorded 6-second clips remain unverified.'
      : !hasCompleteViewportCoverage(viewports, expectedViewports)
        ? 'Distinct phone/tablet portrait/landscape screenshot profiles are missing or duplicated.'
        : 'One or more sampled pages lack reviewed gameplay posters; screenshot UI functionality can still pass.'
  };
}
