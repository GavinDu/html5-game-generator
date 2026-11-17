// Preinstalled, renderer-independent semantic input v2 recorder.
//
// This is a bounded **diagnostic** producer primitive, NOT a proof that a
// client executed any actions, and not wired to the live Pixi score submitter.
// Inputs are caller-provided already normalized fixed-point semantic edges;
// the future player adapter owns iPhone/iPad viewport transforms.

export const SEMANTIC_INPUT_V2_SCHEMA = 'semantic-input-trace-v2';
export const MAX_SEMANTIC_INPUT_V2_EVENTS = 2000;
export const MAX_SEMANTIC_INPUT_V2_TICKS = 108000;
const MAX_BINDINGS = 32; // gameir.MaxInputBindings; reject unpublishable inputs
const MAX_FIXED_COORD = 1_000_000_000;
const MAX_ACTION_BYTES = 128;
const MAX_WIRE_BYTES = 1 << 20;

const textEncoder = new TextEncoder();

function bindingKey(device, action) {
  return device + '\u0000' + action;
}

// Game IR's published action identity is ASCII [A-Za-z0-9_-]{1,128}.
// A generic v2 wire decoder may validate Unicode escapes independently, but
// no published Game IR binding can use a Unicode, whitespace or punctuated ID.
// Keep this producer's binding gate exactly as strict as gameir.validID.
const GAME_IR_ACTION_ID = /^[A-Za-z0-9_-]{1,128}$/;
function canonicalAction(action) {
  return typeof action === 'string' && GAME_IR_ACTION_ID.test(action);
}

// inputBindings must come from a validated, published Game IR. Merely
// constructing a writer does NOT authenticate that IR or its publisher.
export function createSemanticInputV2Recorder(inputBindings, maxEvents = MAX_SEMANTIC_INPUT_V2_EVENTS) {
  if (!Array.isArray(inputBindings) || inputBindings.length === 0 ||
      inputBindings.length > MAX_BINDINGS ||
      !Number.isSafeInteger(maxEvents) || maxEvents < 1 ||
      maxEvents > MAX_SEMANTIC_INPUT_V2_EVENTS) {
    throw new RangeError('invalid semantic input v2 binding or event budget');
  }
  const kinds = new Map();
  for (const binding of inputBindings) {
    if (!binding || !canonicalAction(binding.action) ||
        !['tap', 'drag', 'keyboard'].includes(binding.input)) {
      throw new TypeError('invalid declared semantic input binding');
    }
    const device = binding.input === 'keyboard' ? 'keyboard' : 'pointer';
    const key = bindingKey(device, binding.action);
    // The canonical Go Game IR validator rejects duplicate (action,input)
    // pairs, even when both entries have the same kind. Never collapse an
    // ambiguous published binding list into a diagnostic trace identity.
    if (kinds.has(key)) {
      throw new TypeError('duplicate semantic input action binding');
    }
    kinds.set(key, binding.input);
  }

  const events = [];
  const pressed = new Set();
  let failureReason = null;
  let finished = false;

  const fail = (reason) => {
    if (!failureReason) failureReason = reason;
    return false;
  };
  const integerBetween = (value, min, max) =>
    Number.isSafeInteger(value) && value >= min && value <= max;

  return {
    get eventCount() { return events.length; },
    get complete() { return failureReason === null; },
    get failureReason() { return failureReason; },
    get finished() { return finished; },

    record(edge) {
      if (failureReason || finished) return false;
      if (!edge || typeof edge !== 'object' || Array.isArray(edge) ||
          Object.keys(edge).some(key =>
            !['tick', 'action', 'device', 'phase', 'position'].includes(key))) {
        return fail('invalid_event');
      }
      const { tick, action, device, phase, position } = edge;
      if (!integerBetween(tick, 0, MAX_SEMANTIC_INPUT_V2_TICKS) ||
          !canonicalAction(action) ||
          !['pointer', 'keyboard'].includes(device) ||
          !['down', 'move', 'up', 'cancel'].includes(phase)) {
        return fail('invalid_event');
      }
      const kind = kinds.get(bindingKey(device, action));
      if (!kind) return fail('undeclared_action');
      if (events.length && tick < events[events.length - 1].tick) {
        return fail('backwards_tick');
      }
      if (events.length >= maxEvents) return fail('event_limit');

      const requiresPosition = device === 'pointer' && (phase === 'down' || phase === 'move');
      if (requiresPosition) {
        if (!position || typeof position !== 'object' || Array.isArray(position) ||
            Object.keys(position).length !== 2 ||
            !Object.hasOwn(position, 'x') || !Object.hasOwn(position, 'y') ||
            !integerBetween(position.x, -MAX_FIXED_COORD, MAX_FIXED_COORD) ||
            !integerBetween(position.y, -MAX_FIXED_COORD, MAX_FIXED_COORD)) {
          return fail('invalid_position');
        }
      } else if (position !== undefined) {
        return fail('unexpected_position');
      }

      const key = bindingKey(device, action);
      if (phase === 'down') {
        if (pressed.has(key)) return fail('duplicate_down');
      } else if (phase === 'move') {
        if (kind !== 'drag' || !pressed.has(key)) return fail('invalid_move');
      } else if (!pressed.has(key)) {
        return fail('unbalanced_release');
      }

      // Order is defined by the deterministic fixed-step enqueue order.
      const previous = events.at(-1);
      const ordinal = previous && previous.tick === tick ? previous.ordinal + 1 : 0;
      if (ordinal > MAX_SEMANTIC_INPUT_V2_EVENTS) return fail('ordinal_overflow');
      const event = { tick, ordinal, action, device, phase };
      if (requiresPosition) event.position = { x: position.x, y: position.y };
      events.push(event);
      if (phase === 'down') pressed.add(key);
      if (phase === 'up' || phase === 'cancel') pressed.delete(key);
      return true;
    },

    finish(durationTicks) {
      if (failureReason || finished) return null;
      if (!integerBetween(durationTicks, 1, MAX_SEMANTIC_INPUT_V2_TICKS) ||
          pressed.size !== 0 ||
          (events.length && events[events.length - 1].tick > durationTicks)) {
        fail('incomplete_trace');
        return null;
      }
      const trace = {
        schema: SEMANTIC_INPUT_V2_SCHEMA,
        durationTicks,
        events: events.map(event => ({
          ...event,
          ...(event.position ? { position: { ...event.position } } : {}),
        })),
      };
      if (textEncoder.encode(JSON.stringify(trace)).length > MAX_WIRE_BYTES) {
        fail('wire_size_limit');
        return null;
      }
      finished = true;
      return trace;
    },
  };
}
