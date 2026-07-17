// Comparing a config the panel wants against the one a device reported. Kept in
// one place because the field markers and the "applied / pending" badge must
// agree: a badge saying "applied" next to a marked field would be a bug.

// Reads a dotted path; a missing branch reads as unset rather than throwing,
// since the device omits defaulted proto fields entirely.
export function readPath(root, path) {
  let node = root;
  for (const key of path.split('.')) {
    if (node === null || node === undefined || typeof node !== 'object') return undefined;
    node = node[key];
  }
  return node;
}

function isEmpty(value) {
  return (
    value === undefined ||
    value === null ||
    value === false ||
    value === '' ||
    value === 0 ||
    (Array.isArray(value) && value.length === 0)
  );
}

// Proto3 JSON drops fields sitting at their default, so an absent field and an
// explicit default mean the same thing. Treating them as equal is what keeps
// the marker off fields nobody touched.
export function sameValue(a, b) {
  if (a === b) return true;
  if (isEmpty(a) && isEmpty(b)) return true;
  if (typeof a === 'object' && typeof b === 'object' && a && b) {
    return stableJson(a) === stableJson(b);
  }
  return false;
}

// Key order is not meaningful in the JSON we get back, so sort before comparing.
export function stableJson(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return '[' + value.map(stableJson).join(',') + ']';
  const keys = Object.keys(value)
    .filter((k) => !isEmpty(value[k]))
    .sort();
  return '{' + keys.map((k) => JSON.stringify(k) + ':' + stableJson(value[k])).join(',') + '}';
}

// `like` is the value on the panel's side, used only to read an absent field in
// the field's own language: proto3 omits a false switch, and telling the admin a
// switch is "не задано" when the device plainly has it off is just confusing.
// Numbers stay "не задано" - an omitted number means the app's default, not 0.
export function describeValue(value, like) {
  if (value === undefined || value === null || value === '') {
    if (typeof like === 'boolean') return 'выключено';
    if (Array.isArray(like)) return 'пусто';
    return 'не задано';
  }
  if (value === true) return 'включено';
  if (value === false) return 'выключено';
  if (Array.isArray(value)) return value.length ? value.join(', ') : 'пусто';
  if (typeof value === 'object') return JSON.stringify(value);
  return String(value);
}

// Every leaf the panel actually specifies, as dotted paths. Arrays count as one
// leaf: their contents are compared whole, the way the form edits them.
function leafPaths(node, prefix = []) {
  if (node === null || typeof node !== 'object' || Array.isArray(node)) {
    return prefix.length ? [prefix.join('.')] : [];
  }
  return Object.keys(node).flatMap((key) => leafPaths(node[key], prefix.concat(key)));
}

// True when every field the panel specified already has that value on the
// device. Deliberately a subset check, not an equality one: the device reports
// its whole config, including settings the panel never manages, so demanding
// equality would leave the badge stuck on "pending" forever.
export function desiredApplied(desired, reported) {
  if (!desired || !reported) return false;
  return leafPaths(desired).every((path) => sameValue(readPath(desired, path), readPath(reported, path)));
}
