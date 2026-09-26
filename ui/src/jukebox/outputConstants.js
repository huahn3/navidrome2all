// Shared bits between the output console and its editor dialog. Kept in its
// own module so both files export components only (react-refresh rule).
// SECRET_MASK is what GET /api/jukebox/outputs returns instead of a stored
// credential; sending it back on save keeps the stored value.
export const SECRET_MASK = '********'

export const EMPTY_OUTPUT = {
  id: '',
  name: '',
  type: 'xiaomi',
  address: '',
  password: '',
  token: '',
  did: '',
  model: '',
  account: '',
  passToken: '',
  textDirective: '',
  pathFrom: '',
  pathTo: '',
}

// Slug used to pre-fill the ID from the name, so most people never have to think
// about it: the ID only matters for the config file and the API.
export const slugify = (value) => {
  // A form library can hand back a non-string while a field is registering,
  // so coerce instead of assuming.
  const text = typeof value === 'string' ? value : ''
  return text
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48)
}

// Chinese (and any other non-latin) names slugify to nothing, so fall back to
// "<type>-<short hash of the name>": still readable, and unlikely to collide.
const shortHash = (text) => {
  let h = 5381
  const s = typeof text === 'string' ? text : ''
  for (let i = 0; i < s.length; i += 1) {
    h = (h * 33) ^ s.charCodeAt(i)
  }
  return (h >>> 0).toString(36).slice(0, 4)
}

export const suggestId = (name, type) => {
  const fromName = slugify(name)
  if (fromName) {
    return fromName
  }
  const text = typeof name === 'string' ? name.trim() : ''
  return text ? `${type}-${shortHash(text)}` : ''
}

// The server rejects a duplicate output id, and a household often owns two
// speakers with the same model (or the same generated slug), so keep bumping a
// numeric suffix until the id is free.
export const uniqueOutputId = (name, type, taken) => {
  const used = taken instanceof Set ? taken : new Set(taken || [])
  const base = suggestId(name, type) || type
  if (!used.has(base)) {
    return base
  }
  let n = 2
  while (used.has(`${base}-${n}`)) {
    n += 1
  }
  return `${base}-${n}`
}
