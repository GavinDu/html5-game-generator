// Resolve the actual bot-play window. A longer win budget must extend,
// not be truncated by, the shorter routine play budget.
export function resolvePlayBudgetSeconds(playSeconds, winSeconds) {
  return Math.max(playSeconds, winSeconds);
}
