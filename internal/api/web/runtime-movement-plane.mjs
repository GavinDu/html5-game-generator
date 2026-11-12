// Deterministic movement-plane capability adapter (Epic #202 / #315/#318).
//
// The generic Pixi interpreter only asks a versioned capability whether the
// player is constrained to X. It must never inspect a game's objective name.
// Published old Game IR retains a frozen compatibility policy until a
// content-addressed migration can prove replay equivalence.
const CAPABILITY = 'input.movement_axes.v1';

export function resolveHorizontalMovement(ir) {
  const controls = ir?.experience?.controls || {};
  const caps = ir?.capabilities || [];
  const newContract = Array.isArray(caps) && caps.includes(CAPABILITY);
  if (newContract) {
    if (ir?.irVersion !== '1.2' ||
        !['x', 'xy'].includes(controls.movementAxes)) {
      throw new Error('input.movement_axes.v1 requires Game IR 1.2 and movementAxes x or xy');
    }
    if (((controls.scheme === 'drag_x' || controls.scheme === 'lane_tap') &&
          controls.movementAxes !== 'x') ||
        (controls.scheme === 'drag_free' && controls.movementAxes !== 'xy') ||
        controls.scheme === 'tap_target') {
      throw new Error('movementAxes is incompatible with the authored control scheme');
    }
    return controls.movementAxes === 'x';
  }
  if (controls.movementAxes !== undefined && controls.movementAxes !== '') {
    throw new Error('movementAxes requires input.movement_axes.v1 capability');
  }

  return resolveLegacyHorizontalMovement(ir);
}

// FROZEN legacy Game IR 1.0–1.2 adapter. Older shipped package hashes and
// replays must not be silently reinterpreted. In particular, the
// historically-authored Ribbon Sky catcher had XY tap motion, while Penguin
// and Bobo catcher tap motion was X-only. This old objective convention is
// intentionally quarantined *here*, away from generic runtime semantics.
// Remove only after a separately versioned content migration with golden
// cross-browser replay evidence for every previously shipped game.
function resolveLegacyHorizontalMovement(ir) {
  const exp = ir?.experience;
  const scheme = exp?.controls?.scheme;
  return scheme === 'lane_tap' || scheme === 'drag_x' ||
    (exp?.archetype === 'catcher' && scheme === 'tap' &&
      exp?.objective?.progressVariable !== 'holes_stitched');
}
