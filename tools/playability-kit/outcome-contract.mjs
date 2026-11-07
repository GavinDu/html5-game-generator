// Determine whether an IR is a conventional win objective or a deliberately
// loss-only chase. This is classification, not evidence that the terminal flow
// was actually reached. The browser still has to observe the end card/restart.
export function outcomeContract(rules = [], experience = {}) {
  const ends = (value) => rules.some((rule) => (rule.actions || []).some((action) =>
    action.type === 'end_game' && (action.value === value || action.value === String(value))));
  const hasWin = ends(true);
  const hasLoss = ends(false);
  const lossOnlyChase = !hasWin && hasLoss &&
    experience?.controls?.scheme === 'lane_tap' &&
    Number.isFinite(experience?.chaserGap?.caughtAt) &&
    experience.chaserGap.caughtAt >= 0 &&
    typeof experience?.result?.lose?.title === 'string' &&
    experience.result.lose.title.trim().length > 0;
  return { hasWin, hasLoss, lossOnlyChase };
}
