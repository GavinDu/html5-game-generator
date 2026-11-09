// Review contract for a *real* Penguin intro screenshot used as a poster.
// A successful guided win can happen before the seven-second intro clears;
// in that case a separate real browser session is valid evidence, but must
// never be mislabeled as a frame of the winning playthrough.
export function validatePenguinIntroEvidence(phone) {
  const event = phone?.introEvidence;
  if (phone?.passed !== true || phone?.won !== true ||
      phone?.introClearedAfterSevenSeconds !== true ||
      phone?.compactHintPersists !== true ||
      !phone?.screenshots?.includes('intro-cleared')) {
    throw new Error('Penguin poster requires completed gameplay, cleared intro and real screenshot');
  }
  if (!event || !['guided-play', 'independent-real-browser'].includes(event.session) ||
      !Number.isFinite(event.runtimeSeconds) || event.runtimeSeconds < 7.2 ||
      !Number.isSafeInteger(event.objectiveProgress) ||
      event.objectiveProgress < 0 || event.objectiveProgress >= 12) {
    throw new Error('Penguin poster requires valid real-session intro screenshot provenance');
  }
  return event;
}


// A Bobo candidate must depict the actual rainbow-restoration gameplay,
// not an untouched 0/12-heart start or a winning result screen. The source
// crayon bear and the separate kitten must remain centered and uncut.
export function validateBoboMidplayEvidence(report) {
  const event = report?.midplayEvidence;
  if (report?.passed !== true || report?.gesture !== 'real touchscreen tap' ||
      report?.haloFollowsPlayer !== true || !Array.isArray(report?.screenshots) ||
      !report.screenshots.includes('midplay-restored-rainbow.png') ||
      !Number.isInteger(report?.croppedBrownPixels) ||
      report.croppedBrownPixels > 3) {
    throw new Error('Bobo poster requires passed real-touch screenshot and source-art fidelity checks');
  }
  if (!event || event.source !== 'guided real touch input' ||
      event.screenshot !== 'midplay-restored-rainbow.png' ||
      !Number.isSafeInteger(event.progress) || event.progress < 4 || event.progress >= 12 ||
      !Number.isFinite(event.score) || event.score < 0 ||
      !Number.isFinite(event.playerX) || event.playerX < 390 * .25 ||
      event.playerX > 390 * .75 ||
      !Number.isFinite(report?.before?.x) ||
      !Number.isFinite(report?.after?.x) ||
      report.after.x <= report.before.x + 40) {
    throw new Error('Bobo poster requires measured mid-play progress and centered original-art provenance');
  }
  return event;
}
