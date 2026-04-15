// Translate the JSONB predicate grammar used by collections/rules/asset-sets
// into a plain-English one-liner for the Collections list's Query Preview.
//
// Grammar covered (most common ops; rare shapes fall back to JSON):
//   { field: value }                          → field = value
//   { field: { $eq: v } }                     → field = v
//   { field: { $ne: v } }                     → field != v
//   { field: { $in: [a, b] } }                → field in (a, b)
//   { field: { $regex: "…" } }                → field matches /…/
//   { field: { $cidr: "10/8" } }              → field in 10/8
//   { field: { $gt|$gte|$lt|$lte: v } }       → field > v  (etc.)
//   { field: { $exists: true|false } }        → field is set / field is not set
//   { $and: [ … ] }                           → X AND Y AND …
//   { $or:  [ … ] }                           → (X OR Y OR …)
//   { $not: { … } }                           → NOT (…)
//
// Anything unrecognized renders as raw JSON so the reader still sees it.

type Pred = Record<string, unknown>;

const OP_SYMBOL: Record<string, string> = {
  $eq: '=',
  $ne: '!=',
  $gt: '>',
  $gte: '>=',
  $lt: '<',
  $lte: '<=',
};

function fmtScalar(v: unknown): string {
  if (v === null) return 'null';
  if (typeof v === 'string') return JSON.stringify(v);
  if (typeof v === 'number' || typeof v === 'boolean') return String(v);
  return JSON.stringify(v);
}

function isPlainObject(v: unknown): v is Pred {
  return typeof v === 'object' && v !== null && !Array.isArray(v);
}

function renderField(field: string, rhs: unknown): string {
  if (!isPlainObject(rhs)) {
    return `${field} = ${fmtScalar(rhs)}`;
  }
  const keys = Object.keys(rhs);
  // Single-op object is the common case.
  if (keys.length === 1) {
    const op = keys[0];
    const val = rhs[op];
    if (op in OP_SYMBOL) return `${field} ${OP_SYMBOL[op]} ${fmtScalar(val)}`;
    if (op === '$in' && Array.isArray(val)) {
      return `${field} in (${val.map(fmtScalar).join(', ')})`;
    }
    if (op === '$regex' && typeof val === 'string') {
      return `${field} matches /${val}/`;
    }
    if (op === '$cidr' && typeof val === 'string') {
      return `${field} in ${val}`;
    }
    if (op === '$exists') {
      return val ? `${field} is set` : `${field} is not set`;
    }
  }
  // Multi-op constraint on one field: AND them.
  const parts = keys.map((op) => renderField(field, { [op]: rhs[op] }));
  return parts.join(' AND ');
}

export function predicateToEnglish(pred: unknown): string {
  if (!isPlainObject(pred)) {
    return JSON.stringify(pred);
  }
  const keys = Object.keys(pred);
  if (keys.length === 0) return '(matches everything)';

  // Logical combinators.
  if (keys.length === 1) {
    const k = keys[0];
    const v = pred[k];
    if (k === '$and' && Array.isArray(v)) {
      return v.map((p) => predicateToEnglish(p)).join(' AND ');
    }
    if (k === '$or' && Array.isArray(v)) {
      return '(' + v.map((p) => predicateToEnglish(p)).join(' OR ') + ')';
    }
    if (k === '$not') {
      return 'NOT (' + predicateToEnglish(v) + ')';
    }
  }

  // Implicit $and across the top-level keys.
  return keys.map((k) => renderField(k, pred[k])).join(' AND ');
}
