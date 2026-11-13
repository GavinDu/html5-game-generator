// Deterministic Game IR v1.x rule predicates, independent of rendering,
// physics, browser APIs, network I/O and wall time. This is preinstalled
// trusted engine code; Game IR remains data, never an executable module.

function matchesRef(ref, entity) {
  return ref === undefined || ref === '' ||
    ref === entity.defId || ref === entity.def.kind || ref === entity.runtimeId;
}

export function lookupSemanticValue(name, state) {
  if (name === 'score') return state.score;
  if (name === 'health') return state.health;
  if (name === 'elapsed' || name === 'time') return state.elapsed;
  const key = name.startsWith('variable:') ? name.slice(9) : name;
  return state.variables[key] ?? 0;
}

// Retain the exact v1.x numeric-versus-boolean coercion. A migration to a
// typed strict comparator requires a new declared IR capability/version;
// silently changing comparisons can alter game outcomes and replay hashes.
export function conditionsMet(rule, state) {
  for (const condition of rule.conditions || []) {
    const lhs = lookupSemanticValue(condition.variable, state);
    const rhs = condition.value;
    let ok = false;
    const ln = Number(lhs), rn = Number(rhs);
    if (!Number.isNaN(ln) && !Number.isNaN(rn) && typeof rhs !== 'boolean') {
      switch (condition.operator) {
        case 'eq': ok = ln === rn; break;
        case 'neq': ok = ln !== rn; break;
        case 'gt': ok = ln > rn; break;
        case 'gte': ok = ln >= rn; break;
        case 'lt': ok = ln < rn; break;
        case 'lte': ok = ln <= rn; break;
      }
    } else {
      if (condition.operator === 'eq') ok = lhs === rhs;
      else if (condition.operator === 'neq') ok = lhs !== rhs;
      else ok = false;
    }
    if (!ok) return false;
  }
  return true;
}

export function ruleMatches(rule, event, source, target, value, inputAction) {
  const when = rule.when || {};
  if (when.event !== event) return false;
  if (source && !matchesRef(when.source, source)) return false;
  if (target && !matchesRef(when.target, target)) return false;
  if (inputAction !== undefined && when.inputAction !== undefined &&
      when.inputAction !== inputAction) return false;
  if (value !== undefined && when.value !== undefined && Number(when.value) !== value) return false;
  return true;
}
