// Pure geometry contract for the settled, scroll-snapped social play feed.
// Measure only within the pager's OWN viewport, not the app/window frame.
// A neighboring card may exist in the DOM but must not bleed into a settled
// swipe page. Pixel rounding up to 2 CSS pixels is tolerated.
export function evaluateSettledPager(viewport, cards, tolerancePx = 2) {
  const validRect = r => r && Number.isFinite(r.top) && Number.isFinite(r.bottom) && r.bottom > r.top;
  if (!validRect(viewport) || !Array.isArray(cards) || !Number.isFinite(tolerancePx) || tolerancePx < 0) {
    return { aligned: false, visibleCount: 0, visibleGameIDs: [], reason: 'invalid-geometry' };
  }
  const visible = cards.filter(card => validRect(card) &&
    Math.min(viewport.bottom, card.bottom) - Math.max(viewport.top, card.top) > tolerancePx);
  const only = visible.length === 1 ? visible[0] : null;
  const aligned = !!only && Math.abs(only.top - viewport.top) <= tolerancePx &&
    Math.abs(only.bottom - viewport.bottom) <= tolerancePx;
  return {
    aligned,
    visibleCount: visible.length,
    visibleGameIDs: visible.map(card => card.gameID),
    topOffsetPx: only ? only.top - viewport.top : null,
    bottomOffsetPx: only ? only.bottom - viewport.bottom : null,
    reason: aligned ? 'settled' : visible.length !== 1 ? 'neighboring-card-visible' : 'card-not-aligned',
  };
}
