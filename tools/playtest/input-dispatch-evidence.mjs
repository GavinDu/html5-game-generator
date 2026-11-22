// Playtest input provenance: only successful worker dispatch calls count.
// This does NOT prove the game consumed a touch or establish a replay trace.
export async function recordAfterDispatch(action, record) {
  try {
    if (await action() === false) return false;
    record();
    return true;
  } catch {
    return false;
  }
}

// Run every step, including pointer/key release if an earlier step failed.
// Record the gesture only when all Playwright calls completed successfully.
export async function recordAfterGesture(steps, record) {
  let successful = true;
  for (const step of steps) {
    try {
      if (await step() === false) successful = false;
    } catch {
      successful = false;
    }
  }
  if (successful) record();
  return successful;
}
