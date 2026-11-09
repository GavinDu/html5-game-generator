// Deterministic replay frame indices are measured in fixed 60 Hz simulation
// steps, not browser wall-clock time. This lets an approved preview select
// an unobstructed gameplay window without rewriting its recorded input trace.
export const FIXED_SIMULATION_HZ = 60;
export const MAX_CAPTURE_START_MS = 30_000;

export function previewCaptureStep(index, fps, captureStartMs = 0) {
  if (!Number.isSafeInteger(index) || index < 0 ||
      !Number.isFinite(fps) || fps < 1 || fps > 30 ||
      !Number.isSafeInteger(captureStartMs) ||
      captureStartMs < 0 || captureStartMs > MAX_CAPTURE_START_MS) {
    throw new RangeError('invalid preview frame, fps, or capture start offset');
  }
  return Math.round(captureStartMs * FIXED_SIMULATION_HZ / 1000) +
    Math.round(index * FIXED_SIMULATION_HZ / fps);
}
