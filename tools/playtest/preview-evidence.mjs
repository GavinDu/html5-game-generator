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
