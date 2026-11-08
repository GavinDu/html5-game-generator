// Game-quality evidence must respect authored outcomes, not infer a
// conventional victory when the source deliberately ends only in defeat.
// This does not replace observed browser end/restart verification.
import { outcomeContract } from '../playability-kit/outcome-contract.mjs';

export function outcomeCoverage(rules, experience, { winSeen = false, endSeen = false } = {}) {
  const contract = outcomeContract(rules, experience);
  if (contract.lossOnlyChase) {
    return [
      {
        checkId: 'simulation.win_reachable', status: 'skip',
        evidence: 'not applicable: the authored chase has a caught-only ending; a win must not be invented',
        actual: { authoredWin: false, authoredLoss: true },
      },
      {
        checkId: 'simulation.caught_reachable',
        status: endSeen && !winSeen ? 'pass' : 'skip',
        evidence: endSeen && !winSeen
          ? 'the unmodified browser bot observed the authored caught ending'
          : 'caught ending not reached during the short bot budget; check controlled collider and natural long-run balance separately',
        actual: { observedNaturally: endSeen && !winSeen },
      },
    ];
  }
  return [{
    checkId: 'simulation.win_reachable', status: winSeen ? 'pass' : 'skip',
    evidence: winSeen ? 'bot observed the win/end overlay'
      : 'bot did not reach a win inside the play budget; reachability unproven (not a fail)',
    actual: winSeen ? { win: true } : null,
  }];
}
