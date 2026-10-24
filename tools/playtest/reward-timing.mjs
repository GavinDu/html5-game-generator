// Select the most direct gameplay clock available. The runtime measures
// first reward in visible-play milliseconds; HUD polling is a fallback only.
export function selectFirstRewardMeasurement(runtimeFirstRewardMs, hudChanged, hudElapsedMs) {
  if (Number.isFinite(runtimeFirstRewardMs) && runtimeFirstRewardMs >= 0) {
    return { ms: Math.round(runtimeFirstRewardMs), source: 'runtime probe' };
  }
  if (hudChanged && Number.isFinite(hudElapsedMs) && hudElapsedMs >= 0) {
    return { ms: Math.round(hudElapsedMs), source: 'HUD score observation' };
  }
  return null;
}

// Keep the per-game probe snapshot in sync with hook.first_reward. The
// score-report stage reads probe.firstRewardMs rather than hook evidence.
export function mergeFirstRewardMeasurement(probe, measurement) {
  if (!measurement || !Number.isFinite(measurement.ms) || measurement.ms < 0) {
    return probe;
  }
  return { ...(probe || {}), firstRewardMs: Math.round(measurement.ms) };
}
