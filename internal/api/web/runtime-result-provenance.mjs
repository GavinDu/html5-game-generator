// Pure presentation guard for result responses consumed by the preinstalled
// Pixi runtime. Browser-reported score/rank data is NEVER server-attested
// merely because a number or legacy Verified boolean exists.
//
// This helper has no network calls, no native bridge and no scores to write.
export function provisionalRunPlacementLabel(result) {
  if (!result || typeof result !== 'object' || Array.isArray(result)) return null;
  // Old backends omitted rankingScope; they are never considered verified.
  if (result.rankingScope !== undefined && result.rankingScope !== 'provisional') return null;
  if (!Number.isSafeInteger(result.rank) || result.rank < 1 ||
      result.rank > 1_000_000) return null;
  return `Provisional leaderboard position #${result.rank} this week (unverified)`;
}

// This may ONLY request that the trusted native host independently fetch and
// validate a run ID. Matching these flags in page JSON NEVER attests a score.
// Current deployed API returns attested:false, so this is always false.
export function mayRequestNativeRunVerificationLookup(result) {
  return !!result && typeof result === 'object' && !Array.isArray(result) &&
    result.verified === true && result.attested === true &&
    result.rankingScope === 'verified_only' &&
    result.evidenceStatus === 'verified' &&
    typeof result.runId === 'string' && result.runId.length > 0 &&
    result.runId.length <= 128;
}
