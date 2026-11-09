// Honest media readiness for the full-screen swipe feed.
// A poster proves a still frame was served; it is NOT a verified preview clip.
export function evaluateFeedPreviewReadiness(viewports, expectedViewports = 4) {
  const complete = viewports.length === expectedViewports &&
    viewports.every(v => v.passed === true && v.beforePreview && v.afterPreview);
  const cards = viewports.flatMap(v => [v.beforePreview, v.afterPreview].filter(Boolean));
  const verifiedPosterCount = cards.filter(c => c.type === 'verified-poster' && c.valid).length;
  const honestFallbackCount = cards.filter(c => c.type === 'honest-placeholder' && c.valid).length;
  const unknownOrBrokenCount = cards.length - verifiedPosterCount - honestFallbackCount;
  const posterBacked = complete && cards.length === expectedViewports * 2 &&
    verifiedPosterCount === cards.length;
  return {
    status: posterBacked ? 'poster-backed-clip-unverified' : 'needs-reviewed-gameplay-media',
    posterBacked,
    clipVerified: false, // A still-image test must not claim recorded gameplay video.
    verifiedPosterCount,
    honestFallbackCount,
    unknownOrBrokenCount,
    inspectedCardSurfaces: cards.length,
    notes: posterBacked
      ? 'All sampled pages served real posters; recorded 6-second clips remain unverified.'
      : 'One or more sampled pages lack reviewed gameplay posters; screenshot UI functionality can still pass.'
  };
}
